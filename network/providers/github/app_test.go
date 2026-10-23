package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMintAppJWT_VerifiesWithPublicKeyAndSetsIssuer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	signed, err := MintAppJWT(4242, key, now)
	if err != nil {
		t.Fatal(err)
	}
	claims := &jwt.RegisteredClaims{}
	tok, err := jwt.ParseWithClaims(signed, claims, func(*jwt.Token) (interface{}, error) { return &key.PublicKey, nil }, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil || !tok.Valid {
		t.Fatalf("token did not verify: %v", err)
	}
	if claims.Issuer != "4242" {
		t.Errorf("iss = %q, want 4242", claims.Issuer)
	}
	if ttl := claims.ExpiresAt.Sub(now); ttl > 10*time.Minute {
		t.Errorf("expiry %v exceeds GitHub's 10 minute limit", ttl)
	}
}

func TestExchangeUserCode_TreatsErrorBodyAsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"error":"bad_verification_code","error_description":"The code passed is incorrect or expired."}`)
	}))
	defer srv.Close()
	if _, err := ExchangeUserCode(context.Background(), srv.URL, "id", "secret", "stale"); err == nil || !strings.Contains(err.Error(), "bad_verification_code") {
		t.Fatalf("err = %v, want bad_verification_code", err)
	}
}

func TestExchangeUserCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path != "/login/oauth/access_token" || r.Form.Get("code") != "good" || r.Form.Get("client_secret") != "secret" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprint(w, `{"access_token":"ghu_abc","token_type":"bearer"}`)
	}))
	defer srv.Close()
	tok, err := ExchangeUserCode(context.Background(), srv.URL+"/", "id", "secret", "good")
	if err != nil || tok.AccessToken != "ghu_abc" {
		t.Fatalf("got (%v, %v)", tok, err)
	}
}

func TestListUserInstallations_FollowsPagesAndMapsFields(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghu_x" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `{"installations":[{"id":2,"app_id":7,"account":{"login":"bob","type":"User"},"repository_selection":"all","suspended_at":"2026-01-01T00:00:00Z"}]}`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/user/installations?per_page=100&page=2>; rel="next"`, srv.URL))
		_, _ = fmt.Fprint(w, `{"installations":[{"id":1,"app_id":7,"account":{"login":"acme","type":"Organization"},"repository_selection":"selected","suspended_at":null}]}`)
	}))
	defer srv.Close()

	got, err := ListUserInstallations(context.Background(), srv.URL, "ghu_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d installations, want 2", len(got))
	}
	if got[0].ID != 1 || got[0].AccountLogin != "acme" || got[0].AccountType != "Organization" || got[0].Suspended {
		t.Errorf("first installation mapped wrong: %+v", got[0])
	}
	if got[1].ID != 2 || !got[1].Suspended || got[1].AppID != 7 {
		t.Errorf("second installation mapped wrong: %+v", got[1])
	}
}

func TestListUserInstallationRepositoryIDs_FollowsPages(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghu_x" || r.URL.Path != "/user/installations/42/repositories" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `{"total_count":3,"repositories":[{"id":30}]}`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/user/installations/42/repositories?per_page=100&page=2>; rel="next"`, srv.URL))
		_, _ = fmt.Fprint(w, `{"total_count":3,"repositories":[{"id":10},{"id":20}]}`)
	}))
	defer srv.Close()

	got, err := ListUserInstallationRepositoryIDs(context.Background(), srv.URL, "ghu_x", 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 10 || got[1] != 20 || got[2] != 30 {
		t.Fatalf("got %v, want [10 20 30]", got)
	}
}

func TestNextLink_IgnoresOtherHosts(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Link": []string{`<https://evil.example/x?page=2>; rel="next"`}}}
	if got := nextLink(resp, "https://api.github.com"); got != "" {
		t.Fatalf("followed a next link on a foreign host: %q", got)
	}
}

func TestCreateInstallationToken_ParsesExpiry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/app/installations/77/access_tokens" || r.Header.Get("Authorization") != "Bearer appjwt" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"token":"ghs_tok","expires_at":"2026-10-19T12:00:00Z"}`)
	}))
	defer srv.Close()
	tok, err := CreateInstallationToken(context.Background(), srv.URL, "appjwt", 77)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Token != "ghs_tok" || !tok.ExpiresAt.Equal(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected token: %+v", tok)
	}
}

func TestListInstallationRepositories_FirstPageAndCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/installation/repositories" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"total_count":1,"repositories":[{"id":5,"name":"api","full_name":"acme/api","html_url":"https://github.com/acme/api","private":true,"default_branch":"main"}]}`)
	}))
	defer srv.Close()
	repos, next, err := ListInstallationRepositories(context.Background(), srv.URL, "ghs_x", "")
	if err != nil || next != "" || len(repos) != 1 || repos[0].FullName != "acme/api" || !repos[0].Private {
		t.Fatalf("got (%+v, %q, %v)", repos, next, err)
	}
}

func TestDeleteInstallation(t *testing.T) {
	status := http.StatusNoContent
	var gotPath, gotAuth, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotMethod = r.URL.Path, r.Header.Get("Authorization"), r.Method
		w.WriteHeader(status)
	}))
	defer srv.Close()

	if err := DeleteInstallation(context.Background(), srv.URL, "app.jwt", 42); err != nil {
		t.Fatalf("204 must succeed: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/app/installations/42" || gotAuth != "Bearer app.jwt" {
		t.Fatalf("request was %s %s auth %q", gotMethod, gotPath, gotAuth)
	}
	status = http.StatusNotFound
	if err := DeleteInstallation(context.Background(), srv.URL, "app.jwt", 42); err != nil {
		t.Fatalf("an installation that is already gone must count as success: %v", err)
	}
	status = http.StatusForbidden
	if err := DeleteInstallation(context.Background(), srv.URL, "app.jwt", 42); err == nil {
		t.Fatal("403 must be an error")
	}
}
