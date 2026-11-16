# argus build & release tasks.
# Requirements: Go 1.26+, pnpm 9+.
# The frontend is a separate deployment artifact: build it with `make web`,
# then `make package` puts both the Go binaries and web/dist into the release
# assets consumed by shipit (server, web) and install-agent.sh (agent).

.PHONY: deps web build run-server run-web test lint package release clean

# Local runtime config, gitignored. Generated from the committed example on
# first use. Override CONFIG to point at another file without triggering the
# copy.
LOCAL_CONFIG ?= server/config.local.yaml
CONFIG ?= $(LOCAL_CONFIG)

# Release tag. Empty unless HEAD is exactly a tag; pass TAG=vX.Y.Z to override.
TAG ?= $(shell git describe --tags --exact-match 2>/dev/null)

deps: ## install frontend dependencies
	pnpm --dir web install

web: ## build the frontend into web/dist
	pnpm --dir web build

build: ## build both binaries for the current host
	go build -o bin/argus-agent ./agent/cmd/argus-agent
	go build -o bin/argus-server ./server/cmd/argus-server

run-server: ## run the server locally (creates the local config from the example if missing)
	@if [ "$(CONFIG)" = "$(LOCAL_CONFIG)" ] && [ ! -f "$(CONFIG)" ]; then \
		cp server/config.example.yaml "$(CONFIG)"; \
		echo "created $(CONFIG) from server/config.example.yaml"; \
	fi
	go run ./server/cmd/argus-server -config $(CONFIG)

run-web: ## run the Vite dev server, proxying /api to :8095
	pnpm --dir web dev

test: ## run Go tests with the race detector
	go test -race ./...

lint: ## typecheck + lint the frontend
	pnpm --dir web lint

package: web ## build release assets into dist/ (shipit + agent naming)
	TAG=$(TAG) ./scripts/package.sh

release: package ## alias for package; CI publishes and deploys on a tag push

clean:
	rm -rf bin dist
