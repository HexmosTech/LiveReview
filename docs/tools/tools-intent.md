# Third-Party Tools Integration — Product Intent

## 1. What is Third-Party Tools Integration?

LiveReview lets engineering teams run third-party static analysis tools (like linters, security scanners, and formatters) directly alongside AI code reviews.

When a developer opens a pull request or runs a review, LiveReview executes the enabled tools, collects their findings, categorizes them cleanly, and presents them together with the AI review comments on the exact lines of code.

### Why We Built It
- **Catch Common Bugs Instantly**: Linters and security scanners quickly catch syntax errors, secrets, and known CVEs.
- **Combined Review Experience**: Developers see both AI insights and deterministic tool findings in one single review.
- **Controlled Costs**: Organization owners choose which tools run and track monthly usage with simple credits.

---

## 2. Who Can Use It?

- **Paid Cloud Plans Only**: Tool execution is available for organizations on active paid cloud plans (Team plan and LOC plans).
- **Hidden for Free Plans**: Organizations on free plans do not see tool settings, dropdowns, or credit meters.
- **Strictly Omitted for Self-Hosted / Non-Cloud**: Third-party tools run exclusively in LiveReview's managed cloud environment. Non-cloud (self-hosted) deployments have no tool UI, no settings tab, no KPI counters, and no tool endpoints whatsoever.
- **Manual Management via lr-tools Only**: No new tools can be added or registered via the LiveReview API. All tool configurations, Lambda deployments, and catalog definitions are manually provisioned through the `lr-tools` repository.
- **Catalog Empty State**: If no tools are registered in the cloud catalog, the UI displays *"No tools registered — Tools are not available at the moment."*
- **Role Permissions**:
  - **Organization Owners**: Can turn tools on or off, view credit balances, and adjust settings.
  - **Organization Members**: Can see tool findings and execution status on reviews, but cannot change organization settings.

---

## 3. How It Works (The Review Workflow)

Tools run automatically across all review workflows:
1. **Pull Request Webhooks**: When code is pushed to GitHub or GitLab.
2. **Dashboard Manual Reviews**: When a user clicks "Start Review" in the web UI.
3. **CLI Reviews**: When a developer runs `lrc review` from their terminal.
4. **Scheduled Reviews**: During recurring or nightly review jobs.
5. **API & MCP Triggers**: When reviews are started via API or MCP agent tools.

### Execution Rules
- **Runs in Parallel**: Dispatched tools run in parallel with each other and alongside the AI review in the cloud.
- **One Combined Review**: Findings from both AI and static tools are placed into the same review comment list.
- **Git Provider PR Comments**: Tool findings must also be posted back to the Git provider (GitHub, GitLab, Bitbucket) as inline pull request comments, matching the presentation of AI review comments.
- **Review Completion**: A review marks itself complete only after all active tools and the AI review finish.

---

## 4. How Organizations and Repositories Control Tools

Tool execution uses a three-level control model: **Organization Default**, **Repository UI Override (PR reviews only)**, and **Repository File Policy (`.lrc/policy/tools.toml`)**.

### 4.1 Organization Level (Settings → Third-Party Tools)
- Organization owners choose which tools are allowed to run by default for the organization (`orgs.settings->'tools'`).
- When a repository is set to inherit (default) and provides no `.lrc/policy/tools.toml` file, reviews run these organization-level enabled tools.

### 4.2 Repository UI Level (`repositories.settings->'tools'`, PR Reviews Only)
From **Explore → Repositories → Settings → Third-Party Tools**:
- **Inherit organization default (`inherit`)**: Automatically follows Changes in Organization Settings.
- **Custom for this repository (`custom`)**: Saves a repository-specific selection of enabled tools under `repositories.settings->'tools'`.
- **PR-Only Scope**: Applies **only to pull/merge request reviews**. CLI reviews (`lrc review`) and scheduled branch reviews ignore per-repository UI overrides and continue using organization defaults plus local `.lrc/policy/tools.toml`.

### 4.3 Repository File Policy (`.lrc/policy/tools.toml`)
Repositories can fine-tune and override tool execution by adding a configuration file at `.lrc/policy/tools.toml` (or `.lrc/tools.toml`):

1. **Baseline + Overlay Model**: Organization settings (or custom Repository UI settings on PR reviews) act as the baseline. `.lrc/policy/tools.toml` serves as the final overlay:
   - **Explicit Opt-In**: A tool configured with `enabled = true` (or custom rules) will run even if disabled in the organization's settings.
   - **Explicit Opt-Out**: A tool configured with `enabled = false` will NOT run, even if enabled in the organization's settings:
     ```toml
     [gitleaks]
     enabled = false
     ```
   - **Inherited Default**: Any tool **not mentioned** in `tools.toml` automatically inherits its setting from the organization defaults (runs if enabled globally, skips if disabled globally).
2. **File Pattern Scoping (`include` / `exclude`)**: A repository can choose which files a tool should inspect:
   ```toml
   [ruff]
   include = ["**/*.py"]
   exclude = ["tests/fixtures/**"]
   ```
3. **Zero-Credit Skip**: If a pull request does not change any files matching a tool (for example, a PR with only Markdown changes), the tool does not run and **zero credits** are consumed.
4. **Safe Fallback**: If `tools.toml` has a syntax error, LiveReview logs a warning and falls back safely to the organization's default enabled tools without failing the review.
5. **In-PR Changes**: If `.lrc/policy/tools.toml` is modified or added directly within the pull request diff, LiveReview extracts and applies the PR's version so reviewers test the proposed configuration immediately.

---

## 5. Supported Tools Catalog

LiveReview supports 15 popular tools out of the box:

| Tool Name | Multiplier | What It Does |
|---|:---:|---|
| **ruff** | 1.0× | Fast Python linting and formatting |
| **openapi** | 1.0× | OpenAPI and Swagger specification validator |
| **osv** | 1.0× | Open-source dependency vulnerability scanner |
| **gitleaks** | 12.0× | Scans for committed secrets, passwords, and API keys |
| **trivy** | 4.0× | Container and supply-chain vulnerability scanner |
| **trufflehog** | 12.0× | High-entropy secret and credential scanner |
| **tfsec** | 1.0× | Terraform security and misconfiguration checks |
| **hadolint** | 1.0× | Dockerfile best practices and linting |
| **actionlint** | 1.0× | GitHub Actions workflow syntax checker |
| **golangci-lint** | 12.0× | Comprehensive Go linting and static analysis suite |
| **spectral** | 3.0× | JSON/YAML API style guide enforcement |
| **bandit** | 12.0× | Python security static analysis (SAST) |
| **detect-secrets** | 12.0× | Baseline pattern secret detection |
| **semgrep** | 12.0× | Multi-language semantic security scanner |
| **eslint** | 12.0× | JavaScript and TypeScript linting |

---

## 6. How Credits Work

- **Credit Multiplier**: Each tool has a multiplier based on the memory and time it takes to run (for example, lightweight linters like `ruff` cost 1.0 credit; heavier security scanners like `semgrep` cost 12.0 credits).
- **Review Cost**: The cost of a review is the sum of the multipliers for the tools that actually ran on that review.
- **Plan-Based Allowance**: Tool credits scale automatically with your subscription's Monthly LOC limit based on a single base number (50,000 credits per 100k LOC):
  - **Team Starter (100k LOC)**: 50,000 tool credits / month
  - **LOC 200k**: 100,000 tool credits / month
  - **LOC 400k**: 200,000 tool credits / month
  - **LOC 800k**: 400,000 tool credits / month
  - **LOC 1.6M**: 800,000 tool credits / month
  - **LOC 3.2M**: 1,600,000 tool credits / month
  *(If the base allowance is adjusted later, all plan tiers update automatically in lockstep).*
- **Automatic Payment Upgrade**: When an organization upgrades its plan via payment, its monthly tool credits limit is updated automatically alongside its new LOC limit.
- **Zero Double-Billing**: Every credit deduction is saved with a unique review ID so retries or reloads never charge twice.
- **Monthly Reset**: Credit usage resets automatically to zero at the start of each monthly billing cycle.
- **Insufficient Credits**: If an organization runs out of credits, tool execution is safely skipped with an informative notice, and the review proceeds with AI analysis only.

---

## 7. Diff Size & Payload Limits

To keep reviews fast and respect cloud runner limits:
- **Large Files**: Single files larger than **1 MiB** in the diff are skipped from tool analysis.
- **Zip Limit**: The compressed diff archive cannot exceed **6 MB**. If reached, packaging halts and the review is marked as `partial`.
- **Binaries Skipped**: Images, videos, and compiled binary files are skipped automatically.
- **Whitespace Preserved**: Blank lines in diffs are kept intact so tool findings match the exact line numbers in GitHub/GitLab.

---

## 8. How Findings Are Shown in the UI

### 8.1 Review Details Page
- **Top Bar Status**: Shows a summary button like `2 Tools · 3 findings`. Clicking it opens the Tool Analysis Card.
- **Tool Analysis Card**: Shows each tool that ran, its credit cost, and a status badge:
  - **Clean**: Tool ran and found zero issues.
  - **Findings**: Tool found issues.
  - **Partial**: Tool ran on part of the diff because size limits were reached.
  - **Failed**: Tool crashed or exited with an error.
- **Accounting Tile**: A `Tool Credits` tile in the KPI grid shows total credits used for the review.

### 8.2 Diff Viewer & Filtering
- **Quick Filter Bar**: The top bar contains **Severity chips only** (`Critical`, `Warning`, `Info`) for fast filtering.
- **Filter Drawer**: Tool filters live inside the expandable details drawer so the top bar stays clean.
- **Comment Fidelity**: Review comments show the tool's exact, unmodified message on the code line.
- **Safe Unknown Fallback**: If a tool reports a new or unrecognized rule, LiveReview assigns it a safe informational status (`Maintainability: Technical Debt`, `info` severity) with a `[Proposed]` badge. It **never** defaults unknown rules to critical security alerts, ensuring no false alarms or blocked PRs.

### 8.3 Git Provider Pull Request Comments
- **Posting to Git Providers**: In addition to displaying findings inside the LiveReview web dashboard, static analysis tool findings should also be posted directly to the Git provider (GitHub, GitLab, Bitbucket) as inline review comments on the pull request/merge request.
- **Unified Comment Experience**: Tool findings should be clearly attributed (e.g. `[Semgrep]`, `[TruffleHog]`) with their rule, severity, and suggested remediation, posted alongside the AI review comments so developers receive all static analysis feedback directly in their pull request review workflow.
- **Coordination**: When tool execution completes, findings are submitted to the Git hosting provider's PR comments API using the repository's integration token.

---

## 9. Version Stability & Rule Tracking

LiveReview tracks static tool findings and classifications based on the canonical rule identifier: **`tool:rule`** (e.g. `bandit:b608`, `ruff:f401`).

- **Tracked by `tool:rule`**: Every tool finding is keyed by its normalized `tool:rule_id`. This allows instant deterministic categorization (0 tokens) and permanent caching across server restarts.
- **Upgrades Don't Break Old Reviews**: Upgrading a scanner binary (e.g. Semgrep v1.70 to v1.75) does not change past reviews or overwrite approved rule categories.
- **Clean Diff ≠ Deprecated**: If a tool reports zero findings on a clean PR, that rule is just absent from that review; it is not considered deprecated.
- **New Rules**: Brand new rules are classified using LiveReview's standard taxonomy, saved with `status = 'proposed'`, and queued for owner review.

### TODO / Future Roadmap: Dynamic Per-Finding Confidence & Properties
> **TODO**: Currently, taxonomy caching and lookup are tracked strictly by `tool:rule`. However, some tools (such as Bandit and Semgrep) emit per-finding confidence (e.g. `issue_confidence: "MEDIUM"` vs `"HIGH"`) or update their severity calibration across versions. In a future phase, we need to upgrade the runner payload contract and ingestion pipeline to track dynamic confidence and severity per finding rather than relying solely on the static `tool:rule` taxonomy tuple.

---

## 10. Future Iteration: Async Parallel Batch Execution Model

### The Problem in Current Queue Architecture
In the initial v1 release, `DispatchTools()` enqueues each enabled tool as an independent `ToolReviewJob` into the shared River queue (`"review"`). When a review enables 15 tools, 15 separate database jobs are queued.

Under concurrent team usage (for example, 10 developers creating PRs simultaneously):
- **10 AI Review Jobs + 150 Tool Jobs = 160 jobs** are inserted into the same queue.
- On standard production instances with limited worker concurrency (e.g. 2–10 workers), this causes **Head-of-Line Blocking**: subsequent developers' fast AI reviews wait in line behind preceding developers' 15 individual tool jobs, artificially inflating review completion times from ~1.5 minutes up to 6–8 minutes.

### The Next Iteration Design: Async Parallel Fan-Out
In the upcoming iteration, tool execution will be redesigned around an **Async Parallel Model**:

1. **Single Batch Job per Review**:
   - Instead of 15 individual River queue records per review, LiveReview will enqueue **1 single `ReviewToolsBatchJob`** (or invoke async fan-out directly).
   - For 10 concurrent developers, total River jobs drop from **160 jobs $\rightarrow$ 20 jobs** (an 87.5% reduction in queue overhead).

2. **Parallel Async Invocation (All 15 at Once)**:
   - The batch worker fires all 15 tool executions simultaneously using asynchronous non-blocking Go routines (or AWS Lambda's asynchronous `InvocationType: Event` with a `/callback` webhook).
   - Because cloud runners (AWS Lambda) scale elastically in parallel, all 15 tools execute at the exact same instant in the cloud.

3. **Sub-45-Second Total Review Latency**:
   - Total tool execution time will be determined by the single slowest tool ($\sim 4–6$ seconds) rather than the sequential sum of all 15 tools.
   - Eliminates queue starvation completely: AI reviews and tool runs execute on independent tracks, bringing 10-developer concurrent review turnaround back to **under 45 seconds**.

---

## 11. Technical Specifications & Reference

For complete engineering details, database schemas, API contracts, and implementation architectures, please see:
👉 **[Third-Party Tools Integration — Beta Specification](file:///home/gk/hex/LiveReview/docs/tools/tools-integration-beta.md)**
