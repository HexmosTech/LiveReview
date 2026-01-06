__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

DISABLED_FILE=".git/lrc/disabled"
if [ -f "$DISABLED_FILE" ]; then
	exit 0
fi

# Skip during Git sequencer operations to avoid re-triggering on rebase/merge/cherry-pick
GIT_DIR="$(git rev-parse --git-dir 2>/dev/null || echo .git)"
if [ -d "$GIT_DIR/rebase-apply" ] || [ -d "$GIT_DIR/rebase-merge" ] || [ -f "$GIT_DIR/MERGE_HEAD" ] || [ -f "$GIT_DIR/CHERRY_PICK_HEAD" ]; then
	echo "LiveReview: skipping during rebase/merge/cherry-pick" >&2
	exit 0
fi

# Detect interactive terminal (stdout check; git redirects stdin)
if [ -t 1 ]; then
	echo "LiveReview pre-commit: interactive environment detected; no-op"
	exit 0
fi

# Non-interactive: require attestation for current staged tree
TREE_HASH="$(git write-tree 2>/dev/null || true)"
ATTEST_FILE=".git/lrc/attestations/$TREE_HASH.json"

if [ -z "$TREE_HASH" ]; then
	echo "LiveReview pre-commit: failed to compute staged tree hash; run 'lrc review --staged' before committing"
	exit 1
fi

if [ ! -f "$ATTEST_FILE" ]; then
	echo "LiveReview pre-commit: no attestation found for staged tree ($TREE_HASH). Run 'lrc review --staged' and retry." 
	exit 1
fi

echo "LiveReview pre-commit: attestation present for $TREE_HASH; proceeding"
exit 0
__LRC_MARKER_END__
