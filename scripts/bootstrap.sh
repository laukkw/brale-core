#!/usr/bin/env bash
set -euo pipefail

REPO_URL_DEFAULT="https://github.com/laukkw/brale-core.git"
REF_DEFAULT="main"
TARGET_DIR_DEFAULT="${HOME}/brale-core"
ONBOARDING_URL_DEFAULT="http://127.0.0.1:9992"

repo_url="${BRALE_REPO_URL:-${REPO_URL_DEFAULT}}"
ref="${BRALE_REF:-${REF_DEFAULT}}"
target_dir="${BRALE_DIR:-${TARGET_DIR_DEFAULT}}"
onboarding_url="${BRALE_ONBOARDING_URL:-${ONBOARDING_URL_DEFAULT}}"
open_browser=1

usage() {
  cat <<'EOF'
Usage: bootstrap.sh [options]

Options:
  --dir PATH          Target checkout directory (default: ~/brale-core)
  --ref REF           Git ref to checkout (default: main)
  --repo-url URL      Repository URL
  --onboarding-url U  Expected onboarding URL (default: http://127.0.0.1:9992)
  --no-open           Do not try to open the browser automatically
  -h, --help          Show this help text
EOF
}

log() {
  printf '%s
}

fail() {
  printf '[ERR] %s
  exit 1
}

require_cmd() {
  local cmd="$1"
  local hint="$2"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    fail "$cmd is required. $hint"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      target_dir="$2"
      shift 2
      ;;
    --ref)
      ref="$2"
      shift 2
      ;;
    --repo-url)
      repo_url="$2"
      shift 2
      ;;
    --onboarding-url)
      onboarding_url="$2"
      shift 2
      ;;
    --no-open)
      open_browser=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

require_cmd git "Install Git and retry."
require_cmd docker "Install Docker Desktop / Docker Engine with Compose V2 and retry."

if ! docker compose version >/dev/null 2>&1; then
  fail "docker compose is required. Install Docker Compose V2 and retry."
fi

if ! docker info >/dev/null 2>&1; then
  fail "Docker daemon is not running. Start Docker first."
fi

target_dir="${target_dir/#\~/${HOME}}"
parent_dir="$(dirname "$target_dir")"

if [[ ! -d "$parent_dir" ]]; then
  mkdir -p "$parent_dir"
fi

if [[ ! -d "$target_dir/.git" ]]; then
  log "[INFO] cloning brale-core into $target_dir"
  git clone "$repo_url" "$target_dir"
else
  log "[INFO] reusing existing checkout at $target_dir"
  current_origin="$(git -C "$target_dir" remote get-url origin 2>/dev/null || true)"
  if [[ -z "$current_origin" ]]; then
    fail "existing directory is not a usable git checkout: $target_dir"
  fi
  if [[ "$current_origin" != "$repo_url" && "$current_origin" != git@github.com:laukkw/brale-core.git ]]; then
    fail "existing checkout origin mismatch: $current_origin"
  fi
fi

if [[ -n "$(git -C "$target_dir" status --porcelain)" ]]; then
  log "[WARN] working tree is dirty; skipping git fetch/reset and using local files as-is"
else
  log "[INFO] updating checkout to $ref"
  git -C "$target_dir" fetch --tags origin
  git -C "$target_dir" checkout "$ref"
  if git -C "$target_dir" show-ref --verify --quiet "refs/remotes/origin/$ref"; then
    git -C "$target_dir" pull --ff-only origin "$ref"
  fi
fi

log "[INFO] building and starting onboarding container"
HOST_REPO_ROOT="$target_dir" docker compose --project-directory "$target_dir" -f "$target_dir/docker-compose.yml" up -d --build onboarding

ready=0
for _ in $(seq 1 60); do
  if curl -fsS "$onboarding_url/api/status" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done

if [[ "$ready" -ne 1 ]]; then
  HOST_REPO_ROOT="$target_dir" docker compose --project-directory "$target_dir" -f "$target_dir/docker-compose.yml" logs --tail=200 onboarding || true
  fail "onboarding did not become ready in time"
fi

log "[OK] onboarding is ready"
log "[OPEN] $onboarding_url"

if [[ "$open_browser" -eq 1 ]]; then
  if command -v open >/dev/null 2>&1; then
    open "$onboarding_url" >/dev/null 2>&1 || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$onboarding_url" >/dev/null 2>&1 || true
  fi
fi
