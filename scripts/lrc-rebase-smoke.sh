#!/usr/bin/env bash
set -euo pipefail

# Smoke test: ensure lrc hooks skip during rebase / merge / cherry-pick (global hooks assumed installed)
# Runtime target: ~10s

TMPDIR=$(mktemp -d /tmp/lrc-rebase.XXXX)
LOG_FILE="$TMPDIR/hook.log"

cleanup() {
  status=$?
  if [[ $status -eq 0 ]]; then
    rm -rf "$TMPDIR"
  else
    echo "Keeping temp dir: $TMPDIR" >&2
  fi
  exit $status
}
trap cleanup EXIT

exec > >(tee "$LOG_FILE") 2>&1

echo "Temp dir: $TMPDIR"

# Keep lrc non-interactive/offline for this smoke test
export LRC_SKIP_REVIEW=1
export LRC_NO_COLOR=1

cd "$TMPDIR"
git init >/dev/null

git config user.name "lrc-test"
git config user.email "lrc-test@example.com"

# Baseline commits (pseudo-TTY so global pre-commit treats as interactive)
echo "one" > f.txt
git add f.txt
script -q -c "git commit -m 'c1'" /dev/null

echo "two" >> f.txt
git add f.txt
script -q -c "git commit -m 'c2'" /dev/null

count_skips() {
  grep -ic "LiveReview: skipping during rebase/merge/cherry-pick" "$LOG_FILE" || true
}

TIMEOUT=12

run_rebase_check() {
  local before after
  before=$(count_skips)

  revs=( $(git rev-list --reverse HEAD | head -n 2) )
  todo="$TMPDIR/rebase-todo"
  {
    echo "pick ${revs[0]}"
    echo "edit ${revs[1]}"
  } > "$todo"

  GIT_SEQUENCE_EDITOR="cat $todo >" timeout ${TIMEOUT}s git rebase -i --root
  git commit --amend --no-edit
  timeout ${TIMEOUT}s git rebase --continue

  after=$(count_skips)
  if (( after <= before )); then
    echo "FAIL: rebase skip not observed" >&2
    exit 1
  fi
}

run_merge_check() {
  local before after
  before=$(count_skips)

  git checkout -b feature >/dev/null
  echo "three" >> f.txt
  git add f.txt
  script -q -c "git commit -m 'c3'" /dev/null

  git checkout master >/dev/null
  echo "four" > g.txt
  git add g.txt
  script -q -c "git commit -m 'c4'" /dev/null

  timeout ${TIMEOUT}s git merge feature

  after=$(count_skips)
  if (( after <= before )); then
    echo "FAIL: merge skip not observed" >&2
    exit 1
  fi
}

run_cherrypick_check() {
  local before after newcommit
  before=$(count_skips)

  git checkout feature >/dev/null
  echo "five" > h.txt
  git add h.txt
  script -q -c "git commit -m 'c5'" /dev/null
  newcommit=$(git rev-parse HEAD)

  git checkout master >/dev/null
  timeout ${TIMEOUT}s git cherry-pick "$newcommit"

  after=$(count_skips)
  if (( after <= before )); then
    echo "FAIL: cherry-pick skip not observed" >&2
    exit 1
  fi
}

run_rebase_check
run_merge_check
run_cherrypick_check

if grep -Ei "error|failed" "$LOG_FILE" >/dev/null; then
  echo "FAIL: errors detected in logs" >&2
  exit 1
fi

echo "PASS: rebase/merge/cherry-pick flows all skipped lrc and completed cleanly"
