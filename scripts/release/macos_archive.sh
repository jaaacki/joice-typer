#!/bin/sh
set -eu

if [ "$#" -ne 6 ]; then
  echo "usage: $0 <app-bundle> <archive-path> <version> <metadata-path> <sign-update-tool> <private-key-file>"
  exit 2
fi

app_bundle="$1"
archive_path="$2"
version="$3"
metadata_path="$4"
sign_update_tool="$5"
private_key_file="$6"

if [ ! -d "$app_bundle" ]; then
  echo "fatal: missing app bundle $app_bundle"
  exit 1
fi
if [ ! -x "$sign_update_tool" ]; then
  echo "fatal: missing Sparkle sign_update tool $sign_update_tool"
  exit 1
fi
if [ ! -f "$private_key_file" ]; then
  echo "fatal: missing Sparkle private key file $private_key_file"
  exit 1
fi

mkdir -p "$(dirname "$archive_path")"
rm -f "$archive_path" "$metadata_path"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
cp -R "$app_bundle" "$tmp_dir/"

(cd "$tmp_dir" && ditto -c -k --keepParent "$(basename "$app_bundle")" "$archive_path")

length="$(wc -c < "$archive_path" | tr -d ' ')"
sha256="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
published_at="$(date -u +"%a, %d %b %Y %H:%M:%S +0000")"
eddsa_signature="$("$sign_update_tool" --ed-key-file "$private_key_file" -p "$archive_path")"

cat > "$metadata_path" <<EOF
VERSION=$version
ARCHIVE_PATH=$archive_path
ARCHIVE_LENGTH=$length
ARCHIVE_SHA256=$sha256
PUBLICATION_DATE=$published_at
EDDSA_SIGNATURE=$eddsa_signature
EOF
