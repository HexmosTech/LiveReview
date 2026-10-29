// Package auditlog is LiveReview's audit-trail framework.
//
// It has three jobs:
//
//  1. Provide one typed catalog of audit actions/categories/severities, so
//     action names are a stable contract (SIEM queries depend on them).
//  2. Record events either inside a business transaction (RecordTx, for
//     privileged changes) or best-effort from HTTP middleware (Record).
//  3. Serve audit reads through a visibility-aware query builder, so no
//     handler ever hand-writes an audit WHERE clause.
//
// New mutating routes are captured by the HTTP middleware automatically. Adding
// a route-level Spec upgrades the event from generic (http.put) to semantic
// (user.role_changed). See AGENTS.md, "Audit Logging Is Automatic".
package auditlog

import (
	"sort"
)

type Action string
type Category string
type Severity string
type Outcome string
type ActorKind string
type ActorCapacity string
type Source string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
	OutcomeError   Outcome = "error"
)

const (
	ActorKindMember     ActorKind = "member"
	ActorKindAPIKey     ActorKind = "api_key"
	ActorKindSystem     ActorKind = "system"
	ActorKindWebhook    ActorKind = "webhook"
	ActorKindSuperAdmin ActorKind = "super_admin"
	ActorKindLegacy     ActorKind = "legacy"
)

const (
	ActorCapacityMember ActorCapacity = "member"
	ActorCapacityAdmin  ActorCapacity = "admin"
	ActorCapacitySystem ActorCapacity = "system"
)

const (
	SourceAPI     Source = "api"
	SourceWebhook Source = "webhook"
	SourceSystem  Source = "system"
	SourceLegacy  Source = "legacy"
)

// Categories are the filter chips in the audit UI.
const (
	CategoryAuth       Category = "auth"
	CategoryUser       Category = "user"
	CategoryOrg        Category = "org"
	CategoryAPIKey     Category = "api_key"
	CategoryBilling    Category = "billing"
	CategorySettings   Category = "settings"
	CategoryConnector  Category = "connector"
	CategoryRepository Category = "repository"
	CategoryReview     Category = "review"
	CategoryChat       Category = "chat"
	CategoryExport     Category = "export"
	CategoryAudit      Category = "audit"
	CategoryAI         Category = "ai"
	CategoryAdmin      Category = "admin"
	CategorySystem     Category = "system"
	CategoryOther      Category = "other"
)

var categoryLabels = map[Category]string{
	CategoryAuth:       "Sign-ins & access",
	CategoryUser:       "Users & roles",
	CategoryOrg:        "Organization",
	CategoryAPIKey:     "API keys",
	CategoryBilling:    "Billing",
	CategorySettings:   "Settings",
	CategoryConnector:  "Integrations",
	CategoryRepository: "Repositories",
	CategoryReview:     "Reviews",
	CategoryChat:       "Chat",
	CategoryExport:     "Exports & downloads",
	CategoryAudit:      "Audit log",
	CategoryAI:         "AI & models",
	CategoryAdmin:      "Administration",
	CategorySystem:     "System",
	CategoryOther:      "Other",
}

// Every action that LiveReview emits. Keep names stable: they are a public
// SIEM contract. To rename one, keep the old constant as an alias and document
// both in docs/audit-events.md.
const (
	ActionLoginSuccess       Action = "auth.login.success"
	ActionLoginFailure       Action = "auth.login.failure"
	ActionLogout             Action = "auth.logout"
	ActionTokenRefresh       Action = "auth.token.refresh"
	ActionPasswordChange     Action = "auth.password.change"
	ActionPasswordForceReset Action = "user.password.reset.force"
	ActionPasswordResetDone  Action = "auth.password.reset.complete"
	ActionIdentityAdd        Action = "auth.identity.add"
	ActionIdentityRemove     Action = "auth.identity.remove"
	ActionSSOProvision       Action = "auth.sso.provision"
	ActionSessionRevoke      Action = "auth.session.revoke"

	ActionSCIMUserCreate     Action = "scim.user.create"
	ActionSCIMUserUpdate     Action = "scim.user.update"
	ActionSCIMUserDeactivate Action = "scim.user.deactivate"
	ActionSCIMUserReactivate Action = "scim.user.reactivate"
	ActionSCIMTokenCreate    Action = "scim.token.create"
	ActionSCIMTokenRevoke    Action = "scim.token.revoke"

	ActionUserCreate     Action = "user.create"
	ActionUserUpdate     Action = "user.update"
	ActionUserDeactivate Action = "user.deactivate"
	ActionUserReactivate Action = "user.reactivate"
	ActionUserRoleChange Action = "user.role.changed"
	ActionUserTransfer   Action = "user.transfer"
	ActionUserInvite     Action = "user.invite"
	ActionUserBulkInvite Action = "user.bulk_invite"
	ActionUserAuditRead  Action = "user.audit.read"

	ActionOrgUpdate         Action = "org.update"
	ActionOrgDeactivate     Action = "org.deactivate"
	ActionOrgRoleChange     Action = "org.member.role.changed"
	ActionOrgSettingsUpdate Action = "org.settings.update"

	ActionAPIKeyCreate Action = "api_key.create"
	ActionAPIKeyRevoke Action = "api_key.revoke"
	ActionAPIKeyDelete Action = "api_key.delete"
	ActionAPIKeyReveal Action = "api_key.reveal"

	ActionSubscriptionCreate  Action = "billing.subscription.create"
	ActionSubscriptionCancel  Action = "billing.subscription.cancel"
	ActionSubscriptionConfirm Action = "billing.subscription.confirm"
	ActionPlanChange          Action = "billing.plan.change"
	ActionPaymentFailed       Action = "billing.payment.failed"

	ActionSettingsUpdate        Action = "settings.update"
	ActionSettingsSMTPUpdate    Action = "settings.smtp.update"
	ActionSettingsStorageUpdate Action = "settings.storage.update"
	ActionSettingsManagedAI     Action = "settings.managed_ai.update"
	ActionSettingsAutoApprove   Action = "settings.auto_approve.update"
	ActionSettingsRiskModel     Action = "settings.risk_model.update"
	ActionSettingsRetention     Action = "settings.retention.update"
	ActionSettingsPrompts       Action = "settings.prompts.update"
	ActionSettingsSSOUpdate     Action = "settings.sso.update"
	ActionSettingsSSODelete     Action = "settings.sso.delete"

	ActionConnectorCreate       Action = "connector.create"
	ActionConnectorUpdate       Action = "connector.update"
	ActionConnectorDelete       Action = "connector.delete"
	ActionConnectorConnect      Action = "connector.reconnect"
	ActionGitHubAppConnectStart Action = "connector.github_app.start"
	ActionWebhookInstall        Action = "webhook.install"
	ActionWebhookUninstall      Action = "webhook.uninstall"

	ActionRepositoryCreate Action = "repository.create"
	ActionRepositoryUpdate Action = "repository.update"
	ActionRepositoryDelete Action = "repository.delete"
	ActionRepositorySync   Action = "repository.sync"
	ActionRulesetCreate    Action = "ruleset.create"
	ActionRulesetUpdate    Action = "ruleset.update"
	ActionRulesetDelete    Action = "ruleset.delete"

	ActionReviewCreate        Action = "review.create"
	ActionReviewTrigger       Action = "review.trigger"
	ActionReviewCancel        Action = "review.cancel"
	ActionReviewDelete        Action = "review.delete"
	ActionReviewRetry         Action = "review.retry"
	ActionReviewOutcomeReport Action = "review.outcome.report"
	ActionReviewCommentFail   Action = "review.comment.post.failure"

	ActionChatConversationCreate Action = "chat.conversation.create"
	ActionChatConversationDelete Action = "chat.conversation.delete"
	ActionChatFileUpload         Action = "chat.file.upload"
	ActionChatFileDownload       Action = "chat.file.download"

	ActionExportFindings Action = "export.findings"
	ActionExportReport   Action = "export.report"
	ActionExportAudit    Action = "export.audit"
	ActionExportChat     Action = "export.chat"
	ActionExportPackage  Action = "export.package"

	ActionAuditRead            Action = "audit.read"
	ActionAuditExport          Action = "audit.export"
	ActionAuditArchiveDownload Action = "audit.archive.download"
	ActionAuditIntegrityCheck  Action = "audit.integrity.check"

	ActionAIInvocation      Action = "ai.invocation"
	ActionAIConnectorCreate Action = "ai.connector.create"
	ActionAIConnectorUpdate Action = "ai.connector.update"
	ActionAIConnectorDelete Action = "ai.connector.delete"
	ActionAIModelActivate   Action = "ai.model.activate"

	ActionAdminUserList      Action = "admin.user.list"
	ActionAdminUserCreate    Action = "admin.user.create"
	ActionAdminUserTransfer  Action = "admin.user.transfer"
	ActionAdminOrgDeactivate Action = "admin.org.deactivate"

	ActionSystemAuditPruned      Action = "system.audit.pruned"
	ActionSystemAuditArchived    Action = "system.audit.archived"
	ActionSystemAuditAnchor      Action = "system.audit.anchor"
	ActionSystemAuditMaintenance Action = "system.audit.maintenance.run"
	ActionSystemAuditWriteFail   Action = "system.audit_write_failed"
	ActionSystemPartition        Action = "system.partition.created"
	ActionSystemMaintenanceErr   Action = "system.maintenance.error"

	ActionLearningCreate Action = "learning.create"
	ActionLearningUpdate Action = "learning.update"
	ActionLearningDelete Action = "learning.delete"

	ActionPromptCreate Action = "prompt.create"
	ActionPromptUpdate Action = "prompt.update"
	ActionPromptDelete Action = "prompt.delete"

	ActionAutoApproveCreate Action = "auto_approve.policy.create"
	ActionAutoApproveUpdate Action = "auto_approve.policy.update"
	ActionAutoApproveDelete Action = "auto_approve.policy.delete"
)

// ActionMeta is the documentation + defaulting record for one action.
type ActionMeta struct {
	Action        Action
	Category      Category
	ResourceType  string
	Label         string
	Description   string
	Severity      Severity
	Sensitive     bool // fail-closed when recorded in a business transaction
	SensitiveRead bool
}

func meta(a Action, c Category, resource, label, description string, sev Severity) ActionMeta {
	return ActionMeta{Action: a, Category: c, ResourceType: resource, Label: label, Description: description, Severity: sev}
}

func sensitive(m ActionMeta) ActionMeta     { m.Sensitive = true; return m }
func readSensitive(m ActionMeta) ActionMeta { m.SensitiveRead = true; return m }

var actionCatalog = func() map[Action]ActionMeta {
	items := []ActionMeta{
		meta(ActionLoginSuccess, CategoryAuth, "session", "Signed in", "A user authenticated successfully.", SeverityInfo),
		meta(ActionLoginFailure, CategoryAuth, "session", "Failed sign-in", "An authentication attempt failed.", SeverityWarning),
		meta(ActionLogout, CategoryAuth, "session", "Signed out", "A user ended a session.", SeverityInfo),
		meta(ActionTokenRefresh, CategoryAuth, "session", "Session refreshed", "A session token was refreshed.", SeverityInfo),
		sensitive(meta(ActionPasswordChange, CategoryAuth, "user", "Password changed", "A user changed their password.", SeverityWarning)),
		sensitive(meta(ActionPasswordForceReset, CategoryUser, "user", "Forced password reset", "An owner required a user to reset their password.", SeverityWarning)),
		meta(ActionPasswordResetDone, CategoryAuth, "user", "Password reset completed", "A user completed a required password reset.", SeverityWarning),
		meta(ActionIdentityAdd, CategoryAuth, "identity", "Identity linked", "A login identity was linked to a user.", SeverityWarning),
		meta(ActionIdentityRemove, CategoryAuth, "identity", "Identity unlinked", "A login identity was removed from a user.", SeverityWarning),
		meta(ActionSSOProvision, CategoryAuth, "user", "SSO account provisioned", "An account was provisioned through SSO.", SeverityInfo),
		sensitive(meta(ActionSessionRevoke, CategoryAuth, "session", "Sessions revoked", "A user's active sessions and refresh tokens were revoked, typically following deactivation.", SeverityWarning)),
		sensitive(meta(ActionSCIMUserCreate, CategoryAuth, "user", "SCIM user provisioned", "An identity provider created or linked a user via SCIM.", SeverityInfo)),
		sensitive(meta(ActionSCIMUserUpdate, CategoryAuth, "user", "SCIM user updated", "An identity provider updated a user's attributes via SCIM.", SeverityInfo)),
		sensitive(meta(ActionSCIMUserDeactivate, CategoryAuth, "user", "SCIM user deactivated", "An identity provider deactivated a user via SCIM.", SeverityWarning)),
		sensitive(meta(ActionSCIMUserReactivate, CategoryAuth, "user", "SCIM user reactivated", "An identity provider reactivated a user via SCIM.", SeverityWarning)),
		sensitive(meta(ActionSCIMTokenCreate, CategoryAuth, "scim_token", "SCIM token created", "An org's SCIM bearer token was generated or rotated.", SeverityWarning)),
		sensitive(meta(ActionSCIMTokenRevoke, CategoryAuth, "scim_token", "SCIM token revoked", "An org's SCIM bearer token was revoked.", SeverityWarning)),

		sensitive(meta(ActionUserCreate, CategoryUser, "user", "User created", "A user was added to the organization.", SeverityWarning)),
		sensitive(meta(ActionUserUpdate, CategoryUser, "user", "User updated", "A user's profile or settings were changed.", SeverityWarning)),
		sensitive(meta(ActionUserDeactivate, CategoryUser, "user", "User deactivated", "A user was deactivated.", SeverityWarning)),
		sensitive(meta(ActionUserReactivate, CategoryUser, "user", "User reactivated", "A user was reactivated.", SeverityWarning)),
		sensitive(meta(ActionUserRoleChange, CategoryUser, "user", "Role changed", "A user's organization role was changed.", SeverityWarning)),
		sensitive(meta(ActionUserTransfer, CategoryUser, "user", "User transferred", "A user was moved to another organization.", SeverityCritical)),
		meta(ActionUserInvite, CategoryUser, "user", "User invited", "A user was invited.", SeverityInfo),
		meta(ActionUserBulkInvite, CategoryUser, "user", "Users invited", "Multiple users were invited.", SeverityInfo),
		readSensitive(meta(ActionUserAuditRead, CategoryAudit, "user", "User audit viewed", "A user's audit history was viewed.", SeverityInfo)),

		sensitive(meta(ActionOrgUpdate, CategoryOrg, "org", "Organization updated", "Organization settings were changed.", SeverityWarning)),
		sensitive(meta(ActionOrgDeactivate, CategoryOrg, "org", "Organization deactivated", "The organization was deactivated.", SeverityCritical)),
		sensitive(meta(ActionOrgRoleChange, CategoryOrg, "member", "Member role changed", "An organization member's role was changed.", SeverityWarning)),
		sensitive(meta(ActionOrgSettingsUpdate, CategoryOrg, "org", "Organization settings changed", "Organization-level settings were changed.", SeverityWarning)),

		sensitive(meta(ActionAPIKeyCreate, CategoryAPIKey, "api_key", "API key created", "An API key was created.", SeverityWarning)),
		sensitive(meta(ActionAPIKeyRevoke, CategoryAPIKey, "api_key", "API key revoked", "An API key was revoked.", SeverityWarning)),
		sensitive(meta(ActionAPIKeyDelete, CategoryAPIKey, "api_key", "API key deleted", "An API key was deleted.", SeverityWarning)),
		readSensitive(meta(ActionAPIKeyReveal, CategoryAPIKey, "api_key", "API key viewed", "A stored API key or secret was revealed.", SeverityCritical)),

		sensitive(meta(ActionSubscriptionCreate, CategoryBilling, "subscription", "Subscription created", "A subscription was created.", SeverityWarning)),
		sensitive(meta(ActionSubscriptionCancel, CategoryBilling, "subscription", "Subscription cancelled", "A subscription was cancelled.", SeverityCritical)),
		sensitive(meta(ActionSubscriptionConfirm, CategoryBilling, "subscription", "Purchase confirmed", "A subscription purchase was confirmed.", SeverityWarning)),
		sensitive(meta(ActionPlanChange, CategoryBilling, "subscription", "Plan changed", "The organization's plan changed.", SeverityCritical)),
		meta(ActionPaymentFailed, CategoryBilling, "subscription", "Payment failed", "A billing payment failed.", SeverityWarning),

		sensitive(meta(ActionSettingsUpdate, CategorySettings, "setting", "Settings updated", "Platform or organization settings were changed.", SeverityWarning)),
		sensitive(meta(ActionSettingsSMTPUpdate, CategorySettings, "setting", "SMTP settings updated", "Outbound email settings were changed.", SeverityCritical)),
		sensitive(meta(ActionSettingsStorageUpdate, CategorySettings, "setting", "Storage settings updated", "The blob storage backend was changed.", SeverityCritical)),
		sensitive(meta(ActionSettingsSSOUpdate, CategorySettings, "setting", "SSO settings updated", "An organization's single sign-on (OIDC/SAML) configuration was created or changed.", SeverityCritical)),
		sensitive(meta(ActionSettingsSSODelete, CategorySettings, "setting", "SSO settings deleted", "An organization's single sign-on configuration was removed.", SeverityCritical)),
		sensitive(meta(ActionSettingsManagedAI, CategorySettings, "setting", "Managed AI updated", "LiveReview-managed AI configuration was changed.", SeverityCritical)),
		sensitive(meta(ActionSettingsAutoApprove, CategorySettings, "setting", "Auto-approve updated", "The auto-approve policy was changed.", SeverityWarning)),
		sensitive(meta(ActionSettingsRiskModel, CategorySettings, "setting", "Risk model updated", "Risk model configuration was changed.", SeverityWarning)),
		sensitive(meta(ActionSettingsRetention, CategorySettings, "setting", "Audit retention updated", "The audit retention policy was changed.", SeverityCritical)),
		meta(ActionSettingsPrompts, CategorySettings, "prompt", "Prompts updated", "Review prompts were changed.", SeverityWarning),

		meta(ActionConnectorCreate, CategoryConnector, "connector", "Integration connected", "A source-control integration was connected.", SeverityWarning),
		meta(ActionConnectorUpdate, CategoryConnector, "connector", "Integration updated", "A source-control integration was updated.", SeverityWarning),
		sensitive(meta(ActionConnectorDelete, CategoryConnector, "connector", "Integration removed", "A source-control integration was removed.", SeverityWarning)),
		meta(ActionConnectorConnect, CategoryConnector, "connector", "Integration reconnected", "A source-control integration was reconnected.", SeverityInfo),
		meta(ActionGitHubAppConnectStart, CategoryConnector, "connector", "GitHub App connection started", "A member started connecting a GitHub account or organization through the GitHub App.", SeverityInfo),
		meta(ActionWebhookInstall, CategoryConnector, "webhook", "Webhook installed", "A repository webhook was installed.", SeverityInfo),
		meta(ActionWebhookUninstall, CategoryConnector, "webhook", "Webhook removed", "A repository webhook was removed.", SeverityWarning),

		meta(ActionRepositoryCreate, CategoryRepository, "repository", "Repository added", "A repository was added.", SeverityInfo),
		meta(ActionRepositoryUpdate, CategoryRepository, "repository", "Repository updated", "A repository's settings were changed.", SeverityInfo),
		sensitive(meta(ActionRepositoryDelete, CategoryRepository, "repository", "Repository removed", "A repository was removed.", SeverityWarning)),
		meta(ActionRepositorySync, CategoryRepository, "repository", "Repository synced", "A repository sync ran.", SeverityInfo),
		sensitive(meta(ActionRulesetCreate, CategoryRepository, "ruleset", "Ruleset created", "A CI ruleset was created.", SeverityWarning)),
		sensitive(meta(ActionRulesetUpdate, CategoryRepository, "ruleset", "Ruleset updated", "A CI ruleset was changed.", SeverityWarning)),
		sensitive(meta(ActionRulesetDelete, CategoryRepository, "ruleset", "Ruleset deleted", "A CI ruleset was deleted.", SeverityWarning)),

		meta(ActionReviewCreate, CategoryReview, "review", "Review created", "A review was created.", SeverityInfo),
		meta(ActionReviewTrigger, CategoryReview, "review", "Review triggered", "A review was started.", SeverityInfo),
		meta(ActionReviewCancel, CategoryReview, "review", "Review cancelled", "A running review was cancelled.", SeverityWarning),
		sensitive(meta(ActionReviewDelete, CategoryReview, "review", "Review deleted", "A review was deleted.", SeverityWarning)),
		meta(ActionReviewRetry, CategoryReview, "review", "Review retried", "A review was retried.", SeverityInfo),
		meta(ActionReviewOutcomeReport, CategoryReview, "review", "Review outcome reported", "A production outcome was reported for a review.", SeverityInfo),
		meta(ActionReviewCommentFail, CategoryReview, "review", "Comment posting failed", "LiveReview could not post findings to the provider.", SeverityWarning),

		meta(ActionChatConversationCreate, CategoryChat, "conversation", "Chat started", "A Livi conversation was created.", SeverityInfo),
		meta(ActionChatConversationDelete, CategoryChat, "conversation", "Chat deleted", "A Livi conversation was deleted.", SeverityInfo),
		meta(ActionChatFileUpload, CategoryChat, "file", "Chat file uploaded", "A file was attached to a chat.", SeverityInfo),
		readSensitive(meta(ActionChatFileDownload, CategoryChat, "file", "Chat file downloaded", "A chat attachment was downloaded.", SeverityInfo)),

		readSensitive(meta(ActionExportFindings, CategoryExport, "report", "Findings exported", "Review findings were exported.", SeverityWarning)),
		readSensitive(meta(ActionExportReport, CategoryExport, "report", "Report exported", "An analytics or onboarding report was exported.", SeverityWarning)),
		readSensitive(meta(ActionExportAudit, CategoryExport, "audit", "Audit log exported", "Audit events were exported.", SeverityWarning)),
		readSensitive(meta(ActionExportChat, CategoryExport, "chat", "Chat exported", "A chat conversation was exported.", SeverityInfo)),
		readSensitive(meta(ActionExportPackage, CategoryExport, "audit", "Evidence package exported", "An audit evidence package was generated.", SeverityWarning)),

		readSensitive(meta(ActionAuditRead, CategoryAudit, "audit", "Audit log viewed", "The audit log was viewed.", SeverityInfo)),
		readSensitive(meta(ActionAuditExport, CategoryAudit, "audit", "Audit log export requested", "Audit data was exported through the API.", SeverityWarning)),
		readSensitive(meta(ActionAuditArchiveDownload, CategoryAudit, "archive", "Archive downloaded", "A monthly audit archive was downloaded.", SeverityWarning)),
		meta(ActionAuditIntegrityCheck, CategoryAudit, "audit", "Integrity checked", "The audit hash chain was verified.", SeverityInfo),

		meta(ActionAIInvocation, CategoryAI, "model", "Model invoked", "An AI model was called.", SeverityInfo),
		meta(ActionAIConnectorCreate, CategoryAI, "connector", "AI provider connected", "An AI provider was connected.", SeverityWarning),
		meta(ActionAIConnectorUpdate, CategoryAI, "connector", "AI provider updated", "An AI provider configuration changed.", SeverityWarning),
		sensitive(meta(ActionAIConnectorDelete, CategoryAI, "connector", "AI provider removed", "An AI provider was removed.", SeverityWarning)),
		sensitive(meta(ActionAIModelActivate, CategoryAI, "model", "Managed model activated", "A LiveReview-managed model was activated.", SeverityCritical)),

		meta(ActionAdminUserList, CategoryAdmin, "user", "All users listed", "A super admin listed users across organizations.", SeverityInfo),
		sensitive(meta(ActionAdminUserCreate, CategoryAdmin, "user", "User created by admin", "A super admin created a user in an organization.", SeverityCritical)),
		sensitive(meta(ActionAdminUserTransfer, CategoryAdmin, "user", "User transferred by admin", "A super admin moved a user between organizations.", SeverityCritical)),
		sensitive(meta(ActionAdminOrgDeactivate, CategoryAdmin, "org", "Organization deactivated by admin", "A super admin deactivated an organization.", SeverityCritical)),

		meta(ActionSystemAuditPruned, CategorySystem, "audit", "Audit events pruned", "Expired audit events were removed.", SeverityInfo),
		meta(ActionSystemAuditArchived, CategorySystem, "audit", "Audit events archived", "A month of audit events was archived.", SeverityInfo),
		meta(ActionSystemAuditAnchor, CategorySystem, "audit", "Audit anchor written", "The daily audit hash anchor was written.", SeverityInfo),
		meta(ActionSystemAuditMaintenance, CategorySystem, "audit", "Audit maintenance run", "An operator triggered the retention/archive cycle.", SeverityInfo),
		meta(ActionSystemAuditWriteFail, CategorySystem, "audit", "Audit write failed", "An audit event could not be written.", SeverityCritical),
		meta(ActionSystemPartition, CategorySystem, "audit", "Audit partition created", "A monthly audit partition was created.", SeverityInfo),
		meta(ActionSystemMaintenanceErr, CategorySystem, "audit", "Audit maintenance error", "The audit maintenance job failed.", SeverityCritical),

		meta(ActionLearningCreate, CategoryReview, "learning", "Learning created", "A review learning was created.", SeverityInfo),
		meta(ActionLearningUpdate, CategoryReview, "learning", "Learning updated", "A review learning was updated.", SeverityInfo),
		meta(ActionLearningDelete, CategoryReview, "learning", "Learning deleted", "A review learning was deleted.", SeverityWarning),

		meta(ActionPromptCreate, CategorySettings, "prompt", "Prompt created", "A prompt was created.", SeverityWarning),
		meta(ActionPromptUpdate, CategorySettings, "prompt", "Prompt updated", "A prompt was changed.", SeverityWarning),
		meta(ActionPromptDelete, CategorySettings, "prompt", "Prompt deleted", "A prompt was deleted.", SeverityWarning),

		meta(ActionAutoApproveCreate, CategorySettings, "policy", "Auto-approve policy created", "An auto-approve policy was created.", SeverityWarning),
		meta(ActionAutoApproveUpdate, CategorySettings, "policy", "Auto-approve policy updated", "An auto-approve policy was changed.", SeverityWarning),
		meta(ActionAutoApproveDelete, CategorySettings, "policy", "Auto-approve policy deleted", "An auto-approve policy was deleted.", SeverityWarning),
	}
	out := make(map[Action]ActionMeta, len(items))
	for _, item := range items {
		out[item.Action] = item
	}
	return out
}()

// LookupAction returns the catalog entry for an action.
func LookupAction(a Action) (ActionMeta, bool) {
	m, ok := actionCatalog[a]
	return m, ok
}

// AllActions returns every catalog entry, sorted by action name. Tests use this
// to keep docs/audit-events.md complete.
func AllActions() []ActionMeta {
	out := make([]ActionMeta, 0, len(actionCatalog))
	for _, m := range actionCatalog {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action < out[j].Action })
	return out
}

// CategoryLabel returns a human label for a category.
func CategoryLabel(c Category) string {
	if label, ok := categoryLabels[c]; ok {
		return label
	}
	return string(c)
}

// KnownCategory reports whether c is a registered category.
func KnownCategory(c Category) bool {
	_, ok := categoryLabels[c]
	return ok
}

// AllCategories returns categories in a stable display order.
func AllCategories() []Category {
	order := []Category{
		CategoryAuth, CategoryUser, CategoryOrg, CategoryAPIKey, CategoryBilling,
		CategorySettings, CategoryConnector, CategoryRepository, CategoryReview,
		CategoryChat, CategoryExport, CategoryAudit, CategoryAI, CategoryAdmin,
		CategorySystem, CategoryOther,
	}
	return order
}
