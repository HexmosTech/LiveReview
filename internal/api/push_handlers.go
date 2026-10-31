package api

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/pushnotify"
	"github.com/livereview/pkg/models"
)

// PushHandlers serves browser push-notification subscription management and
// per-user notification preferences. Session-only (see server.go's route
// registration — these sit under authMiddleware.RequireAuth(), never
// RequireAuthOrAPIKey): a push subscription is tied to one specific
// browser's real endpoint, not something automation should be creating or
// reading on a user's behalf.
type PushHandlers struct {
	db *sql.DB
}

func NewPushHandlers(db *sql.DB) *PushHandlers {
	return &PushHandlers{db: db}
}

func currentUser(c echo.Context) (*models.User, error) {
	user, ok := c.Get("user").(*models.User)
	if !ok || user == nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "user not found in context")
	}
	return user, nil
}

// GetVAPIDPublicKey returns the instance's public VAPID key for the
// frontend's pushManager.subscribe({applicationServerKey: ...}) call.
func (h *PushHandlers) GetVAPIDPublicKey(c echo.Context) error {
	publicKey, err := pushnotify.GetVAPIDPublicKey(c.Request().Context(), h.db)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load VAPID public key")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"public_key": publicKey})
}

// PushSubscriptionRequest mirrors the browser's PushSubscription.toJSON() shape.
type PushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// CreateSubscription registers (or re-registers, on conflict) the current
// browser's push subscription for the authenticated user. user_id always
// comes from the session, never from the request body.
func (h *PushHandlers) CreateSubscription(c echo.Context) error {
	user, err := currentUser(c)
	if err != nil {
		return err
	}

	var req PushSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "endpoint and keys.p256dh/keys.auth are required")
	}

	userAgent := c.Request().UserAgent()
	_, err = h.db.ExecContext(c.Request().Context(), `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth_key, user_agent, last_used_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			p256dh = EXCLUDED.p256dh,
			auth_key = EXCLUDED.auth_key,
			user_agent = EXCLUDED.user_agent,
			last_used_at = NOW()
	`, user.ID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth, userAgent)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to save push subscription")
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "subscribed"})
}

// DeleteSubscriptionRequest identifies which subscription to remove.
// DeleteSubscription removes the current browser's push subscription (its
// endpoint passed as a query param, not a body — DELETE-with-body has no
// clean client-side precedent elsewhere in this codebase's apiClient).
// Scoped to the authenticated user's own subscriptions — deleting by
// (endpoint, user_id), never endpoint alone, so one user can never remove
// another's row.
func (h *PushHandlers) DeleteSubscription(c echo.Context) error {
	user, err := currentUser(c)
	if err != nil {
		return err
	}

	endpoint := c.QueryParam("endpoint")
	if endpoint == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "endpoint query param is required")
	}

	if _, err := h.db.ExecContext(c.Request().Context(), `
		DELETE FROM push_subscriptions WHERE endpoint = $1 AND user_id = $2
	`, endpoint, user.ID); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to remove push subscription")
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "unsubscribed"})
}

// NotificationPreferences is both the GET response and the PUT request body.
type NotificationPreferences struct {
	NotifyFirstFinding bool `json:"notify_first_finding"`
	NotifyCompletion   bool `json:"notify_completion"`
}

// GetNotificationPreferences returns the authenticated user's preferences,
// defaulting both to true when no row exists yet (see the migration's
// comment — this is the agreed "on by default" behavior, not a placeholder).
func (h *PushHandlers) GetNotificationPreferences(c echo.Context) error {
	user, err := currentUser(c)
	if err != nil {
		return err
	}

	prefs := NotificationPreferences{NotifyFirstFinding: true, NotifyCompletion: true}
	err = h.db.QueryRowContext(c.Request().Context(), `
		SELECT notify_first_finding, notify_completion
		FROM user_notification_preferences WHERE user_id = $1
	`, user.ID).Scan(&prefs.NotifyFirstFinding, &prefs.NotifyCompletion)
	if err != nil && err != sql.ErrNoRows {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load notification preferences")
	}

	return JSONWithEnvelope(c, http.StatusOK, prefs.toMap())
}

// UpdateNotificationPreferences upserts the authenticated user's preferences.
func (h *PushHandlers) UpdateNotificationPreferences(c echo.Context) error {
	user, err := currentUser(c)
	if err != nil {
		return err
	}

	var prefs NotificationPreferences
	if err := c.Bind(&prefs); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}

	_, err = h.db.ExecContext(c.Request().Context(), `
		INSERT INTO user_notification_preferences (user_id, notify_first_finding, notify_completion, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			notify_first_finding = EXCLUDED.notify_first_finding,
			notify_completion = EXCLUDED.notify_completion,
			updated_at = NOW()
	`, user.ID, prefs.NotifyFirstFinding, prefs.NotifyCompletion)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to save notification preferences")
	}

	return JSONWithEnvelope(c, http.StatusOK, prefs.toMap())
}

func (p NotificationPreferences) toMap() map[string]interface{} {
	return map[string]interface{}{
		"notify_first_finding": p.NotifyFirstFinding,
		"notify_completion":    p.NotifyCompletion,
	}
}
