#!/bin/sh
set -eu

usage() {
  echo "usage: $0 <archive|notarize|publish>"
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

require_cmd() {
  name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "fatal: required command '$name' is unavailable"
    exit 1
  fi
}

require_file() {
  path="$1"
  description="$2"
  if [ ! -f "$path" ]; then
    echo "fatal: missing $description at $path"
    exit 1
  fi
}

require_var() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "fatal: missing required mac release setting $name"
    exit 1
  fi
}

check_codesign_identity() {
  identity="$1"
  if ! security find-identity -v -p codesigning 2>/dev/null | grep -F "$identity" >/dev/null 2>&1; then
    echo "fatal: codesign identity not found in keychain: $identity"
    exit 1
  fi
}

case "$mode" in
  archive)
    require_cmd security
    require_cmd codesign
    require_cmd xcrun
    require_var MACOS_CODESIGN_IDENTITY
    require_var MACOS_SPARKLE_PRIVATE_KEY_FILE
    check_codesign_identity "$MACOS_CODESIGN_IDENTITY"
    require_file "$MACOS_SPARKLE_PRIVATE_KEY_FILE" "Sparkle private key"
    ;;
  notarize)
    require_cmd xcrun
    require_var MACOS_NOTARYTOOL_PROFILE
    if ! xcrun notarytool history --keychain-profile "$MACOS_NOTARYTOOL_PROFILE" >/dev/null 2>&1; then
      echo "fatal: unable to access notarytool profile $MACOS_NOTARYTOOL_PROFILE"
      exit 1
    fi
    ;;
  publish)
    require_cmd gh
    require_var GITHUB_REPOSITORY
    gh auth status >/dev/null 2>&1 || {
      echo "fatal: GitHub CLI is not authenticated"
      exit 1
    }
    ;;
  *)
    usage
    ;;
esac
