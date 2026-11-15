# argus build & release tasks.
# Requirements: Go 1.26+, pnpm 9+, goreleaser v2.
# The frontend is no longer embedded: build it with `make web` and serve
# web/dist either via the server's web_dir or a reverse proxy / Vite dev server.

.PHONY: deps web build run-server run-web test lint release snapshot clean

# Server config used by run-server; override for a local config file.
CONFIG ?= server.yaml

deps: ## install frontend dependencies
	pnpm --dir web install

web: ## build the frontend into web/dist
	pnpm --dir web build

build: ## build both binaries for the current host
	go build -o bin/argus-agent ./agent/cmd/argus-agent
	go build -o bin/argus-server ./server/cmd/argus-server

run-server: ## run the server locally (CONFIG=server.yaml by default)
	go run ./server/cmd/argus-server -config $(CONFIG)

run-web: ## run the Vite dev server, proxying /api to :8095
	pnpm --dir web dev

test: ## run Go tests with the race detector
	go test -race ./...

lint: ## typecheck + lint the frontend
	pnpm --dir web lint

release: ## publish a release via goreleaser (requires a tag)
	goreleaser release --clean

snapshot: ## local smoke-test of the release pipeline (no publish)
	goreleaser release --snapshot --clean

clean:
	rm -rf bin
