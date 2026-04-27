#!/bin/sh
set -eu

if [ "$#" -ne 7 ]; then
  echo "usage: $0 <app-bundle> <archive-path> <dmg-path> <appcast-path> <metadata-path> <checksums-path> <expected-version>"
  exit 2
fi

app_bundle="$1"
archive_path="$2"
dmg_path="$3"
appcast_path="$4"
metadata_path="$5"
checksums_path="$6"
expected_version="$7"

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

for cmd in codesign plutil shasum hdiutil otool zipinfo unzip python3 lipo; do
  require_cmd "$cmd"
done

require_path "$app_bundle" "release app bundle"
require_path "$archive_path" "release zip archive"
require_path "$dmg_path" "release dmg artifact"
require_path "$appcast_path" "release appcast"
require_path "$metadata_path" "release metadata"
require_path "$checksums_path" "release checksums"

if [ ! -d "$app_bundle" ]; then
  echo "fatal: release app bundle is not a directory: $app_bundle"
  exit 1
fi

executable="$app_bundle/Contents/MacOS/JoiceTyper"
plist="$app_bundle/Contents/Info.plist"
framework="$app_bundle/Contents/Frameworks/libportaudio.2.dylib"

require_path "$executable" "release app executable"
require_path "$plist" "release Info.plist"
require_path "$framework" "release bundled PortAudio dylib"

plutil -lint "$plist" >/dev/null
codesign --verify --deep --strict --verbose=2 "$app_bundle" >/dev/null
codesign --verify --deep --strict --verbose=2 "$dmg_path" >/dev/null

signature_details="$(codesign -d --verbose=4 "$app_bundle" 2>&1)"
case "$signature_details" in
  *runtime*) ;;
  *)
    echo "fatal: release app signature does not include hardened runtime"
    exit 1
    ;;
esac
case "$signature_details" in
  *"TeamIdentifier=not set"*)
    echo "fatal: release app is ad-hoc signed, not Developer ID signed"
    exit 1
    ;;
esac

if ! codesign -d --entitlements :- "$app_bundle" 2>/dev/null | plutil -lint - >/dev/null; then
  echo "fatal: release app entitlements are not readable"
  exit 1
fi

if ! lipo -archs "$executable" | grep -F "arm64" >/dev/null 2>&1; then
  echo "fatal: release app executable is not arm64"
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
  echo "fatal: release app bundle is missing icon.icns"
  exit 1
fi
if otool -L "$executable" | grep -F "/opt/homebrew/opt/portaudio" >/dev/null 2>&1; then
  echo "fatal: executable still links Homebrew PortAudio path"
  exit 1
fi

archive_sha="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
dmg_sha="$(shasum -a 256 "$dmg_path" | awk '{print $1}')"
appcast_sha="$(shasum -a 256 "$appcast_path" | awk '{print $1}')"
archive_length="$(wc -c < "$archive_path" | tr -d ' ')"
archive_path_abs="$(CDPATH= cd -- "$(dirname -- "$archive_path")" && pwd)/$(basename -- "$archive_path")"

for entry in \
  "$archive_sha  $(basename "$archive_path")" \
  "$dmg_sha  $(basename "$dmg_path")" \
  "$appcast_sha  $(basename "$appcast_path")"
do
  if ! grep -F "$entry" "$checksums_path" >/dev/null 2>&1; then
    echo "fatal: checksums file does not contain $entry"
    exit 1
  fi
done

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
metadata_signature="$(metadata_value EDDSA_SIGNATURE)"
metadata_value PUBLICATION_DATE >/dev/null

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
if [ "$metadata_signature" = "UNSIGNED" ]; then
  echo "fatal: release archive must have a Sparkle EdDSA signature"
  exit 1
fi

python3 - "$appcast_path" "$expected_version" "$metadata_archive_length" "$metadata_signature" "$(basename "$archive_path")" <<'PY'
import sys
import xml.etree.ElementTree as ET

appcast_path, expected_version, expected_length, expected_signature, archive_name = sys.argv[1:]
root = ET.parse(appcast_path).getroot()
ns = {"sparkle": "http://www.andymatuschak.org/xml-namespaces/sparkle"}
item = root.find("./channel/item")
if item is None:
    raise SystemExit("fatal: appcast missing channel/item")
version = item.findtext("sparkle:version", namespaces=ns)
short_version = item.findtext("sparkle:shortVersionString", namespaces=ns)
if version != expected_version:
    raise SystemExit(f"fatal: appcast sparkle:version {version} does not match {expected_version}")
if short_version != expected_version:
    raise SystemExit(f"fatal: appcast sparkle:shortVersionString {short_version} does not match {expected_version}")
enclosure = item.find("enclosure")
if enclosure is None:
    raise SystemExit("fatal: appcast missing enclosure")
if enclosure.get("length") != expected_length:
    raise SystemExit("fatal: appcast enclosure length does not match archive length")
if enclosure.get("{http://www.andymatuschak.org/xml-namespaces/sparkle}edSignature") != expected_signature:
    raise SystemExit("fatal: appcast Sparkle signature does not match metadata")
url = enclosure.get("url") or ""
if not url.endswith("/" + archive_name):
    raise SystemExit("fatal: appcast download URL does not end with archive filename")
PY

if ! zipinfo -1 "$archive_path" | grep -F "JoiceTyper.app/Contents/MacOS/JoiceTyper" >/dev/null 2>&1; then
  echo "fatal: release zip archive does not contain app executable"
  exit 1
fi
hdiutil imageinfo "$dmg_path" >/dev/null

printf '%s\n' "Validated official macOS release artifacts:"
printf '  app: %s\n' "$app_bundle"
printf '  zip: %s\n' "$archive_path"
printf '  dmg: %s\n' "$dmg_path"
printf '  appcast: %s\n' "$appcast_path"
printf '  checksums: %s\n' "$checksums_path"
