# Settings → Third-Party Tools

**Route:** `/settings#third-party-tools`
**Component:** `ui/src/pages/Settings/ThirdPartyToolsTab.tsx`
**Who sees it:** Organization **owners** (and super admins) on **paid plans**, **cloud mode only** — gated by `isCloudMode() && Boolean(isPaidPlan) && (isSuperAdmin || currentOrg?.role === 'owner')` in `Settings.tsx`. In self-hosted deployments or on free plans, the tab is hidden and the backend endpoints return `403 Forbidden`.

## Purpose

Enables and configures third-party static analysis tools (linters, security scanners, and formatters) to run concurrently as parallel AWS Lambda worker jobs alongside AI reviews.

Findings produced by enabled tools are normalized through the 3-tier taxonomy classification system, stored as `tool_result` events in `review_events`, and rendered directly within review findings, review summary comments, and the CLI (`lrc`).

## Cloud-Only Architecture

Third-Party Static Analysis Tools is strictly a **Cloud-only feature** (`isCloudMode()`):

- The Settings tab is visible only in LiveReview Cloud (`isCloudMode()`).
- Backend endpoints (`GET /api/v1/orgs/:org_id/tools` and `PUT /api/v1/orgs/:org_id/tools/:tool_id`) enforce cloud mode via `s.deploymentConfig.IsCloud`. If invoked on self-hosted instances, the server rejects requests with `403 Forbidden` (`{"error": "Third-party tools are only available in cloud mode"}`).
- Tool executions are orchestrated as isolated AWS Lambda functions managed by the LiveReview Cloud worker fleet.

## Supported Launch Tools

The platform provides 15 supported launch tools (catalog IDs 1–15):

| ID | Tool Name | Description | Multiplier | Primary Use Case |
|---|---|---|---|---|
| 1 | `ruff` | Fast Python linter and formatter | 1.0× | Python lint/format |
| 2 | `openapi` | OpenAPI and YAML specification validation | 1.0× | OpenAPI/YAML validation |
| 3 | `osv` | Google open-source vulnerability scanner | 1.0× | Dependencies & Supply Chain |
| 4 | `gitleaks` | Git history secret and credential scanner | 12.0× | Secrets & Security |
| 5 | `trivy` | Container and SCA vulnerability scanner | 4.0× | Dependencies & Supply Chain |
| 6 | `trufflehog` | Deep entropy-based secret detection | 12.0× | Secrets & Security |
| 7 | `tfsec` | Terraform security and misconfiguration scanner | 1.0× | Infrastructure & Config |
| 8 | `hadolint` | Dockerfile best practices linter | 1.0× | Containers & Docker |
| 9 | `actionlint` | GitHub Actions workflow linter | 1.0× | CI/CD & Workflow |
| 10 | `golangci-lint` | Go meta-linter suite | 12.0× | Go Code Quality |
| 11 | `spectral` | API style guide enforcement linter | 3.0× | API Schema |
| 12 | `bandit` | Python security scanner (SAST) | 12.0× | Python SAST |
| 13 | `detect-secrets` | Baseline pattern-based secrets detection | 12.0× | Secrets & Security |
| 14 | `semgrep` | Fast multi-language SAST analyzer | 12.0× | Security & Code Quality |
| 15 | `eslint` | JavaScript / TypeScript linter | 12.0× | JS/TS Code Quality |

## Scoping & Configuration Hierarchy

Tool execution follows strict organization and repository scoping guardrails:

- **Organization Level (Authority)**: Organization Owners (and Super Admins) have authoritative control over which tools are enabled for the organization. Tool enablement is toggled in this tab and persisted in `orgs.settings->'tools'`.
- **Repository Level (`.lrc/policy/tools.toml`)**: Individual repositories can include a `.lrc/policy/tools.toml` file to fine-tune tool execution, define diff pattern matchers, or opt out of specific tools.
- **Enforcement Guardrail**: Repository-level configuration can **only tune or disable tools that are already enabled at the organization level**. A repository **can never force-enable a tool that is disabled at the organization level**. Org-level disabling acts as an absolute administrative boundary.

## Credit Pool & Multiplier Model

Tool execution consumes credits from the organization's monthly credit pool (default limit: 50,000 credits/month):

- **Tool Multiplier**: Each tool has an assigned credit multiplier proportional to its container memory allocation and execution timeout:
  $$\text{Multiplier} = \left(\frac{\text{Memory\_MB}}{1024}\right) \times \left(\frac{\text{Timeout\_Seconds}}{2.5}\right)$$
- **Total Review Cost**: When a review runs, the total credit deduction equals the sum of multipliers of all active enabled tools.
- **Budget Tracking**: The tab displays real-time credit consumption, remaining credits, and estimated review capacity (`remainingCredits / totalMultiplier`) before exhausting the monthly pool.

## Key Actions

- **Toggle tool enablement** — enable or disable any of the 15 launch tools using the status toggle switches.
- **Sort tools** — sort the tool catalog by tool name, use case, credit multiplier, or enabled status.
- **Paginate catalog** — navigate through tools in batches of 10 items per page.
- **Review credit usage and impact** — inspect current monthly credit usage, progress bar, active total multiplier, and estimated remaining reviews.
- **Save configurations** — commit changed tool statuses in a batch update via `PUT /api/v1/orgs/:org_id/tools/:tool_id`.
- **Discard changes** — reload current settings from the backend by navigating away or refreshing if unsaved changes are pending.

## Related Pages

[Settings Overview](settings-overview.md), [Plan & Usage](plan-and-usage.md), [Integrations](integrations.md), [Review Detail](../reviews/review-detail.md)
