# ADR-002: Go single binary serving the API and a static Astro build, with PostgreSQL, deployed via docker compose

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

The language choices are settled: Go for the backend, Astro + React for the frontend, PostgreSQL for storage. The open question is how these pieces are packaged and deployed on a single self-hosted machine behind an existing TLS-terminating reverse proxy, with the lowest operational burden.

### Decision drivers

- Keep it simple: fewest containers and runtimes
- One-week MVP
- Self-hosted; must run with `docker compose up`
- No server-side rendering need: the UI is one interactive flow

## Considered Options

| Option | Description |
|--------|-------------|
| A | Go binary serves `/v1` API and the static Astro build (`embed.FS`); PostgreSQL container |
| B | Go API container + Node container running Astro SSR + PostgreSQL |
| C | Go binary + SQLite |

## Decision Outcome

Chosen option: **A**, because it yields two containers, one runtime and no CORS, while keeping the already-chosen database.

Stack details:

| Concern | Choice |
|---------|--------|
| HTTP | stdlib `net/http` with method/pattern routing; no web framework |
| DB access | `pgx/v5` (`pgxpool`) with hand-written SQL in `internal/store`; no ORM, no code generation |
| Migrations | `golang-migrate` (`iofs` source + `pgx/v5` database driver), SQL files embedded and applied at startup |
| Frontend | Astro `output: "static"`; one React island for the mixtape UI; Tailwind optional |
| SPA routes | The Go static handler serves `m/index.html` for `/m/{slug}` (and the `/es/` equivalent) |
| Image | Multi-stage Dockerfile: Node build → Go build → distroless runtime |
| Compose | `app` (port bound to `127.0.0.1:8080`) + `db` (named volume) |
| TLS | Terminated by a host-level reverse proxy; out of the compose file |
| Backups | Nightly `pg_dump` from the host scheduler to a local directory (documented in README) |

### Positive consequences

- A single artifact to build, version and roll back.
- No cross-origin configuration; cookies are first-party.

### Negative consequences

- A UI-only change still requires rebuilding and redeploying the binary (seconds; acceptable).
- Astro static output cannot pre-render per-share pages; share pages render client-side (see ADR-011).

### Revisit trigger

Sustained CPU > 70 % or p95 API latency (excluding LLM time) > 500 ms on the single host, per ADR-013 metrics.

## Pros and Cons of the Options

### A — Single binary + PostgreSQL

- Good, because of the minimal moving parts.
- Bad, because frontend and backend deploy together.

### B — Separate SSR frontend

- Good, because the frontend could deploy independently.
- Bad, because it adds a Node runtime in production, a third container and proxy/CORS setup for no user-facing benefit.

### C — SQLite

- Good, because it removes the database container.
- Bad, because PostgreSQL is already chosen (`jsonb`, concurrent writers) and migrating later costs more than running it now.

## Links

- Plan §5, ADR-001, ADR-013
