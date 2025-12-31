# LRC — Git-Integrated AI Review Attestation System

**Specification v0.1**

## 1. Goal and Non-Goals

### 1.1 Goal

LRC integrates AI code review into the Git commit lifecycle in a **non-intrusive, incremental, and developer-respecting** manner by:

* Binding AI review (or explicit skip) to the **exact staged content**
* Enforcing checks in **non-interactive contexts**
* Encouraging review in **interactive contexts**
* Working uniformly across VS Code, lazygit, CLI, and CI
* Centralizing update logic via **global Git hooks**

### 1.2 Non-Goals

* LRC does **not** guarantee correctness or safety
* LRC does **not** block commits interactively
* LRC does **not** replace peer review or CI

---

## 2. Core Concepts

### 2.1 Staged Tree Identity

LRC uses Git’s staged tree hash as the canonical identity:

```bash
TREE_HASH = git write-tree
```

This hash uniquely represents the exact staged state.

### 2.2 Attestation

An **attestation** is an explicit developer action bound to a `TREE_HASH`, indicating:

* `reviewed` — AI review was run and acknowledged
* `skipped` — developer consciously skipped AI review

### 2.3 Storage Location

All LRC state is stored locally under a minimal, content-addressed file per staged tree hash:

```
.git/lrc/
  attestations/
    <tree-hash>.json   # {"action": "reviewed"|"skipped"}
  version
```

This data:

* Is not committed
* Is repo-local
* Is content-addressed
* Stores only what Git does not already know: the chosen LRC action (hash lives in the filename)

---

## 3. Command Interface

### 3.1 Binary Naming

The installer MUST install the same binary as:

* `lrc`
* `git-lrc`

This enables:

```bash
lrc review
git lrc review
```

with identical behavior.

### 3.2 Supported Commands

#### 3.2.1 Review

```bash
git lrc review
lrc review
```

Behavior:

1. Compute staged tree hash
2. Run AI review **only if TTY is present** when triggered via pre-commit.TODO:  We also plan to have trigger mechanisms via VSCode extension, or they can triggeer from their terminal, etc
3. Write attestation with action `reviewed`

#### 3.2.2 Skip

```bash
git lrc review --skip
lrc review --skip
```

Behavior:

1. Compute staged tree hash
2. Do NOT run AI
3. Write attestation with action `skipped`

### 3.3 Attestation Format (Normative)

Just have something like the following appended after the usual commit messages. this is already mostly implemented in the system

LiveReview Pre-Commit Check: skipped manually
LiveReview Pre-Commit Check: ran

```

---

## 4. Git Hook Behavior

### 4.1 Hook Type

We already have hook implementation for prepare-commit-message I think.

### 4.2 Interactive vs Non-Interactive Detection

We already have this probably

```sh
test -t 0 && test -t 1
```

* **TTY present** → interactive
* **No TTY** → non-interactive (VS Code, lazygit, CI)

---

## 5. Hook Logic (Normative)

### 5.1 Shared Logic

```sh
TREE=$(git write-tree)
FILE=".git/lrc/attestations/$TREE.json"
```

---

### 5.2 Interactive Mode (TTY present)

In interactive mode - the user experience has to be as it is not. Only difference is
we get the hash of the staged tree, and check that it is properly reviewed, etc


---

### 5.3 Non-Interactive Mode (No TTY)

| Condition          | Behavior |
| ------------------ | -------- |
| Attestation exists | exit 0   |
| No attestation     | exit 1   |

Notes:

* Hook MUST NOT run AI
* If the review is not done for the given hash or no manual skip - then it must block the commit



## 6. Global Hook Installation

### 6.1 Global Hooks Path

Installer MUST set:

```bash
git config --global core.hooksPath ~/.git-hooks
```

### 6.2 Global Pre-Commit Hook

The global hook MUST:

1. Detect if repo has opted into LRC
2. Run LRC logic
3. Delegate to repo-local hooks

Example:

```sh
#!/bin/sh

if [ -d ".git/lrc" ]; then
  lrc hook pre-commit || exit 1
fi

if [ -x ".git/hooks/pre-commit" ]; then
  exec ".git/hooks/pre-commit" "$@"
fi

exit 0
```

### 6.3 Repo Opt-Out

By defaault all git repos have this setup. They can opt out with a custom command such as:

lrc disable


That'll disable locally
---

## 7. Installer Responsibilities

The installer MUST:

1. Install binary as:

   * `/usr/local/bin/lrc`
   * `/usr/local/bin/git-lrc`
2. Configure `core.hooksPath`
3. Install global hook dispatcher
4. Preserve existing repo hooks
5. Be idempotent
6. Be reversible (uninstall)

---

## 8. CI / Policy Enforcement (Out of Hooks)

We can keep it out of scope. But can probably have some TODO functions or such 
setup so that in CI the attestations can be done.

CI SHOULD enforce:

```bash
TREE=$(git show -s --format=%T HEAD)
test -f ".git/lrc/attestations/$TREE.json"
```

This ensures:

* Local bypass is visible
* Enforcement is centralized
* Hooks remain developer-friendly

---

## 9. Design Guarantees

This system guarantees:

* Content-bound attestation
* No workflow breakage
* Incremental adoption
* Clear developer intent
* Compatibility with all Git UIs

---

## 10. Explicit Design Philosophy

> LRC enforces **intent**, not correctness.
> It encourages care without coercion.
> It respects Git’s trust model.

---

## Implementation Plan (multi-phase, no further research required)

### Immediate milestone (do first)
- Move hash/attestation enforcement into Go (hook helpers), not shell. Hooks should call Go to compute staged tree hash and read/write attestations.
- Both interactive and non-interactive paths must key off the staged tree hash. In non-interactive mode, commit fails when no attestation (reviewed or skipped) exists. In interactive mode, UX stays the same but still uses the hash-backed attestation store.
- Make this the first acceptance test to turn green before touching later phases.

Context: Today the `lrc` CLI already installs repo-local hooks and writes per-commit state into `.git/livereview_state` with commit trailers (see [cmd/lrc/main.go](cmd/lrc/main.go#L1697-L2049)). There is no tree-hash–bound attestation store yet, no global hook dispatcher, and no `git-lrc` alias binary. This plan turns the spec into concrete, file-scoped work items.

### Phase 0 — Decide canonical attestation format and storage (decision: minimal JSON, no hash duplication)
- Use minimal JSON at `.git/lrc/attestations/<tree-hash>.json` with only `{ "action": "reviewed"|"skipped" }`, relying on the filename for the hash to avoid duplication.
- Add repo version file `.git/lrc/version` to allow migrations.
- Files to touch: create new package `internal/lrcattest` (new), add schema doc here. No `.gitignore` change needed because `.git/` is already ignored.
- Visible result: `go test ./internal/lrcattest -run TestFormatDecision` passes, showing chosen format is locked in code/doc.

### Phase 1 — Attestation library (content-addressed, tree-aware)
- Add helper package `internal/lrcattest` with functions: `WriteAttestation(treeHash, action)`, `ReadAttestation(treeHash)`, `HasAttestation(treeHash)`, `ListAttestations()`, `PruneOld(limit)`; data model is the minimal JSON `{ "action": "..." }` (hash comes from filename).
- Implement tree hash helper `StagedTreeHash()` using `git write-tree`; place in `internal/gitutil` or within `internal/lrcattest`.
- Ensure all writes are atomic (write temp + rename) and that directories `.git/lrc/attestations` are created.
- Files to add/update: new `internal/lrcattest/*.go`; if reusing git helpers, update/create `internal/gitutil/git.go`.
- Visible result: `go test ./internal/lrcattest` writes a temp attestation and verifies round-trip; manual check `git write-tree` then `ls .git/lrc/attestations` after running a sample helper.

### Phase 2 — CLI commands for review + skip bound to tree hash
- Extend `runReviewWithOptions` in [cmd/lrc/main.go](cmd/lrc/main.go#L240-L760) to compute `TREE_HASH=git write-tree` whenever review runs. Make staged the default mode. After successful AI review, call `WriteAttestation(treeHash,"reviewed",meta)`; when invoked with `--skip`, only write `skipped` attestation and exit 0.
- Add explicit subcommand `lrc attest skip` (alias `review --skip`) and `lrc attest show <tree>` for debugging (in `main.go` command wiring near [cmd/lrc/main.go#L196-L246]).
- Ensure non-interactive invocation (`--output json`) never prompts and only writes attestation; interactive retains current UI flow but also writes attestation.
- Visible result: run `lrc review --skip` (staged default) then `TREE=$(git write-tree) && cat .git/lrc/attestations/$TREE.json` shows `{ "action": "skipped" }`; `lrc attest show $TREE` prints the same.

### Phase 3 — Hook generation aligned to attestations
- Replace state file logic in generated hooks with attestation lookups:
  - Update `generatePrepareCommitMsgHook` and `generateCommitMsgHook` in [cmd/lrc/main.go#L2020-L2109] to compute `TREE=$(git write-tree)` and read/write `.git/lrc/attestations/$TREE.json` instead of `.git/livereview_state`.
  - Interactive (TTY) path: keep existing UX but gate commit on attestation presence; if attestation absent, prompt/launch review; on completion, write attestation (reviewed/ skipped) via `lrc review --staged --precommit` (which now writes attestation itself).
  - Non-interactive path: do not run AI; just fail when attestation missing; success when attestation exists.
- Remove legacy state file writes/reads and trailers that are not tree-bound; keep trailer writing but derive text from attestation action.
- Visible result: In non-interactive mode, `HOOK_DEBUG=1 git commit --dry-run` fails when no attestation exists and succeeds after `lrc review --skip --staged` creates it; in interactive mode, the prompt still appears and commit proceeds once attestation is written.

### Phase 4 — Repo-local install/uninstall hooks commands (align existing commands)
- Update existing `lrc install-hooks` / `lrc uninstall-hooks` to install the new attestation-aware hook bodies (prepare-commit-msg, commit-msg, post-commit) and to cleanly remove them.
- Ensure backups remain, but drop legacy `.git/livereview_state` usage from installed hook content.
- Files to change: hook generation functions in [cmd/lrc/main.go#L1697-L2109]; command wiring near [cmd/lrc/main.go#L196-L246].
- Visible result: `lrc install-hooks` reports updates; `grep lrc_marker .git/hooks/prepare-commit-msg` shows new content; `lrc uninstall-hooks` removes lrc sections and restores prior state.

### Phase 5 — Global hook dispatcher + repo opt-in/out
- Add new command `lrc install-global-hooks` to set `core.hooksPath=~/.git-hooks` and drop dispatcher script `~/.git-hooks/pre-commit` (and `prepare-commit-msg` if needed) that calls `lrc hook pre-commit` only when repo has `.git/lrc` directory, then chains to repo-local hooks.
- Add `lrc hook pre-commit` subcommand to encapsulate hook logic so global dispatcher can reuse it without duplicating shell.
- Add opt-out command `lrc disable` that writes `.git/lrc/disabled` sentinel; dispatcher should skip when sentinel exists.
- Files to change: command wiring in [cmd/lrc/main.go#L196-L246]; hook generation helpers near [cmd/lrc/main.go#L1697-L2049]; new dispatcher template file under `cmd/lrc/static/` if needed.
- Visible result: `lrc install-global-hooks` followed by `git config --global core.hooksPath` shows `~/.git-hooks`; `ls ~/.git-hooks/pre-commit` exists; `lrc disable` creates `.git/lrc/disabled` and dispatcher skips.

### Phase 6 — Commit message trailer alignment
- Standardize trailer lines to match spec (`LiveReview Pre-Commit Check: ran|skipped|skipped manually`). Ensure commit-msg hook maps attestation action → trailer.
- Update trailer generation in `generateCommitMsgHook` [cmd/lrc/main.go#L2067-L2105] to pull action from attestation file for current `TREE_HASH` instead of transient state.
- Visible result: after creating an attestation and making a commit, `tail -n5 .git/COMMIT_EDITMSG` shows the correct trailer matching the attestation action.

### Phase 7 — CI policy helper
- Add `lrc attest verify --ref <git-ref>` to compute tree hash from ref (`git show -s --format=%T <ref>`) and require attestation file; use in CI scripts.
- Provide sample CI snippet in docs and `README.md` to call this command.
- Visible result: `lrc attest verify --ref HEAD` exits 0 when the attestation exists and non-zero when it does not (check `$?`).

### Phase 8 — Installer/build outputs
- Ensure build installs both `lrc` and `git-lrc` names; update `Makefile` target `lrc-build-local` in [Makefile#L17-L55] (and `scripts/lrc_build.py` if required) to install/ship symlink or duplicate binary name.
- Add uninstall path that restores previous `core.hooksPath` if we changed it.
- Visible result: `which lrc && which git-lrc` both resolve; `git lrc version` prints version; `lrc uninstall-global-hooks` (or equivalent) restores prior hooksPath.

### Phase 9 — VS Code / GUI triggers (optional but scoped)
- Add lightweight command `lrc attest status --json` to let the UI/extension display attestation state for staged tree; no prompting.
- Document how the extension should invoke `lrc review --staged` and then read attestation.
- Visible result: with staged changes, `lrc attest status --json` prints `{ "action": ... }` or indicates missing attestation.

### Phase 10 — Migration and cleanup
- On first run of new hooks, migrate legacy `.git/livereview_state` into a tree-hash attestation if possible (write hash from current index, mark action `skipped` if prior state was `skipped*`, else `reviewed`).
- Add cleanup to remove old lock/state files once migration completes.
- Visible result: after running migration, `find .git -maxdepth 1 -name 'livereview_state*'` shows nothing, and `.git/lrc/attestations/$(git write-tree).json` exists with an action.

### Acceptance checklist per phase (each has a visible command/output)
- Phase 0: `go test ./internal/lrcattest -run TestFormatDecision` passes (format locked).
- Phase 1: `go test ./internal/lrcattest` passes; helper writes/reads minimal JSON attestation in temp dir.
- Phase 2: `lrc review --skip` creates `.git/lrc/attestations/$TREE.json`; `lrc attest show $TREE` displays `{ "action": "skipped" }`.
- Phase 3: Non-interactive `git commit --dry-run` fails without attestation, succeeds after attestation exists; interactive flow still works.
- Phase 4: `lrc install-hooks` installs new hook bodies; `lrc uninstall-hooks` removes lrc sections; hook files show attestation logic.
- Phase 5: `git config --global core.hooksPath` shows `~/.git-hooks`; dispatcher file exists; `lrc disable` prevents dispatcher execution.
- Phase 6: Commit message contains correct `LiveReview Pre-Commit Check:` trailer derived from attestation.
- Phase 7: `lrc attest verify --ref HEAD` returns correct exit status (0 when present, non-zero when absent).
- Phase 8: `which lrc` and `which git-lrc` both resolve; `git lrc version` works; uninstall restores prior hooksPath.
- Phase 9: With staged changes, `lrc attest status --json` shows current staged-tree attestation (or missing).
- Phase 10: `find .git -maxdepth 1 -name 'livereview_state*'` shows none; attestation for current tree exists.
