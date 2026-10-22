package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/livereview/internal/api/auth"
	"github.com/livereview/pkg/models"
)

func TestManagedAIConfigRequestValidate(t *testing.T) {
	cases := []struct {
		name        string
		req         managedAIConfigRequest
		keyRequired bool
		wantErr     bool
	}{
		{"openrouter ok", managedAIConfigRequest{TierName: "default", ProviderName: "openrouter", ModelName: "deepseek/deepseek-v4-flash", APIKey: "k"}, true, false},
		{"unknown tier", managedAIConfigRequest{TierName: "premium", ProviderName: "openrouter", ModelName: "m", APIKey: "k"}, true, true},
		{"unsupported provider", managedAIConfigRequest{TierName: "default", ProviderName: "ollama", ModelName: "m", APIKey: "k"}, true, true},
		{"missing model", managedAIConfigRequest{TierName: "default", ProviderName: "openrouter", APIKey: "k"}, true, true},
		{"missing key on create", managedAIConfigRequest{TierName: "default", ProviderName: "openrouter", ModelName: "m"}, true, true},
		{"blank key on edit keeps stored key", managedAIConfigRequest{TierName: "default", ProviderName: "openrouter", ModelName: "m"}, false, false},
		{"vertex needs a location", managedAIConfigRequest{TierName: "default_lite", ProviderName: "gemini-enterprise", ModelName: "m", APIKey: "{}"}, true, true},
		{"vertex with location", managedAIConfigRequest{TierName: "default_lite", ProviderName: "gemini-enterprise", ModelName: "m", APIKey: "{}", GCPLocation: "us"}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.normalize()
			if got := tc.req.validate(tc.keyRequired); (got != "") != tc.wantErr {
				t.Fatalf("validate() = %q, wantErr %v", got, tc.wantErr)
			}
		})
	}
}

func TestManagedAIConfigRequestDropsLocationForNonVertex(t *testing.T) {
	r := managedAIConfigRequest{ProviderName: "openrouter", GCPLocation: "us"}
	r.normalize()
	if r.GCPLocation != "" {
		t.Fatalf("location kept for a non-Vertex provider: %q", r.GCPLocation)
	}
}

func TestManagedAIWritesRejectAPIKeys(t *testing.T) {
	s := &Server{}
	handlers := map[string]echo.HandlerFunc{
		"create":   s.CreateManagedAIConfig,
		"update":   s.UpdateManagedAIConfig,
		"activate": s.ActivateManagedAIConfig,
		"test":     s.TestManagedAIConfig,
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(authMethodContextKey, authMethodAPIKey)
			c.Set(string(auth.UserContextKey), &models.User{ID: 1})
			if err := h(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
		})
	}
}
