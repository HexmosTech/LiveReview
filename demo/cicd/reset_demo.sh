#!/usr/bin/env bash
# Closes every open PR from a demo/* branch and deletes those branches
# (local clone + remote), so the repo is clean before the next take.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./config.sh
source "$SCRIPT_DIR/config.sh"

echo "Closing open demo PRs on $REPO..."
gh pr list --repo "$REPO" --state open --json number,headRefName \
  --jq ".[] | select(.headRefName | startswith(\"$BRANCH_PREFIX/\")) | .number" |
  while read -r num; do
    echo "  closing PR #$num"
    gh pr close "$num" --repo "$REPO" --delete-branch || true
  done

WORKDIR="$SCRIPT_DIR/.worktree/claude-world"
if [ -d "$WORKDIR/.git" ]; then
  cd "$WORKDIR"
  git fetch --prune origin
  git checkout master
  git reset --hard origin/master
  for b in $(git branch --list "$BRANCH_PREFIX/*" | tr -d '* '); do
    git branch -D "$b" || true
  done
fi

echo "Demo repo reset. Ready for another take."
