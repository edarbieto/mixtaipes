# ADR-015: Asynchronous turns with a PostgreSQL-backed queue and client polling

- Status: **Accepted**
- Date: 2026-10-12
- Deciders: Maintainer

## Context and Problem Statement

A turn calls the LLM, verifies up to 30 tracks against Spotify and may run a backfill round (ADR-006). Day 1 and Day 2 checks measured 9–17 s for the LLM call alone, and the provider repeatedly answered HTTP 503 ("model is experiencing high demand"); a single retry one second later sometimes failed again. Running the whole turn inside the `POST` request means:

- the browser holds a connection open for 15–30 s with only a spinner;
- a longer backoff between retries would make that wait worse, so overloads surface as errors;
- a restart or deploy in the middle of a turn loses it.

How should turns be executed and delivered to the browser?

### Decision drivers

- Keep it simple: one binary, one database (ADR-001, ADR-002)
- Survive transient provider overloads without user-visible failures where possible
- Show real progress during a turn
- Bounded quota per turn (daily cap, ADR-010)

## Considered Options

| Option | Description |
|--------|-------------|
| A | Synchronous: the `POST` handler runs the whole turn and returns it |
| B | Asynchronous: the `POST` handler stores a `pending` turn; in-process workers claim it from PostgreSQL (`FOR UPDATE SKIP LOCKED`); the browser polls |
| C | Asynchronous with a message broker (RabbitMQ or Redis) and a separate worker process |

Delivery to the browser, for B and C: **polling** `GET /v1/mixtapes/{id}`, Server-Sent Events, or WebSockets.

## Decision Outcome

Chosen option: **B with polling**, because it gets the benefits of asynchronous execution (retries with real backoff, restart safety, progress) without adding a service, and polling reuses an endpoint the contract already has.

### Lifecycle

```
pending ──claim──▶ running ──▶ ok | refused | failed
   ▲                  │
   └── retry (backoff)┘
```

1. `POST /v1/mixtapes` and `POST /v1/mixtapes/{id}/turns` validate input, apply the per-IP rate limit and the daily cap (ADR-010), insert the turn with `status = pending`, nudge the workers and return **`202 Accepted`** with the pending turn.
2. Only one unfinished turn per mixtape: a refinement while the latest turn is `pending` or `running` returns `409 TURN_IN_PROGRESS`.
3. **Workers**: `TURN_WORKERS` goroutines (default 2, sized to the provider's free-tier rate limit) inside the same binary. A worker claims the oldest due turn:

   ```sql
   UPDATE turns SET status = 'running', attempts = attempts + 1,
          locked_until = now() + interval '90 seconds'
   WHERE id = (
     SELECT id FROM turns
     WHERE (status = 'pending' AND run_after <= now())
        OR (status = 'running' AND locked_until < now())   -- reclaim after a crash
     ORDER BY created_at
     FOR UPDATE SKIP LOCKED
     LIMIT 1)
   RETURNING …;
   ```

   Workers wake on an in-process signal when a turn is inserted and poll every second as a fallback (for reclaimed and retried turns).
4. **Retries**: when the LLM is unavailable because of an overload or a timeout, the turn goes back to `pending` with `run_after = now() + backoff` (2 s, then 8 s; at most 3 attempts). Quota errors (HTTP 429), invalid output and catalog failures are not retried. A turn not finished 90 s after creation fails with `LLM_UNAVAILABLE`. This sits on top of the adapter's own single quick retry (ADR-003).
5. **Progress**: the worker records `progress_phase` (`generating`, `matching`, `backfilling`) and matched/total counts while it runs.
6. **Terminal states**: `ok` and `refused` as before; failures are stored on the turn (`status = failed`, `error_code`) instead of being returned as HTTP 502 errors.
7. **Accounting**: `llm_calls` counts every LLM call across attempts; the worker re-checks the daily cap before each call and fails the turn with `DAILY_CAP_REACHED` when it is exhausted.
8. **Shutdown**: on SIGTERM workers stop claiming and finish in-flight turns within the graceful shutdown timeout; anything left is reclaimed after its lease expires.

### Delivery: polling

While the latest turn is `pending` or `running`, the UI polls `GET /v1/mixtapes/{id}` every second (giving up after 2 minutes) and renders `progress`. Reads are not subject to the per-IP turn rate limit.

- **SSE** would push the same state over one long-lived connection, but it needs proxy settings (no buffering, long timeouts), reconnection handling and a notification path from the worker to the handler. Deferred: the state lives in PostgreSQL, so SSE can be added later without changing the model.
- **WebSockets** are bidirectional; the client only listens after submitting, so they add protocol and proxy complexity for no benefit. Rejected.

### Schema changes (migration `000002`)

`turns` gains: `status` values `pending` and `running`; `attempts` (smallint, default 0), `run_after` (timestamptz, default `now()`), `locked_until` (timestamptz, NULL), `progress_phase` (text, NULL), `progress_done` / `progress_total` (smallint, default 0); a partial index on `(run_after) WHERE status IN ('pending', 'running')`. The migration replaces the `status` CHECK constraint and only adds columns, so existing rows stay valid.

### Positive consequences

- Provider overloads are absorbed by backoff instead of reaching the user.
- The user sees what is happening ("matching 12/20").
- Deploys and crashes do not lose turns.
- No new service: the queue is a table the app already owns.

### Negative consequences

- More moving parts in the app: claim/lease/reclaim logic and its tests.
- Polling costs ~15–30 extra cheap requests per turn.
- The UI gains a pending state and must handle `failed` turns from a poll instead of an HTTP error.

### Revisit trigger

- Polling exceeds 50 % of API requests or noticeably loads the database → add SSE on top of the same state.
- Turn throughput beyond what row claiming handles (hundreds of turns per second) or a second consuming service appears → consider a broker (option C).

## Pros and Cons of the Options

### A — Synchronous

- Good, because it is the least code.
- Bad, because overloads become errors, waits have no progress and restarts lose turns.

### B — PostgreSQL-backed queue, in-process workers, polling

- Good, because it adds no infrastructure and keeps all state in one place.
- Good, because multiple app replicas could share the queue safely (`SKIP LOCKED`).
- Bad, because the worker and lease logic must be written and tested by hand.

### C — Broker and separate worker

- Good, because of mature delivery, retry and dead-letter features.
- Bad, because it adds a service to run, monitor and back up, plus a second deployable, for a workload of a few turns per minute.

## Links

- ADR-003 (provider errors), ADR-006 (turn pipeline), ADR-008 (refinement context), ADR-010 (rate limits, daily cap), ADR-013 (metrics)
