#!/usr/bin/env bash
# Syncs the chatbot's RAG corpus at internal/docindex/docs/ (embedded via
# `//go:embed docs` in internal/docindex/docs.go) from git-lrc's
# docs/LRC_README.md, git-lrc's wiki, LiveReview's wiki, and the public
# docs site (hexmoshomepage's pages/livereview/docs/ - what's published at
# hexmos.com/livereview/docs) - each pinned to an exact commit in
# scripts/docindex/docs_sources.env, fetched ONLY when the pinned commit
# differs from what's already synced (internal/docindex/docs/.synced-commits.env),
# and even then only that one commit's needed subtree - never a full/branch
# clone.
#
# hexmoshomepage is a PRIVATE repo (SSH-auth'd via git.apps.hexmos.com) -
# unlike the 3 public GitHub sources, a machine without SSH access to it
# just gets a "skipped: fetch failed" for that one source (see
# fetch_commit()) and keeps whatever it last had, rather than failing the
# build.
#
# internal/docindex/docs/routes_guide/ (the hand/LLM-written per-UI-route
# "how do I do X" docs) is NOT touched here - it's tracked directly in git
# and needs no sync step. LiveReview's own top-level docs/ (engineering
# plans, release notes, etc.) is deliberately NOT part of this corpus - it's
# internal engineering documentation, not user-facing product docs, and has
# no business being in the chatbot's RAG index.
#
# See docs/docs-sources-pinning-plan.md for the full design and rationale.
#
# Usage: make sync-docs-sources  (or: scripts/docindex/sync_docs_sources.sh)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DOCS_SOURCES_FILE="$ROOT_DIR/scripts/docindex/docs_sources.env"
OUT_DIR="$ROOT_DIR/internal/docindex/docs"
MARKER_FILE="$OUT_DIR/.synced-commits.env"

mkdir -p "$OUT_DIR/routes_guide" "$OUT_DIR/lr_wiki/wiki" \
         "$OUT_DIR/lrc_wiki/git-lrc" "$OUT_DIR/lrc_wiki/wiki" \
         "$OUT_DIR/hexmos_docs"

# One-time cleanup: older versions of this pipeline copied fetched content
# directly into lr_wiki/ and lrc_wiki/ (scripts/prep_training_data.sh), or
# into a since-removed lr_wiki/local/ (an earlier, wrong revision of this
# very script that leaked LiveReview's own internal docs/ into the RAG
# corpus - see AGENTS.md/docs/docs-sources-pinning-plan.md history). Remove
# any such leftovers so they don't get embedded.
find "$OUT_DIR/lr_wiki" "$OUT_DIR/lrc_wiki" -maxdepth 1 -type f -delete 2>/dev/null || true
rm -rf "${OUT_DIR:?}/lr_wiki/local"

# --- 1. Auto-bump stale pins to their current remote branch tip, best-effort ---
# Runs 3 parallel `git ls-remote` lookups (no cloning) and rewrites
# docs_sources.env in place for anything that moved upstream - this is what
# removes "an agent/human must remember to bump the pin" as a failure mode.
# Never fails the build: offline, or GitHub unreachable, just means "keep
# whatever's already pinned" (same as any other offline dev-server start).
# Set SKIP_DOCS_SOURCES_CHECK=1 to skip this network step entirely.
if [ "${SKIP_DOCS_SOURCES_CHECK:-}" != "1" ]; then
  python3 "$ROOT_DIR/scripts/docindex/check_docs_sources.py" --auto || true
fi

# --- 2. Load pinned commits (source of truth: scripts/docindex/docs_sources.env,
#         possibly just auto-bumped above) ---
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

# Non-interactive SSH for git@... URLs (hexmoshomepage): without this, an
# unrecognized host key prompts on stdin and hangs forever in a build with
# no attached TTY, instead of failing fast. No effect on the HTTPS sources.
export GIT_SSH_COMMAND="ssh -o BatchMode=yes -o ConnectTimeout=10"

# Fetches exactly one commit of $url into a fresh temp checkout, restricted
# to $subdir when given (empty = whole repo, used for wiki repos where
# everything is docs). Prints the temp dir path on success. Never performs
# a full/branch clone - blobs outside $subdir are never downloaded at all.
#
# NOTE: this relies on the server supporting reachable-SHA1-in-want (true on
# github.com) AND cheap on-demand blob fetch during the sparse checkout.
# Self-hosted GitLab has been observed taking minutes for the latter on a
# large repo even for a handful of files - see fetch_archive() below for the
# alternative used for such sources.
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

# Fetches $subdir at the tip of $branch via `git archive --remote` - the
# server extracts and streams just that subtree, no local clone/checkout,
# no partial-clone lazy-blob-fetch involved at all. Used for sources whose
# server doesn't support (or is too slow at) fetching an arbitrary pinned
# commit SHA directly - notably self-hosted GitLab, which only serves
# archives for named refs, not arbitrary SHAs ("no such ref" otherwise).
# Trade-off: this always gets whatever $branch currently points to, not
# necessarily the exact SHA in $pinned - acceptable here because --auto
# (see above) keeps the pin tracking $branch's tip on every sync anyway, so
# by the time this runs they're expected to already match.
fetch_archive() {
  local url="$1" branch="$2" subdir="$3" tmp
  tmp="$(mktemp -d)"
  if ! git archive --remote="$url" "$branch:$subdir" 2>/dev/null | tar -x -C "$tmp"; then
    echo "    skipped: archive fetch failed for $url $branch:$subdir" >&2
    rm -rf "$tmp"
    return 1
  fi
  echo "$tmp"
}

# sync_source NAME KEY URL SUBDIR DEST_DIR [ONLY_FILE] [ARCHIVE_BRANCH]
# ONLY_FILE (relative to SUBDIR, or repo root when SUBDIR is empty): copy
# just that one file instead of every .md file under SUBDIR.
# ARCHIVE_BRANCH: when given, fetch via fetch_archive() (see above) instead
# of fetch_commit() - use for servers that can't/won't serve an arbitrary
# pinned SHA directly.
sync_source() {
  local name="$1" key="$2" url="$3" subdir="$4" dest_dir="$5" only_file="${6:-}" archive_branch="${7:-}"
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
  local tmp src
  if [ -n "$archive_branch" ]; then
    if ! tmp="$(fetch_archive "$url" "$archive_branch" "$subdir")"; then
      return
    fi
    src="$tmp"
  else
    if ! tmp="$(fetch_commit "$url" "$pinned" "$subdir")"; then
      return
    fi
    src="$tmp"
    [ -n "$subdir" ] && src="$tmp/$subdir"
  fi

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
        -type f \( -name '*.md' -o -name '*.mdx' \) -print) \
      | while IFS= read -r relpath; do
          mkdir -p "$dest_dir/$(dirname "$relpath")"
          cp "$src/$relpath" "$dest_dir/$relpath"
        done
    local count
    count=$(find "$dest_dir" -type f \( -name '*.md' -o -name '*.mdx' \) | wc -l | tr -d ' ')
    echo "    synced $count markdown file(s) to ${dest_dir#$ROOT_DIR/}"
  else
    echo "    skipped: subdirectory '$subdir' not found in $name at $pinned"
  fi

  rm -rf "$tmp"
  write_marker "$key" "$pinned"
}

sync_source "git-lrc"            GIT_LRC_COMMIT             "https://github.com/HexmosTech/git-lrc.git"                        "docs"                  "$OUT_DIR/lrc_wiki/git-lrc" "LRC_README.md"
sync_source "git-lrc wiki"       GIT_LRC_WIKI_COMMIT        "https://github.com/HexmosTech/git-lrc.wiki.git"                   ""                      "$OUT_DIR/lrc_wiki/wiki"
sync_source "LiveReview wiki"    LIVEREVIEW_WIKI_COMMIT     "https://github.com/HexmosTech/LiveReview.wiki.git"                ""                      "$OUT_DIR/lr_wiki/wiki"
sync_source "hexmoshomepage docs" HEXMOSHOMEPAGE_DOCS_COMMIT "git@git.apps.hexmos.com:hexmos/frontend/hexmoshomepage.git"      "pages/livereview/docs" "$OUT_DIR/hexmos_docs" "" "main"

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
