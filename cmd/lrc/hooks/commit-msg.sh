__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates
STATE_FILE=".git/livereview_state"
LOCK_DIR=".git/livereview_state.lock"
COMMIT_MSG_FILE="$1"
COMMIT_MSG_OVERRIDE=".git/__LRC_COMMIT_MESSAGE_FILE__"

# Apply commit-message override from lrc (if present)
if [ -f "$COMMIT_MSG_OVERRIDE" ]; then
	if [ -s "$COMMIT_MSG_OVERRIDE" ]; then
		cat "$COMMIT_MSG_OVERRIDE" > "$COMMIT_MSG_FILE"
	fi
	rm -f "$COMMIT_MSG_OVERRIDE" 2>/dev/null || true
fi

# Read state if exists
if [ -f "$STATE_FILE" ]; then
    STATE=$(cat "$STATE_FILE" 2>/dev/null | cut -d: -f1)
    
	if [ "$STATE" = "ran" ]; then
		echo "" >> "$COMMIT_MSG_FILE"
		echo "LiveReview Pre-Commit Check: ran" >> "$COMMIT_MSG_FILE"
	elif [ "$STATE" = "skipped_manual" ]; then
		echo "" >> "$COMMIT_MSG_FILE"
		echo "LiveReview Pre-Commit Check: skipped manually" >> "$COMMIT_MSG_FILE"
	elif [ "$STATE" = "skipped" ] || [ "$STATE" = "skipped_env" ] || [ "$STATE" = "skipped_lock" ]; then
		echo "" >> "$COMMIT_MSG_FILE"
		echo "LiveReview Pre-Commit Check: skipped" >> "$COMMIT_MSG_FILE"
	fi
    
    # Clean up state file and lock
    rm -f "$STATE_FILE" 2>/dev/null || true
    rmdir "$LOCK_DIR" 2>/dev/null || true
fi

# Always exit 0
exit 0
__LRC_MARKER_END__
