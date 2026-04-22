#!/bin/sh
set -eu

usage() {
  echo "usage: $0 <archive|appcast|notarize|sparkle>"
  exit 2
}

mode="${1:-}"
[ -n "$mode" ] || usage

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
env_file="${MACOS_RELEASE_ENV_FILE:-$repo_root/packaging/macos/release.env.local}"

if [ ! -f "$env_file" ]; then
  echo "fatal: missing mac release env file $env_file"
  exit 1
fi

# shellcheck disable=SC1090
. "$env_file"

require_var() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "fatal: missing required mac release setting $name"
    exit 1
  fi
}

case "$mode" in
  archive)
    require_var MACOS_CODESIGN_IDENTITY
    require_var MACOS_SPARKLE_PRIVATE_KEY_FILE
    ;;
  appcast)
    require_var MACOS_APPCAST_BASE_URL
    require_var MACOS_SPARKLE_PUBLIC_ED_KEY
    ;;
  notarize)
    require_var MACOS_NOTARYTOOL_PROFILE
    ;;
  sparkle)
    archive_path="${MACOS_SPARKLE_ARCHIVE_PATH:-}"
    download_url="${MACOS_SPARKLE_DOWNLOAD_URL:-}"
    if [ -z "$archive_path" ] && [ -z "$download_url" ]; then
      echo "fatal: set MACOS_SPARKLE_ARCHIVE_PATH or MACOS_SPARKLE_DOWNLOAD_URL"
      exit 1
    fi
    ;;
  *)
    usage
    ;;
esac
