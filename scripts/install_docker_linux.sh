#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "[ERR] this script is for Linux only"
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "[ERR] curl is required"
  exit 1
fi

target_user="${SUDO_USER:-$(id -un)}"
target_home="${HOME}"
if [[ -n "${SUDO_USER:-}" ]]; then
  target_home="$(eval echo "~${SUDO_USER}")"
fi

echo "[INFO] installing Docker Engine via official convenience script"
echo "[INFO] source: https://get.docker.com"
if [[ "$(id -u)" -eq 0 ]]; then
  curl -fsSL https://get.docker.com | sh
else
  curl -fsSL https://get.docker.com | sudo sh
fi

if command -v usermod >/dev/null 2>&1; then
  if [[ "${target_user}" == "root" ]]; then
    echo "[WARN] target user is root; skip docker group assignment"
  elif id -nG "${target_user}" | grep -qw docker; then
    echo "[INFO] user ${target_user} is already in docker group"
  else
    echo "[INFO] adding ${target_user} to docker group"
    if [[ "$(id -u)" -eq 0 ]]; then
      usermod -aG docker "${target_user}" || true
    else
      sudo usermod -aG docker "${target_user}" || true
    fi
    echo "[NEXT] re-login or run: newgrp docker"
  fi
fi

if docker compose version >/dev/null 2>&1; then
  echo "[OK] docker compose plugin is available"
else
  echo "[WARN] docker compose plugin missing; installing official standalone binary"
  compose_version="v2.40.0"
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) bin_arch="x86_64" ;;
    aarch64|arm64) bin_arch="aarch64" ;;
    armv7l) bin_arch="armv7" ;;
    *)
      echo "[ERR] unsupported architecture for docker-compose standalone: ${arch}"
      exit 1
      ;;
  esac
  plugin_dir="${target_home}/.docker/cli-plugins"
  mkdir -p "${plugin_dir}"
  curl -fSL "https://github.com/docker/compose/releases/download/${compose_version}/docker-compose-linux-${bin_arch}" -o "${plugin_dir}/docker-compose"
  chmod +x "${plugin_dir}/docker-compose"
  if [[ "$(id -u)" -eq 0 && "${target_user}" != "root" ]]; then
    chown -R "${target_user}:${target_user}" "${target_home}/.docker"
  fi
fi

echo "[OK] install done"
echo "[CHECK] docker --version"
echo "[CHECK] docker compose version"
