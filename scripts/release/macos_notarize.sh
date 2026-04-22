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

xcrun notarytool submit "$artifact_path" --keychain-profile "$profile" --wait
