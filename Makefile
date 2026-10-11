# Toolchain images: the only host requirements are Docker and make.
GO_IMAGE   := golang:1.27-alpine
NODE_IMAGE := node:24-alpine
LINT_IMAGE := golangci/golangci-lint:v2.14.0-alpine

# Caches live on the host so containers stay disposable and runs stay fast.
CACHE := $(HOME)/.cache/mixtaipes

# Host networking lets containers reach the compose database on localhost (Linux).
DOCKER_RUN := docker run --rm --network host --user $$(id -u):$$(id -g) \
	-v $(CURDIR):/src -w /src -v $(CACHE):/cache -e HOME=/tmp
GO_ENV     := -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod \
	-e GOLANGCI_LINT_CACHE=/cache/golangci-lint -e GOTOOLCHAIN=local
GO         := $(DOCKER_RUN) $(GO_ENV) $(GO_IMAGE) go
NPM        := $(DOCKER_RUN) -e npm_config_cache=/cache/npm -w /src/web $(NODE_IMAGE) npm

.PHONY: dev-db run web web-build fmt vet test lint tidy check build up down

$(CACHE):
	mkdir -p $@

# Start only the database (for local development).
dev-db:
	docker compose up -d db

# Run the Go server with variables from .env (applies migrations at startup).
run: | $(CACHE)
	$(DOCKER_RUN) $(GO_ENV) --env-file .env $(GO_IMAGE) go run ./cmd/mixtaipes

# Astro dev server with hot reload; proxies /v1 to the Go server (API_URL in .env).
web: | $(CACHE)
	$(DOCKER_RUN) -e npm_config_cache=/cache/npm --env-file .env -w /src/web $(NODE_IMAGE) npm run dev

web-build: | $(CACHE)
	$(NPM) ci --no-audit --no-fund
	$(NPM) run build

fmt: | $(CACHE)
	$(GO) fmt ./...

vet: | $(CACHE)
	$(GO) vet ./...

# Database tests run when TEST_DATABASE_URL is set, e.g. after `make dev-db`.
test: | $(CACHE)
	$(DOCKER_RUN) $(GO_ENV) -e TEST_DATABASE_URL $(GO_IMAGE) go test ./...

lint: | $(CACHE)
	$(DOCKER_RUN) $(GO_ENV) $(LINT_IMAGE) golangci-lint run

tidy: | $(CACHE)
	$(GO) mod tidy

check: vet lint test

build: web-build
	$(DOCKER_RUN) $(GO_ENV) -e CGO_ENABLED=0 $(GO_IMAGE) go build -trimpath -o bin/mixtaipes ./cmd/mixtaipes

up:
	docker compose up -d --build

down:
	docker compose down
