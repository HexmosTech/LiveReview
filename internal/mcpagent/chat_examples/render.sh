#!/usr/bin/env bash
# Renders every *.json example in this directory to a same-named .png,
# using vl-convert (https://github.com/vega/vl-convert). Each example
# file wraps the real Vega-Lite spec under a "spec" key (matching what
# Livi actually emits: title/description/query + spec) -- jq pulls
# spec out before handing it to vl-convert, which only accepts a raw
# Vega-Lite document.
set -euo pipefail

cd "$(dirname "$0")"

for f in *.json; do
  [[ "$f" == *.spec.tmp.json ]] && continue
  out="${f%.json}.png"
  tmp="${f%.json}.spec.tmp.json"

  jq '.spec' "$f" > "$tmp"
  vl-convert vl2png --input "$tmp" --output "$out"
  rm -f "$tmp"

  echo "rendered $f -> $out"
done
