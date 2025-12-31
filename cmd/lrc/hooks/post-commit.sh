__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

PUSH_FLAG=".git/__LRC_PUSH_REQUEST_FILE__"
UPSTREAM=""
UPSTREAM_REMOTE=""
UPSTREAM_BRANCH=""

# Only act when flag exists
if [ ! -f "$PUSH_FLAG" ]; then
	exit 0
fi

cleanup_flag() {
	rm -f "$PUSH_FLAG" 2>/dev/null || true
}

echo "lrc: commit-and-push requested; verifying state and pushing if safe"

# 1. Require clean working tree (including index)
if ! git diff --quiet || ! git diff --cached --quiet; then
	echo "lrc: push skipped – working tree not clean"
	cleanup_flag
	exit 0
fi

# 2. Abort if HEAD is detached
if ! git symbolic-ref -q HEAD >/dev/null; then
	echo "lrc: push skipped – detached HEAD"
	cleanup_flag
	exit 0
fi

# 3. Abort if no upstream
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

# 4. Fetch upstream
if ! git fetch --prune; then
	echo "lrc: push skipped – fetch failed"
	cleanup_flag
	exit 0
fi
echo "lrc: fetched $UPSTREAM_REMOTE"

# 5. Fast-forward only
if ! git merge --ff-only @{u}; then
	echo "lrc: push skipped – fast-forward merge failed"
	cleanup_flag
	exit 0
fi
echo "lrc: fast-forwarded to $UPSTREAM"

# 6. Push
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
