package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestCreateReview_MissingPullRequestID and TestCreateReview_PullRequestNotFound
// cover the org-scoped lookup logic unique to this handler (before it
// delegates to TriggerReviewV2's shared phases). The full success path
// (authentication, provider enrichment, background job launch) is not
// covered here, matching this codebase's existing test boundary: even
// TriggerReviewV2 itself - the more mature, longer-standing endpoint this
// handler reuses - has no full end-to-end test coverage today, since doing
// so requires a live River job queue and a stubbed provider API.
func newCreateReviewContext(orgID int64, body string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reviews", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	return c, rec
}

func TestCreateReview_MissingPullRequestID(t *testing.T) {
	server := setupBotUserTestServer(t)
	orgID := getAnyOrgID(t, server.db)

	c, rec := newCreateReviewContext(orgID, `{}`)
	if err := server.createReview(c); err != nil {
		t.Fatalf("createReview: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing pull_request_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateReview_PullRequestNotFound(t *testing.T) {
	server := setupBotUserTestServer(t)
	orgID := getAnyOrgID(t, server.db)

	c, rec := newCreateReviewContext(orgID, `{"pull_request_id": 99999999}`)
	if err := server.createReview(c); err != nil {
		t.Fatalf("createReview: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent pull_request_id, got %d: %s", rec.Code, rec.Body.String())
	}
}
