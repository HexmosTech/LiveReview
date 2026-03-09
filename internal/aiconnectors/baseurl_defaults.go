package aiconnectors

import "strings"

// DefaultBaseURL returns the canonical default base URL for providers that require one.
func DefaultBaseURL(provider Provider) string {
	switch provider {
	case ProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	case ProviderDeepSeek:
		return "https://api.deepseek.com/v1"
	default:
		return ""
	}
}

// DefaultBaseURLForProviderName returns provider default base URL from a raw provider string.
func DefaultBaseURLForProviderName(providerName string) string {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case string(ProviderOpenRouter):
		return DefaultBaseURL(ProviderOpenRouter)
	case string(ProviderDeepSeek):
		return DefaultBaseURL(ProviderDeepSeek)
	default:
		return ""
	}
}

// ResolveBaseURL keeps explicit URL when set, otherwise falls back to the provider default.
func ResolveBaseURL(provider Provider, configured string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return DefaultBaseURL(provider)
}

// ResolveBaseURLForProviderName keeps explicit URL when set, otherwise uses provider default.
func ResolveBaseURLForProviderName(providerName, configured string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return DefaultBaseURLForProviderName(providerName)
}
