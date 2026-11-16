#!/usr/bin/env bash
#
# package.sh builds the release assets into dist/ using the naming conventions
# the deployment tool (shipit) and the agent's own updater expect. It never
# publishes anything: the release workflow does that with `gh release`.
#
# Usage:
#   TAG=v1.2.3 scripts/package.sh
#   make package                 # builds web/dist first, then runs this
#
# Requires web/dist to be built already (`pnpm --dir web build`).
#
# Assets produced:
#   dist/argus-server-<tag>-linux-amd64.tar.gz   root: ./argus-server
#   dist/argus-server-<tag>-linux-arm64.tar.gz
#   dist/argus-web-<tag>-any.tar.gz              root: ./index.html, ./assets/...
#   dist/argus-agent_linux_amd64                 raw binary (install-agent.sh)
#   dist/argus-agent_linux_arm64                 raw binary (install-agent.sh)
#   dist/checksums.txt                           sha256 of every asset above
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

TAG="${TAG:-$(git describe --tags --exact-match 2>/dev/null || true)}"
if [ -z "$TAG" ]; then
  echo "error: TAG is not set and HEAD is not an exact tag (use TAG=vX.Y.Z)" >&2
  exit 1
fi

DIST="${DIST:-dist}"
ARCHES="${ARCHES:-amd64 arm64}"

if [ ! -f web/dist/index.html ]; then
  echo "error: web/dist/index.html is missing; run: pnpm --dir web build" >&2
  exit 1
fi

sha256() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    echo "error: need sha256sum or shasum to write checksums.txt" >&2
    exit 1
  fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$DIST"
# Remove only the assets this script owns. Without this, a stale file from an
# earlier tag could be uploaded alongside the current release (gh release
# uploads dist/*) or end up in checksums.txt.
rm -f "$DIST"/argus-server-*.tar.gz "$DIST"/argus-web-*.tar.gz \
      "$DIST"/argus-agent_linux_* "$DIST/checksums.txt"

for arch in $ARCHES; do
  server_dir="$tmp/server-$arch"
  mkdir -p "$server_dir"

  echo "==> building argus-server linux/$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.serverVersion=$TAG" \
    -o "$server_dir/argus-server" ./server/cmd/argus-server
  tar -czf "$DIST/argus-server-$TAG-linux-$arch.tar.gz" -C "$server_dir" .

  # The agent keeps its existing raw-binary asset name: deploy/install-agent.sh
  # and `argus-agent update` download exactly argus-agent_linux_<arch>.
  echo "==> building argus-agent linux/$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.agentVersion=$TAG" \
    -o "$DIST/argus-agent_linux_$arch" ./agent/cmd/argus-agent
done

echo "==> packaging argus-web ($TAG)"
# .gitkeep is a build marker (see web/package.json), not part of the site.
tar -czf "$DIST/argus-web-$TAG-any.tar.gz" --exclude=.gitkeep -C web/dist .

(
  cd "$DIST"
  sha256 ./argus-server-*.tar.gz ./argus-web-*.tar.gz ./argus-agent_linux_* \
    | sed 's#  \./#  #' > checksums.txt
  echo "==> checksums.txt"
  cat checksums.txt
)

echo "==> done: $DIST/"
