__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates
STATE_FILE=".git/livereview_state"
LOCK_DIR=".git/livereview_state.lock"
COMMIT_MSG_FILE="$1"
COMMIT_MSG_OVERRIDE=".git/__LRC_COMMIT_MESSAGE_FILE__"
LRC_DIR=".git/lrc"
ATTEST_DIR="$LRC_DIR/attestations"
DISABLED_FILE="$LRC_DIR/disabled"

if [ -f "$DISABLED_FILE" ]; then
	exit 0
fi

# Non-interactive: require attestation for current staged tree before trailers
if [ ! -t 1 ]; then
	TREE_HASH="$(git write-tree 2>/dev/null || true)"
	ATTEST_FILE="$ATTEST_DIR/$TREE_HASH.json"
	if [ -z "$TREE_HASH" ]; then
		echo "LiveReview commit-msg: failed to compute staged tree hash; run 'lrc review --staged' before committing" >&2
		exit 1
	fi
	if [ ! -f "$ATTEST_FILE" ]; then
		echo "LiveReview commit-msg: no attestation for staged tree ($TREE_HASH). Run 'lrc review --staged' and retry." >&2
		exit 1
	fi
	echo "LiveReview commit-msg: attestation present for $TREE_HASH; proceeding" >&2
fi

# Resolve attestation action if present (preferred source for trailer)
TREE_HASH="$(git write-tree 2>/dev/null || true)"
ATTEST_FILE="$ATTEST_DIR/$TREE_HASH.json"
ATTEST_ACTION=""
if [ -n "$TREE_HASH" ] && [ -f "$ATTEST_FILE" ]; then
	ATTEST_ACTION=$(sed -n 's/.*"action"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$ATTEST_FILE" | head -n1)
fi

# Apply commit-message override from lrc (if present)
if [ -f "$COMMIT_MSG_OVERRIDE" ]; then
	if [ -s "$COMMIT_MSG_OVERRIDE" ]; then
		cat "$COMMIT_MSG_OVERRIDE" > "$COMMIT_MSG_FILE"
	fi
	rm -f "$COMMIT_MSG_OVERRIDE" 2>/dev/null || true
fi

TRAILER_ADDED=0

add_trailer() {
	echo "" >> "$COMMIT_MSG_FILE"
	echo "$1" >> "$COMMIT_MSG_FILE"
	TRAILER_ADDED=1
}

# Prefer attestation-derived action
if [ -n "$ATTEST_ACTION" ]; then
	if [ "$ATTEST_ACTION" = "reviewed" ]; then
		add_trailer "LiveReview Pre-Commit Check: ran"
	elif [ "$ATTEST_ACTION" = "skipped" ]; then
		add_trailer "LiveReview Pre-Commit Check: skipped"
	fi
fi

# Fallback to legacy state file if no attestation-derived trailer was added
if [ $TRAILER_ADDED -eq 0 ] && [ -f "$STATE_FILE" ]; then
    STATE=$(cat "$STATE_FILE" 2>/dev/null | cut -d: -f1)
    
	if [ "$STATE" = "ran" ]; then
		add_trailer "LiveReview Pre-Commit Check: ran"
	elif [ "$STATE" = "skipped_manual" ]; then
		add_trailer "LiveReview Pre-Commit Check: skipped manually"
	elif [ "$STATE" = "skipped" ] || [ "$STATE" = "skipped_env" ] || [ "$STATE" = "skipped_lock" ]; then
		add_trailer "LiveReview Pre-Commit Check: skipped"
	fi
    
    # Clean up state file and lock
    rm -f "$STATE_FILE" 2>/dev/null || true
    rmdir "$LOCK_DIR" 2>/dev/null || true
fi

# Always exit 0
exit 0
__LRC_MARKER_END__
