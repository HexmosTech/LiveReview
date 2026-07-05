package jobqueue

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	storagejobqueue "github.com/livereview/storage/jobqueue"
)

// Azure DevOps webhook (Service Hooks) installation methods.
//
// Azure DevOps has no single "webhook" resource: each event type requires its
// own subscription object under the Service Hooks REST API, and
// publisherInputs.projectId/repository must be GUIDs rather than names, so a
// repo-lookup call is needed first to resolve them (analogous to GitLab's
// getProjectID). There is no HMAC signature scheme, so a static shared-secret
// header carries authentication instead (validated by
// AzureDevOpsV2Provider.ValidateWebhookSignature).

// azureDevOpsCommentEventType is the Service Hooks event id for "pull request
// commented on". It deliberately does not follow the git.pullrequest.*
// naming convention used by the created/updated events - confirmed against
// https://learn.microsoft.com/azure/devops/service-hooks/events. Keep in
// sync with provider_input/azuredevops.CommentEventType.
const azureDevOpsCommentEventType = "ms.vss-code.git-pullrequest-comment-event"

// azureDevOpsSubscriptionEventTypes are the Service Hooks events subscribed
// per repository to drive the webhook-based interactive review flow.
var azureDevOpsSubscriptionEventTypes = []string{
	"git.pullrequest.created",
	"git.pullrequest.updated",
	azureDevOpsCommentEventType,
}

// azureRepoInfo carries the GUIDs Service Hooks subscriptions require.
type azureRepoInfo struct {
	RepositoryID string `json:"id"`
	Project      struct {
		ID string `json:"id"`
	} `json:"project"`
}

// azureSubscription mirrors the subset of a Service Hooks subscription needed
// for idempotency checks and registry bookkeeping.
type azureSubscription struct {
	ID              string         `json:"id"`
	EventType       string         `json:"eventType"`
	PublisherID     string         `json:"publisherId"`
	PublisherInputs map[string]any `json:"publisherInputs"`
	ConsumerInputs  map[string]any `json:"consumerInputs"`
}

// makeAzureDevOpsRequest makes an authenticated request against the given
// Azure DevOps organization API base URL (e.g. https://dev.azure.com/myorg).
func (w *WebhookInstallWorker) makeAzureDevOpsRequest(ctx context.Context, method, apiURL string, payload interface{}, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := w.httpClient.NewRequestWithContext(ctx, method, apiURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+pat)))
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	return resp, nil
}

// resolveAzureRepoIDs resolves a project/repo name pair to the GUIDs Azure
// DevOps Service Hooks subscriptions require in publisherInputs.
func (w *WebhookInstallWorker) resolveAzureRepoIDs(ctx context.Context, apiBase, project, repo, pat string) (projectID, repositoryID string, err error) {
	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s?api-version=7.1",
		apiBase, url.PathEscape(project), url.PathEscape(repo))

	resp, err := w.makeAzureDevOpsRequest(ctx, http.MethodGet, apiURL, nil, pat)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("azure devops repository fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var info azureRepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", fmt.Errorf("failed to decode repository response: %w", err)
	}
	return info.Project.ID, info.RepositoryID, nil
}

// listAzureDevOpsSubscriptions lists all Service Hooks subscriptions published by "tfs" (Azure Repos).
func (w *WebhookInstallWorker) listAzureDevOpsSubscriptions(ctx context.Context, apiBase, pat string) ([]azureSubscription, error) {
	apiURL := fmt.Sprintf("%s/_apis/hooks/subscriptions?publisherId=tfs&api-version=7.1", apiBase)

	resp, err := w.makeAzureDevOpsRequest(ctx, http.MethodGet, apiURL, nil, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops subscriptions list failed (status %d): %s", resp.StatusCode, string(body))
	}

	var out struct {
		Value []azureSubscription `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode subscriptions response: %w", err)
	}
	return out.Value, nil
}

// azureSubscriptionMatches reports whether an existing subscription already
// covers eventType for the given repository and webhook URL, so installation
// stays idempotent across repeated "Enable Manual Trigger" clicks.
func azureSubscriptionMatches(sub azureSubscription, eventType, repositoryID, webhookURL string) bool {
	if sub.EventType != eventType || sub.PublisherID != "tfs" {
		return false
	}
	if repo, _ := sub.PublisherInputs["repository"].(string); repo != repositoryID {
		return false
	}
	consumerURL, _ := sub.ConsumerInputs["url"].(string)
	return consumerURL == webhookURL
}

// installAzureDevOpsSubscriptions creates the 3 Service Hooks subscriptions
// (one per event type - Azure DevOps has no multi-event subscription) needed
// to drive the webhook-based interactive flow for one repository. Existing
// matching subscriptions are left untouched (idempotent).
func (w *WebhookInstallWorker) installAzureDevOpsSubscriptions(ctx context.Context, apiBase, projectID, repositoryID, pat string, connectorID int) ([]string, error) {
	currentEndpoint, err := w.store.GetWebhookPublicEndpoint(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current webhook endpoint: %w", err)
	}
	if currentEndpoint == "" {
		return nil, fmt.Errorf("webhook endpoint not configured: please set livereview_prod_url in settings before installing webhooks")
	}
	webhookURL := w.getWebhookEndpointForProviderWithCustomEndpoint("azuredevops", currentEndpoint, connectorID)

	existing, err := w.listAzureDevOpsSubscriptions(ctx, apiBase, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing subscriptions: %w", err)
	}

	secretHeader := fmt.Sprintf("X-LiveReview-Secret: %s", w.config.WebhookConfig.Secret)

	var subscriptionIDs []string
	for _, eventType := range azureDevOpsSubscriptionEventTypes {
		var matched *azureSubscription
		for i := range existing {
			if azureSubscriptionMatches(existing[i], eventType, repositoryID, webhookURL) {
				matched = &existing[i]
				break
			}
		}
		if matched != nil {
			subscriptionIDs = append(subscriptionIDs, matched.ID)
			continue
		}

		payload := map[string]interface{}{
			"publisherId":      "tfs",
			"eventType":        eventType,
			"resourceVersion":  "1.0",
			"consumerId":       "webHooks",
			"consumerActionId": "httpRequest",
			"publisherInputs": map[string]interface{}{
				"projectId":  projectID,
				"repository": repositoryID,
			},
			"consumerInputs": map[string]interface{}{
				"url":         webhookURL,
				"httpHeaders": secretHeader,
			},
		}

		apiURL := fmt.Sprintf("%s/_apis/hooks/subscriptions?api-version=7.1", apiBase)
		resp, err := w.makeAzureDevOpsRequest(ctx, http.MethodPost, apiURL, payload, pat)
		if err != nil {
			return subscriptionIDs, fmt.Errorf("failed to create subscription for %s: %w", eventType, err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return subscriptionIDs, fmt.Errorf("azure devops subscription creation failed for %s (status %d): %s",
				eventType, resp.StatusCode, string(respBody))
		}

		var created azureSubscription
		if err := json.Unmarshal(respBody, &created); err != nil {
			return subscriptionIDs, fmt.Errorf("failed to decode subscription response for %s: %w", eventType, err)
		}
		subscriptionIDs = append(subscriptionIDs, created.ID)
		log.Printf("Created Azure DevOps subscription %s for event %s (repo=%s)", created.ID, eventType, repositoryID)
	}

	return subscriptionIDs, nil
}

// handleAzureDevOpsWebhookInstall handles Azure DevOps Service Hooks installation for one repository.
func (w *WebhookInstallWorker) handleAzureDevOpsWebhookInstall(ctx context.Context, args WebhookInstallJobArgs) error {
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid Azure DevOps repository format: %s (expected: project/repo)", args.ProjectPath)
	}
	project, repo := parts[0], parts[1]
	apiBase := strings.TrimSuffix(args.BaseURL, "/")

	log.Printf("Installing Azure DevOps subscriptions for repository: %s/%s", project, repo)

	projectID, repositoryID, err := w.resolveAzureRepoIDs(ctx, apiBase, project, repo, args.PAT)
	if err != nil {
		return fmt.Errorf("failed to resolve repository ids: %w", err)
	}

	subscriptionIDs, err := w.installAzureDevOpsSubscriptions(ctx, apiBase, projectID, repositoryID, args.PAT, args.ConnectorID)
	if err != nil {
		return fmt.Errorf("failed to install subscriptions: %w", err)
	}

	log.Printf("Successfully installed %d Azure DevOps subscriptions for repository %s/%s", len(subscriptionIDs), project, repo)

	if err := w.updateWebhookRegistryAzureDevOps(ctx, args, subscriptionIDs); err != nil {
		log.Printf("Failed to update webhook registry for Azure DevOps repository %s/%s: %v", project, repo, err)
		// Do not fail the job if registry update fails after subscriptions were created
	}

	log.Printf("Azure DevOps webhook installation completed for repository: %s/%s", project, repo)
	return nil
}

// updateWebhookRegistryAzureDevOps creates or updates the webhook registry
// entry for an Azure DevOps repository. All 3 subscription ids are stored,
// comma-joined, in WebhookID for observability.
func (w *WebhookInstallWorker) updateWebhookRegistryAzureDevOps(ctx context.Context, args WebhookInstallJobArgs, subscriptionIDs []string) error {
	existingID, err := w.store.GetWebhookRegistryID(ctx, args.ConnectorID, args.ProjectPath)
	now := time.Now()

	projectName := args.ProjectPath
	if slash := strings.LastIndex(args.ProjectPath, "/"); slash != -1 {
		projectName = args.ProjectPath[slash+1:]
	}

	webhookID := strings.Join(subscriptionIDs, ",")
	webhookName := "LiveReview Service Hooks"
	events := strings.Join(azureDevOpsSubscriptionEventTypes, ",")
	status := "automatic"

	if errors.Is(err, storagejobqueue.ErrWebhookRegistryNotFound) {
		err = w.store.InsertWebhookRegistry(ctx, storagejobqueue.WebhookRegistryRecord{
			Provider:           args.Provider,
			ProviderProjectID:  args.ProjectPath,
			ProjectName:        projectName,
			ProjectFullName:    args.ProjectPath,
			WebhookID:          webhookID,
			WebhookURL:         w.getWebhookEndpointForProvider("azuredevops"),
			WebhookSecret:      w.config.WebhookConfig.Secret,
			WebhookName:        webhookName,
			Events:             events,
			Status:             status,
			LastVerifiedAt:     now,
			CreatedAt:          now,
			UpdatedAt:          now,
			IntegrationTokenID: args.ConnectorID,
		})
		if err != nil {
			return fmt.Errorf("failed to insert Azure DevOps webhook registry: %w", err)
		}

		log.Printf("Created webhook_registry entry for Azure DevOps project %s with status '%s'", args.ProjectPath, status)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check existing webhook registry: %w", err)
	}

	err = w.store.UpdateWebhookRegistryByID(ctx, existingID, storagejobqueue.WebhookRegistryUpdate{
		WebhookID:      webhookID,
		WebhookURL:     w.getWebhookEndpointForProvider("azuredevops"),
		WebhookSecret:  w.config.WebhookConfig.Secret,
		WebhookName:    webhookName,
		Events:         events,
		Status:         status,
		LastVerifiedAt: now,
		UpdatedAt:      now,
	})
	if err != nil {
		return fmt.Errorf("failed to update Azure DevOps webhook registry: %w", err)
	}

	log.Printf("Updated webhook_registry entry for Azure DevOps project %s with status '%s'", args.ProjectPath, status)
	return nil
}

// Azure DevOps webhook (Service Hooks) removal methods.
// Reuses the azureSubscription/azureRepoInfo types and azureDevOpsSubscriptionEventTypes
// declared alongside WebhookInstallWorker's Azure DevOps methods above (same package).

// makeAzureDevOpsRequest makes an authenticated request against the given
// Azure DevOps organization API base URL (e.g. https://dev.azure.com/myorg).
func (w *WebhookRemovalWorker) makeAzureDevOpsRequest(ctx context.Context, method, apiURL string, payload interface{}, pat string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := w.httpClient.NewRequestWithContext(ctx, method, apiURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+pat)))
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	return resp, nil
}

// resolveAzureRepoIDs resolves a project/repo name pair to the GUIDs Azure
// DevOps Service Hooks subscriptions are keyed on.
func (w *WebhookRemovalWorker) resolveAzureRepoIDs(ctx context.Context, apiBase, project, repo, pat string) (projectID, repositoryID string, err error) {
	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s?api-version=7.1",
		apiBase, url.PathEscape(project), url.PathEscape(repo))

	resp, err := w.makeAzureDevOpsRequest(ctx, http.MethodGet, apiURL, nil, pat)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", "", fmt.Errorf("repository not found (already deleted or inaccessible)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("azure devops repository fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var info azureRepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", fmt.Errorf("failed to decode repository response: %w", err)
	}
	return info.Project.ID, info.RepositoryID, nil
}

// listAzureDevOpsSubscriptions lists all Service Hooks subscriptions published by "tfs" (Azure Repos).
func (w *WebhookRemovalWorker) listAzureDevOpsSubscriptions(ctx context.Context, apiBase, pat string) ([]azureSubscription, error) {
	apiURL := fmt.Sprintf("%s/_apis/hooks/subscriptions?publisherId=tfs&api-version=7.1", apiBase)

	resp, err := w.makeAzureDevOpsRequest(ctx, http.MethodGet, apiURL, nil, pat)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops subscriptions list failed (status %d): %s", resp.StatusCode, string(body))
	}

	var out struct {
		Value []azureSubscription `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode subscriptions response: %w", err)
	}
	return out.Value, nil
}

// handleAzureDevOpsWebhookRemoval removes all LiveReview Service Hooks
// subscriptions for one Azure DevOps repository.
func (w *WebhookRemovalWorker) handleAzureDevOpsWebhookRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	parts := strings.SplitN(args.ProjectPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid Azure DevOps repository format: %s (expected: project/repo)", args.ProjectPath)
	}
	project, repo := parts[0], parts[1]
	apiBase := strings.TrimSuffix(args.BaseURL, "/")

	log.Printf("Removing Azure DevOps subscriptions for repository: %s/%s", project, repo)

	if err := w.removeAzureDevOpsSubscriptions(ctx, apiBase, project, repo, args.PAT, args.ConnectorID); err != nil {
		log.Printf("Failed to remove Azure DevOps subscriptions for repository %s/%s: %v", project, repo, err)
		// continue to registry update even on API failure, mirroring Gitea's removal tolerance
	}

	if !args.SkipRegistryUpdate {
		if err := w.updateWebhookRegistryForAzureDevOpsRemoval(ctx, args); err != nil {
			return fmt.Errorf("failed to update webhook registry: %w", err)
		}
	}

	log.Printf("Azure DevOps webhook removal completed for repository: %s/%s", project, repo)
	return nil
}

// removeAzureDevOpsSubscriptions deletes every subscription pointing at this
// connector's webhook URL for the given repository.
func (w *WebhookRemovalWorker) removeAzureDevOpsSubscriptions(ctx context.Context, apiBase, project, repo, pat string, connectorID int) error {
	_, repositoryID, err := w.resolveAzureRepoIDs(ctx, apiBase, project, repo, pat)
	if err != nil {
		return fmt.Errorf("failed to resolve repository ids: %w", err)
	}

	webhookURL := w.getWebhookEndpointForProviderWithConnector("azuredevops", connectorID)

	subs, err := w.listAzureDevOpsSubscriptions(ctx, apiBase, pat)
	if err != nil {
		return fmt.Errorf("failed to list subscriptions: %w", err)
	}

	removed := 0
	for _, sub := range subs {
		subRepo, _ := sub.PublisherInputs["repository"].(string)
		consumerURL, _ := sub.ConsumerInputs["url"].(string)
		if subRepo != repositoryID || consumerURL != webhookURL {
			continue
		}

		deleteURL := fmt.Sprintf("%s/_apis/hooks/subscriptions/%s?api-version=7.1", apiBase, sub.ID)
		resp, err := w.makeAzureDevOpsRequest(ctx, http.MethodDelete, deleteURL, nil, pat)
		if err != nil {
			log.Printf("Failed to delete Azure DevOps subscription %s: %v", sub.ID, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
			removed++
			log.Printf("Removed Azure DevOps subscription %s (event=%s) for repository %s/%s", sub.ID, sub.EventType, project, repo)
		} else {
			log.Printf("Failed to delete Azure DevOps subscription %s (status %d)", sub.ID, resp.StatusCode)
		}
	}

	if removed == 0 {
		log.Printf("No LiveReview Azure DevOps subscriptions found for repository %s/%s", project, repo)
	}
	return nil
}

// updateWebhookRegistryForAzureDevOpsRemoval marks an Azure DevOps repository as unconnected in the webhook registry.
func (w *WebhookRemovalWorker) updateWebhookRegistryForAzureDevOpsRemoval(ctx context.Context, args WebhookRemovalJobArgs) error {
	existingID, err := w.store.GetWebhookRegistryID(ctx, args.ConnectorID, args.ProjectPath)
	now := time.Now()

	projectName := args.ProjectPath
	if slash := strings.LastIndex(args.ProjectPath, "/"); slash >= 0 {
		projectName = args.ProjectPath[slash+1:]
	}

	status := "unconnected"
	webhookID := ""
	webhookURL := w.getWebhookEndpointForProvider("azuredevops")
	webhookName := "LiveReview Service Hooks"
	events := strings.Join(azureDevOpsSubscriptionEventTypes, ",")

	if errors.Is(err, storagejobqueue.ErrWebhookRegistryNotFound) {
		err = w.store.InsertWebhookRegistry(ctx, storagejobqueue.WebhookRegistryRecord{
			Provider:           args.Provider,
			ProviderProjectID:  args.ProjectPath,
			ProjectName:        projectName,
			ProjectFullName:    args.ProjectPath,
			WebhookID:          webhookID,
			WebhookURL:         webhookURL,
			WebhookSecret:      w.config.WebhookConfig.Secret,
			WebhookName:        webhookName,
			Events:             events,
			Status:             status,
			LastVerifiedAt:     now,
			CreatedAt:          now,
			UpdatedAt:          now,
			IntegrationTokenID: args.ConnectorID,
		})
		if err != nil {
			return fmt.Errorf("failed to insert Azure DevOps removal registry: %w", err)
		}

		log.Printf("Created webhook_registry entry for Azure DevOps repository %s with status '%s'", args.ProjectPath, status)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check existing webhook registry entry: %w", err)
	}

	err = w.store.UpdateWebhookRegistryByID(ctx, existingID, storagejobqueue.WebhookRegistryUpdate{
		WebhookID:      webhookID,
		WebhookURL:     webhookURL,
		WebhookSecret:  w.config.WebhookConfig.Secret,
		WebhookName:    webhookName,
		Events:         events,
		Status:         status,
		LastVerifiedAt: now,
		UpdatedAt:      now,
	})
	if err != nil {
		return fmt.Errorf("failed to update Azure DevOps removal registry: %w", err)
	}

	log.Printf("Updated webhook_registry entry for Azure DevOps repository %s with status '%s'", args.ProjectPath, status)
	return nil
}
