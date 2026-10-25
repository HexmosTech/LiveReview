# Reports → Audit Log

**Routes:** `/reports/audit`
**Component:** `ui/src/pages/Reports/AuditLog.tsx`
**APIs:**
- `GET /api/v1/orgs/:org_id/audit-events` (list, filters, cursor, live `since`)
- `GET /api/v1/orgs/:org_id/audit-events/meta` (retention policy)
- `GET /api/v1/orgs/:org_id/audit-events/facets` (filter values)
- `GET /api/v1/orgs/:org_id/audit-events/:id` (detail)
- `GET /api/v1/orgs/:org_id/audit-events/export?format=csv|ndjson|package`
- `GET /api/v1/orgs/:org_id/audit-events/archive?month=YYYY-MM`
- `GET /api/v1/orgs/:org_id/audit-events/ai-usage`
- `GET /api/v1/admin/audit-events` (super admin, cross-org)
- `GET /api/v1/admin/audit-events/integrity?org_id=` (hash-chain check)
- `GET/PUT /api/v1/admin/audit-settings` (retention policy)

## Purpose

The enterprise audit trail. It answers "who did what, when, from where, and
with what outcome" for every privileged action — sign-ins (including failures),
user and role changes, API-key lifecycle, billing, settings, connectors,
repositories, reviews, exports, and AI/model calls.

Unlike the operational review-events log, audit rows are append-only,
org-scoped, retained beyond a single review, and exportable as evidence.

## Sections

| Section | Covers |
|---|---|
| Activity | The live, filterable event table. Filter by date range, category, action, actor, outcome, or free text. Click a row for the full details JSON, request id, IP, user agent, and hash-chain sequence. |
| AI usage | Model invocations recorded in the billing ledger: provider, model, operation, token counts, cost, actor, and review. Raw prompts are never stored; chat turns contribute a prompt fingerprint and size through the audit event stream. |
| Retention & integrity | Super-admin panel: hot window, total retention, archive/anchor/hash toggles, and a one-click hash-chain verification per organization. |
| Exports | CSV, NDJSON, an evidence package (zip with a manifest and SHA-256), and monthly archive download for data older than the hot window. |

## Who can access it

- **Super admin:** everything, including their own support actions, the cross-org view, and retention settings.
- **Organization owner:** every member event in their organization. Actions performed in super-admin capacity are hidden from owners and visible only to super admins.
- **Organization member:** only actions they performed. The archive, settings, and integrity panel are hidden.

API access follows the same rule. Audit reads require either a normal user
session or a dedicated **audit-only API key** (`kind: audit_reader`, created by
an owner). A standard API key cannot read audit data. Cross-org admin reads are
session-only.

## How to get here

- Mega menu: **Reports → Audit Log**.
- Directly at `/reports/audit`.
- From a user's profile, "View full audit trail" filters the same page to that
  actor.

## Key actions

- Watch the live tail (5-second polling, paused while the tab is hidden).
- Filter to a security question, then export exactly that filtered view.
- Open an event to see its sanitized details and correlation id.
- Download a monthly archive when the event is older than the hot window.
- Super admins can verify that an organization's hash chain is intact and
  adjust the retention policy.

## Related pages

[Reports → Taxonomy Reports](taxonomy-reports.md),
[Settings → API Keys](../settings/api-keys.md),
[Settings → User Management](../settings/user-management.md)
