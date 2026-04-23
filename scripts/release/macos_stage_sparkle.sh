#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <stage-dir>"
  exit 2
fi

stage_dir="$1"
repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
env_file="${MACOS_RELEASE_ENV_FILE:-$repo_root/packaging/macos/release.env.local}"

if [ ! -f "$env_file" ]; then
  echo "fatal: missing mac release env file $env_file"
  exit 1
fi

# shellcheck disable=SC1090
. "$env_file"

archive_path="${MACOS_SPARKLE_ARCHIVE_PATH:-}"
download_url="${MACOS_SPARKLE_DOWNLOAD_URL:-}"
download_sha256="${MACOS_SPARKLE_DOWNLOAD_SHA256:-}"

mkdir -p "$stage_dir"
archive_file="$stage_dir/sparkle.tar.xz"

if [ -n "$archive_path" ]; then
  cp "$archive_path" "$archive_file"
elif [ -n "$download_url" ]; then
  if [ -z "$download_sha256" ]; then
    echo "fatal: set MACOS_SPARKLE_DOWNLOAD_SHA256 when using MACOS_SPARKLE_DOWNLOAD_URL"
    exit 1
  fi
  curl -L --fail --output "$archive_file" "$download_url"
  actual_sha256="$(shasum -a 256 "$archive_file" | awk '{print $1}')"
  if [ "$actual_sha256" != "$download_sha256" ]; then
    echo "fatal: Sparkle archive checksum mismatch"
    echo "expected: $download_sha256"
    echo "actual:   $actual_sha256"
    exit 1
  fi
else
  echo "fatal: set MACOS_SPARKLE_ARCHIVE_PATH or MACOS_SPARKLE_DOWNLOAD_URL"
  exit 1
fi

rm -rf "$stage_dir/Sparkle"
mkdir -p "$stage_dir/Sparkle"
tar -xf "$archive_file" -C "$stage_dir/Sparkle" --strip-components=1
