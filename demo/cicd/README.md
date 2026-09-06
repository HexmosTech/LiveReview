# CI/CD Gate Demo — shrsv/claude-world

Proves LiveReview's CI/CD Gates live, on demand: a PR that introduces a
critical/security finding gets its `livereview-gate` check blocked (and,
since branch protection is enforced, the merge button too); a clean PR gets
a passing check and can merge freely.

## TL;DR — demo it right now

```bash
make demo-cicd-block   # opens a PR with a SQL injection -> gate BLOCKS the merge
make demo-cicd-allow   # opens a PR with a harmless change -> gate ALLOWS the merge
make demo-cicd-reset   # closes both PRs + deletes their branches, ready for another take
```

That's it — no env vars to export, nothing to remember. Each command prints
the PR URL immediately and then polls locally so you can narrate the
outcome (`ALLOW`/`BLOCK`) before switching to the browser to show the
red ✗ / green ✓ check and the merge button. Real review time is roughly
10–60 seconds; the command waits for it.

## How it works

Opening a PR on `claude-world` runs `.github/workflows/livereview-gate.yml`,
which calls LiveReview's public tunnel
(`https://manual-talent2.apps.hexmos.com`) to (1) trigger a review of that
PR, (2) poll until it completes, (3) evaluate the **"Block on Critical or
Security"** ruleset (`(.counts.by_severity.critical > 0) or
(.counts.by_category.security > 0)`) against it, and exit 0 (allow) or 1
(block) accordingly. No inbound GitHub webhook is involved — the workflow
triggers its own review directly via `POST /connectors/trigger-review`.

Credentials and the ruleset id live in `demo/cicd/.env` (gitignored, already
set up on this machine) — every script sources it automatically.

## One-time setup (already done, only needed again if you rebuild the repo)

```bash
make demo-cicd-setup
```

This creates (or reuses) the ruleset, saves its id to `demo/cicd/.env`,
pushes `app/user_lookup.py` + the workflow file to `claude-world`, sets the
`LIVEREVIEW_API_KEY` / `LIVEREVIEW_ORG_ID` / `LIVEREVIEW_RULESET_ID` repo
secrets, and requires the `livereview-gate` check on `master` (enforced even
for admins, so the merge button is genuinely disabled on a failing PR).

If you're setting this up fresh (new machine, new org): copy
`demo/cicd/.env.example` to `demo/cicd/.env`, fill in an **owner**-role
LiveReview API key and your org id, then run `make demo-cicd-setup`.

## Demoing to someone live

1. `make demo-cicd-block` — while it polls, narrate: "this PR inlines a SQL
   query with string concatenation — a classic injection bug." When it
   prints `BLOCK`, open the printed PR URL and show the failing check + the
   disabled merge button.
2. `make demo-cicd-allow` — narrate: "this PR only adds a debug log line,
   nothing risky." When it prints `ALLOW`, open the PR and show the green
   check + enabled merge button.
3. `make demo-cicd-reset` before the next audience/take.

Want to skip the local polling narration and just watch it happen live in
GitHub's Actions tab instead? Run the underlying script without `--wait`:
`demo/cicd/make_blocked_pr.sh` / `demo/cicd/make_allowed_pr.sh` — it still
prints the PR URL immediately, it just won't print ALLOW/BLOCK in your
terminal.

## Files

- `.env` (gitignored) / `.env.example` — credentials + ruleset id.
  `create_ruleset.sh` writes `LIVEREVIEW_RULESET_ID` into `.env`
  automatically the first time it runs.
- `config.sh` — shared constants (`BASE_URL`, repo, ruleset name/jq), the
  `lr_curl` helper, and the `.env` auto-loader. Sourced by every other
  script, not run directly.
- `lib.sh` — `wait_and_evaluate` / `trigger_review`, the same polling logic
  baked into the GitHub Actions workflow, usable standalone for a local dry
  run; also `skip_local_lrc_attestation`, which bypasses this machine's
  local `lrc` pre-commit review-attestation hook for the synthetic demo
  commits (the gate that actually matters for the demo runs later via
  GitHub Actions against the pushed commit, so local commit-time review is
  irrelevant here).
- `create_ruleset.sh` — idempotently creates the ruleset, saves its id to
  `.env`.
- `setup_repo.sh` — pushes `repo_files/` into `claude-world`, sets secrets,
  configures branch protection.
- `repo_files/` — the actual files copied into `claude-world`: the sample
  `app/user_lookup.py` baseline and `.github/workflows/livereview-gate.yml`.
- `make_blocked_pr.sh` / `make_allowed_pr.sh` — open one demo PR each
  (`--wait` to also poll locally and print ALLOW/BLOCK).
- `reset_demo.sh` — cleans up demo PRs/branches for a re-take.
- `.worktree/` (gitignored) — the local clone of `claude-world` the scripts
  operate on; safe to delete, `setup_repo.sh` reclones if missing.

## Troubleshooting

- **Both commands report ALLOW, even the "blocked" one**: check
  `review_logs/review_<id>_*.log` in the LiveReview repo root for the
  matching review id (printed in the script's output) — if the LLM call
  failed (e.g. a 401 from the configured AI provider), the review completes
  with zero findings instead of erroring, which reads as an incorrect
  ALLOW. Fix the AI provider key in Settings and re-run.
- **`gh` auth errors**: `gh auth status` should show you logged in as
  `shrsv` with `repo` scope.
- **Stale local clone**: delete `demo/cicd/.worktree/` and re-run
  `make demo-cicd-setup` — it reclones automatically.
