#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <archive-path> <dmg-path>"
  exit 2
fi

archive_path="$1"
dmg_path="$2"

require_path() {
  path="$1"
  description="$2"
  if [ ! -e "$path" ]; then
    echo "fatal: missing $description at $path"
    exit 1
  fi
}

require_cmd() {
  name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "fatal: required command '$name' is unavailable"
    exit 1
  fi
}

require_path "$archive_path" "zip archive"
require_path "$dmg_path" "dmg artifact"
require_cmd codesign
require_cmd ditto
require_cmd hdiutil

work_dir="$(mktemp -d)"
mount_dir="$work_dir/mount"
detached=0
cleanup() {
  if [ "$detached" -eq 0 ] && mount | grep -F "on $mount_dir " >/dev/null 2>&1; then
    hdiutil detach "$mount_dir" >/dev/null 2>&1 || true
  fi
  rm -rf "$work_dir"
}
trap cleanup EXIT INT TERM

zip_dir="$work_dir/zip"
mkdir -p "$zip_dir"
ditto -x -k "$archive_path" "$zip_dir"
zip_app="$zip_dir/JoiceTyper.app"
require_path "$zip_app/Contents/MacOS/JoiceTyper" "zip app executable"
codesign --verify --deep --strict --verbose=2 "$zip_app" >/dev/null

mkdir -p "$mount_dir"
hdiutil attach "$dmg_path" -nobrowse -readonly -mountpoint "$mount_dir" >/dev/null
install_dir="$work_dir/install"
mkdir -p "$install_dir"
ditto "$mount_dir/JoiceTyper.app" "$install_dir/JoiceTyper.app"
hdiutil detach "$mount_dir" >/dev/null
detached=1

installed_app="$install_dir/JoiceTyper.app"
require_path "$installed_app/Contents/MacOS/JoiceTyper" "installed app executable"
codesign --verify --deep --strict --verbose=2 "$installed_app" >/dev/null

if [ "${MACOS_LOCAL_RC_SMOKE_LAUNCH:-0}" = "1" ]; then
  open -n "$installed_app"
  launched=0
  i=0
  while [ "$i" -lt 20 ]; do
    if pgrep -f "$installed_app/Contents/MacOS/JoiceTyper" >/dev/null 2>&1; then
      launched=1
      break
    fi
    i=$((i + 1))
    sleep 0.5
  done
  if [ "$launched" -ne 1 ]; then
    echo "fatal: app did not launch from installed DMG copy"
    exit 1
  fi
  pkill -f "$installed_app/Contents/MacOS/JoiceTyper" >/dev/null 2>&1 || true
fi

printf '%s\n' "Smoke-tested local macOS release candidate packages:"
printf '  zip extraction: %s\n' "$zip_app"
printf '  dmg install copy: %s\n' "$installed_app"
