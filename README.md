# mixtaipes

Describe a moment, get its mixtape.

mixtaipes turns a sentence — *"driving through a rainy city at night in a 90s BMW"* — into a playlist. An LLM proposes the tracks, every track is verified against the Spotify catalog, you refine the result conversationally, and you paste the final list into Spotify.

> Status: early development. See the [roadmap](docs/roadmap/ROADMAP.md).

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
