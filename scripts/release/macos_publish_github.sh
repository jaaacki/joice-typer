#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
  echo "usage: $0 <archive-path> <dmg-path> <appcast-path> <checksums-path>"
  exit 2
fi

archive_path="$1"
dmg_path="$2"
appcast_path="$3"
checksums_path="$4"

tag="${RELEASE_TAG:-}"
if [ -z "$tag" ]; then
  echo "fatal: RELEASE_TAG is required"
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "fatal: GitHub CLI 'gh' is required to publish release artifacts"
  exit 1
fi

for path in "$archive_path" "$dmg_path" "$appcast_path" "$checksums_path"; do
  if [ ! -f "$path" ]; then
    echo "fatal: missing release artifact $path"
    exit 1
  fi
done

repo="${GITHUB_REPOSITORY:-}"
if [ -z "$repo" ]; then
  origin_url="$(git config --get remote.origin.url || true)"
  case "$origin_url" in
    git@github.com:*)
      repo="${origin_url#git@github.com:}"
      repo="${repo%.git}"
      ;;
    https://github.com/*)
      repo="${origin_url#https://github.com/}"
      repo="${repo%.git}"
      ;;
  esac
fi

if [ -z "$repo" ]; then
  echo "fatal: set GITHUB_REPOSITORY=owner/repo or configure a GitHub origin remote"
  exit 1
fi

gh release view "$tag" --repo "$repo" >/dev/null
gh release upload "$tag" "$archive_path" "$dmg_path" "$appcast_path" "$checksums_path" --clobber --repo "$repo"
