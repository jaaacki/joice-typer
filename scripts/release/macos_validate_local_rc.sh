#!/bin/sh
set -eu

if [ "$#" -ne 6 ]; then
  echo "usage: $0 <app-bundle> <archive-path> <dmg-path> <metadata-path> <checksums-path> <expected-version>"
  exit 2
fi

app_bundle="$1"
archive_path="$2"
dmg_path="$3"
metadata_path="$4"
checksums_path="$5"
expected_version="$6"

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

require_path "$app_bundle" "app bundle"
require_path "$archive_path" "zip archive"
require_path "$dmg_path" "dmg artifact"
require_path "$metadata_path" "metadata file"
require_path "$checksums_path" "checksums file"

if [ ! -d "$app_bundle" ]; then
  echo "fatal: app bundle is not a directory: $app_bundle"
  exit 1
fi

require_cmd codesign
require_cmd plutil
require_cmd shasum
require_cmd hdiutil
require_cmd otool
require_cmd zipinfo
require_cmd lipo

executable="$app_bundle/Contents/MacOS/JoiceTyper"
plist="$app_bundle/Contents/Info.plist"
framework="$app_bundle/Contents/Frameworks/libportaudio.2.dylib"

require_path "$executable" "app executable"
require_path "$plist" "Info.plist"
require_path "$framework" "bundled PortAudio dylib"

plutil -lint "$plist" >/dev/null
codesign --verify --deep --strict --verbose=2 "$app_bundle" >/dev/null

signature_details="$(codesign -d --verbose=4 "$app_bundle" 2>&1)"
case "$signature_details" in
  *"flags="*"runtime"*) ;;
  *)
    echo "fatal: app signature does not include hardened runtime"
    exit 1
    ;;
esac
case "$signature_details" in
  *"adhoc"*) ;;
  *)
    echo "fatal: local release candidate must be ad-hoc signed"
    exit 1
    ;;
esac

entitlements_xml="$(codesign -d --entitlements :- "$app_bundle" 2>/dev/null)"
if ! printf '%s' "$entitlements_xml" | plutil -lint - >/dev/null; then
  echo "fatal: app entitlements are not readable"
  exit 1
fi
entitlements_tmp="$(mktemp)"
printf '%s' "$entitlements_xml" | plutil -convert xml1 -o "$entitlements_tmp" -
if ! grep -F "<dict/>" "$entitlements_tmp" >/dev/null 2>&1; then
  echo "fatal: local release candidate entitlements must match the empty Developer ID rehearsal profile"
  rm -f "$entitlements_tmp"
  exit 1
fi
rm -f "$entitlements_tmp"

if ! lipo -archs "$executable" | grep -F "arm64" >/dev/null 2>&1; then
  echo "fatal: app executable is not arm64"
  exit 1
fi

bundle_id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$plist")"
if [ "$bundle_id" != "com.joicetyper.app" ]; then
  echo "fatal: unexpected bundle id $bundle_id"
  exit 1
fi

short_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$plist")"
bundle_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$plist")"
if [ "$short_version" != "$expected_version" ]; then
  echo "fatal: CFBundleShortVersionString $short_version does not match $expected_version"
  exit 1
fi
if [ "$bundle_version" != "$expected_version" ]; then
  echo "fatal: CFBundleVersion $bundle_version does not match $expected_version"
  exit 1
fi

min_os="$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$plist")"
if [ -z "$min_os" ]; then
  echo "fatal: missing LSMinimumSystemVersion"
  exit 1
fi

mic_usage="$(/usr/libexec/PlistBuddy -c 'Print :NSMicrophoneUsageDescription' "$plist")"
if [ -z "$mic_usage" ]; then
  echo "fatal: missing NSMicrophoneUsageDescription"
  exit 1
fi

if ! otool -L "$executable" | grep -F "@executable_path/../Frameworks/libportaudio.2.dylib" >/dev/null 2>&1; then
  echo "fatal: executable does not link bundled PortAudio dylib"
  exit 1
fi
if otool -L "$executable" | grep -F "/opt/homebrew/opt/portaudio" >/dev/null 2>&1; then
  echo "fatal: executable still links Homebrew PortAudio path"
  exit 1
fi

if [ ! -f "$app_bundle/Contents/Resources/icon.icns" ]; then
  echo "fatal: app bundle is missing icon.icns"
  exit 1
fi

if ! zipinfo -1 "$archive_path" | grep -F "JoiceTyper.app/Contents/MacOS/JoiceTyper" >/dev/null 2>&1; then
  echo "fatal: zip archive does not contain the app executable"
  exit 1
fi

hdiutil imageinfo "$dmg_path" >/dev/null

archive_sha="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
dmg_sha="$(shasum -a 256 "$dmg_path" | awk '{print $1}')"
archive_length="$(wc -c < "$archive_path" | tr -d ' ')"
archive_path_abs="$(CDPATH= cd -- "$(dirname -- "$archive_path")" && pwd)/$(basename -- "$archive_path")"

if ! grep -F "$archive_sha  $(basename "$archive_path")" "$checksums_path" >/dev/null 2>&1; then
  echo "fatal: checksums file does not contain zip checksum"
  exit 1
fi

if ! grep -F "$dmg_sha  $(basename "$dmg_path")" "$checksums_path" >/dev/null 2>&1; then
  echo "fatal: checksums file does not contain dmg checksum"
  exit 1
fi

metadata_value() {
  key="$1"
  value="$(grep -E "^$key=" "$metadata_path" | sed -n '1s/^[^=]*=//p')"
  if [ -z "$value" ]; then
    echo "fatal: metadata file missing $key"
    exit 1
  fi
  printf '%s' "$value"
}

metadata_version="$(metadata_value VERSION)"
metadata_archive_path="$(metadata_value ARCHIVE_PATH)"
metadata_archive_length="$(metadata_value ARCHIVE_LENGTH)"
metadata_archive_sha="$(metadata_value ARCHIVE_SHA256)"
metadata_value PUBLICATION_DATE >/dev/null
metadata_signature="$(metadata_value EDDSA_SIGNATURE)"

if [ "$metadata_version" != "$expected_version" ]; then
  echo "fatal: metadata VERSION $metadata_version does not match expected version $expected_version"
  exit 1
fi

if [ "$metadata_archive_path" != "$archive_path_abs" ]; then
  echo "fatal: metadata ARCHIVE_PATH $metadata_archive_path does not match $archive_path_abs"
  exit 1
fi

if [ "$metadata_archive_length" != "$archive_length" ]; then
  echo "fatal: metadata ARCHIVE_LENGTH $metadata_archive_length does not match $archive_length"
  exit 1
fi

if [ "$metadata_archive_sha" != "$archive_sha" ]; then
  echo "fatal: metadata ARCHIVE_SHA256 $metadata_archive_sha does not match $archive_sha"
  exit 1
fi

if [ "$metadata_signature" != "UNSIGNED" ]; then
  echo "fatal: local release candidate metadata must be UNSIGNED"
  exit 1
fi

printf '%s\n' "Validated local macOS release candidate:"
printf '  app: %s\n' "$app_bundle"
printf '  zip: %s\n' "$archive_path"
printf '  dmg: %s\n' "$dmg_path"
printf '  checksums: %s\n' "$checksums_path"
