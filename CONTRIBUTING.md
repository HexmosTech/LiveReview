## Contributing to LiveReview

## TL;DR

- Start with a Discussion if the work is not already agreed and clearly scoped.
- Do not open direct PRs for unscoped work. The preferred flow is Discussion -> Issue -> PR.
- Follow [Local-Setup-Guide.md](./Local-Setup-Guide.md) to get a local database and environment running, then `make build`.
- Run the most specific test that proves your change (`make test`, or `go test ./path/to/package/...`).
- If your change touches UI, a GIF or video walkthrough is required in the PR. This is a hard requirement.
- Private security reports should go through GitHub Security Advisories, not public issues. See [SECURITY.md](./SECURITY.md).
- By contributing, you agree to the Contributor License Agreement.

This guide is here to help you contribute in a way that is clear, scoped, and easy to review.

## Security At A Glance

LiveReview has explicit security and disclosure guidance. Please use it.

- If you are reporting a vulnerability, use the private reporting flow described in [SECURITY.md](./SECURITY.md) and open a GitHub Security Advisory instead of a public issue.
- If your change affects storage, network behavior, credentials, review payloads, licensing/entitlement enforcement, or disclosure handling, read [SECURITY.md](./SECURITY.md) before opening or updating the PR.

## Start With the Right Forum

There are several valid ways to contribute, but they serve different purposes:

- Use [GitHub Discussions](https://github.com/HexmosTech/LiveReview/discussions) for ideas, problem framing, design discussion, and early proposals.
- Use [GitHub Issues](https://github.com/HexmosTech/LiveReview/issues) for concrete, agreed, scoped work.
- Use pull requests to implement an agreed issue.

If the work is still fuzzy, start in Discussions.

## Preferred Contribution Flow

We do **not** recommend directly raising PRs for unscoped work.

The preferred path is:

1. Open a Discussion and describe the problem, the user impact, and the proposed direction.
2. Refine the scope until the work is concrete.
3. Promote the agreed work into an [Issue](https://github.com/HexmosTech/LiveReview/issues).
4. Open a PR that fulfills that issue.

This keeps the project focused and avoids PRs that arrive before the problem has been agreed on.

## Getting Started Locally

Follow [Local-Setup-Guide.md](./Local-Setup-Guide.md) to set up a local PostgreSQL database and the environment variables LiveReview needs.

Then build and run:

```bash
make build
make run-api
```

## Testing Your Change

Run the most specific test that proves your change:

```bash
go test ./path/to/package/...
```

Use the broader lanes only when the scope justifies them:

```bash
make test      # verbose run across all packages
make testall   # the full test package list
```

If a bug escaped once, add a regression test alongside the fix before moving on.

## Quality Bar

Please keep these project rules in mind when proposing or implementing changes:

- A name must match its function or meaning. If a name says one thing and the code does another, treat that as a major bug.
- Do not add fallback implementations, feature flags, or backwards-compatibility shims unless explicitly asked for.
- Prefer incremental changes over sweeping refactors. Make the smallest coherent change that solves the agreed problem.
- Keep the why visible in the Discussion, Issue, and PR so reviewers can evaluate the change on purpose, not just code shape.

## PR Expectations

A good pull request in LiveReview is:

- Based on an agreed issue.
- Narrow enough to review without guessing at hidden scope.
- Clear about what changed and why.
- Explicit about what you tested.

If your PR changes behavior, call that out directly.

## UI Changes Require a GIF or Video

When your work changes the user interface, a GIF or video walkthrough is required.

The walkthrough should make it easy for a reviewer to see:

- what changed,
- how the interaction behaves,
- and that the change was actually exercised.

PRs that change UI without this walkthrough are not complete.

AI-assisted programming is fine, but UI changes still need to be tested and demonstrated clearly.

## Contributor License Agreement

If you contribute to LiveReview, you agree that you have read and accepted the terms in the [Contributor License Agreement](./CLA.md).
