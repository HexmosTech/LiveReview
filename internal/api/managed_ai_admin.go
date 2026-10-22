package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/pkg/models"
	storageaiconnectors "github.com/livereview/storage/aiconnectors"
)

// Managed AI admin: super admins choose which provider/model/key runs the "LiveReview AI Model" for each tier
// (default = main model, default_lite = helper). The key is write-only: it is never returned or logged.

var managedAITiers = map[string]bool{"default": true, "default_lite": true}

var managedAIProviders = map[string]bool{
	string(aiconnectors.ProviderOpenRouter):       true,
	string(aiconnectors.ProviderDeepSeek):         true,
	string(aiconnectors.ProviderGemini):           true,
	string(aiconnectors.ProviderGeminiEnterprise): true,
	string(aiconnectors.ProviderOpenAI):           true,
	string(aiconnectors.ProviderClaude):           true,
}

const managedAITestTimeout = 45 * time.Second

type managedAIConfigRequest struct {
	TierName     string `json:"tier_name"`
	ProviderName string `json:"provider_name"`
	ModelName    string `json:"model_name"`
	APIKey       string `json:"api_key"`
	GCPLocation  string `json:"gcp_location"`
	Label        string `json:"label"`
}

type managedAITestRequest struct {
	managedAIConfigRequest
	// ID tests an existing configuration; a blank api_key then means "use the stored key".
	ID int64 `json:"id"`
}

func (r *managedAIConfigRequest) normalize() {
	r.TierName = strings.TrimSpace(r.TierName)
	r.ProviderName = strings.TrimSpace(r.ProviderName)
	r.ModelName = strings.TrimSpace(r.ModelName)
	r.APIKey = strings.TrimSpace(r.APIKey)
	r.GCPLocation = strings.TrimSpace(r.GCPLocation)
	r.Label = strings.TrimSpace(r.Label)
	if r.ProviderName != string(aiconnectors.ProviderGeminiEnterprise) {
		r.GCPLocation = "" // only Vertex AI has a location
	}
}

// validate checks everything but the key; keyRequired is false when editing (blank keeps the stored key).
func (r *managedAIConfigRequest) validate(keyRequired bool) string {
	switch {
	case !managedAITiers[r.TierName]:
		return "tier_name must be default or default_lite"
	case !managedAIProviders[r.ProviderName]:
		return "unsupported provider"
	case r.ModelName == "":
		return "model_name is required"
	case keyRequired && r.APIKey == "":
		return "api_key is required"
	case r.ProviderName == string(aiconnectors.ProviderGeminiEnterprise) && r.GCPLocation == "":
		return "gcp_location is required for Gemini Enterprise (for example us, eu or global)"
	}
	return ""
}

func (r *managedAIConfigRequest) input() storageaiconnectors.ManagedAIConfigInput {
	return storageaiconnectors.ManagedAIConfigInput{
		TierName: r.TierName, ProviderName: r.ProviderName, ModelName: r.ModelName,
		APIKey: r.APIKey, GCPLocation: r.GCPLocation, Label: r.Label,
	}
}

// managedAISessionUser returns the signed-in super admin. API keys are rejected (these settings change what every
// org on the LiveReview model runs on): it then writes the error response and returns a nil user together with
// c.JSON's result, which the handler returns as is.
func managedAISessionUser(c echo.Context) (*models.User, error) {
	if authMethod, _ := c.Get(authMethodContextKey).(string); authMethod == authMethodAPIKey {
		return nil, c.JSON(http.StatusForbidden, map[string]string{"error": "Managed AI settings can only be changed from a signed-in session, not an API key"})
	}
	user, ok := c.Get(string(auth.UserContextKey)).(*models.User)
	if !ok || user == nil {
		return nil, c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not signed in"})
	}
	return user, nil
}

// testManagedAIConfig makes one small call with the given settings. It returns a message safe to show the admin.
func (s *Server) testManagedAIConfig(ctx context.Context, cfg storageaiconnectors.ManagedAIConfigInput) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, managedAITestTimeout)
	defer cancel()
	provider := aiconnectors.Provider(cfg.ProviderName)
	project := ""
	if provider == aiconnectors.ProviderGeminiEnterprise {
		if project = aiconnectors.ServiceAccountProjectID(cfg.APIKey); project == "" {
			return false, "The key isn't a Google service-account JSON key"
		}
	}
	ok, err := aiconnectors.ValidateAPIKey(ctx, s.db, provider, cfg.APIKey,
		aiconnectors.ResolveBaseURLForProviderName(cfg.ProviderName, ""), cfg.ModelName, project, cfg.GCPLocation, "", "")
	if err != nil {
		log.Warn().Err(err).Str("provider", cfg.ProviderName).Str("model", cfg.ModelName).Msg("[ManagedAI] test call failed")
		return false, "The test call failed. Check the key, the model name and (for Vertex) the location."
	}
	if !ok {
		return false, "The provider rejected the key or the model."
	}
	return true, "The model answered."
}

// ListManagedAIConfigs returns every tier's configurations (without keys).
func (s *Server) ListManagedAIConfigs(c echo.Context) error {
	configs, err := storageaiconnectors.NewManagedAIStore(s.db).ListManagedAIConfigs(c.Request().Context())
	if err != nil {
		log.Error().Err(err).Msg("[ManagedAI] list failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load managed AI configurations"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"configs": configs})
}

// CreateManagedAIConfig adds an inactive configuration to a tier. It is only saved if a test call succeeds.
func (s *Server) CreateManagedAIConfig(c echo.Context) error {
	user, err := managedAISessionUser(c)
	if user == nil {
		return err
	}
	var req managedAIConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}
	req.normalize()
	if msg := req.validate(true); msg != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": msg})
	}
	if ok, msg := s.testManagedAIConfig(c.Request().Context(), req.input()); !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Not saved: " + msg})
	}
	id, err := storageaiconnectors.NewManagedAIStore(s.db).CreateManagedAIConfig(c.Request().Context(), req.input(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("[ManagedAI] create failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save the configuration"})
	}
	log.Info().Int64("id", id).Int64("by_user", user.ID).Str("tier", req.TierName).Str("provider", req.ProviderName).
		Str("model", req.ModelName).Msg("[ManagedAI] configuration added")
	return c.JSON(http.StatusCreated, map[string]int64{"id": id})
}

// UpdateManagedAIConfig edits a configuration. A blank api_key keeps the stored one. Changes are only saved if a
// test call with the new values succeeds (for the active configuration they apply to the next review).
func (s *Server) UpdateManagedAIConfig(c echo.Context) error {
	user, err := managedAISessionUser(c)
	if user == nil {
		return err
	}
	id, perr := strconv.ParseInt(c.Param("id"), 10, 64)
	if perr != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid id"})
	}
	var req managedAIConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}
	ctx := c.Request().Context()
	store := storageaiconnectors.NewManagedAIStore(s.db)
	existing, storedKey, err := store.GetManagedAIConfig(ctx, id)
	if errors.Is(err, storageaiconnectors.ErrManagedAIConfigNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Configuration not found"})
	}
	if err != nil {
		log.Error().Err(err).Msg("[ManagedAI] load failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load the configuration"})
	}
	req.TierName = existing.TierName // a configuration stays in its tier
	req.normalize()
	if msg := req.validate(false); msg != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": msg})
	}
	test := req.input()
	if test.APIKey == "" {
		test.APIKey = storedKey
	}
	if ok, msg := s.testManagedAIConfig(ctx, test); !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Not saved: " + msg})
	}
	if err := store.UpdateManagedAIConfig(ctx, id, req.input(), user.ID); err != nil {
		log.Error().Err(err).Msg("[ManagedAI] update failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save the configuration"})
	}
	log.Info().Int64("id", id).Int64("by_user", user.ID).Str("provider", req.ProviderName).Str("model", req.ModelName).
		Bool("key_changed", req.APIKey != "").Msg("[ManagedAI] configuration updated")
	return c.JSON(http.StatusOK, map[string]string{"status": "updated"})
}

// ActivateManagedAIConfig makes a configuration the one its tier uses. It must pass a test first.
func (s *Server) ActivateManagedAIConfig(c echo.Context) error {
	user, err := managedAISessionUser(c)
	if user == nil {
		return err
	}
	id, perr := strconv.ParseInt(c.Param("id"), 10, 64)
	if perr != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid id"})
	}
	ctx := c.Request().Context()
	store := storageaiconnectors.NewManagedAIStore(s.db)
	cfg, key, err := store.GetManagedAIConfig(ctx, id)
	if errors.Is(err, storageaiconnectors.ErrManagedAIConfigNotFound) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Configuration not found"})
	}
	if err != nil {
		log.Error().Err(err).Msg("[ManagedAI] load failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load the configuration"})
	}
	if ok, msg := s.testManagedAIConfig(ctx, storageaiconnectors.ManagedAIConfigInput{
		TierName: cfg.TierName, ProviderName: cfg.ProviderName, ModelName: cfg.ModelName, APIKey: key, GCPLocation: cfg.GCPLocation,
	}); !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Not activated: " + msg})
	}
	if err := store.ActivateManagedAIConfig(ctx, id, user.ID); err != nil {
		log.Error().Err(err).Msg("[ManagedAI] activate failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to activate the configuration"})
	}
	log.Info().Int64("id", id).Int64("by_user", user.ID).Str("tier", cfg.TierName).Str("provider", cfg.ProviderName).
		Str("model", cfg.ModelName).Msg("[ManagedAI] configuration activated")
	return c.JSON(http.StatusOK, map[string]string{"status": "activated"})
}

// TestManagedAIConfig makes one small call with the form's values, or with a saved configuration's key when id is
// given and api_key is blank. Nothing is saved.
func (s *Server) TestManagedAIConfig(c echo.Context) error {
	if user, err := managedAISessionUser(c); user == nil {
		return err
	}
	var req managedAITestRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}
	ctx := c.Request().Context()
	if req.ID > 0 && strings.TrimSpace(req.APIKey) == "" {
		existing, key, err := storageaiconnectors.NewManagedAIStore(s.db).GetManagedAIConfig(ctx, req.ID)
		if errors.Is(err, storageaiconnectors.ErrManagedAIConfigNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Configuration not found"})
		}
		if err != nil {
			log.Error().Err(err).Msg("[ManagedAI] load failed")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load the configuration"})
		}
		req.APIKey = key
		req.TierName = existing.TierName
	}
	req.normalize()
	if msg := req.validate(true); msg != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": msg})
	}
	ok, msg := s.testManagedAIConfig(ctx, req.input())
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": ok, "message": msg})
}
