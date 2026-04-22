#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
  echo "usage: $0 <app-bundle> <archive-path> <version> <metadata-path>"
  exit 2
fi

app_bundle="$1"
archive_path="$2"
version="$3"
metadata_path="$4"
archive_dir="$(CDPATH= cd -- "$(dirname -- "$archive_path")" && pwd)"
archive_name="$(basename "$archive_path")"
metadata_dir="$(CDPATH= cd -- "$(dirname -- "$metadata_path")" && pwd)"
metadata_name="$(basename "$metadata_path")"
archive_path_abs="$archive_dir/$archive_name"
metadata_path_abs="$metadata_dir/$metadata_name"

if [ ! -d "$app_bundle" ]; then
  echo "fatal: missing app bundle $app_bundle"
  exit 1
fi

mkdir -p "$archive_dir" "$metadata_dir"
rm -f "$archive_path_abs" "$metadata_path_abs"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
cp -R "$app_bundle" "$tmp_dir/"

(cd "$tmp_dir" && ditto -c -k --keepParent "$(basename "$app_bundle")" "$archive_path_abs")

length="$(wc -c < "$archive_path_abs" | tr -d ' ')"
sha256="$(shasum -a 256 "$archive_path_abs" | awk '{print $1}')"
published_at="$(date -u +"%a, %d %b %Y %H:%M:%S +0000")"

cat > "$metadata_path_abs" <<EOF
VERSION=$version
ARCHIVE_PATH=$archive_path_abs
ARCHIVE_LENGTH=$length
ARCHIVE_SHA256=$sha256
PUBLICATION_DATE=$published_at
EDDSA_SIGNATURE=UNSIGNED
EOF
