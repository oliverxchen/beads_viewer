#!/bin/bash

set -euo pipefail

OUTPUT=""

usage() {
  cat <<'EOF'
Build a stripped local bv-kanban binary that keeps --watch support.

Usage:
  ./scripts/build_bv_kanban_local.sh [-o output-path]

Options:
  -o      Output path (default: ~/go/bin/bv-kanban, or .exe on Windows targets)
  -h      Show help

Build behavior:
  - Builds ./cmd/bv-kanban with watch mode enabled.
  - Uses a size-oriented configuration:
      CGO_ENABLED=0
      -trimpath
      -buildvcs=false
      -ldflags "-s -w -buildid="
  - Honors GOOS/GOARCH environment overrides when provided.

Examples:
  ./scripts/build_bv_kanban_local.sh
  GOOS=linux GOARCH=amd64 ./scripts/build_bv_kanban_local.sh -o ./dist/bv-kanban-linux-amd64
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: required command not found: $cmd" >&2
    exit 1
  fi
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      if [ "$#" -lt 2 ]; then
        echo "Error: -o requires a value" >&2
        exit 1
      fi
      OUTPUT="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

require_cmd go

GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"

if [ -z "$OUTPUT" ]; then
  OUTPUT="$HOME/go/bin/bv-kanban"
  if [ "$GOOS_VALUE" = "windows" ]; then
    OUTPUT="${OUTPUT}.exe"
  fi
fi

mkdir -p "$(dirname "$OUTPUT")"

echo "==> Building stripped local bv-kanban (${GOOS_VALUE}/${GOARCH_VALUE})"
echo "==> Output: ${OUTPUT}"
CGO_ENABLED=0 GOTOOLCHAIN=auto go build \
  -trimpath \
  -buildvcs=false \
  -ldflags "-s -w -buildid=" \
  -o "$OUTPUT" \
  ./cmd/bv-kanban

echo ""
echo "Built: ${OUTPUT}"
