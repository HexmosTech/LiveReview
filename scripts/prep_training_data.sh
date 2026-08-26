#!/usr/bin/env bash
# Pulls all Markdown docs from git-lrc, its wiki, LiveReview, and its wiki
# into ui/docs/training_data/ so the chatbot (Livi) has an up-to-date RAG
# corpus of external docs alongside the hand-written route docs in
# ui/docs/training_data/lr_routes/. See root AGENTS.md.
#
# Usage: make prep-training-data  (or: scripts/prep_training_data.sh)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/ui/docs/training_data"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

# name -> clone URL. Wikis are separate git repos at <repo>.wiki.git.
declare -A SOURCES=(
  [git-lrc]="https://github.com/HexmosTech/git-lrc.git"
  [git-lrc-wiki]="https://github.com/HexmosTech/git-lrc.wiki.git"
  [livereview]="https://github.com/HexmosTech/LiveReview.git"
  [livereview-wiki]="https://github.com/HexmosTech/LiveReview.wiki.git"
)

# name -> subdirectory (relative to the clone) to pull .md files from.
# Defaults to "." (whole repo) when not listed here. Both repos have a ton
# of unrelated internal .md files (rules, status docs, prompts, etc.) —
# only their docs/ subtree is actual product documentation.
declare -A SUBDIRS=(
  [git-lrc]="docs"
  [livereview]="docs"
)

# Authenticate git's https transport with the gh CLI token when available,
# so private repos/wikis clone the same way public ones do.
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  gh auth setup-git >/dev/null 2>&1 || true
fi

for name in "${!SOURCES[@]}"; do
  url="${SOURCES[$name]}"
  dest="$OUT_DIR/$name"
  clone_dir="$WORK_DIR/$name"

  echo "==> Fetching $name ($url)"
  if ! git clone --depth 1 --quiet "$url" "$clone_dir"; then
    echo "    skipped: clone failed (repo/wiki may not exist or need auth)"
    continue
  fi

  rm -rf "$dest"
  mkdir -p "$dest"

  subdir="${SUBDIRS[$name]:-.}"
  src_dir="$clone_dir/$subdir"
  if [ ! -d "$src_dir" ]; then
    echo "    skipped: subdirectory '$subdir' not found in $name"
    continue
  fi

  # Recursively copy every .md file under $subdir, preserving its relative
  # directory structure, skipping VCS/build/dependency noise.
  (cd "$src_dir" && find . \
      -type d \( -name .git -o -name node_modules -o -name vendor -o -name dist \) -prune -o \
      -type f -name '*.md' -print) \
    | while IFS= read -r relpath; do
        mkdir -p "$dest/$(dirname "$relpath")"
        cp "$src_dir/$relpath" "$dest/$relpath"
      done

  count=$(find "$dest" -type f -name '*.md' | wc -l | tr -d ' ')
  echo "    copied $count markdown file(s) to ${dest#$ROOT_DIR/}"
done

# Single content hash for the whole training-data corpus (all sources plus
# lr_routes/), so callers (e.g. the RAG indexer) can detect "did anything
# change" without diffing hundreds of files. Hashes path+content of every
# file in sorted order, so it's stable regardless of filesystem iteration
# order and changes if any file's content, name, or path changes.
HASH_FILE="$OUT_DIR/.content-hash"
CONTENT_HASH=$(
  cd "$OUT_DIR" && find . -type f ! -name '.content-hash' -print0 \
    | sort -z \
    | xargs -0 sha256sum \
    | sha256sum \
    | cut -d' ' -f1
)
echo "$CONTENT_HASH" > "$HASH_FILE"
echo "Content hash: $CONTENT_HASH (written to ${HASH_FILE#$ROOT_DIR/})"

echo "Done. Hand-written route docs remain in ui/docs/training_data/lr_routes/ (untouched)."
