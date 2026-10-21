package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/database"
)

const testCloudSecret = "test-cloud-secret"

func cloudTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	t.Setenv("CLOUD_JWT_SECRET", testCloudSecret)
	t.Setenv("DISCORD_SIGNUP_WEBHOOK_URL", "")
	return db
}

func hexmosJWT(t *testing.T, email string) string {
	t.Helper()
	claims := JWTClaims{Email: email, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testCloudSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// cleanupCloudUser removes everything provisioning (and a token pair) creates for the email.
func cleanupCloudUser(t *testing.T, db *sql.DB, email string) {
	t.Helper()
	t.Cleanup(func() {
		var userID, orgID sql.NullInt64
		_ = db.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
		_ = db.QueryRow(`SELECT id FROM orgs WHERE name = $1`, email).Scan(&orgID)
		if userID.Valid {
			_, _ = db.Exec(`DELETE FROM auth_tokens WHERE user_id = $1`, userID.Int64)
			_, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = $1`, userID.Int64)
			_, _ = db.Exec(`UPDATE users SET default_org_id = NULL WHERE id = $1`, userID.Int64)
		}
		if orgID.Valid {
			_, _ = db.Exec(`DELETE FROM prompt_application_context WHERE org_id = $1`, orgID.Int64)
			_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID.Int64)
		}
		if userID.Valid {
			_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID.Int64)
		}
	})
}

func TestEnsureCloudUserCore_CreatesOnceAndIsIdempotent(t *testing.T) {
	db := cloudTestDB(t)
	h := NewAuthHandlers(NewTokenService(db, "x"), db)
	email := fmt.Sprintf("ensure-core-%d@example.test", time.Now().UnixNano())
	cleanupCloudUser(t, db, email)

	in := EnsureCloudUserInput{JWT: hexmosJWT(t, email), Email: strings.ToUpper(email), FirstName: "Ada", Source: "test"}
	first, err := h.EnsureCloudUserCore(in)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !first.CreatedUser || !first.CreatedOrg || !first.AssignedSuperAdmin || first.Email != email || first.UserID == 0 || first.OrgID == 0 {
		t.Fatalf("first call result wrong: %+v", first)
	}

	second, err := h.EnsureCloudUserCore(in)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if second.CreatedUser || second.CreatedOrg || second.AssignedSuperAdmin || second.UserID != first.UserID || second.OrgID != first.OrgID {
		t.Fatalf("second call must be a no-op on the same ids: %+v vs %+v", second, first)
	}
}

func TestEnsureCloudUserCore_RejectsBadJWTAndEmailMismatch(t *testing.T) {
	db := cloudTestDB(t)
	h := NewAuthHandlers(NewTokenService(db, "x"), db)

	if _, err := h.EnsureCloudUserCore(EnsureCloudUserInput{JWT: "not-a-jwt", Email: "a@b.test"}); err == nil {
		t.Fatal("expected a malformed JWT to be rejected")
	} else if ce, ok := err.(*EnsureCloudUserError); !ok || ce.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want a 401 EnsureCloudUserError", err)
	}

	_, err := h.EnsureCloudUserCore(EnsureCloudUserInput{JWT: hexmosJWT(t, "owner@example.test"), Email: "someone-else@example.test"})
	if ce, ok := err.(*EnsureCloudUserError); !ok || ce.Status != http.StatusForbidden {
		t.Fatalf("err = %v, want a 403 token-user mismatch", err)
	}
}

// The HTTP endpoint must keep the response shape Cloud.tsx depends on.
func TestEnsureCloudUserEndpoint_ResponseShapeUnchanged(t *testing.T) {
	db := cloudTestDB(t)
	h := NewAuthHandlers(NewTokenService(db, "x"), db)
	email := fmt.Sprintf("ensure-http-%d@example.test", time.Now().UnixNano())
	cleanupCloudUser(t, db, email)

	body := fmt.Sprintf(`{"email":%q,"first_name":"Ada","last_name":"L"}`, email)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ensure-cloud-user", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+hexmosJWT(t, email))
	rec := httptest.NewRecorder()
	if err := h.EnsureCloudUser(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"status", "user_id", "org_id", "created_user", "created_org", "super_admin_assigned", "email", "user", "tokens", "organizations"} {
		if _, ok := got[key]; !ok {
			t.Errorf("response is missing %q", key)
		}
	}

	// No Authorization header => the same 401 as before.
	rec = httptest.NewRecorder()
	if err := h.EnsureCloudUser(echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")), rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "authorization header required") {
		t.Fatalf("missing header: %d %s", rec.Code, rec.Body.String())
	}
}
