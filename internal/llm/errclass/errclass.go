// Package errclass classifies an error returned by an AI provider call into
// one of three buckets, so callers (the adaptive concurrency limiter in
// internal/llm/adaptivelimiter) can react differently to "the provider is
// rate-limiting us" than to "this call failed for an unrelated reason."
//
// github.com/tmc/langchaingo v0.1.15 defines a structured rate-limit error
// type (llms.Error, llms.ErrCodeRateLimit, llms.IsRateLimitError) with
// per-provider MapError functions (llms/openai/errors.go,
// llms/anthropic/errors.go, etc.) - but none of those MapError functions are
// actually called from the library's real request path (confirmed by reading
// the vendored source: llms/openai/openaillm.go, llms/anthropic/anthropicllm.go,
// llms/googleai/googleai.go, llms/bedrock/bedrockllm.go never invoke them).
// Every provider backend's HTTP client returns a plain string-wrapped error,
// so errors.As/llms.IsRateLimitError never match in practice. Substring
// matching on the error message, keyed by provider type, is therefore the
// only option available today - not a design preference.
package errclass

import "strings"

// Class is the outcome of classifying an error.
type Class int

const (
	// Transient means the call failed for a reason unrelated to provider
	// capacity (network blip, timeout, 5xx) - safe to retry, must NOT shrink
	// the adaptive limiter's capacity.
	Transient Class = iota
	// RateLimited means the provider told us to slow down - the one signal
	// that should shrink the adaptive limiter's capacity.
	RateLimited
	// Permanent means the call failed in a way retrying won't fix (bad
	// request, auth failure, etc.).
	Permanent
)

func (c Class) String() string {
	switch c {
	case RateLimited:
		return "rate_limited"
	case Permanent:
		return "permanent"
	default:
		return "transient"
	}
}

// ClassifyHTTPStatus classifies using a real HTTP status code, when the
// caller already has one in hand. This is the one reliable, non-string-match
// signal available - used by internal/ai/langchain/provider.go's own custom
// HTTP paths (anthropic-compatible, direct-POST), which decode the response
// status directly instead of going through langchaingo's call surface.
func ClassifyHTTPStatus(statusCode int) Class {
	switch {
	case statusCode == 429:
		return RateLimited
	case statusCode >= 500:
		return Transient
	case statusCode >= 400:
		return Permanent
	default:
		return Transient
	}
}

// rateLimitSignals holds the characteristic substring(s) each provider's
// client library is observed to produce for a rate-limit/throttling response.
// Ollama has no entry: it's a local model with no provider-side rate limit.
var rateLimitSignals = map[string][]string{
	"openai":               {"429", "rate limit exceeded", "rate_limit_exceeded"},
	"openai-compatible":    {"429", "rate limit exceeded", "rate_limit_exceeded"},
	"anthropic":            {"429", "rate_limit_error", "overloaded_error"},
	"anthropic-compatible": {"429", "rate_limit_error", "overloaded_error"},
	"googleai":             {"429", "resource_exhausted", "quota exceeded", "rate limit"},
	"gemini":               {"429", "resource_exhausted", "quota exceeded", "rate limit"},
	"vertex":               {"429", "resource_exhausted", "quota exceeded", "rate limit"},
	"vertexai":             {"429", "resource_exhausted", "quota exceeded", "rate limit"},
	"bedrock":              {"throttlingexception", "toomanyrequestsexception"},
}

// fallbackRateLimitSignals is used for an unrecognized/unset provider type -
// broad enough to catch the common cases without being provider-specific.
var fallbackRateLimitSignals = []string{"429", "rate limit", "too many requests", "quota exceeded"}

var transientSignals = []string{
	"timeout", "deadline exceeded", "connection reset", "connection refused",
	"502", "503", "504", "temporarily unavailable", "eof", "broken pipe",
}

// Classify classifies an error returned by a call to the given provider type
// (LiveReview's own providerType string, e.g. "openai", "anthropic",
// "googleai", "bedrock", "ollama"). providerType is matched case-insensitively;
// an empty/unrecognized value falls back to a generic signal set.
func Classify(providerType string, err error) Class {
	if err == nil {
		return Transient
	}
	msg := strings.ToLower(err.Error())
	provider := strings.ToLower(strings.TrimSpace(providerType))

	if provider == "ollama" {
		// Local model: never rate-limited by a remote provider. Still check
		// transient signals below (a local timeout is still a timeout).
	} else {
		signals, ok := rateLimitSignals[provider]
		if !ok {
			signals = fallbackRateLimitSignals
		}
		for _, s := range signals {
			if strings.Contains(msg, s) {
				return RateLimited
			}
		}
	}

	for _, s := range transientSignals {
		if strings.Contains(msg, s) {
			return Transient
		}
	}

	return Permanent
}
