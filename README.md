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

Requirements: Go 1.25+, Node 24+, Docker.

```sh
cp .env.example .env
make dev-db     # PostgreSQL in Docker
make run        # Go server on the host (applies migrations at startup)
make web        # Astro dev server with hot reload, proxying /v1 to the Go server
make test lint
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Decisions (ADRs)](docs/adr/)
- [API](docs/contracts/api.md) and [database](docs/contracts/db-schema.md) contracts
- [Contributing](CONTRIBUTING.md)

## License

[MIT](LICENSE)
