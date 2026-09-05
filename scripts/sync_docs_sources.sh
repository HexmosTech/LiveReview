#!/usr/bin/env bash
# Syncs the chatbot's RAG corpus at internal/docindex/docs/ (embedded via
# `//go:embed docs` in internal/docindex/docs.go) from:
#   - this repo's own docs/ subtree - no pin, no network, always current
#   - git-lrc's docs/LRC_README.md, git-lrc's wiki, and LiveReview's wiki -
#     each pinned to an exact commit in scripts/docs_sources.env, fetched
#     ONLY when the pinned commit differs from what's already synced
#     (internal/docindex/docs/.synced-commits.env), and even then only that
#     one commit's needed subtree - never a full/branch clone.
#
# See docs/docs-sources-pinning-plan.md for the full design and rationale.
#
# Usage: make sync-docs-sources  (or: scripts/sync_docs_sources.sh)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCS_SOURCES_FILE="$ROOT_DIR/scripts/docs_sources.env"
OUT_DIR="$ROOT_DIR/internal/docindex/docs"
MARKER_FILE="$OUT_DIR/.synced-commits.env"

mkdir -p "$OUT_DIR/routes_guide" "$OUT_DIR/lr_wiki/local" "$OUT_DIR/lr_wiki/wiki" \
         "$OUT_DIR/lrc_wiki/git-lrc" "$OUT_DIR/lrc_wiki/wiki"

# One-time cleanup: older versions of this pipeline (scripts/prep_training_data.sh)
# copied fetched content directly into lr_wiki/ and lrc_wiki/ rather than into
# the local/wiki/git-lrc subfolders used now. Remove any such loose leftovers
# so they don't get embedded twice.
find "$OUT_DIR/lr_wiki" "$OUT_DIR/lrc_wiki" -maxdepth 1 -type f -delete 2>/dev/null || true

# --- 1. This repo's own docs/ subtree: no pin, no network, always current ---
rm -rf "${OUT_DIR:?}/lr_wiki/local"/*
if [ -d "$ROOT_DIR/docs" ]; then
  (cd "$ROOT_DIR/docs" && find . -type d \( -name .git -o -name node_modules -o -name vendor -o -name dist \) -prune -o \
      -type f -name '*.md' -print) \
    | while IFS= read -r relpath; do
        mkdir -p "$OUT_DIR/lr_wiki/local/$(dirname "$relpath")"
        cp "$ROOT_DIR/docs/$relpath" "$OUT_DIR/lr_wiki/local/$relpath"
      done
fi
echo "==> LiveReview docs/ (local): copied from working tree, no network"

# --- 2. Load pinned commits (source of truth: scripts/docs_sources.env) ---
declare -A PINNED
if [ -f "$DOCS_SOURCES_FILE" ]; then
  while IFS='=' read -r key value; do
    [[ -z "$key" || "$key" == \#* ]] && continue
    PINNED["$key"]="$value"
  done < "$DOCS_SOURCES_FILE"
fi

# --- 3. Load last-synced commits (what's actually embedded on disk right now) ---
declare -A SYNCED
if [ -f "$MARKER_FILE" ]; then
  while IFS='=' read -r key value; do
    [[ -z "$key" || "$key" == \#* ]] && continue
    SYNCED["$key"]="$value"
  done < "$MARKER_FILE"
fi

write_marker() {
  local key="$1" sha="$2"
  SYNCED["$key"]="$sha"
  {
    for k in "${!SYNCED[@]}"; do
      echo "$k=${SYNCED[$k]}"
    done
  } | sort > "$MARKER_FILE.tmp"
  mv "$MARKER_FILE.tmp" "$MARKER_FILE"
}

# Fetches exactly one commit of $url into a fresh temp checkout, restricted
# to $subdir when given (empty = whole repo, used for wiki repos where
# everything is docs). Prints the temp dir path on success. Never performs
# a full/branch clone - blobs outside $subdir are never downloaded at all.
fetch_commit() {
  local url="$1" sha="$2" subdir="${3:-}" tmp
  tmp="$(mktemp -d)"
  git init -q "$tmp"
  git -C "$tmp" remote add origin "$url"
  if [ -n "$subdir" ]; then
    git -C "$tmp" config core.sparseCheckoutCone true
    git -C "$tmp" sparse-checkout set "$subdir"
  fi
  if ! git -C "$tmp" fetch -q --depth 1 --filter=blob:none origin "$sha"; then
    echo "    skipped: fetch failed for $url @ $sha" >&2
    rm -rf "$tmp"
    return 1
  fi
  git -C "$tmp" checkout -q FETCH_HEAD
  echo "$tmp"
}

# sync_source NAME KEY URL SUBDIR DEST_DIR [ONLY_FILE]
# ONLY_FILE (relative to SUBDIR, or repo root when SUBDIR is empty): copy
# just that one file instead of every .md file under SUBDIR.
sync_source() {
  local name="$1" key="$2" url="$3" subdir="$4" dest_dir="$5" only_file="${6:-}"
  local pinned="${PINNED[$key]:-}"
  local synced="${SYNCED[$key]:-}"

  if [ -z "$pinned" ]; then
    echo "==> $name: no pinned commit in ${DOCS_SOURCES_FILE#$ROOT_DIR/}, skipping"
    return
  fi
  if [ "$pinned" == "$synced" ]; then
    echo "==> $name: already synced to $pinned"
    return
  fi

  echo "==> $name: syncing ${synced:-<none>} -> $pinned"
  local tmp
  if ! tmp="$(fetch_commit "$url" "$pinned" "$subdir")"; then
    return
  fi

  local src="$tmp"
  [ -n "$subdir" ] && src="$tmp/$subdir"

  mkdir -p "$dest_dir"
  rm -rf "${dest_dir:?}"/*

  if [ -n "$only_file" ]; then
    if [ -f "$src/$only_file" ]; then
      cp "$src/$only_file" "$dest_dir/$(basename "$only_file")"
      echo "    synced $only_file to ${dest_dir#$ROOT_DIR/}"
    else
      echo "    skipped: $only_file not found in $name at $pinned"
    fi
  elif [ -d "$src" ]; then
    (cd "$src" && find . -type d \( -name .git -o -name node_modules -o -name vendor -o -name dist \) -prune -o \
        -type f -name '*.md' -print) \
      | while IFS= read -r relpath; do
          mkdir -p "$dest_dir/$(dirname "$relpath")"
          cp "$src/$relpath" "$dest_dir/$relpath"
        done
    local count
    count=$(find "$dest_dir" -type f -name '*.md' | wc -l | tr -d ' ')
    echo "    synced $count markdown file(s) to ${dest_dir#$ROOT_DIR/}"
  else
    echo "    skipped: subdirectory '$subdir' not found in $name at $pinned"
  fi

  rm -rf "$tmp"
  write_marker "$key" "$pinned"
}

sync_source "git-lrc"         GIT_LRC_COMMIT         "https://github.com/HexmosTech/git-lrc.git"         "docs" "$OUT_DIR/lrc_wiki/git-lrc" "LRC_README.md"
sync_source "git-lrc wiki"    GIT_LRC_WIKI_COMMIT    "https://github.com/HexmosTech/git-lrc.wiki.git"    ""     "$OUT_DIR/lrc_wiki/wiki"
sync_source "LiveReview wiki" LIVEREVIEW_WIKI_COMMIT "https://github.com/HexmosTech/LiveReview.wiki.git" ""     "$OUT_DIR/lr_wiki/wiki"

# Sanitize non-ASCII hyphens (U+2010) and commas in filenames - required for
# Go embed, which rejects some Unicode punctuation in embedded paths.
find "$OUT_DIR" -type f -print0 | while IFS= read -r -d '' f; do
  dir=$(dirname "$f")
  base=$(basename "$f" | sed 's/‐/-/g; s/,//g')
  if [ "$f" != "$dir/$base" ]; then
    mv "$f" "$dir/$base"
  fi
done

echo "==> docs sources sync complete"
