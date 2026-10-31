package errclass

import (
	"errors"
	"testing"
)

func TestClassifyHTTPStatus(t *testing.T) {
	cases := []struct {
		status int
		want   Class
	}{
		{429, RateLimited},
		{500, Transient},
		{503, Transient},
		{400, Permanent},
		{401, Permanent},
		{200, Transient}, // not really an error path, but must not panic/misclassify as permanent
	}
	for _, c := range cases {
		if got := ClassifyHTTPStatus(c.status); got != c.want {
			t.Errorf("ClassifyHTTPStatus(%d) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestClassify_RateLimitPerProvider(t *testing.T) {
	cases := []struct {
		provider string
		errMsg   string
	}{
		{"openai", "API returned unexpected status code: 429: rate limit exceeded"},
		{"anthropic", "api error: 429 rate_limit_error"},
		{"googleai", "googleapi: Error 429: Resource has been exhausted (e.g. check quota)., RESOURCE_EXHAUSTED"},
		{"bedrock", "ThrottlingException: Rate exceeded"},
		{"bedrock", "operation error Bedrock Runtime: TooManyRequestsException"},
	}
	for _, c := range cases {
		got := Classify(c.provider, errors.New(c.errMsg))
		if got != RateLimited {
			t.Errorf("Classify(%q, %q) = %v, want RateLimited", c.provider, c.errMsg, got)
		}
	}
}

func TestClassify_OllamaNeverRateLimited(t *testing.T) {
	// Ollama is local - even a message containing "429" must not be treated
	// as a provider rate-limit signal for it.
	got := Classify("ollama", errors.New("unexpected error: 429 something"))
	if got == RateLimited {
		t.Errorf("Classify(ollama, ...) = RateLimited, want non-rate-limited (ollama has no provider rate limit)")
	}
}

func TestClassify_TransientIsNotRateLimited(t *testing.T) {
	cases := []string{
		"request timeout: API call exceeded deadline",
		"context deadline exceeded",
		"connection reset by peer",
		"502 Bad Gateway",
		"EOF",
	}
	for _, msg := range cases {
		got := Classify("openai", errors.New(msg))
		if got != Transient {
			t.Errorf("Classify(openai, %q) = %v, want Transient", msg, got)
		}
	}
}

func TestClassify_PermanentFallback(t *testing.T) {
	got := Classify("openai", errors.New("invalid request: missing required field 'model'"))
	if got != Permanent {
		t.Errorf("Classify(...) = %v, want Permanent", got)
	}
}

func TestClassify_UnrecognizedProviderUsesFallbackSignals(t *testing.T) {
	got := Classify("some-future-provider", errors.New("429 too many requests"))
	if got != RateLimited {
		t.Errorf("Classify(unrecognized provider, 429 msg) = %v, want RateLimited via fallback signals", got)
	}
}

func TestClassify_NilError(t *testing.T) {
	if got := Classify("openai", nil); got != Transient {
		t.Errorf("Classify(_, nil) = %v, want Transient (no-op default)", got)
	}
}
