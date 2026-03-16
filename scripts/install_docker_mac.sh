#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "[ERR] this script is for macOS only"
  exit 1
fi

arch="$(uname -m)"
case "${arch}" in
  arm64)
    dmg_url="https://desktop.docker.com/mac/main/arm64/Docker.dmg"
    ;;
  x86_64)
    dmg_url="https://desktop.docker.com/mac/main/amd64/Docker.dmg"
    ;;
  *)
    echo "[ERR] unsupported macOS arch: ${arch}"
    exit 1
    ;;
esac

tmp_dir="$(mktemp -d)"
dmg_path="${tmp_dir}/Docker.dmg"

cleanup() {
  if mount | grep -q "${tmp_dir}/mnt"; then
    hdiutil detach "${tmp_dir}/mnt" >/dev/null 2>&1 || true
  fi
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

echo "[INFO] downloading Docker Desktop from official source"
curl -fL "${dmg_url}" -o "${dmg_path}"

mkdir -p "${tmp_dir}/mnt"
hdiutil attach "${dmg_path}" -nobrowse -mountpoint "${tmp_dir}/mnt"

echo "[INFO] installing Docker.app to /Applications (sudo required)"
sudo rm -rf /Applications/Docker.app
sudo cp -R "${tmp_dir}/mnt/Docker.app" /Applications/Docker.app
hdiutil detach "${tmp_dir}/mnt" >/dev/null

echo "[OK] Docker Desktop installed"
echo "[NEXT] run: open -a Docker"
echo "[NEXT] wait until Docker daemon is ready, then run: docker compose version"
