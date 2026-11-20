package bitbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestIsHexSHA(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"0ae64a60810fe8307d269f5b82f1a831464b990b", true},
		{"0AE64A60810FE8307D269F5B82F1A831464B990B", true},
		{"test/tool-trufflehog", false},
		{"main", false},
		{"0ae64a60810fe8307d269f5b82f1a831464b990", false},  // 39 chars
		{"0ae64a60810fe8307d269f5b82f1a831464b990bb", false}, // 41 chars
		{"0ae64a60810fe8307d269f5b82f1a831464b990z", false}, // 'z' not hex
	}

	for _, tt := range tests {
		got := isHexSHA(tt.input)
		if got != tt.want {
			t.Errorf("isHexSHA(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestResolveBitbucketRef(t *testing.T) {
	ctx := context.Background()

	// Test 1: Branch with no slash returns immediately
	if got := resolveBitbucketRef(ctx, http.DefaultClient, "ws", "repo", "main", "tok", "mail"); got != "main" {
		t.Errorf("expected 'main', got %q", got)
	}

	// Test 2: Commit SHA returns immediately
	sha := "0ae64a60810fe8307d269f5b82f1a831464b990b"
	if got := resolveBitbucketRef(ctx, http.DefaultClient, "ws", "repo", sha, "tok", "mail"); got != sha {
		t.Errorf("expected %q, got %q", sha, got)
	}

	// Test 3: Branch with slash queries Bitbucket API to resolve commit hash
	branchWithSlash := "test/tool-trufflehog"
	mockTargetSHA := "1234567890abcdef1234567890abcdef12345678"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2.0/repositories/ws/repo/refs/branches" {
			q := r.URL.Query().Get("q")
			expectedQ := `name = "test/tool-trufflehog"`
			if q != expectedQ {
				t.Errorf("expected query %q, got %q", expectedQ, q)
			}
			resp := map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"name": branchWithSlash,
						"target": map[string]interface{}{
							"hash": mockTargetSHA,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	// Custom client routing to mock server
	mockClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			u, _ := url.Parse(ts.URL + req.URL.Path + "?" + req.URL.RawQuery)
			req.URL = u
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	resolved := resolveBitbucketRef(ctx, mockClient, "ws", "repo", branchWithSlash, "token", "email@example.com")
	if resolved != mockTargetSHA {
		t.Errorf("expected resolved hash %q, got %q", mockTargetSHA, resolved)
	}

	// Test 4: Tag resolution fallback when branch lookup returns empty
	tagWithSlash := "release/v1.0.0"
	mockTagSHA := "abcdef1234567890abcdef1234567890abcdef12"

	tsTag := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2.0/repositories/ws/repo/refs/branches" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"values": []interface{}{}})
			return
		}
		if r.URL.Path == "/2.0/repositories/ws/repo/refs/tags" {
			resp := map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"name": tagWithSlash,
						"target": map[string]interface{}{
							"hash": mockTagSHA,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer tsTag.Close()

	mockTagClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			u, _ := url.Parse(tsTag.URL + req.URL.Path + "?" + req.URL.RawQuery)
			req.URL = u
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	resolvedTag := resolveBitbucketRef(ctx, mockTagClient, "ws", "repo", tagWithSlash, "token", "email@example.com")
	if resolvedTag != mockTagSHA {
		t.Errorf("expected resolved tag hash %q, got %q", mockTagSHA, resolvedTag)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

