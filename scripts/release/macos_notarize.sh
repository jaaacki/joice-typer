#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <artifact-path> <notarytool-profile>"
  exit 2
fi

artifact_path="$1"
profile="$2"

if [ ! -e "$artifact_path" ]; then
  echo "fatal: missing notarization artifact $artifact_path"
  exit 1
fi

submit_path="$artifact_path"
staple_path=""
tmp_dir=""

case "$artifact_path" in
  *.app)
    if [ ! -d "$artifact_path" ]; then
      echo "fatal: app notarization target is not a bundle directory: $artifact_path"
      exit 1
    fi
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' EXIT
    submit_path="$tmp_dir/$(basename "$artifact_path").zip"
    (cd "$(dirname "$artifact_path")" && ditto -c -k --keepParent "$(basename "$artifact_path")" "$submit_path")
    staple_path="$artifact_path"
    ;;
  *.dmg)
    staple_path="$artifact_path"
    ;;
  *.zip|*.pkg)
    ;;
  *)
    echo "fatal: unsupported notarization artifact format: $artifact_path"
    exit 1
    ;;
esac

xcrun notarytool submit "$submit_path" --keychain-profile "$profile" --wait

if [ -n "$staple_path" ]; then
  xcrun stapler staple "$staple_path"
fi
