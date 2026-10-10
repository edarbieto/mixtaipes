# ADR-013: Prometheus metrics on an internal listener; optional Prometheus + Grafana profile

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

Scaling decisions (bigger host, more instances, a managed platform) should be driven by data, not guesses. The product also needs its North Star and quality numbers. What do we measure, and with how much infrastructure?

### Decision drivers

- Data for future scaling and provider decisions
- Keep it simple: nothing mandatory beyond the app and the database
- Metrics must not be publicly exposed

## Considered Options

| Option | Description |
|--------|-------------|
| A | `prometheus/client_golang` on `:9090/metrics` (not published); Prometheus + Grafana as an opt-in compose profile; product events in PostgreSQL |
| B | Hosted APM / observability SaaS |
| C | Logs only |

## Decision Outcome

Chosen option: **A**, because the endpoint is nearly free to add and the stack is opt-in (`docker compose --profile observability up`).

Metrics:

| Metric | Type | Labels | Answers |
|--------|------|--------|---------|
| `http_requests_total` / `http_request_duration_seconds` | counter / histogram | route, status | Traffic, API latency |
| `llm_requests_total` | counter | status (ok, refused, invalid, error) | Provider health |
| `llm_request_duration_seconds` | histogram | — | LLM latency |
| `llm_tokens_total` | counter | direction (input, output) | Cost projection for a paid tier |
| `catalog_search_total` | counter | result (match, miss, error, rate_limited) | Spotify health, 429 pressure |
| `catalog_search_duration_seconds` | histogram | — | Catalog latency |
| `turn_duration_seconds` | histogram | kind (generate, refine) | End-to-end latency (MVP target) |
| `turn_match_ratio` | histogram | — | Prompt quality in production |
| `rate_limited_total` | counter | scope (ip, daily_cap) | Quota pressure |
| `daily_llm_calls` | gauge | — | Distance to the cap |

Product events (`events` table): `copy`, `share_created`, `share_view`, `preview_play` — the North Star (copies/week) is a SQL query.

Logs: structured JSON via `log/slog` to stdout; request id per request; prompts are not logged.

Scaling triggers to watch (documented here so they are not re-invented):

- p95 `turn_duration_seconds` > 15 s for a week → investigate LLM/catalog latency first, not the host.
- `rate_limited_total{scope="daily_cap"}` > 0 on 3+ days/week → paid LLM tier (ADR-003).
- Host CPU > 70 % sustained or API p95 (non-LLM routes) > 500 ms → bigger host before any orchestration.

### Positive consequences

- Every scaling or provider decision has a measurable trigger.

### Negative consequences

- Optional dashboards must be built by hand (one Grafana dashboard JSON checked into `deploy/grafana/`).

### Revisit trigger

More than one instance is deployed, or alerting becomes necessary.

## Pros and Cons of the Options

### A — Prometheus endpoint + opt-in stack

- Good, because it is self-hosted, free and standard.
- Bad, because there is no alerting in the MVP.

### B — Hosted APM

- Good, because of zero setup dashboards.
- Bad, because of cost, a third-party data flow, and lock-in.

### C — Logs only

- Good, because it is the simplest.
- Bad, because there are no latency distributions or trend data for decisions.

## Links

- ADR-002, ADR-003, ADR-006
