__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

PUSH_FLAG=".git/__LRC_PUSH_REQUEST_FILE__"
LRC_DIR=".git/lrc"
ATTEST_DIR="$LRC_DIR/attestations"
DISABLED_FILE="$LRC_DIR/disabled"
UPSTREAM=""
UPSTREAM_REMOTE=""
UPSTREAM_BRANCH=""

if [ -f "$DISABLED_FILE" ]; then
	exit 0
fi

# Skip during Git sequencer operations to avoid re-triggering on rebase/merge/cherry-pick
GIT_DIR="$(git rev-parse --git-dir 2>/dev/null || echo .git)"
if [ -d "$GIT_DIR/rebase-apply" ] || [ -d "$GIT_DIR/rebase-merge" ] || [ -f "$GIT_DIR/MERGE_HEAD" ] || [ -f "$GIT_DIR/CHERRY_PICK_HEAD" ]; then
	echo "LiveReview: skipping during rebase/merge/cherry-pick" >&2
	exit 0
fi

cleanup_flag() {
	rm -f "$PUSH_FLAG" 2>/dev/null || true
}

cleanup_attestation() {
	TREE_HASH="$(git rev-parse --verify HEAD^{tree} 2>/dev/null || true)"
	if [ -n "$TREE_HASH" ] && [ -f "$ATTEST_DIR/$TREE_HASH.json" ]; then
		rm -f "$ATTEST_DIR/$TREE_HASH.json" 2>/dev/null || true
		echo "lrc: cleared attestation for committed tree $TREE_HASH"
	fi
}

# Always clear attestation for the committed tree
cleanup_attestation

# If push was not requested, we're done
if [ ! -f "$PUSH_FLAG" ]; then
	exit 0
fi

echo "lrc: commit-and-push requested; verifying state and pushing if safe"

# 1. Abort if HEAD is detached
if ! git symbolic-ref -q HEAD >/dev/null; then
	echo "lrc: push skipped – detached HEAD"
	cleanup_flag
	exit 0
fi

# 2. Abort if no upstream
if ! git rev-parse --abbrev-ref --symbolic-full-name @{u} >/dev/null 2>&1; then
	echo "lrc: push skipped – no upstream configured"
	cleanup_flag
	exit 0
fi

UPSTREAM=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null)
UPSTREAM_REMOTE=${UPSTREAM%%/*}
UPSTREAM_BRANCH=${UPSTREAM#*/}
if [ -z "$UPSTREAM_REMOTE" ] || [ -z "$UPSTREAM_BRANCH" ]; then
	echo "lrc: push skipped – unable to resolve upstream"
	cleanup_flag
	exit 0
fi
echo "lrc: upstream detected -> $UPSTREAM_REMOTE/$UPSTREAM_BRANCH"

# 3. Fetch upstream
echo "lrc: fetching $UPSTREAM_REMOTE"
if ! git fetch --prune "$UPSTREAM_REMOTE"; then
	echo "lrc: push skipped – fetch failed"
	cleanup_flag
	exit 0
fi

# 4. Fast-forward only
echo "lrc: attempting fast-forward merge"
if ! git merge --ff-only @{u}; then
	echo "lrc: push skipped – fast-forward merge failed (remote has diverged)"
	cleanup_flag
	exit 0
fi
echo "lrc: fast-forwarded to $UPSTREAM"

# 5. Push
echo "lrc: pushing to $UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
if ! git push "$UPSTREAM_REMOTE" HEAD:"$UPSTREAM_BRANCH"; then
	echo "lrc: push failed"
	cleanup_flag
	exit 0
fi
echo "lrc: push complete -> $UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
cleanup_flag
exit 0
__LRC_MARKER_END__
