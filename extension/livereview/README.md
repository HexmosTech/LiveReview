# LiveReview — Reviewed-by-Default for VS Code

**LiveReview** brings reviewed-by-default engineering into your editor.

It integrates with the **LiveReview CLI (`lrc`)** to enforce **AI-assisted code review plus explicit human attestation at commit time**—without changing how you normally write code.

This extension is not about automation for its own sake.
It exists to help teams build a **culture of checking, not believing**.


## Why LiveReview Exists

Modern teams routinely commit code that hasn’t been fully read or understood.

Not out of negligence—but because:

* diffs grow large,
* reviews are deferred to merge time,
* and the system allows unread code to slip through.

The result is:

* weak understanding,
* poor debugging,
* avoidable production issues,
* and diffused responsibility.

LiveReview fixes this by making **review the default**, not an afterthought.


## What This Extension Does

The VS Code extension provides **editor-level visibility and ergonomics** for LiveReview:

* Surfaces **pre-commit review status** inside VS Code
* Integrates cleanly with Git workflows
* Reinforces reviewed-by-default behavior without disruption
* Makes review outcomes explicit and visible

The enforcement itself happens via the **LiveReview CLI (`lrc`)**.
The extension ensures developers stay aware and aligned while coding.


## How the Workflow Works

LiveReview is intentionally minimal and Git-native.

```bash
git add .
git lrc review              # or: git lrc review --skip
git commit -m "message"
```

Each commit records an explicit outcome:

```
LiveReview pre-commit check: ran
```

or, if consciously bypassed:

```
LiveReview pre-commit check: manually skipped
```

There is no silent path.

Either the code was reviewed—or the decision to skip was deliberate and recorded.


## What the VS Code Extension Adds

Inside VS Code, LiveReview:

* Reflects the **current review state** of your working tree
* Signals whether a review has been run or skipped
* Keeps review top-of-mind while context is fresh
* Reinforces responsibility before the commit happens

No new workflow.
No modal interruptions.
Just clear signals at the right time.


## Design Principles

LiveReview is built around a few non-negotiable principles:

### Reviewed by Default

Code should not exist unless it has been reviewed.

### Incremental, Not Overwhelming

Review small changes early—when understanding is cheapest.

### Responsibility Stays with the Author

Reviews do not transfer ownership. They sharpen it.

### Checking Over Believing

We rely on verification, not trust or seniority.


## What LiveReview Is *Not*

* Not a replacement for human judgment
* Not a post-merge review tool
* Not a heavy process or workflow overhaul
* Not “AI for AI’s sake”

LiveReview is a **hygiene standard**, enforced by defaults.


## Requirements

* Git
* LiveReview CLI (`lrc`) installed and configured
* VS Code (recent stable version)

> The extension assumes `lrc` is available in your environment.


## Who This Is For

* Teams that care about **engineering professionalism**
* Developers who want **stronger understanding of their own code**
* Organizations building a **culture of checking**
* Anyone who believes unread code is a liability


## The Standard We’re Normalizing

In mature engineering cultures, this should feel obvious:

> *I don’t commit code I haven’t read.*

LiveReview exists to make that standard enforceable, visible, and routine.