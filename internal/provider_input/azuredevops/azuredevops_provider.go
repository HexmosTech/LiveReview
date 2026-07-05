package azuredevops

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/livereview/internal/capture"
	coreprocessor "github.com/livereview/internal/core_processor"
	azuredevopsutils "github.com/livereview/internal/providers/azuredevops"
)

// extractThreadIDFromMetadata reads the numeric thread_id stashed on
// UnifiedCommentV2.Metadata by ConvertAzureDevOpsCommentEvent.
func extractThreadIDFromMetadata(metadata map[string]any) (int, bool) {
	if metadata == nil {
		return 0, false
	}
	switch v := metadata["thread_id"].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

// buildMRID reconstructs the "org/project/repo/id" composite id used by the
// Azure DevOps provider package, from the connector's org URL and the
// "{project}/{repo}" repository full name.
func buildMRID(orgURL, repoFullName string, prNumber int) (string, error) {
	org, err := azuredevopsutils.OrgNameFromURL(orgURL)
	if err != nil {
		return "", fmt.Errorf("failed to derive org name from url %q: %w", orgURL, err)
	}
	parts := strings.SplitN(repoFullName, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid Azure DevOps repository full name: %s", repoFullName)
	}
	return fmt.Sprintf("%s/%s/%s/%d", org, parts[0], parts[1], prNumber), nil
}

type (
	UnifiedTimelineV2      = coreprocessor.UnifiedTimelineV2
	UnifiedReviewCommentV2 = coreprocessor.UnifiedReviewCommentV2
)

// SharedSecretHeader is the custom HTTP header Phase 3 configures on the
// Service Hooks subscription (consumerInputs.httpHeaders) to carry a static
// shared secret, since Azure DevOps has no HMAC payload-signing scheme.
const SharedSecretHeader = "X-LiveReview-Secret"

// AzureDevOpsOutputClient captures the outbound capabilities required by the provider.
type AzureDevOpsOutputClient interface {
	PostCommentReply(event *UnifiedWebhookEventV2, token, content string) error
	PostEmojiReaction(event *UnifiedWebhookEventV2, token, emoji string) error
	PostReviewComments(mr UnifiedMergeRequestV2, token string, comments []UnifiedReviewCommentV2) error
}

// AzureDevOpsV2Provider implements api.WebhookProviderV2 (plus the optional
// bot-info/timeline/signature interfaces) for Azure DevOps.
type AzureDevOpsV2Provider struct {
	db     *sql.DB
	output AzureDevOpsOutputClient
}

// NewAzureDevOpsV2Provider creates an Azure DevOps provider with the required dependencies.
func NewAzureDevOpsV2Provider(db *sql.DB, output AzureDevOpsOutputClient) *AzureDevOpsV2Provider {
	if output == nil {
		panic("azuredevops output client is required")
	}
	return &AzureDevOpsV2Provider{db: db, output: output}
}

// ProviderName returns the provider name.
func (p *AzureDevOpsV2Provider) ProviderName() string {
	return "azuredevops"
}

// azureEnvelope is the minimal shape needed to detect and dispatch Azure DevOps webhooks.
type azureEnvelope struct {
	EventType          string                            `json:"eventType"`
	PublisherID        string                            `json:"publisherId"`
	ResourceContainers *coreAzureResourceContainersMarker `json:"resourceContainers"`
}

// coreAzureResourceContainersMarker only needs to exist (non-nil) to confirm the
// resourceContainers key is present; its shape isn't otherwise used for detection.
type coreAzureResourceContainersMarker struct{}

// CommentEventType is the Azure DevOps Service Hooks event id for "pull
// request commented on" (confirmed against https://learn.microsoft.com/azure/devops/service-hooks/events
// and a live subscription creation call - note this does NOT follow the
// git.pullrequest.* naming convention used by the created/updated events).
const CommentEventType = "ms.vss-code.git-pullrequest-comment-event"

// CanHandleWebhook detects Azure DevOps Service Hooks payloads. Unlike other
// providers, Azure DevOps sends no distinctive headers by default, so
// detection is entirely body-based: publisherId=="tfs", a recognized
// git.pullrequest.* or CommentEventType eventType, and a resourceContainers
// key that no other provider's payload has.
func (p *AzureDevOpsV2Provider) CanHandleWebhook(headers map[string]string, body []byte) bool {
	var env azureEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return false
	}
	if env.PublisherID != "tfs" {
		return false
	}
	if env.ResourceContainers == nil {
		return false
	}
	if env.EventType == CommentEventType {
		return true
	}
	return len(env.EventType) >= len("git.pullrequest") && env.EventType[:len("git.pullrequest")] == "git.pullrequest"
}

// ConvertCommentEvent converts an Azure DevOps webhook to unified format.
func (p *AzureDevOpsV2Provider) ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	var env azureEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps webhook envelope: %w", err)
	}

	var (
		event *UnifiedWebhookEventV2
		err   error
	)

	switch env.EventType {
	case CommentEventType:
		event, err = ConvertAzureDevOpsCommentEvent(body)
	case "git.pullrequest.created", "git.pullrequest.updated":
		event, err = ConvertAzureDevOpsPullRequestEvent(body)
	default:
		err = fmt.Errorf("unsupported Azure DevOps event type: %q", env.EventType)
	}

	if capture.Enabled() {
		recordAzureDevOpsWebhook(env.EventType, headers, body, event, err)
	}

	if err != nil {
		// The orchestrator's convertToUnifiedEvent silently discards this error
		// and reports a generic "unable to convert" message, so log it here -
		// otherwise a payload-shape mismatch is undiagnosable from server logs.
		preview := string(body)
		if len(preview) > 4000 {
			preview = preview[:4000] + "...(truncated)"
		}
		log.Printf("[ERROR] AzureDevOpsV2Provider.ConvertCommentEvent failed for eventType=%q: %v", env.EventType, err)
		log.Printf("[DEBUG] AzureDevOpsV2Provider raw webhook body: %s", preview)
		return nil, err
	}
	return event, nil
}

// ConvertReviewerEvent is not applicable to Azure DevOps: reviewer changes are
// not among the subscribed Service Hooks event types for this integration.
func (p *AzureDevOpsV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	return nil, fmt.Errorf("reviewer events not implemented for Azure DevOps")
}

// FetchMergeRequestData enriches the event with connector context and, for
// comment events, resolves the inline file/line position from the thread.
func (p *AzureDevOpsV2Provider) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event")
	}

	var (
		token  *IntegrationToken
		orgURL string
		err    error
	)

	if event.Repository.FullName == "" {
		// Comment events carry only a repository GUID (no names, see
		// ConvertAzureDevOpsCommentEvent) - resolve the connector by
		// organization URL, then resolve the repo/project names via API.
		rawOrgURL, _ := event.MergeRequest.Metadata["org_url"].(string)
		if rawOrgURL == "" {
			return fmt.Errorf("missing org_url for Azure DevOps comment event")
		}
		token, err = FindIntegrationTokenForAzureDevOpsOrg(p.db, rawOrgURL)
		if err != nil {
			return fmt.Errorf("failed to get Azure DevOps token: %w", err)
		}
		orgURL = token.ProviderURL

		repoID, _ := event.MergeRequest.Metadata["repo_id"].(string)
		if repoID == "" {
			return fmt.Errorf("missing repo_id for Azure DevOps comment event")
		}

		provider, perr := azuredevopsutils.NewProvider(azuredevopsutils.Config{BaseURL: orgURL, Token: token.PatToken})
		if perr != nil {
			return fmt.Errorf("failed to construct azure devops provider: %w", perr)
		}
		projectName, repoName, rerr := provider.ResolveRepositoryByID(context.Background(), repoID)
		if rerr != nil {
			return fmt.Errorf("failed to resolve azure devops repository %s: %w", repoID, rerr)
		}
		event.Repository.FullName = fmt.Sprintf("%s/%s", projectName, repoName)
		event.Repository.Name = repoName
		if event.Repository.Metadata == nil {
			event.Repository.Metadata = map[string]any{}
		}
		event.Repository.Metadata["project_name"] = projectName
	} else {
		token, orgURL, err = FindIntegrationTokenForAzureDevOpsRepo(p.db, event.Repository.FullName)
		if err != nil {
			return fmt.Errorf("failed to get Azure DevOps token: %w", err)
		}
	}

	if event.MergeRequest.Metadata == nil {
		event.MergeRequest.Metadata = map[string]any{}
	}
	event.MergeRequest.Metadata["repository_full_name"] = event.Repository.FullName
	event.MergeRequest.Metadata["base_url"] = orgURL
	event.MergeRequest.Metadata["connector_id"] = token.ID

	if event.Comment == nil || event.Comment.Metadata == nil {
		return nil
	}

	threadID, ok := extractThreadIDFromMetadata(event.Comment.Metadata)
	if !ok {
		return nil
	}

	mrID, err := buildMRID(orgURL, event.Repository.FullName, event.MergeRequest.Number)
	if err != nil {
		log.Printf("[WARN] Azure DevOps: failed to build mrID for thread lookup: %v", err)
		return nil
	}

	provider, err := azuredevopsutils.NewProvider(azuredevopsutils.Config{BaseURL: orgURL, Token: token.PatToken})
	if err != nil {
		log.Printf("[WARN] Azure DevOps: failed to construct provider for thread lookup: %v", err)
		return nil
	}

	thread, err := provider.GetThread(context.Background(), mrID, threadID)
	if err != nil {
		log.Printf("[WARN] Azure DevOps: failed to fetch thread %d for position enrichment: %v", threadID, err)
		return nil
	}
	if thread.ThreadContext == nil || thread.ThreadContext.FilePath == "" {
		return nil
	}

	tc := thread.ThreadContext
	if tc.LeftFileStart != nil && tc.RightFileStart == nil {
		event.Comment.Position = &UnifiedPositionV2{
			FilePath:   tc.FilePath,
			LineNumber: tc.LeftFileStart.Line,
			LineType:   "old",
		}
	} else if tc.RightFileStart != nil {
		event.Comment.Position = &UnifiedPositionV2{
			FilePath:   tc.FilePath,
			LineNumber: tc.RightFileStart.Line,
			LineType:   "new",
		}
	}

	return nil
}

// FindIntegrationTokenForRepo returns the integration token associated with the given repository.
func (p *AzureDevOpsV2Provider) FindIntegrationTokenForRepo(repoFullName string) (*IntegrationToken, error) {
	token, _, err := FindIntegrationTokenForAzureDevOpsRepo(p.db, repoFullName)
	return token, err
}

// GetBotUserInfo fetches the authenticated identity (bot/service account) for a repository's connector.
func (p *AzureDevOpsV2Provider) GetBotUserInfo(repository UnifiedRepositoryV2) (*UnifiedBotUserInfoV2, error) {
	// The bot identity is org-level (it's just the PAT owner's profile), so
	// for comment events - which arrive with no repository name, only a
	// repo_id/org_url pair (see ConvertAzureDevOpsCommentEvent) - resolve the
	// connector by org URL directly rather than requiring a repository name.
	var (
		token *IntegrationToken
		err   error
	)
	if repository.FullName != "" {
		token, _, err = FindIntegrationTokenForAzureDevOpsRepo(p.db, repository.FullName)
	} else if orgURL, ok := repository.Metadata["org_url"].(string); ok && orgURL != "" {
		token, err = FindIntegrationTokenForAzureDevOpsOrg(p.db, orgURL)
	} else {
		return nil, fmt.Errorf("missing repository full name and org_url")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure DevOps token: %w", err)
	}

	provider, err := azuredevopsutils.NewProvider(azuredevopsutils.Config{BaseURL: token.ProviderURL, Token: token.PatToken})
	if err != nil {
		return nil, fmt.Errorf("failed to construct azure devops provider: %w", err)
	}

	profile, err := provider.GetBotIdentity(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Azure DevOps identity: %w", err)
	}

	return &UnifiedBotUserInfoV2{
		UserID:   profile.ID,
		Username: profile.DisplayName,
		Name:     profile.DisplayName,
		IsBot:    false, // Azure DevOps PATs represent a regular/service identity, no bot flag
		Metadata: map[string]any{
			"base_url": token.ProviderURL,
			"email":    profile.EmailAddress,
		},
	}, nil
}

// PostCommentReply posts a reply within the thread that triggered the event.
func (p *AzureDevOpsV2Provider) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	if event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	token, _, err := FindIntegrationTokenForAzureDevOpsRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Azure DevOps token: %w", err)
	}

	return p.output.PostCommentReply(event, token.PatToken, content)
}

// PostEmojiReaction is a no-op for Azure DevOps (see AzureDevOpsOutputClient.PostEmojiReaction).
func (p *AzureDevOpsV2Provider) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	if event.Comment == nil {
		return fmt.Errorf("no comment in event for emoji reaction")
	}

	token, _, err := FindIntegrationTokenForAzureDevOpsRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Azure DevOps token: %w", err)
	}

	return p.output.PostEmojiReaction(event, token.PatToken, emoji)
}

// PostFullReview posts a comprehensive review comment to an Azure DevOps PR.
func (p *AzureDevOpsV2Provider) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event for full review")
	}

	token, _, err := FindIntegrationTokenForAzureDevOpsRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Azure DevOps token: %w", err)
	}

	if overallComment != "" {
		if err := p.output.PostCommentReply(event, token.PatToken, overallComment); err != nil {
			return fmt.Errorf("failed to post overall review comment: %w", err)
		}
	}

	return nil
}

// FetchMRTimeline is not implemented for Azure DevOps yet: contextual replies
// fall back to whatever timeline the caller already has (may be empty).
func (p *AzureDevOpsV2Provider) FetchMRTimeline(mr UnifiedMergeRequestV2) (*UnifiedTimelineV2, error) {
	return &UnifiedTimelineV2{Items: []coreprocessor.UnifiedTimelineItemV2{}}, nil
}

// ValidateWebhookSignature validates the shared-secret header configured on the
// Service Hooks subscription (Azure DevOps has no HMAC payload-signing scheme).
//
// The DB secret is looked up FIRST, before inspecting the request. Checking
// the incoming header first would let an attacker bypass validation simply by
// omitting the header - the "no secret configured" fallback below must only
// apply when the connector genuinely has no secret on record, not whenever a
// request happens to leave the header out.
func (p *AzureDevOpsV2Provider) ValidateWebhookSignature(connectorID int64, headers map[string]string, body []byte) bool {
	secret, err := FindWebhookSecretByConnectorID(p.db, int(connectorID))
	if err != nil {
		log.Printf("[ERROR] Failed to lookup webhook secret for connector_id=%d: %v", connectorID, err)
		return false
	}
	if secret == "" {
		log.Printf("[WARN] No webhook secret configured for connector_id=%d, accepting webhook", connectorID)
		return true
	}

	provided := headers[SharedSecretHeader]
	if provided == "" {
		log.Printf("[ERROR] Azure DevOps webhook missing %s header for connector_id=%d (secret is configured)", SharedSecretHeader, connectorID)
		return false
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
		log.Printf("[ERROR] Invalid Azure DevOps webhook secret for connector_id=%d", connectorID)
		return false
	}
	return true
}

func recordAzureDevOpsWebhook(eventType string, headers map[string]string, body []byte, unified *UnifiedWebhookEventV2, err error) {
	if eventType == "" {
		eventType = "unknown"
	}
	if len(body) > 0 {
		capture.WriteBlob(fmt.Sprintf("azuredevops-webhook-%s-body", eventType), "json", body)
	}
	meta := map[string]any{
		"event_type": eventType,
		"headers":    headers,
		"recorded_at": time.Now().Format(time.RFC3339),
	}
	if err != nil {
		meta["error"] = err.Error()
	}
	capture.WriteJSON(fmt.Sprintf("azuredevops-webhook-%s-meta", eventType), meta)
	if unified != nil && err == nil {
		capture.WriteJSON(fmt.Sprintf("azuredevops-webhook-%s-unified", eventType), unified)
	}
}
