# Requirements Document

## Introduction

This document describes the requirements for Third-Party Tools Integration in LiveReview — a cloud-only, owner-gated feature that allows organisations to enable external static-analysis tools (e.g. ruff, pylint) that run as parallel Lambda-backed jobs whenever a review is triggered. Results are stored in the `review_events` table and surfaced in a dedicated beta review UI and in the existing git-lrc CLI output.

The feature is delivered in three sequential phases:

1. **Phase 1 – DB + Settings tab**: Catalog and per-org tool tables, dbmate migrations, settings tab gated to cloud-mode owners.
2. **Phase 2 – Settings UI + API gating**: Full CRUD UI for tool selection, REST endpoints with role enforcement.
3. **Phase 3 – Queue, Lambda trigger + review UI**: River job fan-out per tool, Lambda invocation, result storage, and two dedicated UI surfaces.

---

## Glossary

- **AvailableTools**: The global catalog table (`available_tools`) that seeds all tools LiveReview knows about.
- **OrgTools**: The per-organisation configuration table (`org_tools`) that records which tools an org has enabled and any per-tool configuration.
- **Tool**: A record in `available_tools` with a unique id, human-readable name, description, and an AWS Lambda ARN to invoke.
- **ToolResult**: A `review_events` row with `event_type = 'tool_result'` that carries the output of a single tool invocation for a review.
- **ToolJob**: A River background job (kind `tool_invocation`) that reads a review's diff and invokes a Tool's Lambda ARN, producing a ToolResult.
- **PermissionContext**: The Echo middleware-constructed object placed in the request context under `permission_context`, encoding the authenticated user's `org_id`, `user_id`, and `role`.
- **CloudModeGate**: The server-side `isCloudMode()` check (`LIVEREVIEW_IS_CLOUD=true`) combined with the `isCloudMode()` client-side utility in the React UI.
- **BillingCheck**: Verification that the org has an active billing plan via the existing `apimiddleware.BuildOrgBillingPlanContext` + `BuildPlanContext` middleware chain.
- **SettingsTab**: The `/#/settings#third-party-tools` hash route within the existing Settings page component.
- **BetaReviewUI**: The beta review page accessible at `/#/reviews-tools/new`, not linked in the main navigation.
- **ReviewDetail**: The existing review detail page that renders events associated with a review.
- **git-lrc**: The CLI binary whose binary name is `lrc`, used for local review triggering.
- **River**: The PostgreSQL-backed job queue library used for background processing.
- **dbmate**: The migration tool used exclusively for local development database schema changes.
- **LambdaInvoker**: The component responsible for making AWS Lambda HTTP invocations for a given tool's ARN.

---

## Requirements

### Requirement 1 – Available Tools Catalog

**User Story:** As a LiveReview platform operator, I want a seeded catalog of available third-party tools, so that organisations can choose from a known set of tools without manual configuration.

#### Acceptance Criteria

1. THE System SHALL provide an `available_tools` table with columns: `id` (bigserial primary key), `name` (text not null unique), `description` (text not null), and `lambda_arn` (text not null).
2. THE System SHALL seed the `available_tools` table with at least two initial rows: one for `ruff` and one for `pylint`, each with a non-empty `description` and a placeholder `lambda_arn`.
3. THE System SHALL manage the `available_tools` schema exclusively through dbmate migration files located in `db/migrations/`, and SHALL NOT apply these migrations directly to any production database.

---

### Requirement 2 – Per-Organisation Tool Configuration

**User Story:** As an organisation owner on the cloud plan, I want to enable or disable specific tools for my organisation, so that only the tools relevant to my stack are invoked during reviews.

#### Acceptance Criteria

1. THE System SHALL provide an `org_tools` table with columns: `org_id` (bigint not null, references `organizations.id`), `tool_id` (bigint not null, references `available_tools.id`), `enabled` (boolean not null default false), `config_json` (jsonb not null default `'{}'`), and a composite primary key on (`org_id`, `tool_id`).
2. THE System SHALL enforce that every row in `org_tools` references a valid `org_id` in the `organizations` table and a valid `tool_id` in the `available_tools` table via foreign key constraints.
3. THE System SHALL manage the `org_tools` schema exclusively through dbmate migration files and SHALL NOT apply these migrations directly to any production database.

---

### Requirement 3 – Settings Tab Access Control

**User Story:** As a cloud-mode organisation owner, I want a dedicated third-party tools settings tab, so that I can manage tool configuration without exposing it to members or self-hosted users.

#### Acceptance Criteria

1. WHEN `isCloudMode()` returns `true` AND the authenticated user's role is `owner`, THE Settings Page SHALL render a navigable tab at the hash route `third-party-tools` within `/#/settings`.
2. WHEN `isCloudMode()` returns `false`, THE Settings Page SHALL NOT render the `third-party-tools` tab.
3. WHEN the authenticated user's role is not `owner`, THE Settings Page SHALL NOT render the `third-party-tools` tab as a clickble navigation item.
4. WHEN a non-owner org member navigates directly to `/#/settings#third-party-tools`, THE Settings Page SHALL render a read-only view of the enabled tools without controls to modify them.

---

### Requirement 4 – List Org Tools API Endpoint

**User Story:** As an authenticated organisation member, I want to retrieve the tool configuration for my organisation, so that the UI can display which tools are currently enabled.

#### Acceptance Criteria

1. THE Server SHALL expose a `GET /api/v1/orgs/:org_id/tools` endpoint that returns the list of all tools in `available_tools` joined with the corresponding `org_tools` row (if present) for the requesting org.
2. WHEN a request is received at `GET /api/v1/orgs/:org_id/tools`, THE Server SHALL apply the full Echo middleware chain: `RequireAuthOrAPIKey`, `BuildOrgContext`, `ValidateOrgAccess`, `BuildPermissionContext`.
3. WHEN `isCloudMode()` returns `false` on the server, THE Server SHALL respond to `GET /api/v1/orgs/:org_id/tools` with HTTP 403.
4. WHEN the `PermissionContext` `org_id` does not match the `:org_id` path parameter, THE Server SHALL respond with HTTP 403.
5. THE Server SHALL scope the `org_tools` query exclusively by the `org_id` resolved from `PermissionContext`, and SHALL NOT use the client-supplied `:org_id` path parameter as the filter value.

---

### Requirement 5 – Update Org Tool API Endpoint

**User Story:** As a cloud-mode organisation owner, I want to enable or disable a specific tool for my organisation via the API, so that tool selection changes are persisted securely.

#### Acceptance Criteria

1. THE Server SHALL expose a `PUT /api/v1/orgs/:org_id/tools/:tool_id` endpoint that creates or updates the `org_tools` row for the given `(org_id, tool_id)` pair.
2. WHEN a request is received at `PUT /api/v1/orgs/:org_id/tools/:tool_id`, THE Server SHALL apply the full Echo middleware chain: `RequireAuthOrAPIKey`, `BuildOrgContext`, `ValidateOrgAccess`, `BuildPermissionContext`.
3. WHEN `isCloudMode()` returns `false` on the server, THE Server SHALL respond to `PUT /api/v1/orgs/:org_id/tools/:tool_id` with HTTP 403.
4. WHEN the authenticated user's role resolved from `PermissionContext` is not `owner`, THE Server SHALL respond to `PUT /api/v1/orgs/:org_id/tools/:tool_id` with HTTP 403.
5. THE Server SHALL validate the request body contains an `enabled` boolean field; IF the field is absent or not a boolean, THEN THE Server SHALL respond with HTTP 400.
6. THE Server SHALL upsert the `org_tools` row using the `org_id` from `PermissionContext` as the scoping value, and SHALL NOT use the client-supplied `:org_id` as the insert/update value.

---

### Requirement 6 – Settings UI for Tool Selection

**User Story:** As a cloud-mode organisation owner, I want a UI inside the settings tab to see all available tools and toggle each one on or off, so that I have direct control over which tools run on my reviews.

#### Acceptance Criteria

1. WHEN the `third-party-tools` settings tab is active and `isCloudMode()` returns `true` and the user is an `owner`, THE ThirdPartyToolsTab Component SHALL render a list of all available tools fetched from `GET /api/v1/orgs/:org_id/tools`.
2. WHEN an owner toggles a tool's enabled state in the UI, THE ThirdPartyToolsTab Component SHALL call `PUT /api/v1/orgs/:org_id/tools/:tool_id` with the new `enabled` value.
3. IF `PUT /api/v1/orgs/:org_id/tools/:tool_id` returns an error, THEN THE ThirdPartyToolsTab Component SHALL display an inline error message and SHALL revert the toggle to the previous state.
4. WHILE the UI is fetching or submitting tool configuration, THE ThirdPartyToolsTab Component SHALL render a loading indicator and disable all tool toggles.
5. WHEN the user is a non-owner member and `isCloudMode()` returns `true`, THE ThirdPartyToolsTab Component SHALL render the tool list in a read-only state with no toggles.

---

### Requirement 7 – Tool Job Fan-Out on Review

**User Story:** As a developer whose organisation has tools enabled, I want the tools to run automatically whenever a review runs, so that I receive tool feedback alongside the AI review without manual intervention.

#### Acceptance Criteria

1. WHEN a review job completes initial diff extraction and at least one tool is enabled for the review's `org_id`, THE ReviewOrchestrator SHALL enqueue one River job of kind `tool_invocation` per enabled tool in parallel (fan-out).
2. THE ToolJob SHALL follow the same River job registration pattern as `webhook_install` and `webhook_removal` in `internal/jobqueue/`, including a `Kind()` method and a dedicated worker struct.
3. THE ToolJob SHALL read the diff content from the review's database record identified by `review_id`, and SHALL NOT re-fetch the diff from the VCS provider.
4. WHEN the ToolJob executes, THE LambdaInvoker SHALL invoke the Lambda function identified by the tool's `lambda_arn` from `available_tools`, passing the diff as the request payload.
5. IF the Lambda invocation returns a non-2xx HTTP status, THEN THE ToolJob SHALL mark the job as failed and River SHALL apply the standard retry policy.

---

### Requirement 8 – Tool Result Storage

**User Story:** As a developer, I want tool results stored alongside review events, so that the review detail page can display them with the correct source attribution.

#### Acceptance Criteria

1. WHEN the LambdaInvoker receives a successful response, THE ToolJob SHALL insert a row into `review_events` with `event_type = 'tool_result'`, the `review_id`, `org_id`, and a `data` JSONB payload containing at minimum `tool_name`, `tool_id`, and the Lambda response body.
2. THE ToolJob SHALL scope the `review_events` insert using the `org_id` from the review record and SHALL NOT derive `org_id` from any other source.
3. WHEN multiple ToolJobs complete for the same review, THE System SHALL store each tool's result as a separate `review_events` row, identified by a distinct `tool_name` value in the `data` JSONB field.

---

### Requirement 9 – Beta Review UI Page

**User Story:** As a developer participating in the beta, I want a dedicated review UI page for tool-based reviews at a hidden route, so that I can access tool results without the route appearing in the main navigation.

#### Acceptance Criteria

1. THE Router SHALL register the path `/#/reviews-tools/new` as a valid client-side route rendering the BetaToolReviewPage component.
2. THE Router SHALL NOT add a navigation entry for `/#/reviews-tools/new` in the main sidebar or any other navigation surface.
3. THE BetaToolReviewPage Component SHALL render a layout similar to the existing AI review UI, adapted to display tool-based review results.
4. WHEN `isCloudMode()` returns `false`, THE BetaToolReviewPage Component SHALL render an informational message indicating the feature is only available in cloud mode.

---

### Requirement 10 – Tool Results in Review Detail UI

**User Story:** As a developer, I want to see tool results labelled by tool name in the review detail page, so that I can distinguish tool findings from AI findings at a glance.

#### Acceptance Criteria

1. WHEN the ReviewDetail Component fetches `review_events` for a review and encounters an event with `event_type = 'tool_result'`, THE ReviewDetail Component SHALL render the event with a badge displaying the value of `data.tool_name`.
2. THE badge for a tool result SHALL be visually distinct from AI review event badges already present in the ReviewDetail Component.
3. WHEN there are no `tool_result` events for a review, THE ReviewDetail Component SHALL NOT render any tool result section.

---

### Requirement 11 – Tool Result Tag in git-lrc CLI Output

**User Story:** As a developer using the lrc CLI, I want tool results tagged with the tool name in the CLI output, so that I can identify which finding came from which tool when reviewing locally.

#### Acceptance Criteria

1. WHEN the `lrc` CLI renders review events and encounters an event with `event_type = 'tool_result'`, THE CLIRenderer SHALL prefix or tag each finding with the value of `data.tool_name` from the event's JSONB payload.
2. THE tag format for tool results in CLI output SHALL be `[<tool_name>]`, where `<tool_name>` is the exact string stored in `data.tool_name`.
3. WHEN there are no `tool_result` events in the review output, THE CLIRenderer SHALL NOT render any tool result section.

---

### Requirement 12 – Full Echo Middleware Chain Enforcement

**User Story:** As a security-conscious operator, I want all third-party tools API endpoints to go through the standard middleware chain, so that org isolation and role-based access are automatically enforced.

#### Acceptance Criteria

1. THE Server SHALL register `GET /api/v1/orgs/:org_id/tools` and `PUT /api/v1/orgs/:org_id/tools/:tool_id` under an Echo group that applies `RequireAuthOrAPIKey`, `BuildOrgContext`, `ValidateOrgAccess`, and `BuildPermissionContext` in that order.
2. WHEN any middleware in the chain returns a non-nil error, THE Server SHALL propagate the HTTP error response and SHALL NOT execute the route handler.
3. THE Server SHALL apply `apimiddleware.BuildOrgBillingPlanContext` and `apimiddleware.BuildPlanContext` to the tools endpoints to enforce the cloud billing check in addition to the role check.

---

### Requirement 13 – API Documentation

**User Story:** As a developer integrating with the tools API, I want up-to-date documentation for each phase's endpoints, so that I can understand the expected request/response formats without reading source code.

#### Acceptance Criteria

1. THE System SHALL maintain API documentation for all three phases in `docs/tools/tools-integration-beta.md`.
2. WHEN a new endpoint or schema change is introduced in a phase, THE System SHALL update `docs/tools/tools-integration-beta.md` to document the endpoint path, method, request body, response schema, and applicable error codes for that phase.
3. THE documentation file SHALL include at minimum: `GET /api/v1/orgs/:org_id/tools`, `PUT /api/v1/orgs/:org_id/tools/:tool_id`, the `ToolResult` event schema, and the `tool_invocation` River job schema.
