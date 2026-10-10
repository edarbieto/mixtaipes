# ADR-001: Single repository with a flat Go layout and in-repo docs

- Status: **Accepted**
- Date: 2026-10-10
- Deciders: Maintainer

## Context and Problem Statement

mixtaipes has a Go backend, an Astro/React frontend, versioned system prompts, an evaluation case set and design docs. One person maintains it and it is published as open source. How should code, prompts and documentation be laid out so that a newcomer understands the project in minutes and a single build produces the deployable artifact?

### Decision drivers

- One maintainer; contributors arrive from a public repository
- The Go binary embeds the frontend build and the prompts, so they must be reachable at build time
- Keep it simple: no build orchestration tools beyond `go` and `npm`

## Considered Options

| Option | Description |
|--------|-------------|
| A | Single repo: Go module at the root, `web/` for the frontend, `prompts/`, `evals/`, `migrations/`, `docs/` |
| B | Monorepo tooling (Nx/Turborepo) with `apps/api` and `apps/web` |
| C | Separate repositories for backend and frontend |

## Decision Outcome

Chosen option: **A**, because it is the smallest layout that lets one `docker build` produce one binary containing UI, prompts and migrations.

```
mixtaipes/
├── cmd/
│   ├── mixtaipes/          # server entrypoint
│   └── eval/               # prompt evaluation runner
├── internal/
│   ├── config/  httpapi/  mixtape/  llm/  catalog/  store/  metrics/
├── migrations/             # golang-migrate SQL: NNNNNN_name.up.sql / .down.sql (embedded)
├── prompts/                # versioned system prompts (embedded)
├── evals/                  # evaluation cases
├── web/                    # Astro + React island; `npm run build` → web/dist (embedded)
├── deploy/                 # prometheus.yml, grafana/ (optional observability profile)
├── docs/                   # ARCHITECTURE, adr/, contracts/, roadmap/
├── .github/workflows/ci.yaml
├── Dockerfile  compose.yaml  Makefile
├── LICENSE                 # MIT
└── README.md
```

Conventions:

- English everywhere: code, comments, docs, commits.
- Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`...).
- ADRs in MADR format, numbered sequentially, one decision each.
- Contracts (`docs/contracts/`) change in the same commit as the code that implements them.
- CI (GitHub Actions): `go vet`, `golangci-lint`, `go test`, `npm ci && npm run build`, `docker build` — on every push and pull request.

### Positive consequences

- One clone, one build, one artifact.
- Prompts are reviewed like code: diffs, history, PRs.

### Negative consequences

- The Docker build needs both Node and Go stages (multi-stage build; acceptable).

### Revisit trigger

A second deployable (e.g., a mobile client or a separate worker) appears.

## Pros and Cons of the Options

### A — Single repo, flat layout

- Good, because there is nothing to learn beyond Go and npm.
- Good, because `embed.FS` can reach `web/dist`, `prompts/` and `migrations/` directly.
- Bad, because frontend and backend share one CI pipeline (fast enough at this size).

### B — Monorepo tooling

- Good, because of task caching and dependency graphs.
- Bad, because it is tooling for many packages; this project has two.

### C — Separate repositories

- Good, because of independent release cycles.
- Bad, because the binary embeds the UI; cross-repo builds add friction for no gain.

## Links

- Plan §5, ADR-002
