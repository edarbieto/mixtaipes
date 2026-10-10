# Roadmap — mixtaipes MVP

> Milestones toward a public instance where anyone can turn a sentence into a verified, copyable playlist.
> Tasks are checkbox-tracked. The MVP is a one-week sprint; W1 is broken down by day.
> Status: v1 (M0) — Last updated: 2026-10-10

## Now / Next / Later

| Now (this week) | Next (weeks 2–3) | Later |
|-----------------|------------------|-------|
| Prompt → verified playlist → refine → preview → copy → share, live over HTTPS | Refinement UX: version diff, undo, pinned tracks; "under the hood" panel; plain-text export | Google OIDC for cross-device history |
| Guarded, versioned system prompt with evaluation numbers | Second prompt-tuning pass driven by production metrics | Paid LLM tier or another provider (budget ~USD 10/month) |
| Metrics in place to decide on scaling later | Grafana dashboard, backup restore drill | Additional catalogs (Apple Music, Tidal, YouTube) behind `catalog.Catalog`; voice input |

## Milestone overview

| Milestone | Focus | Key deliverable |
|-----------|-------|-----------------|
| **M0** | Planning | This documentation suite (done) |
| **M1** | MVP (W1) | Public instance over HTTPS meeting the plan's "MVP done" criteria |
| **M2** | Refinement UX & tuning (W2–W3) | Diff/undo/pin shipped; prompt v-next beats v1 on the evaluation set |
| **M3** | Growth options (later) | One of: OIDC, paid LLM tier, second catalog — chosen by metrics |

---

## M1 — MVP (Week 1)

### W1 · Day 1: Foundations and unknowns
- [x] Contracts drafted: `docs/contracts/api.md`, `docs/contracts/db-schema.md`
- [x] ADR-001 … ADR-014 accepted
- [ ] Create the Spotify app (Development Mode); confirm the client-credentials flow and `GET /v1/search` work for it — **#1 technical unknown** (R1)
- [ ] Manually confirm Spotify Desktop accepts a pasted list of `open.spotify.com/track/…` links into a playlist (R8)
- [ ] Manually confirm the embed player (`open.spotify.com/embed/track/{id}`) plays without login (R13)
- [ ] Create a Gemini API key; confirm structured-output mode with a 20-track schema
- [ ] Repository skeleton per ADR-001; `go.mod`, Astro app in `web/`, MIT `LICENSE`, README stub
- [ ] `compose.yaml` (app + db), multi-stage `Dockerfile`, `/healthz`, `/readyz`
- [ ] Initial `golang-migrate` migration (`000001_init.up.sql` / `.down.sql`) matching `db-schema.md`; `internal/store` on `pgx/v5`
- [ ] CI: vet, lint, test, web build, image build — green on `master` (default branch)

### W1 · Day 2: LLM + catalog core, prompt v1
- [ ] `llm.Provider` + Gemini adapter with schema, validation and single retry (ADR-003)
- [ ] `prompts/v1.md`: role, schema, count rules (20 default, ≤ 30), diversity, locale, refusals, delimited input (ADR-004)
- [ ] `catalog.Catalog` + Spotify adapter: token cache, field-filtered search, fallback query, normalization + fuzzy match, concurrency 5, 429 handling (ADR-006)
- [ ] Backfill round for misses using only LLM text (ADR-007), with an import-boundary test
- [ ] `evals/cases.yaml` with ~25 cases; `cmd/eval` printing the ADR-005 metrics
- [ ] First evaluation run of v1 recorded

### W1 · Day 3: API and persistence
- [ ] `POST /v1/mixtapes`, `POST /v1/mixtapes/{id}/turns`, `GET /v1/mixtapes`, `GET /v1/mixtapes/{id}` per contract
- [ ] Refinement context builder (original request + prior refinements + latest `llm_output`) (ADR-008)
- [ ] Client cookie middleware with hashed storage; ownership checks; JSON content-type guard (ADR-010)
- [ ] Per-IP token bucket with trusted-proxy handling; daily LLM-call cap; `Retry-After`
- [ ] Market resolution from `Accept-Language` region with `SPOTIFY_MARKET` fallback (ADR-006), with table-driven tests
- [ ] `POST /v1/events`; failed turns persisted with `llm_calls` for accounting
- [ ] Handler tests with fake `Provider` and fake `Catalog`

### W1 · Day 4: Frontend core
- [ ] Astro layout + React island: prompt box, loading state, track list with "why"
- [ ] Suggested-refinement chips and refinement input
- [ ] Per-track play button loading one embed iframe at a time (ADR-009)
- [ ] "Copy for Spotify" button + inline paste guide; desktop/mobile messaging
- [ ] Localized refusal and error states from `refusal_reason` / `error.code`
- [ ] Mixtape list (this browser's history)

### W1 · Day 5: Sharing, i18n, metrics, PWA shell
- [ ] Share endpoint + `/m/{slug}` read-only page with `noindex` (ADR-011)
- [ ] i18n: `en` default + `es` routes and dictionaries; parity check in CI (ADR-012)
- [ ] Prometheus metrics on `:9090` per ADR-013; `observability` compose profile with `deploy/prometheus.yml`
- [ ] Retention job behind `RETENTION_DAYS` (default 0) with a test (ADR-014)
- [ ] Web manifest + icons (no service worker)
- [ ] `/privacy` page (anonymous cookie, LLM provider data terms, retention)

### W1 · Day 6: Prompt tuning and production deploy
- [ ] Prompt tuning loop: iterate `prompts/v2.md`+ against the evaluation set until MVP targets are met (match ≥ 90 %, correct refusals ≥ 95 %, wrong refusals 0 %)
- [ ] Record per-version evaluation summaries in the PR descriptions
- [ ] Production `.env`; deploy with `docker compose` behind the host reverse proxy; HTTPS verified
- [ ] Confirm real client IPs reach the rate limiter through the proxy
- [ ] Nightly `pg_dump` scheduled on the host; one restore tested

### W1 · Day 7: Buffer and launch
- [ ] Fix-only buffer for anything that slipped
- [ ] README: what it is, screenshots, self-hosting guide, configuration table, Spotify app setup, backups
- [ ] Verify all four "MVP done" criteria from the plan; tag `v0.1.0`
- [ ] Public URL live; launch announcement drafted with screenshots and evaluation numbers

## M2 — Refinement UX & tuning (Weeks 2–3)

### W2: Refinement UX
- [ ] Version diff between turns (added/removed highlighting, client-side)
- [ ] Version history and "restore this version" (new turn copying a previous `llm_output`)
- [ ] Pin tracks: pinned tracks passed as "must keep" in the refinement context; ADR for the prompt change
- [ ] Plain-text export ("Artist – Title") for third-party transfer tools

### W3: Tuning and visibility
- [ ] "Under the hood" panel: model, prompt version, tokens, latencies, match rate per turn
- [ ] Prompt v-next driven by production `turn_match_ratio` and refusal data; evaluation set grown to ~50 cases
- [ ] Grafana dashboard JSON in `deploy/grafana/`
- [ ] Review LLM quota usage; decide on a paid tier (ADR-003 revisit trigger)

## M3 — Growth options (later)

- [ ] Google OIDC login (`openid email profile`), linking existing anonymous mixtapes on first sign-in — new ADR superseding the identity part of ADR-010
- [ ] Paid LLM tier or alternative provider, validated with the evaluation harness
- [ ] Second catalog adapter, if its terms allow open one-click import
- [ ] Voice input (backlog): assess reliability of browser dictation (Web Speech API) vs. a self-hosted Whisper service before committing; new ADR if adopted
- [ ] Scaling decisions only when an ADR-013 trigger fires
