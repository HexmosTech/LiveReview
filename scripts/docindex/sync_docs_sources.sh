#!/usr/bin/env bash
# Syncs the chatbot's RAG corpus at internal/docindex/docs/ (embedded via
# `//go:embed docs` in internal/docindex/docs.go) from git-lrc's
# docs/LRC_README.md, git-lrc's wiki, LiveReview's wiki, and the public
# docs site (hexmoshomepage's pages/livereview/docs/ - what's published at
# hexmos.com/livereview/docs).
#
# There is no separate lockfile of "pinned" commits - the source of truth
# for what SHOULD be synced is simply "whatever each source's branch tip
# currently is" (looked up live, in parallel, via check_docs_sources.py
# --print), and the source of truth for what IS currently synced is
# internal/docindex/docs/.synced-commits.env. Both the fetched content and
# that marker file are committed to git, so a fresh `git pull` already has
# current content, and this script only does real fetch work when a live
# branch tip has actually moved past what's committed - never a full/branch
# clone even then, just that one commit's needed subtree.
#
# An earlier version of this kept a second, separately-committed lockfile
# (scripts/docindex/docs_sources.env) as an intermediate "pinned" target
# that a --auto step bumped before this script compared it against the
# marker. That file always ended up equal to the marker in steady state
# (content only ever changes together with the marker, via an actual
# fetch), so it added a file and a two-phase dance without adding any real
# capability - removed.
#
# hexmoshomepage is a PRIVATE repo (SSH-auth'd via git.apps.hexmos.com) -
# unlike the 3 public GitHub sources, a machine without SSH access to it
# just gets a "skipped: archive fetch failed" for that one source (see
# fetch_archive()) and keeps whatever it last had, rather than failing the
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

# --- 1. Look up each source's live branch tip, in parallel, read-only ---
# Never fails the build: offline, or a source unreachable, just means that
# source keeps whatever's already committed (see sync_source() below).
# Set SKIP_DOCS_SOURCES_CHECK=1 to skip this network step entirely - every
# source then falls back to "already synced" against the committed marker.
declare -A LIVE
if [ "${SKIP_DOCS_SOURCES_CHECK:-}" != "1" ]; then
  while IFS='=' read -r key value; do
    [[ -z "$key" || "$key" == \#* ]] && continue
    LIVE["$key"]="$value"
  done < <(python3 "$ROOT_DIR/scripts/docindex/check_docs_sources.py" --print)
fi

# --- 2. Load what's currently committed/synced on disk ---
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

# Prints whatever git/tar actually said on stderr, indented, right under a
# "skipped" line - so a missing SSH key and an unreachable host don't both
# just look like an unexplained "skipped: ...".
log_fetch_failure_reason() {
  local err_log="$1"
  if [ -s "$err_log" ]; then
    echo "    reason:" >&2
    sed 's/^/      /' "$err_log" >&2
  else
    echo "    reason: (no output captured - command exited non-zero with nothing on stderr)" >&2
  fi
}

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
  local url="$1" sha="$2" subdir="${3:-}" tmp err_log
  tmp="$(mktemp -d)"
  err_log="$(mktemp)"
  git init -q "$tmp"
  git -C "$tmp" remote add origin "$url"
  if [ -n "$subdir" ]; then
    git -C "$tmp" config core.sparseCheckoutCone true
    git -C "$tmp" sparse-checkout set "$subdir"
  fi
  if ! git -C "$tmp" fetch -q --depth 1 --filter=blob:none origin "$sha" 2>"$err_log"; then
    echo "    skipped: fetch failed for $url @ $sha" >&2
    log_fetch_failure_reason "$err_log"
    rm -rf "$tmp"
    rm -f "$err_log"
    return 1
  fi
  rm -f "$err_log"
  git -C "$tmp" checkout -q FETCH_HEAD
  echo "$tmp"
}

# Fetches $subdir at the tip of $branch via `git archive --remote` - the
# server extracts and streams just that subtree, no local clone/checkout,
# no partial-clone lazy-blob-fetch involved at all. Used for sources whose
# server doesn't support (or is too slow at) fetching an arbitrary commit
# SHA directly - notably self-hosted GitLab, which only serves archives for
# named refs, not arbitrary SHAs ("no such ref" otherwise). This always
# gets whatever $branch currently points to; sync_source() below already
# looked that tip up moments earlier via the same `git ls-remote` mechanism
# used everywhere else, so the two are expected to agree.
fetch_archive() {
  local url="$1" branch="$2" subdir="$3" tmp err_log
  tmp="$(mktemp -d)"
  err_log="$(mktemp)"
  if ! git archive --remote="$url" "$branch:$subdir" 2>"$err_log" | tar -x -C "$tmp" 2>>"$err_log"; then
    echo "    skipped: archive fetch failed for $url $branch:$subdir" >&2
    log_fetch_failure_reason "$err_log"
    rm -rf "$tmp"
    rm -f "$err_log"
    return 1
  fi
  rm -f "$err_log"
  echo "$tmp"
}

# sync_source NAME KEY URL SUBDIR DEST_DIR [ONLY_FILE] [ARCHIVE_BRANCH]
# ONLY_FILE (relative to SUBDIR, or repo root when SUBDIR is empty): copy
# just that one file instead of every .md/.mdx file under SUBDIR.
# ARCHIVE_BRANCH: when given, fetch via fetch_archive() (see above) instead
# of fetch_commit() - use for servers that can't/won't serve an arbitrary
# commit SHA directly.
sync_source() {
  local name="$1" key="$2" url="$3" subdir="$4" dest_dir="$5" only_file="${6:-}" archive_branch="${7:-}"
  local synced="${SYNCED[$key]:-}"
  # Prefer the live tip; fall back to whatever's already committed if the
  # lookup was skipped or failed - that's what makes an offline/unreachable
  # source a no-op instead of a build failure.
  local target="${LIVE[$key]:-$synced}"

  if [ -z "$target" ]; then
    echo "==> $name: no commit known yet (lookup unavailable and nothing synced), skipping"
    return
  fi
  if [ "$target" == "$synced" ]; then
    echo "==> $name: already synced to $target"
    return
  fi

  echo "==> $name: syncing ${synced:-<none>} -> $target"
  local tmp src
  if [ -n "$archive_branch" ]; then
    if ! tmp="$(fetch_archive "$url" "$archive_branch" "$subdir")"; then
      return
    fi
    src="$tmp"
  else
    if ! tmp="$(fetch_commit "$url" "$target" "$subdir")"; then
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
      echo "    skipped: $only_file not found in $name at $target"
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
    echo "    skipped: subdirectory '$subdir' not found in $name at $target"
  fi

  rm -rf "$tmp"
  write_marker "$key" "$target"
}

sync_source "git-lrc"             GIT_LRC_COMMIT             "https://github.com/HexmosTech/git-lrc.git"                   "docs"                  "$OUT_DIR/lrc_wiki/git-lrc" "LRC_README.md"
sync_source "git-lrc wiki"        GIT_LRC_WIKI_COMMIT        "https://github.com/HexmosTech/git-lrc.wiki.git"              ""                      "$OUT_DIR/lrc_wiki/wiki"
sync_source "LiveReview wiki"     LIVEREVIEW_WIKI_COMMIT     "https://github.com/HexmosTech/LiveReview.wiki.git"           ""                      "$OUT_DIR/lr_wiki/wiki"
sync_source "hexmoshomepage docs" HEXMOSHOMEPAGE_DOCS_COMMIT "git@git.apps.hexmos.com:hexmos/frontend/hexmoshomepage.git" "pages/livereview/docs" "$OUT_DIR/hexmos_docs"      "" "main"

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
