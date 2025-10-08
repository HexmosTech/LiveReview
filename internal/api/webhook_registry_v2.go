package api

import (
	"fmt"
	"log"
	"strings"

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
	registry.providers["gitlab"] = NewGitLabV2Provider(server)
	registry.providers["github"] = NewGitHubV2Provider(server)
	registry.providers["bitbucket"] = NewBitbucketV2Provider(server)

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

	// Check each provider to see if it can handle this webhook
	for providerName, provider := range r.providers {
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
	// Read headers
	headers := make(map[string]string)
	for key, values := range c.Request().Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Read body (we need to be careful not to consume it)
	body := make([]byte, 0)
	if c.Request().Body != nil {
		// For provider detection, we'll use headers primarily
		// and only read body if absolutely necessary
		// TODO: In production, we might want to buffer the body
	}

	// Detect provider
	providerName, provider := r.DetectProvider(headers, body)
	if provider == nil {
		return r.handleUnknownProvider(c, headers)
	}

	// Route to the detected provider
	log.Printf("[INFO] Routing webhook to %s provider", providerName)

	// For now, we'll route back to the existing handlers
	// In the future, we can call provider methods directly
	switch providerName {
	case "github":
		return r.server.GitHubWebhookHandlerV2(c)
	case "gitlab":
		return r.server.GitLabWebhookHandlerV2(c)
	case "bitbucket":
		return r.server.BitbucketWebhookHandler(c)
	default:
		return fmt.Errorf("provider %s detected but no handler available", providerName)
	}
}

// handleUnknownProvider handles webhooks from unknown providers
func (r *WebhookProviderRegistry) handleUnknownProvider(c echo.Context, headers map[string]string) error {
	log.Printf("[WARN] Unknown webhook provider, headers: %v", getRelevantHeaders(headers))

	// Try to determine provider from headers for fallback
	if userAgent, exists := headers["User-Agent"]; exists {
		userAgentLower := strings.ToLower(userAgent)

		// GitHub fallback
		if strings.Contains(userAgentLower, "github") {
			log.Printf("[INFO] Fallback to GitHub based on User-Agent: %s", userAgent)
			return r.server.GitHubWebhookHandlerV1(c)
		}

		// GitLab fallback
		if strings.Contains(userAgentLower, "gitlab") {
			log.Printf("[INFO] Fallback to GitLab based on User-Agent: %s", userAgent)
			return r.server.GitLabWebhookHandlerV1(c)
		}

		// Bitbucket fallback
		if strings.Contains(userAgentLower, "bitbucket") {
			log.Printf("[INFO] Fallback to Bitbucket based on User-Agent: %s", userAgent)
			return r.server.BitbucketWebhookHandler(c)
		}
	}

	// Last resort: return error
	return c.JSON(400, map[string]string{
		"error":   "Unknown webhook provider",
		"message": "Could not determine webhook provider from request headers",
		"headers": fmt.Sprintf("%v", getRelevantHeaders(headers)),
	})
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
