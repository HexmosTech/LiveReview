#!/usr/bin/env bash
# Prints a single sha256 hash covering every file's path+content under
# ui/docs/training_data/ (excluding the hash file itself). Stable regardless
# of filesystem iteration order. Shared by scripts/prep_training_data.sh
# (which writes it after fetching) and the Makefile (which checks it before
# starting the dev server).
set -euo pipefail

OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/ui/docs/training_data"

if [ ! -d "$OUT_DIR" ]; then
  echo "MISSING"
  exit 0
fi

cd "$OUT_DIR"
find . -type f ! -name '.content-hash' -print0 \
  | sort -z \
  | xargs -0 sha256sum \
  | sha256sum \
  | cut -d' ' -f1
