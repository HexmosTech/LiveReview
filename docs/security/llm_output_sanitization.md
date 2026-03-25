# LiveReview LLM Output Sanitization

## Executive Summary

LiveReview already protects model input (diffs, comments, metadata) before an LLM call. The post-LLM phase solves the next security gap: model output can still contain sensitive data or unsafe text patterns before it is shown to users.

This control adds an output safety layer that is:

1. Non-blocking for developer workflow.
2. Focused on redaction of high-confidence sensitive patterns.
3. Applied consistently across key response paths.
4. Verifiable through focused tests and operational logs.

The core policy is redact-and-continue, not reject-and-fail.

## Problem Statement

### Why Input-Only Controls Are Not Enough

Even with strong input sanitization, model output can still introduce security risk:

1. The model can echo or synthesize secrets in output text.
2. The model can include PII-like values in summaries or comments.
3. The model can produce instruction-like control phrases that are unsafe to relay directly.
4. Output formatting/repair paths can produce content that bypasses earlier assumptions unless sanitized at the end.

From a governance perspective, this is an egress-control problem: sensitive content leaving the model boundary and reaching user-visible channels.

### Risk to Security and Compliance Programs

Without post-output controls, organizations face:

1. Data exposure risk in review comments and summaries.
2. Audit findings around incomplete AI control coverage.
3. Higher incident response cost for accidental leakage.
4. Loss of trust in AI-assisted review workflows.

## Solution Approach

### Control Objective

Before model output is returned to users, sanitize user-visible text for high-confidence sensitive patterns while preserving normal review flow.

### Design Principles

1. Minimal workflow disruption: no new hard-fail gate in this phase.
2. High ROI coverage: focus on secrets, PII markers, and known control-token artifacts.
3. Shared policy surface: use one sanitizer policy rather than separate per-feature rules.
4. Defense in depth: pair with existing pre-LLM controls.

### What This Control Does

1. Redacts high-confidence secret patterns.
2. Redacts PII-like markers in natural-language output.
3. Sanitizes markdown output to neutralize raw HTML tags.
4. Neutralizes unsafe markdown link schemes while preserving safe links.
5. Applies in both plain-text and structured output paths.
6. Preserves developer experience by returning sanitized output instead of hard failures.

### What This Control Does Not Do (Current Phase)

1. It does not enforce broad semantic moderation policy.
2. It does not reject all malformed outputs.
3. It does not replace upstream JSON repair logic.

## Mid-Level Architecture View

LiveReview now has two complementary sanitization phases:

1. Pre-LLM sanitization: neutralize and scrub untrusted input before model calls.
2. Post-LLM sanitization: scrub user-visible output before return.

In practical terms:

1. Input protections reduce prompt-injection and data leakage risk into the model.
2. Output protections reduce data leakage risk out of the model.

This closes the key control loop: ingress and egress controls around model interaction.

## Technical Implementation (Technical, Not Code-Level)

### Shared Output Sanitization Policy

The shared post-output entrypoint is implemented in:

1. [internal/aisanitize/sanitizer.go](internal/aisanitize/sanitizer.go)
2. [internal/aisanitize/markdown.go](internal/aisanitize/markdown.go)

Key function:

1. SanitizationPostflight

This function applies the same core redaction policy used by existing sanitization capabilities, tuned for user-visible text.
It also applies markdown/link safety controls so unsafe rendered content is neutralized before output leaves the backend.

Markdown and link normalization is implemented as a dedicated helper stage in the sanitizer module so behavior is consistent across output paths.

### Plain-Text Response Path

For plain-text LLM responses, sanitization is applied immediately after model output is received and before it is returned.

Implementation location:

1. [internal/api/unified_processor_v2.go](internal/api/unified_processor_v2.go)

Key behavior:

1. Response is sanitized via applyPostOutputSanitization.
2. A panic-safe wrapper prevents sanitizer panics from breaking provider response flow.
3. Metadata-only logs record whether sanitization was applied and high-level counts.
4. Internal sanitizer error signals (for example PII redaction internal failures) are logged for observability.
5. No raw sensitive output is logged.

### Structured Repair-Aware Path

For structured outputs that pass through parse/repair logic, sanitization is applied after repair and successful parse.

Implementation location:

1. [internal/ai/langchain/json_repair_integration.go](internal/ai/langchain/json_repair_integration.go)

Ordering:

1. Parse attempt.
2. Repair path if needed.
3. Successful parse.
4. Post-output sanitization on user-visible fields.

Scope sanitized in this path:

1. Technical summary text.
2. Comment content.
3. Suggestion text.

Reliability and error-handling behavior in this path:

1. Post-sanitization in parsed results is panic-safe.
2. Internal sanitizer errors are counted and surfaced through logs.
3. Sanitization failures do not crash parsing; processing continues with best-effort safe output.

Structural fields are preserved as-is (for example: file path and line mapping data).

### Provider Posting Boundary Hardening

In addition to central post-output sanitization, comment posting formatters/providers apply output sanitization before sending comment bodies to external Git provider markdown renderers.

Implemented in:

1. [internal/providers/github/github.go](internal/providers/github/github.go)
2. [internal/providers/gitlab/gitlab.go](internal/providers/gitlab/gitlab.go)
3. [internal/providers/gitea/gitea_provider.go](internal/providers/gitea/gitea_provider.go)

This ensures markdown/link safety is enforced even at the final outbound boundary.
Provider formatters also emit explicit warnings when sanitizer internals report errors, improving operational visibility without blocking comment posting.

### Markdown and Link Safety Rules

Output markdown is normalized with these rules:

1. Raw HTML tags in model output are escaped (rendered inert).
2. Markdown links with unsafe schemes are rewritten to inert targets.
3. Safe schemes are preserved: `http`, `https`, and `mailto`.
4. Relative/anchor links remain allowed.
5. Link parsing supports nested parentheses in URLs and labels with whitespace.

This keeps useful markdown formatting while removing high-risk rendering vectors.

### UI Rendering Posture

The current web UI review/event views render content as plain text, not markdown HTML rendering.
This reduces browser-side rendering risk and complements backend output sanitization.

## Verification and Evidence

Focused validation exists for each control point:

1. [internal/aisanitize/postflight_test.go](internal/aisanitize/postflight_test.go)
2. [internal/api/unified_processor_v2_post_sanitize_test.go](internal/api/unified_processor_v2_post_sanitize_test.go)
3. [internal/ai/langchain/json_repair_integration_test.go](internal/ai/langchain/json_repair_integration_test.go)
4. [internal/aisanitize/markdown_test.go](internal/aisanitize/markdown_test.go)
5. [internal/providers/github/github_comment_format_test.go](internal/providers/github/github_comment_format_test.go)
6. [internal/providers/gitlab/gitlab_comment_format_test.go](internal/providers/gitlab/gitlab_comment_format_test.go)
7. [internal/providers/gitea/gitea_provider_test.go](internal/providers/gitea/gitea_provider_test.go)

Operational no-LLM verification mode is also available through:

1. [scripts/sample_review.py](scripts/sample_review.py)

The post-LLM mode executes focused checks and writes artifacts to:

1. [scripts/.sample_review_logs/sample_review.latest.log](scripts/.sample_review_logs/sample_review.latest.log)
2. [scripts/.sample_review_logs/sample_review.latest.json](scripts/.sample_review_logs/sample_review.latest.json)

Additional focused tests for markdown/link safety exist in provider and sanitizer packages.

Recent validation run coverage includes:

1. `go test ./internal/aisanitize -run Markdown -count=1`
2. `go test ./internal/providers/github -run Comment -count=1`
3. `go test ./internal/providers/gitlab -run Comment -count=1`
4. `go test ./internal/providers/gitea -run Comment -count=1`
5. `bash -lc 'go build livereview.go'`

## Practical Interpretation

This phase introduces a practical egress control with low operational friction:

1. It reduces accidental sensitive-data exposure in AI outputs.
2. It maintains developer productivity by avoiding brittle reject behavior.
3. It provides measurable evidence through tests and run artifacts.
4. It creates a stable base for stricter policy gates in future phases if needed.