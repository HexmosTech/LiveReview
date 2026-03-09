package aiconnectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateAPIKeyOpenAIResponsesSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses endpoint, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected Authorization header with bearer token, got %q", got)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request payload: %v", err)
		}
		if got, _ := payload["model"].(string); got != "o4-mini" {
			t.Fatalf("expected model o4-mini, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","output_text":"hello world"}`))
	}))
	defer server.Close()

	valid, err := ValidateAPIKey(context.Background(), ProviderOpenAI, "test-key", server.URL, "o4-mini")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !valid {
		t.Fatal("expected key to be valid")
	}
}

func TestValidateAPIKeyOpenAIResponsesAuthFailureReturnsInvalidNotError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	valid, err := ValidateAPIKey(context.Background(), ProviderOpenAI, "bad-key", server.URL, "o4-mini")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if valid {
		t.Fatal("expected key to be invalid")
	}
}
