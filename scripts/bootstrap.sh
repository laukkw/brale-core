#!/usr/bin/env bash
set -euo pipefail

# Detect defaults from the current repo context when possible.
_detect_repo_url() {
  if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git remote get-url origin 2>/dev/null || true
  fi
}

_detect_ref() {
  if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git symbolic-ref --short HEAD 2>/dev/null || true
  fi
}

_detected_url="$(_detect_repo_url)"
_detected_ref="$(_detect_ref)"

REPO_URL_DEFAULT="${_detected_url:-https://github.com/laukkw/brale-core.git}"
REF_DEFAULT="${_detected_ref:-master}"
TARGET_DIR_DEFAULT="${HOME}/brale-core"
WAIT_STACK_TIMEOUT="${BRALE_WAIT_STACK_TIMEOUT:-120}"

repo_url="${BRALE_REPO_URL:-${REPO_URL_DEFAULT}}"
ref="${BRALE_REF:-${REF_DEFAULT}}"
target_dir="${BRALE_DIR:-${TARGET_DIR_DEFAULT}}"
compose_project_name="${BRALE_COMPOSE_PROJECT_NAME:-brale-core}"
run_init=1
with_mcp=0
run_setup=0
setup_lang=""
full_clone=0
host_uid="$(id -u)"
host_gid="$(id -g)"

usage() {
  cat <<'EOF'
Usage: bootstrap.sh [options]

Options:
  --dir PATH          Target checkout directory (default: ~/brale-core)
  --ref REF           Git ref to checkout (default: master)
  --repo-url URL      Repository URL
  --no-init           Skip the interactive init wizard and start with the existing .env
  --no-onboarding     Deprecated alias for --no-init
  --with-mcp          Start the stack with the optional MCP Streamable HTTP service
  --setup             Run 'make setup' after clone/update
  --setup-lang LANG   Preselect setup wizard language (zh or en)
  --full-clone        Clone the full Git history instead of the default shallow checkout
  -h, --help          Show this help text
EOF
}

log() {
  printf '%s\n' "$1"
}

fail() {
  printf '[ERR] %s\n' "$1" >&2
  exit 1
}

require_cmd() {
  local cmd="$1"
  local hint="$2"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    fail "$cmd is required. $hint"
  fi
}

run_make_with_tty() {
  local -a cmd=("$@")
  if [[ -r /dev/tty && -w /dev/tty ]]; then
    (cd "$target_dir" && HOST_UID="$host_uid" HOST_GID="$host_gid" COMPOSE_PROJECT_NAME="$compose_project_name" "${cmd[@]}") </dev/tty >/dev/tty
  else
    (cd "$target_dir" && HOST_UID="$host_uid" HOST_GID="$host_gid" COMPOSE_PROJECT_NAME="$compose_project_name" "${cmd[@]}")
  fi
}

detect_env_enable_mcp() {
  if [[ "$with_mcp" -eq 1 ]]; then
    return 0
  fi
  if [[ ! -f "$target_dir/.env" ]]; then
    return 0
  fi
  local value
  value="$(awk -F= '/^[[:space:]]*ENABLE_MCP[[:space:]]*=/{gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2; exit}' "$target_dir/.env" 2>/dev/null || true)"
  case "${value,,}" in
    1|true|yes|on)
      with_mcp=1
      ;;
  esac
}

wait_for_stack() {
  local ready=0
  for _ in $(seq 1 "$WAIT_STACK_TIMEOUT"); do
    if curl -fsS "http://127.0.0.1:9991/healthz" >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 1
  done

  if [[ "$ready" -ne 1 ]]; then
    (cd "$target_dir" && HOST_UID="$host_uid" HOST_GID="$host_gid" COMPOSE_PROJECT_NAME="$compose_project_name" docker compose -f docker-compose.yml logs --tail=200 brale freqtrade) || true
    return 1
  fi
}

print_stack_endpoints() {
  log "[OK] stack started"
  log "[OPEN] Freqtrade API: http://127.0.0.1:8080/api/v1/ping"
  log "[OPEN] Brale health:  http://127.0.0.1:9991/healthz"
  if [[ "$with_mcp" -eq 1 ]]; then
    log "[OPEN] MCP HTTP:      http://127.0.0.1:8765/mcp"
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
    --no-init)
      run_init=0
      shift
      ;;
    --no-onboarding)
      run_init=0
      log "[WARN] --no-onboarding is deprecated; use --no-init instead"
      shift
      ;;
    --with-mcp)
      with_mcp=1
      shift
      ;;
    --setup)
      run_setup=1
      shift
      ;;
    --setup-lang)
      case "$2" in
        zh|en)
          setup_lang="$2"
          ;;
        *)
          fail "invalid --setup-lang: must be 'zh' or 'en'"
          ;;
      esac
      shift 2
      ;;
    --full-clone)
      full_clone=1
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
  if [[ "$full_clone" -eq 1 ]]; then
    log "[INFO] cloning full brale-core history into $target_dir"
    git clone "$repo_url" "$target_dir"
  else
    log "[INFO] shallow cloning brale-core ref $ref into $target_dir"
    if ! git clone --depth 1 --branch "$ref" "$repo_url" "$target_dir"; then
      fail "shallow clone failed for ref '$ref'. Check the ref name or rerun with --full-clone."
    fi
  fi
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
  is_shallow="$(git -C "$target_dir" rev-parse --is-shallow-repository 2>/dev/null || printf 'false')"
  if [[ "$full_clone" -eq 1 || "$is_shallow" != "true" ]]; then
    git -C "$target_dir" fetch --tags origin
    git -C "$target_dir" checkout "$ref"
    if git -C "$target_dir" show-ref --verify --quiet "refs/remotes/origin/$ref"; then
      git -C "$target_dir" pull --ff-only origin "$ref"
    fi
  else
    if ! git -C "$target_dir" fetch --depth 1 origin "$ref"; then
      fail "failed to update shallow checkout for ref '$ref'. Check the ref name or rerun with --full-clone."
    fi
    if git -C "$target_dir" show-ref --verify --quiet "refs/heads/$ref"; then
      git -C "$target_dir" checkout "$ref"
      git -C "$target_dir" reset --hard FETCH_HEAD
    else
      git -C "$target_dir" checkout --detach FETCH_HEAD
    fi
  fi
fi

log "[INFO] ensuring .env exists"
env_output="$(cd "$target_dir" && HOST_UID="$host_uid" HOST_GID="$host_gid" COMPOSE_PROJECT_NAME="$compose_project_name" make env-init 2>&1)" || {
  printf '%s\n' "$env_output"
  fail "failed to initialize .env from .env.example"
}
printf '%s\n' "$env_output"

if [[ "$run_setup" -eq 1 ]]; then
  log "[INFO] running interactive setup wizard"
  setup_cmd=(make setup)
  if [[ -n "$setup_lang" ]]; then
    setup_cmd+=("SETUP_LANG=$setup_lang")
  fi
  if ! run_make_with_tty "${setup_cmd[@]}"; then
    fail "setup wizard failed"
  fi
fi

detect_env_enable_mcp

if [[ "$run_init" -ne 1 ]]; then
  log "[INFO] init wizard skipped; checking whether the stack can start headlessly"
  if check_output="$(cd "$target_dir" && HOST_UID="$host_uid" HOST_GID="$host_gid" COMPOSE_PROJECT_NAME="$compose_project_name" make check 2>&1)"; then
    printf '%s\n' "$check_output"
    log "[INFO] starting core stack"
    start_cmd=(make start)
    if [[ "$with_mcp" -eq 1 ]]; then
      start_cmd+=("ENABLE_MCP=1")
    fi
    start_output="$(cd "$target_dir" && HOST_UID="$host_uid" HOST_GID="$host_gid" COMPOSE_PROJECT_NAME="$compose_project_name" "${start_cmd[@]}" 2>&1)" || {
      printf '%s\n' "$start_output"
      fail "failed to start stack without init"
    }
    printf '%s\n' "$start_output"
    wait_for_stack || fail "stack did not become ready in time"
    print_stack_endpoints
    exit 0
  fi
  printf '%s\n' "$check_output"
  log "[WARN] .env is not ready for headless startup; skipping docker compose up"
  log "[NEXT] edit .env manually, run 'make setup', or rerun bootstrap without --no-init"
  exit 0
fi

log "[INFO] running interactive init wizard"
if ! run_make_with_tty make init; then
  fail "failed to complete make init"
fi
detect_env_enable_mcp

log "[INFO] starting Docker stack"
start_cmd=(make start)
if [[ "$with_mcp" -eq 1 ]]; then
  start_cmd+=("ENABLE_MCP=1")
fi
start_output="$(cd "$target_dir" && HOST_UID="$host_uid" HOST_GID="$host_gid" COMPOSE_PROJECT_NAME="$compose_project_name" "${start_cmd[@]}" 2>&1)" || {
  printf '%s\n' "$start_output"
  fail "failed to start Docker stack"
}
printf '%s\n' "$start_output"
wait_for_stack || fail "stack did not become ready in time"
print_stack_endpoints
