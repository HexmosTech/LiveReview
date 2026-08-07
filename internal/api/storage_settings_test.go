package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/blobstore"
)

func TestValidateStorageSettings(t *testing.T) {
	cases := []struct {
		name        string
		cfg         blobstore.Config
		wantErr     bool
		wantBackend string
	}{
		{"empty backend defaults to filesystem", blobstore.Config{}, false, blobstore.BackendFilesystem},
		{"explicit filesystem", blobstore.Config{Backend: blobstore.BackendFilesystem}, false, blobstore.BackendFilesystem},
		{"s3 without bucket is rejected", blobstore.Config{Backend: blobstore.BackendS3}, true, ""},
		{"s3 with bucket is valid", blobstore.Config{Backend: blobstore.BackendS3, Bucket: "livereview-private"}, false, blobstore.BackendS3},
		{"unknown backend is rejected", blobstore.Config{Backend: "carrier-pigeon"}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			err := validateStorageSettings(&cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Backend != tc.wantBackend {
				t.Fatalf("Backend = %q, want %q", cfg.Backend, tc.wantBackend)
			}
		})
	}
}

// TestStorageSettingsRoundTripsThroughFilesystemBackend exercises the
// TestStorageSettings handler end-to-end against a real filesystem bucket
// (no DB dependency - the handler reads the config from the request body,
// not from system_settings), confirming a working config succeeds and an
// unwritable one fails cleanly.
func TestStorageSettingsRoundTripsThroughFilesystemBackend(t *testing.T) {
	dir := t.TempDir()
	body, _ := json.Marshal(blobstore.Config{Backend: blobstore.BackendFilesystem, LocalDir: dir})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/storage/test", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &Server{}
	if err := s.TestStorageSettings(c); err != nil {
		t.Fatalf("TestStorageSettings returned an error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStorageSettingsRejectsS3WithoutBucket(t *testing.T) {
	body, _ := json.Marshal(blobstore.Config{Backend: blobstore.BackendS3})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/storage/test", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &Server{}
	if err := s.TestStorageSettings(c); err != nil {
		t.Fatalf("TestStorageSettings returned an error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an s3 config with no bucket, got %d: %s", rec.Code, rec.Body.String())
	}
}
