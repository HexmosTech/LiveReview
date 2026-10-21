package hexmos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func parseServer(t *testing.T, status int, body string, check func(r *http.Request, payload map[string]interface{})) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if check != nil {
			check(r, payload)
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FW_PARSE_BASE_URL", srv.URL+"/")
	t.Setenv("FW_PARSE_APP_ID", "")
	t.Setenv("FW_PARSE_ADMIN_SECRET", "adm")
}

func TestExternalProviderSignup_Success(t *testing.T) {
	parseServer(t, 200, `{"result":{"jwt":"hexmos.jwt","success":true,"data":{"email":"a@b.com","first_name":"A","profilePicUrl":"http://p"}}}`,
		func(r *http.Request, p map[string]interface{}) {
			if r.URL.Path != "/parse/functions/externalProviderSignup" || r.Header.Get("X-Parse-Application-Id") != "impressionserver" ||
				r.Header.Get("X-Internal-Admin-Secret") != "adm" {
				t.Errorf("unexpected request: %s headers=%v", r.URL.Path, r.Header)
			}
			if p["externalServiceProvider"] != "github_app" || p["accessToken"] != "ghu_x" {
				t.Errorf("unexpected payload: %v", p)
			}
		})
	res, err := ExternalProviderSignup(context.Background(), "github_app", "ghu_x", "livereview")
	if err != nil || res.JWT != "hexmos.jwt" || res.User.Email != "a@b.com" || res.User.FirstName != "A" || res.User.ProfilePicURL != "http://p" {
		t.Fatalf("got (%+v, %v)", res, err)
	}
}

// Parse answers HTTP 200 with success:false for business failures; that must be an error.
func TestExternalProviderSignup_Http200FailureIsAnError(t *testing.T) {
	parseServer(t, 200, `{"result":{"success":false,"message":"Error: GitHub account has no verified primary email"}}`, nil)
	_, err := ExternalProviderSignup(context.Background(), "github_app", "ghu_x", "livereview")
	if err == nil || !strings.Contains(err.Error(), "no verified primary email") {
		t.Fatalf("err = %v, want the Parse message", err)
	}
}

func TestExternalProviderSignup_Misconfigured(t *testing.T) {
	t.Setenv("FW_PARSE_BASE_URL", "")
	if _, err := ExternalProviderSignup(context.Background(), "github_app", "x", "livereview"); err == nil {
		t.Fatal("expected an error when FW_PARSE_BASE_URL is unset")
	}
	parseServer(t, 500, `oops`, nil)
	if _, err := ExternalProviderSignup(context.Background(), "github_app", "x", "livereview"); err == nil {
		t.Fatal("expected an error on a 500")
	}
}
