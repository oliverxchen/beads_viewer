#!/bin/bash

set -euo pipefail

TAG="bv-kanban-latest"
TITLE="bv-kanban (rolling latest)"
REPO=""

usage() {
  cat <<'EOF'
Publish a minimal bv-kanban binary to a single rolling GitHub release.

Usage:
  ./scripts/publish_bv_kanban_release.sh [-repo owner/repo] [-tag release-tag] [-title release-title]

Options:
  -repo   GitHub repo in owner/name form (default: inferred from git remote origin)
  -tag    Release tag name (default: bv-kanban-latest)
  -title  Release title (default: bv-kanban (rolling latest))
  -h      Show help

Build behavior:
  - Builds ./cmd/bv-kanban with a minimal configuration:
      CGO_ENABLED=0
      -trimpath
      -buildvcs=false
      -ldflags "-s -w -buildid="
  - Defaults to GOOS=linux GOARCH=amd64.
  - Honors GOOS/GOARCH environment overrides when provided.

Release behavior:
  - Keeps a single public release point (fixed tag).
  - Moves the tag to current HEAD each run.
  - Uploads/overwrites the asset each run.
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: required command not found: $cmd" >&2
    exit 1
  fi
}

infer_repo_from_origin() {
  local url
  url="$(git config --get remote.origin.url 2>/dev/null || true)"

  if [ -z "$url" ]; then
    return 1
  fi

  case "$url" in
    git@github.com:*.git)
      echo "${url#git@github.com:}" | sed 's/\.git$//'
      return 0
      ;;
    git@github.com:*)
      echo "${url#git@github.com:}"
      return 0
      ;;
    https://github.com/*/*.git)
      echo "${url#https://github.com/}" | sed 's/\.git$//'
      return 0
      ;;
    https://github.com/*/*)
      echo "${url#https://github.com/}"
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    -repo)
      if [ "$#" -lt 2 ]; then
        echo "Error: -repo requires a value" >&2
        exit 1
      fi
      REPO="$2"
      shift 2
      ;;
    -tag)
      if [ "$#" -lt 2 ]; then
        echo "Error: -tag requires a value" >&2
        exit 1
      fi
      TAG="$2"
      shift 2
      ;;
    -title)
      if [ "$#" -lt 2 ]; then
        echo "Error: -title requires a value" >&2
        exit 1
      fi
      TITLE="$2"
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

require_cmd git
require_cmd go
require_cmd gh

if [ -z "$REPO" ]; then
  REPO="$(infer_repo_from_origin || true)"
fi

if [ -z "$REPO" ]; then
  echo "Error: could not infer GitHub repo from remote.origin.url; pass -repo owner/name" >&2
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "Error: gh is not authenticated. Run: gh auth login" >&2
  exit 1
fi

GOOS_VALUE="${GOOS:-linux}"
GOARCH_VALUE="${GOARCH:-amd64}"

ASSET_NAME="bv-kanban_${GOOS_VALUE}_${GOARCH_VALUE}"
if [ "$GOOS_VALUE" = "windows" ]; then
  ASSET_NAME="${ASSET_NAME}.exe"
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

BIN_PATH="${TMP_DIR}/${ASSET_NAME}"

echo "==> Building minimal bv-kanban binary (${GOOS_VALUE}/${GOARCH_VALUE})"
CGO_ENABLED=0 GOTOOLCHAIN=auto go build \
  -trimpath \
  -buildvcs=false \
  -ldflags "-s -w -buildid=" \
  -o "$BIN_PATH" \
  ./cmd/bv-kanban

HEAD_SHA="$(git rev-parse HEAD)"

echo "==> Updating tag ${TAG} to ${HEAD_SHA} in ${REPO}"
if gh api "repos/${REPO}/git/ref/tags/${TAG}" >/dev/null 2>&1; then
  gh api "repos/${REPO}/git/refs/tags/${TAG}" \
    -X PATCH \
    -f sha="${HEAD_SHA}" \
    -F force=true \
    >/dev/null
else
  gh api "repos/${REPO}/git/refs" \
    -X POST \
    -f ref="refs/tags/${TAG}" \
    -f sha="${HEAD_SHA}" \
    >/dev/null
fi

NOTES="Rolling release for bv-kanban.\n\nThis release tag is intentionally reused and updated in place."

if gh release view "${TAG}" -R "${REPO}" >/dev/null 2>&1; then
  echo "==> Updating existing release ${TAG}"
  gh release upload "${TAG}" "${BIN_PATH}#${ASSET_NAME}" -R "${REPO}" --clobber
  gh release edit "${TAG}" -R "${REPO}" --title "${TITLE}" --notes "${NOTES}" --latest
else
  echo "==> Creating release ${TAG}"
  gh release create "${TAG}" "${BIN_PATH}#${ASSET_NAME}" -R "${REPO}" \
    --title "${TITLE}" \
    --notes "${NOTES}" \
    --latest
fi

echo ""
echo "Published: https://github.com/${REPO}/releases/tag/${TAG}"
echo "Asset:     ${ASSET_NAME}"
