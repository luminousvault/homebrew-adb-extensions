#!/usr/bin/env bash
# Renders Formula/ak.rb from the template with the version and the SHA-256 of
# each release archive, read from GoReleaser's checksums.txt.
#
#   .github/formula/render.sh 2.0.0 dist/checksums.txt > Formula/ak.rb
set -euo pipefail

version=$1
checksums=$2
template="$(dirname "$0")/ak.rb.tmpl"

sha() {
  local file="ak_${version}_$1.tar.gz" sum
  sum=$(awk -v f="$file" '$2 == f { print $1 }' "$checksums")
  if [ -z "$sum" ]; then
    echo "Error: no checksum for $file in $checksums" >&2
    exit 1
  fi
  echo "$sum"
}

sed -e "s|@VERSION@|${version}|g" \
    -e "s|@SHA256_MACOS@|$(sha macos)|" \
    -e "s|@SHA256_LINUX_ARM64@|$(sha linux_arm64)|" \
    -e "s|@SHA256_LINUX_X86_64@|$(sha linux_x86_64)|" \
    "$template"
