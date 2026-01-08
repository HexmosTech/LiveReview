package api

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// WebhookProviderRegistry manages all V2 webhook providers and routes webhooks dynamically
type WebhookProviderRegistry struct {
	providers map[string]WebhookProviderV2
	server    *Server
}

// NewWebhookProviderRegistry creates a new webhook provider registry
func NewWebhookProviderRegistry(server *Server) *WebhookProviderRegistry {
	registry := &WebhookProviderRegistry{
		providers: make(map[string]WebhookProviderV2),
		server:    server,
	}

	// Initialize all V2 providers
	registry.providers["gitlab"] = server.gitlabProviderV2
	registry.providers["github"] = server.githubProviderV2
	registry.providers["bitbucket"] = server.bitbucketProviderV2
	registry.providers["gitea"] = server.giteaProviderV2

	log.Printf("[INFO] Webhook provider registry initialized with providers: %v",
		registry.getProviderNames())

	return registry
}

// RegisterProvider registers a new webhook provider
func (r *WebhookProviderRegistry) RegisterProvider(name string, provider WebhookProviderV2) {
	r.providers[name] = provider
	log.Printf("[INFO] Registered webhook provider: %s", name)
}

// getProviderNames returns list of registered provider names
func (r *WebhookProviderRegistry) getProviderNames() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// DetectProvider detects which provider should handle the webhook based on headers and body
func (r *WebhookProviderRegistry) DetectProvider(headers map[string]string, body []byte) (string, WebhookProviderV2) {
	log.Printf("[DEBUG] Detecting provider for webhook with headers: %v", getRelevantHeaders(headers))

	// Check providers in priority order (Gitea before GitHub since Gitea sends GitHub-compatible headers)
	priorityOrder := []string{"gitea", "gitlab", "github", "bitbucket"}

	for _, providerName := range priorityOrder {
		provider, exists := r.providers[providerName]
		if !exists {
			continue
		}
		if provider.CanHandleWebhook(headers, body) {
			log.Printf("[INFO] Provider detected: %s", providerName)
			return providerName, provider
		}
	}

	log.Printf("[WARN] No provider detected for webhook")
	return "", nil
}

// getRelevantHeaders extracts webhook-relevant headers for logging
func getRelevantHeaders(headers map[string]string) map[string]string {
	relevant := make(map[string]string)

	// Look for common webhook headers
	webhookHeaders := []string{
		"X-GitHub-Event", "X-GitHub-Delivery", "X-Hub-Signature",
		"X-Gitlab-Event", "X-Gitlab-Token", "X-Gitlab-Event-UUID",
		"X-Event-Key", "X-Request-UUID", "X-Hook-UUID", // Bitbucket
		"X-Gitea-Event", "X-Gitea-Delivery", "X-Gitea-Signature", // Gitea
		"User-Agent", "Content-Type",
	}

	for _, headerName := range webhookHeaders {
		if value, exists := headers[headerName]; exists {
			relevant[headerName] = value
		}
	}

	return relevant
}

// RouteWebhook routes a webhook to the appropriate provider
func (r *WebhookProviderRegistry) RouteWebhook(c echo.Context) error {
	if r.server == nil || r.server.webhookOrchestratorV2 == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Webhook orchestrator not available",
		})
	}

	log.Printf("[INFO] Routing webhook request through orchestrator")
	return r.server.webhookOrchestratorV2.ProcessWebhookEvent(c)
}

// ProcessWebhookEvent processes a webhook event using the registry
func (r *WebhookProviderRegistry) ProcessWebhookEvent(c echo.Context) error {
	return r.RouteWebhook(c)
}

// GetProviderStats returns statistics about registered providers
func (r *WebhookProviderRegistry) GetProviderStats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["total_providers"] = len(r.providers)
	stats["providers"] = r.getProviderNames()
	stats["registry_initialized"] = true

	return stats
}

// Generic webhook handler that uses the provider registry
func (r *WebhookProviderRegistry) GenericWebhookHandler(c echo.Context) error {
	log.Printf("[INFO] Generic webhook handler processing request")
	return r.ProcessWebhookEvent(c)
}
