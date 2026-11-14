# argus build & release tasks.
# Requirements: Go 1.26+, pnpm 9+, goreleaser v2.
# The server embeds web/dist (built by `make web`); without it the server still
# compiles but serves no dashboard.

.PHONY: deps web build test lint release snapshot clean

deps: ## install frontend dependencies
	pnpm --dir web install

web: ## build the frontend into web/dist
	pnpm --dir web build

build: ## build both binaries for the current host
	go build -o bin/argus-agent ./cmd/agent
	go build -o bin/argus-server ./cmd/server

test: ## run Go tests with the race detector
	go test -race ./...

lint: ## typecheck + lint the frontend
	pnpm --dir web lint

release: ## build frontend + publish a release via goreleaser (requires a tag)
	goreleaser release --clean

snapshot: ## local smoke-test of the release pipeline (no publish)
	goreleaser release --snapshot --clean

clean:
	rm -rf bin
