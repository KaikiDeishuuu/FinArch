#!/usr/bin/env bash
set -euo pipefail

base=${1:?public base URL required}
expected_dir=${2:?image frontend directory required}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

entry_asset() {
  sed -nE 's/.*<script[^>]+src="(\/assets\/[^"]+\.js)".*/\1/p' "$1"
}

# Use the stable URLs an installed browser requests, including the CDN cache.
fetch=(curl --fail --silent --show-error --compressed --connect-timeout 10 --max-time 30)
"${fetch[@]}" "${base%/}/" -o "$tmp/index.html"
entry=$(entry_asset "$expected_dir/index.html")
if [[ -z "$entry" || $(entry_asset "$tmp/index.html") != "$entry" ]]; then
  echo "Public HTML does not reference the entry script in the release image" >&2
  exit 1
fi
"${fetch[@]}" "${base%/}/sw.js" -D "$tmp/headers" -o "$tmp/sw.js"
if ! cmp -s "$expected_dir/sw.js" "$tmp/sw.js"; then
  echo "Public /sw.js differs from the release image" >&2
  exit 1
fi
if ! grep -Eiq '^cache-control:.*\bno-store\b' "$tmp/headers"; then
  echo "Public /sw.js lacks the no-store cache policy" >&2
  exit 1
fi
"${fetch[@]}" "${base%/}$entry" -o "$tmp/entry.js"
cmp "$expected_dir$entry" "$tmp/entry.js"
echo "Public frontend verified ($entry)"
