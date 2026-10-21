package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"GITHUB_APP_ID", "GITHUB_APP_SLUG", "GITHUB_APP_CLIENT_ID", "GITHUB_APP_CLIENT_SECRET", "GITHUB_APP_PRIVATE_KEY", "GITHUB_APP_WEBHOOK_SECRET", "GITHUB_APP_BASE_URL"} {
		t.Setenv(k, kv[k])
	}
}

func fullEnv(t *testing.T) map[string]string {
	return map[string]string{
		"GITHUB_APP_ID": "123", "GITHUB_APP_SLUG": "livereview-test", "GITHUB_APP_CLIENT_ID": "Iv1.abc",
		"GITHUB_APP_CLIENT_SECRET": "sec", "GITHUB_APP_WEBHOOK_SECRET": "whsec", "GITHUB_APP_PRIVATE_KEY": testPEM(t),
	}
}

func TestFromEnv_OffWhenAppIDUnset(t *testing.T) {
	setEnv(t, map[string]string{})
	cfg, err := fromEnv()
	if cfg != nil || err != nil {
		t.Fatalf("got (%v, %v), want (nil, nil)", cfg, err)
	}
}

func TestFromEnv_PartialConfigNamesMissingVars(t *testing.T) {
	setEnv(t, map[string]string{"GITHUB_APP_ID": "123", "GITHUB_APP_SLUG": "x"})
	cfg, err := fromEnv()
	if cfg != nil || err == nil {
		t.Fatalf("got (%v, %v), want an error", cfg, err)
	}
	for _, name := range []string{"GITHUB_APP_CLIENT_ID", "GITHUB_APP_CLIENT_SECRET", "GITHUB_APP_WEBHOOK_SECRET", "GITHUB_APP_PRIVATE_KEY"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestFromEnv_InlinePEMWithEscapedNewlines(t *testing.T) {
	env := fullEnv(t)
	env["GITHUB_APP_PRIVATE_KEY"] = strings.ReplaceAll(env["GITHUB_APP_PRIVATE_KEY"], "\n", `\n`)
	setEnv(t, env)
	cfg, err := fromEnv()
	if err != nil || cfg == nil || cfg.PrivateKey == nil {
		t.Fatalf("got (%v, %v), want a parsed key", cfg, err)
	}
	if cfg.BaseURL != "https://github.com" || cfg.AppID != 123 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestFromEnv_KeyFromFile(t *testing.T) {
	env := fullEnv(t)
	path := filepath.Join(t.TempDir(), "app.pem")
	if err := os.WriteFile(path, []byte(env["GITHUB_APP_PRIVATE_KEY"]), 0o600); err != nil {
		t.Fatal(err)
	}
	env["GITHUB_APP_PRIVATE_KEY"] = path
	setEnv(t, env)
	cfg, err := fromEnv()
	if err != nil || cfg == nil {
		t.Fatalf("got (%v, %v)", cfg, err)
	}
}

func TestFromEnv_InvalidKeyAndID(t *testing.T) {
	env := fullEnv(t)
	env["GITHUB_APP_PRIVATE_KEY"] = "-----BEGIN RSA PRIVATE KEY-----\nnope\n-----END RSA PRIVATE KEY-----"
	setEnv(t, env)
	if _, err := fromEnv(); err == nil {
		t.Fatal("expected an error for an invalid key")
	}
	env = fullEnv(t)
	env["GITHUB_APP_ID"] = "abc"
	setEnv(t, env)
	if _, err := fromEnv(); err == nil {
		t.Fatal("expected an error for a non-numeric app id")
	}
}

func TestURLs_GitHubDotComVersusGHES(t *testing.T) {
	com := &Config{Slug: "lr app", ClientID: "cid", BaseURL: "https://github.com"}
	if got, want := com.InstallURL("a b"), "https://github.com/apps/lr%20app/installations/new?state=a+b"; got != want {
		t.Errorf("InstallURL = %q, want %q", got, want)
	}
	if got, want := com.InstallURLForTarget("a b", 42), "https://github.com/apps/lr%20app/installations/new/permissions?target_id=42&state=a+b"; got != want {
		t.Errorf("InstallURLForTarget = %q, want %q", got, want)
	}
	if got := com.APIBaseURL(); got != "https://api.github.com" {
		t.Errorf("APIBaseURL = %q", got)
	}
	if got, want := com.AuthorizeURL("s"), "https://github.com/login/oauth/authorize?client_id=cid&state=s"; got != want {
		t.Errorf("AuthorizeURL = %q, want %q", got, want)
	}
	if got, want := com.InstallationSettingsURL("Organization", "acme", 9), "https://github.com/organizations/acme/settings/installations/9"; got != want {
		t.Errorf("org settings = %q, want %q", got, want)
	}
	if got, want := com.InstallationSettingsURL("User", "bob", 9), "https://github.com/settings/installations/9"; got != want {
		t.Errorf("user settings = %q, want %q", got, want)
	}
	if com.BotLogin() != "lr app[bot]" {
		t.Errorf("BotLogin = %q", com.BotLogin())
	}

	ghes := &Config{Slug: "lr", ClientID: "cid", BaseURL: "https://git.corp.example"}
	if got, want := ghes.InstallURL("s"), "https://git.corp.example/github-apps/lr/installations/new?state=s"; got != want {
		t.Errorf("GHES InstallURL = %q, want %q", got, want)
	}
	if got := ghes.APIBaseURL(); got != "https://git.corp.example/api/v3" {
		t.Errorf("GHES APIBaseURL = %q", got)
	}
}
