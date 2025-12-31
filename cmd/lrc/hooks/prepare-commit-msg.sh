__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

COMMIT_MSG_FILE="$1"
SKIP_REVIEW="${LRC_SKIP_REVIEW:-}" 

# Detect if running in TTY (check stdout, not stdin - Git redirects stdin)
if [ -t 1 ]; then
	LRC_INTERACTIVE=1
else
	LRC_INTERACTIVE=0
fi

# State file for hook coordination
STATE_FILE=".git/livereview_state"
LOCK_DIR=".git/livereview_state.lock"

# Cleanup function
cleanup_lock() {
	rmdir "$LOCK_DIR" 2>/dev/null || true
}

# Set up cleanup trap
trap cleanup_lock EXIT INT TERM

# Allow explicit bypass (analogous to --no-verify)
if [ "$SKIP_REVIEW" = "1" ]; then
	echo "LiveReview: skipping review (LRC_SKIP_REVIEW=1)" >&2
	echo "skipped_env:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 0
fi

# Acquire lock with timeout (5 minutes)
MAX_WAIT=300
WAITED=0

# Check for stale locks (>5 minutes old)
if [ -d "$LOCK_DIR" ]; then
	if command -v stat >/dev/null 2>&1; then
		LOCK_AGE=$(($(date +%s) - $(stat -c %Y "$LOCK_DIR" 2>/dev/null || stat -f %m "$LOCK_DIR" 2>/dev/null || echo 0)))
		if [ "$LOCK_AGE" -gt 300 ]; then
			echo "Removing stale lock (${LOCK_AGE}s old)" >&2
			rmdir "$LOCK_DIR" 2>/dev/null || true
		fi
	fi
fi

while ! mkdir "$LOCK_DIR" 2>/dev/null; do
	if [ $WAITED -ge $MAX_WAIT ]; then
		echo "Could not acquire LiveReview lock after ${MAX_WAIT}s, skipping review" >&2
		echo "skipped_lock:$$:$(date +%s)" > "${STATE_FILE}.tmp"
		mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
		exit 0
	fi
	sleep 1
	WAITED=$((WAITED + 1))
done

# Capture current commit message (available in prepare-commit-msg)
INITIAL_MSG_FILE=".git/livereview_initial_message.$$"
if [ -n "$COMMIT_MSG_FILE" ] && [ -f "$COMMIT_MSG_FILE" ]; then
	cat "$COMMIT_MSG_FILE" > "$INITIAL_MSG_FILE" 2>/dev/null || true
fi

# Run review
if [ "$LRC_INTERACTIVE" = "1" ]; then
	echo "Running LiveReview commit check..."
	exec 2>&1
	LRC_INITIAL_MESSAGE_FILE="$INITIAL_MSG_FILE" lrc review --staged --precommit
	REVIEW_EXIT=$?
else
	LRC_INITIAL_MESSAGE_FILE="$INITIAL_MSG_FILE" lrc review --staged --output json >/dev/null 2>&1
	REVIEW_EXIT=$?
fi

# Cleanup initial message file
rm -f "$INITIAL_MSG_FILE"

# Check exit code
if [ $REVIEW_EXIT -eq 0 ]; then
	echo "ran:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 0
elif [ $REVIEW_EXIT -eq 2 ]; then
	echo "skipped_manual:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 0
else
	echo "skipped:$$:$(date +%s)" > "${STATE_FILE}.tmp"
	mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
	exit 1
fi
__LRC_MARKER_END__
