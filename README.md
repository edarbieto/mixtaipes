# mixtaipes

Describe a moment, get its mixtape.

mixtaipes turns a sentence — *"driving through a rainy city at night in a 90s BMW"* — into a playlist. An LLM proposes the tracks, every track is verified against the Spotify catalog, you refine the result conversationally, and you paste the final list into Spotify.

> Status: **paused** (2026-10-12) after the core pipeline was built. See [Status](#status) and the [roadmap](docs/roadmap/ROADMAP.md).

## Status

Development is paused at the end of W1 · Day 2. What exists and works:

- LLM provider with a Gemini adapter, structured output, validation and overload retry ([ADR-003](docs/adr/003-llm-provider.md)).
- Versioned system prompt with refusals and delimited user input (`prompts/v1.md`).
- Spotify catalog adapter with fuzzy matching ([ADR-006](docs/adr/006-track-resolution.md)).
- Turn pipeline with dedupe and a backfill round that never sends Spotify content to the LLM ([ADR-007](docs/adr/007-spotify-content-isolation.md)).
- Evaluation cases and a runner printing the [ADR-005](docs/adr/005-prompt-evaluation.md) metrics (`make eval`).

A partial evaluation run of prompt v1 met the targets it could measure: 100 % schema-valid on the first try, 100 % correct refusals, 0 % wrong refusals, 93.7 % match rate before backfill, no duplicates, p95 turn latency 10.4 s.

### Why it is paused

The plan was a hosted web app where every visitor shares one Spotify app and one LLM key. Both quotas turned out too small for that:

- **Spotify.** A personal Spotify app stays in Development Mode, whose rate limits are not published, and Extended Quota is only granted to registered organizations. A single evaluation run (a few hundred searches within an hour, about 20–40 per playlist) was answered with `429` and a `Retry-After` of about 21 hours. One busy hour would lock every visitor out for a day.
- **LLM.** The Gemini free tier is a daily request quota per key (20 requests/day for Gemini 3.x Flash, 500/day for `gemini-3.5-flash-lite`). Shared by all visitors it lasts a few dozen playlists; a paid tier fixes this, but at a cost that grows with every visitor.

### Possible future: a CLI or desktop app

The same core fits a local tool where each person brings their own credentials:

- their own Gemini API key and their own Spotify app, so every quota belongs to one person and one person's usage stays far below it;
- Spotify user authorization instead of client credentials, so the playlist can be created directly in the user's account instead of pasted;
- no server, database, rate limiting or cookies to run.

`internal/llm`, `internal/catalog`, `internal/mixtape` and `prompts` already have no HTTP or database dependencies and would carry over as they are.

## Quick start

Requirements: Docker with Compose.

```sh
cp .env.example .env   # set POSTGRES_PASSWORD
make up                # builds the image, starts app + PostgreSQL
curl localhost:8080/healthz
```

Open http://localhost:8080.

## Development

Requirements: Docker and make. Go, Node and linters run in containers (`golang:1.27`, `node:24`, `golangci-lint`); nothing else is installed on the host. Database access from tool containers uses host networking (Linux).

```sh
cp .env.example .env
make dev-db     # PostgreSQL in Docker
make run        # Go server (applies migrations at startup)
make web        # Astro dev server with hot reload, proxying /v1 to the Go server
make check      # vet + lint + test
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Decisions (ADRs)](docs/adr/)
- [API](docs/contracts/api.md) and [database](docs/contracts/db-schema.md) contracts
- [Contributing](CONTRIBUTING.md)

## License

[MIT](LICENSE)
