#!/usr/bin/env bash
# Prints a single sha256 hash covering every file's path+content under
# ui/docs/training_data/. Stable regardless of filesystem iteration order.
# Shared by scripts/prep_training_data.sh (which mirrors it into the
# Makefile's TRAINING_DATA_HASH after fetching) and `make check-training-data`
# (which compares against that value before starting the dev server).
set -euo pipefail

OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/ui/docs/training_data"

if [ ! -d "$OUT_DIR" ]; then
  echo "MISSING"
  exit 0
fi

cd "$OUT_DIR"
find . -type f -print0 \
  | sort -z \
  | xargs -0 sha256sum \
  | sha256sum \
  | cut -d' ' -f1
