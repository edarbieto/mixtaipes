# Contributing

## Principles

- **Keep it simple.** One binary, one database. New dependencies, services or abstractions need a written justification; non-trivial decisions get an ADR in `docs/adr/` (MADR format, next sequential number).
- **English everywhere:** code, comments, docs, commit messages, pull requests.
- **Contracts first.** `docs/contracts/api.md` and `docs/contracts/db-schema.md` change in the same commit as the code that implements them.

## Workflow

- Default branch: `master`. Work on a branch (`feat/…`, `fix/…`, `docs/…`, `chore/…`) and open a pull request; CI must be green.
- [Conventional Commits](https://www.conventionalcommits.org/): `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `ci:`. Imperative mood, subject ≤ 72 characters. One commit per logical step.
- Migrations: `migrations/NNNNNN_name.up.sql` + `.down.sql` (golang-migrate). Additive only until v1; down migrations are for local development.
- Roadmap tasks live in `docs/roadmap/ROADMAP.md`; tick the checkbox in the pull request that completes it.

## Commands

| Command | Purpose |
|---------|---------|
| `make dev-db` | Start PostgreSQL in Docker |
| `make run` | Run the Go server with `.env` |
| `make web` | Astro dev server (hot reload) |
| `make test` | Go tests (`TEST_DATABASE_URL` enables database tests) |
| `make lint` | `go vet` + `golangci-lint` if installed |
| `make build` | Frontend build + Go binary in `bin/` |
| `make up` / `make down` | Full stack with Docker Compose |

## Layout

See [ADR-001](docs/adr/001-structure.md) and [ARCHITECTURE.md](docs/ARCHITECTURE.md).
