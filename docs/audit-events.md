# Audit Event Catalog

This file is the public contract for audit action names. Customers write
SIEM/Splunk/Datadog queries against these strings, so:

- Never rename an action without keeping an alias.
- Never reuse an old action name for a different meaning.
- Add new actions to `internal/auditlog/catalog.go` and regenerate this file.

Generated from `internal/auditlog/catalog.go`.

## Categories

| Category | Label |
|---|---|
| `auth` | Sign-ins & access |
| `user` | Users & roles |
| `org` | Organization |
| `api_key` | API keys |
| `billing` | Billing |
| `settings` | Settings |
| `connector` | Integrations |
| `repository` | Repositories |
| `review` | Reviews |
| `chat` | Chat |
| `export` | Exports & downloads |
| `audit` | Audit log |
| `ai` | AI & models |
| `admin` | Administration |
| `system` | System |
| `other` | Other |

## Actions

| Action | Category | Resource | Severity | Label | Description |
|---|---|---|---|---|---|
| `admin.org.deactivate` | `admin` | org | critical | Organization deactivated by admin | A super admin deactivated an organization. |
| `admin.user.create` | `admin` | user | critical | User created by admin | A super admin created a user in an organization. |
| `admin.user.list` | `admin` | user | info | All users listed | A super admin listed users across organizations. |
| `admin.user.transfer` | `admin` | user | critical | User transferred by admin | A super admin moved a user between organizations. |
| `ai.connector.create` | `ai` | connector | warning | AI provider connected | An AI provider was connected. |
| `ai.connector.delete` | `ai` | connector | warning | AI provider removed | An AI provider was removed. |
| `ai.connector.update` | `ai` | connector | warning | AI provider updated | An AI provider configuration changed. |
| `ai.invocation` | `ai` | model | info | Model invoked | An AI model was called. |
| `ai.model.activate` | `ai` | model | critical | Managed model activated | A LiveReview-managed model was activated. |
| `api_key.create` | `api_key` | api_key | warning | API key created | An API key was created. |
| `api_key.delete` | `api_key` | api_key | warning | API key deleted | An API key was deleted. |
| `api_key.reveal` | `api_key` | api_key | critical | API key viewed | A stored API key or secret was revealed. |
| `api_key.revoke` | `api_key` | api_key | warning | API key revoked | An API key was revoked. |
| `audit.archive.download` | `audit` | archive | warning | Archive downloaded | A monthly audit archive was downloaded. |
| `audit.export` | `audit` | audit | warning | Audit log export requested | Audit data was exported through the API. |
| `audit.integrity.check` | `audit` | audit | info | Integrity checked | The audit hash chain was verified. |
| `audit.read` | `audit` | audit | info | Audit log viewed | The audit log was viewed. |
| `auth.identity.add` | `auth` | identity | warning | Identity linked | A login identity was linked to a user. |
| `auth.identity.remove` | `auth` | identity | warning | Identity unlinked | A login identity was removed from a user. |
| `auth.login.failure` | `auth` | session | warning | Failed sign-in | An authentication attempt failed. |
| `auth.login.success` | `auth` | session | info | Signed in | A user authenticated successfully. |
| `auth.logout` | `auth` | session | info | Signed out | A user ended a session. |
| `auth.password.change` | `auth` | user | warning | Password changed | A user changed their password. |
| `auth.password.reset.complete` | `auth` | user | warning | Password reset completed | A user completed a required password reset. |
| `auth.session.revoke` | `auth` | session | warning | Sessions revoked | A user's active sessions and refresh tokens were revoked, typically following deactivation. |
| `auth.sso.provision` | `auth` | user | info | SSO account provisioned | An account was provisioned through SSO. |
| `auth.token.refresh` | `auth` | session | info | Session refreshed | A session token was refreshed. |
| `auto_approve.policy.create` | `settings` | policy | warning | Auto-approve policy created | An auto-approve policy was created. |
| `auto_approve.policy.delete` | `settings` | policy | warning | Auto-approve policy deleted | An auto-approve policy was deleted. |
| `auto_approve.policy.update` | `settings` | policy | warning | Auto-approve policy updated | An auto-approve policy was changed. |
| `billing.payment.failed` | `billing` | subscription | warning | Payment failed | A billing payment failed. |
| `billing.plan.change` | `billing` | subscription | critical | Plan changed | The organization's plan changed. |
| `billing.subscription.cancel` | `billing` | subscription | critical | Subscription cancelled | A subscription was cancelled. |
| `billing.subscription.confirm` | `billing` | subscription | warning | Purchase confirmed | A subscription purchase was confirmed. |
| `billing.subscription.create` | `billing` | subscription | warning | Subscription created | A subscription was created. |
| `chat.conversation.create` | `chat` | conversation | info | Chat started | A Livi conversation was created. |
| `chat.conversation.delete` | `chat` | conversation | info | Chat deleted | A Livi conversation was deleted. |
| `chat.file.download` | `chat` | file | info | Chat file downloaded | A chat attachment was downloaded. |
| `chat.file.upload` | `chat` | file | info | Chat file uploaded | A file was attached to a chat. |
| `connector.create` | `connector` | connector | warning | Integration connected | A source-control integration was connected. |
| `connector.delete` | `connector` | connector | warning | Integration removed | A source-control integration was removed. |
| `connector.github_app.start` | `connector` | connector | info | GitHub App connection started | A member started connecting a GitHub account or organization through the GitHub App. |
| `connector.reconnect` | `connector` | connector | info | Integration reconnected | A source-control integration was reconnected. |
| `connector.update` | `connector` | connector | warning | Integration updated | A source-control integration was updated. |
| `export.audit` | `export` | audit | warning | Audit log exported | Audit events were exported. |
| `export.chat` | `export` | chat | info | Chat exported | A chat conversation was exported. |
| `export.findings` | `export` | report | warning | Findings exported | Review findings were exported. |
| `export.package` | `export` | audit | warning | Evidence package exported | An audit evidence package was generated. |
| `export.report` | `export` | report | warning | Report exported | An analytics or onboarding report was exported. |
| `learning.create` | `review` | learning | info | Learning created | A review learning was created. |
| `learning.delete` | `review` | learning | warning | Learning deleted | A review learning was deleted. |
| `learning.update` | `review` | learning | info | Learning updated | A review learning was updated. |
| `org.deactivate` | `org` | org | critical | Organization deactivated | The organization was deactivated. |
| `org.member.role.changed` | `org` | member | warning | Member role changed | An organization member's role was changed. |
| `org.settings.update` | `org` | org | warning | Organization settings changed | Organization-level settings were changed. |
| `org.update` | `org` | org | warning | Organization updated | Organization settings were changed. |
| `prompt.create` | `settings` | prompt | warning | Prompt created | A prompt was created. |
| `prompt.delete` | `settings` | prompt | warning | Prompt deleted | A prompt was deleted. |
| `prompt.update` | `settings` | prompt | warning | Prompt updated | A prompt was changed. |
| `repository.create` | `repository` | repository | info | Repository added | A repository was added. |
| `repository.delete` | `repository` | repository | warning | Repository removed | A repository was removed. |
| `repository.sync` | `repository` | repository | info | Repository synced | A repository sync ran. |
| `repository.update` | `repository` | repository | info | Repository updated | A repository's settings were changed. |
| `review.cancel` | `review` | review | warning | Review cancelled | A running review was cancelled. |
| `review.comment.post.failure` | `review` | review | warning | Comment posting failed | LiveReview could not post findings to the provider. |
| `review.create` | `review` | review | info | Review created | A review was created. |
| `review.delete` | `review` | review | warning | Review deleted | A review was deleted. |
| `review.outcome.report` | `review` | review | info | Review outcome reported | A production outcome was reported for a review. |
| `review.retry` | `review` | review | info | Review retried | A review was retried. |
| `review.trigger` | `review` | review | info | Review triggered | A review was started. |
| `ruleset.create` | `repository` | ruleset | warning | Ruleset created | A CI ruleset was created. |
| `ruleset.delete` | `repository` | ruleset | warning | Ruleset deleted | A CI ruleset was deleted. |
| `ruleset.update` | `repository` | ruleset | warning | Ruleset updated | A CI ruleset was changed. |
| `scim.token.create` | `auth` | scim_token | warning | SCIM token created | An org's SCIM bearer token was generated or rotated. |
| `scim.token.revoke` | `auth` | scim_token | warning | SCIM token revoked | An org's SCIM bearer token was revoked. |
| `scim.user.create` | `auth` | user | info | SCIM user provisioned | An identity provider created or linked a user via SCIM. |
| `scim.user.deactivate` | `auth` | user | warning | SCIM user deactivated | An identity provider deactivated a user via SCIM. |
| `scim.user.reactivate` | `auth` | user | warning | SCIM user reactivated | An identity provider reactivated a user via SCIM. |
| `scim.user.update` | `auth` | user | info | SCIM user updated | An identity provider updated a user's attributes via SCIM. |
| `settings.auto_approve.update` | `settings` | setting | warning | Auto-approve updated | The auto-approve policy was changed. |
| `settings.managed_ai.update` | `settings` | setting | critical | Managed AI updated | LiveReview-managed AI configuration was changed. |
| `settings.prompts.update` | `settings` | prompt | warning | Prompts updated | Review prompts were changed. |
| `settings.retention.update` | `settings` | setting | critical | Audit retention updated | The audit retention policy was changed. |
| `settings.risk_model.update` | `settings` | setting | warning | Risk model updated | Risk model configuration was changed. |
| `settings.smtp.update` | `settings` | setting | critical | SMTP settings updated | Outbound email settings were changed. |
| `settings.sso.delete` | `settings` | setting | critical | SSO settings deleted | An organization's single sign-on configuration was removed. |
| `settings.sso.update` | `settings` | setting | critical | SSO settings updated | An organization's single sign-on (OIDC/SAML) configuration was created or changed. |
| `settings.storage.update` | `settings` | setting | critical | Storage settings updated | The blob storage backend was changed. |
| `settings.update` | `settings` | setting | warning | Settings updated | Platform or organization settings were changed. |
| `system.audit.anchor` | `system` | audit | info | Audit anchor written | The daily audit hash anchor was written. |
| `system.audit.archived` | `system` | audit | info | Audit events archived | A month of audit events was archived. |
| `system.audit.maintenance.run` | `system` | audit | info | Audit maintenance run | An operator triggered the retention/archive cycle. |
| `system.audit.pruned` | `system` | audit | info | Audit events pruned | Expired audit events were removed. |
| `system.audit_write_failed` | `system` | audit | critical | Audit write failed | An audit event could not be written. |
| `system.maintenance.error` | `system` | audit | critical | Audit maintenance error | The audit maintenance job failed. |
| `system.partition.created` | `system` | audit | info | Audit partition created | A monthly audit partition was created. |
| `user.audit.read` | `audit` | user | info | User audit viewed | A user's audit history was viewed. |
| `user.bulk_invite` | `user` | user | info | Users invited | Multiple users were invited. |
| `user.create` | `user` | user | warning | User created | A user was added to the organization. |
| `user.deactivate` | `user` | user | warning | User deactivated | A user was deactivated. |
| `user.invite` | `user` | user | info | User invited | A user was invited. |
| `user.password.reset.force` | `user` | user | warning | Forced password reset | An owner required a user to reset their password. |
| `user.reactivate` | `user` | user | warning | User reactivated | A user was reactivated. |
| `user.role.changed` | `user` | user | warning | Role changed | A user's organization role was changed. |
| `user.transfer` | `user` | user | critical | User transferred | A user was moved to another organization. |
| `user.update` | `user` | user | warning | User updated | A user's profile or settings were changed. |
| `webhook.install` | `connector` | webhook | info | Webhook installed | A repository webhook was installed. |
| `webhook.uninstall` | `connector` | webhook | warning | Webhook removed | A repository webhook was removed. |
