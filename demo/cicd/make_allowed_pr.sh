#!/usr/bin/env bash
# Opens a PR against shrsv/claude-world with an inert, safe change -- no
# critical/security findings expected, so the LiveReview gate should ALLOW
# the merge.
#
# Usage: demo/cicd/make_allowed_pr.sh [--wait]

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

WORKDIR="$SCRIPT_DIR/.worktree/claude-world"
if [ ! -d "$WORKDIR/.git" ]; then
  echo "Run setup_repo.sh first (no local clone at $WORKDIR)." >&2
  exit 1
fi

BRANCH="$BRANCH_PREFIX/allowed-$(date +%s)"
cd "$WORKDIR"
git fetch origin
git checkout master
git reset --hard origin/master
git checkout -b "$BRANCH"

cat > app/user_lookup.py <<'PY'
import logging
import sqlite3


def get_user(conn, user_id):
    """Fetch a user record by id."""
    logging.debug("Looking up user %s", user_id)
    cursor = conn.cursor()
    cursor.execute("SELECT id, name, email FROM users WHERE id = ?", (user_id,))
    return cursor.fetchone()
PY

git add app/user_lookup.py
skip_local_lrc_attestation
git commit -m "Add debug logging to user lookup"
git push origin "$BRANCH"

PR_URL=$(gh pr create --repo "$REPO" --base master --head "$BRANCH" \
  --title "Add debug logging to user lookup" \
  --body "Small observability improvement, no behavior change." | tail -1)
echo "Opened PR: $PR_URL"

if [ "${1:-}" = "--wait" ]; then
  HEAD_SHA=$(git rev-parse HEAD)
  : "${LIVEREVIEW_RULESET_ID:?Set LIVEREVIEW_RULESET_ID to poll locally}"
  echo "Polling locally (this is a preview -- the real gate check runs in GitHub Actions):"
  trigger_review "$PR_URL" || true
  wait_and_evaluate "$LIVEREVIEW_RULESET_ID" "$HEAD_SHA" || true
fi
