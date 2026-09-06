#!/usr/bin/env bash
# One-time setup of shrsv/claude-world for the CI/CD Gate demo:
#   - pushes app/user_lookup.py + .github/workflows/livereview-gate.yml to master
#   - sets the LIVEREVIEW_API_KEY / LIVEREVIEW_ORG_ID / LIVEREVIEW_RULESET_ID repo secrets
#   - requires the livereview-gate check to pass before merging into master
#
# Requires: LIVEREVIEW_API_KEY, LIVEREVIEW_ORG_ID, LIVEREVIEW_RULESET_ID in env
# (the first two are your own LiveReview credentials; the third comes from
# create_ruleset.sh). Uses `gh`, already authenticated as shrsv.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

: "${LIVEREVIEW_RULESET_ID:?Set LIVEREVIEW_RULESET_ID first (see create_ruleset.sh)}"
: "${LIVEREVIEW_API_KEY:?Set LIVEREVIEW_API_KEY first}"
: "${LIVEREVIEW_ORG_ID:?Set LIVEREVIEW_ORG_ID first}"

WORKDIR="$SCRIPT_DIR/.worktree/claude-world"
mkdir -p "$SCRIPT_DIR/.worktree"

if [ -d "$WORKDIR/.git" ]; then
  echo "Reusing existing clone at $WORKDIR"
  git -C "$WORKDIR" fetch origin
  git -C "$WORKDIR" checkout master
  git -C "$WORKDIR" reset --hard origin/master
else
  gh repo clone "$REPO" "$WORKDIR"
fi

mkdir -p "$WORKDIR/app" "$WORKDIR/.github/workflows"
cp "$SCRIPT_DIR/repo_files/app/user_lookup.py" "$WORKDIR/app/user_lookup.py"
cp "$SCRIPT_DIR/repo_files/workflows/livereview-gate.yml" "$WORKDIR/.github/workflows/livereview-gate.yml"

cd "$WORKDIR"
git add app/user_lookup.py .github/workflows/livereview-gate.yml
skip_local_lrc_attestation
if git diff --cached --quiet; then
  echo "master already up to date, nothing to commit."
else
  git commit -m "Add LiveReview CI/CD gate demo workflow and sample app"
  git push origin master
fi

echo "Setting repo secrets on $REPO..."
gh secret set LIVEREVIEW_API_KEY --repo "$REPO" --body "$LIVEREVIEW_API_KEY"
gh secret set LIVEREVIEW_ORG_ID --repo "$REPO" --body "$LIVEREVIEW_ORG_ID"
gh secret set LIVEREVIEW_RULESET_ID --repo "$REPO" --body "$LIVEREVIEW_RULESET_ID"

echo "Requiring the 'livereview-gate' check on master before merge..."
gh api -X PUT "repos/$REPO/branches/master/protection" \
  --input - <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["livereview-gate"]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": null,
  "restrictions": null
}
JSON

echo "Done. shrsv/claude-world is demo-ready."
