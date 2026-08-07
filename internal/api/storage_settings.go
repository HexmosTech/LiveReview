package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/blobstore"
	"github.com/rs/zerolog/log"
)

func validateStorageSettings(cfg *blobstore.Config) error {
	switch cfg.Backend {
	case "":
		cfg.Backend = blobstore.BackendFilesystem
	case blobstore.BackendFilesystem:
	case blobstore.BackendS3:
		if cfg.Bucket == "" {
			return errors.New("bucket is required for the S3-compatible backend")
		}
	default:
		return errors.New("unknown storage backend")
	}
	return nil
}

// GetStorageSettings fetches the global blob-storage configuration from
// system_settings. Absent config (no row yet) returns the filesystem
// default, matching blobstore.OpenBucket's own zero-config behavior - so a
// fresh instance that's never touched this settings page still reports a
// coherent, working configuration.
func (s *Server) GetStorageSettings(c echo.Context) error {
	var data []byte
	err := s.db.QueryRow("SELECT data FROM system_settings WHERE name = 'blob_storage'").Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, blobstore.Config{Backend: blobstore.BackendFilesystem})
		}
		log.Error().Err(err).Msg("Failed to fetch storage settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch storage settings"})
	}

	var cfg blobstore.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Error().Err(err).Msg("Failed to parse storage settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to parse storage settings"})
	}
	return c.JSON(http.StatusOK, cfg)
}

// UpdateStorageSettings saves the global blob-storage configuration to
// system_settings. internal/api/diff_review.go's artifact Put/Get handlers
// read this fresh on every request (no caching), so a change here takes
// effect for the very next artifact call - no restart required.
func (s *Server) UpdateStorageSettings(c echo.Context) error {
	var cfg blobstore.Config
	if err := c.Bind(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}
	if err := validateStorageSettings(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal storage settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to process settings"})
	}

	_, err = s.db.Exec(`
		INSERT INTO system_settings (name, data)
		VALUES ('blob_storage', $1)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP
	`, data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save storage settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save storage settings"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Storage settings updated successfully"})
}

// TestStorageSettings round-trips a small marker object through the given
// config's bucket (write, read, delete) to confirm it's reachable and
// writable before an admin commits to it. Takes the config from the request
// body (not the saved settings) so "Test connection" works before saving.
func (s *Server) TestStorageSettings(c echo.Context) error {
	var cfg blobstore.Config
	if err := c.Bind(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}
	if err := validateStorageSettings(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()

	bucket, err := blobstore.OpenBucket(ctx, cfg)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to open bucket: " + err.Error()})
	}
	defer bucket.Close()

	const testKey = "_livereview_storage_settings_test"
	if err := bucket.WriteAll(ctx, testKey, []byte("ok"), nil); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to write test object: " + err.Error()})
	}
	if _, err := bucket.ReadAll(ctx, testKey); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Failed to read test object back: " + err.Error()})
	}
	if err := bucket.Delete(ctx, testKey); err != nil {
		log.Warn().Err(err).Msg("storage settings test: failed to clean up test object")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Storage connection succeeded"})
}
