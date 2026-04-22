#!/bin/sh
set -eu

if [ "$#" -ne 9 ]; then
  echo "usage: $0 <source-app> <release-app> <sparkle-stage-dir> <version> <feed-url> <public-ed-key> <codesign-identity> <plist-template> <plist-render-script>"
  exit 2
fi

source_app="$1"
release_app="$2"
sparkle_stage_dir="$3"
version="$4"
feed_url="$5"
public_ed_key="$6"
codesign_identity="$7"
plist_template="$8"
plist_render_script="$9"

if [ ! -d "$source_app" ]; then
  echo "fatal: missing source app bundle $source_app"
  exit 1
fi

framework_source=""
for candidate in \
  "$sparkle_stage_dir/Sparkle.framework" \
  "$sparkle_stage_dir/Sparkle/Sparkle.framework"
do
  if [ -d "$candidate" ]; then
    framework_source="$candidate"
    break
  fi
done

if [ -z "$framework_source" ]; then
  echo "fatal: missing Sparkle.framework in $sparkle_stage_dir"
  exit 1
fi

mkdir -p "$(dirname "$release_app")"
rm -rf "$release_app"
ditto "$source_app" "$release_app"

mkdir -p "$release_app/Contents/Frameworks"
rm -rf "$release_app/Contents/Frameworks/Sparkle.framework"
ditto "$framework_source" "$release_app/Contents/Frameworks/Sparkle.framework"

python3 "$plist_render_script" "$plist_template" "$release_app/Contents/Info.plist" "$version" "$feed_url" "$public_ed_key" "true"

if [ -d "$release_app/Contents/Frameworks/Sparkle.framework" ]; then
  # Sparkle carries nested helpers inside the framework bundle. Sign the framework
  # recursively before signing the containing app bundle.
  codesign --force --sign "$codesign_identity" --timestamp --options runtime --deep "$release_app/Contents/Frameworks/Sparkle.framework"
fi

codesign --force --sign "$codesign_identity" --timestamp --options runtime --deep "$release_app"
