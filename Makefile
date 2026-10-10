.PHONY: dev-db run web web-build test lint build up down

# Start only the database (for local development).
dev-db:
	docker compose up -d db

# Run the Go server on the host with variables from .env.
run:
	set -a && . ./.env && set +a && go run ./cmd/mixtaipes

# Astro dev server with hot reload; proxies /v1 to the Go server.
web:
	set -a && . ./.env && set +a && cd web && npm run dev

web-build:
	cd web && npm ci --no-audit --no-fund && npm run build

test:
	go test ./...

lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null; then golangci-lint run; else echo "golangci-lint not installed; skipped"; fi

build: web-build
	go build -trimpath -o bin/mixtaipes ./cmd/mixtaipes

up:
	docker compose up -d --build

down:
	docker compose down
