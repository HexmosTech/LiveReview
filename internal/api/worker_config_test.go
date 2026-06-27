package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerConfigEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	// Retrieve database URL from environment variable or load from local .env config
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		if env, err := loadEnvFile("../../.env"); err == nil {
			dbURL = env["DATABASE_URL"]
		}
	}
	require.NotEmpty(t, dbURL, "DATABASE_URL is required to run database integration tests (set it in environment or in .env)")

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	s := &Server{
		db: db,
	}

	// Helper to reset and seed DB with the singleton row before each test.
	// instance_details always has exactly one row in production (created during admin password setup).
	setupCleanDB := func(t *testing.T) {
		_, err := db.Exec("DELETE FROM instance_details")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO instance_details (id, admin_password, worker_concurrent_reviews) VALUES (1, 'test_hash', 10)")
		require.NoError(t, err)
	}

	t.Run("GetWorkerConfig - Returns saved value from database", func(t *testing.T) {
		setupCleanDB(t)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/worker-config", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := s.GetWorkerConfig(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp WorkerConfigResponse
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 10, resp.WorkerConcurrentReviews)
	})

	t.Run("UpdateWorkerConfig - Saves value successfully", func(t *testing.T) {
		setupCleanDB(t) // row always exists in production (created during admin password setup)

		e := echo.New()
		reqBody, err := json.Marshal(UpdateWorkerConfigRequest{WorkerConcurrentReviews: 25})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/worker-config", bytes.NewBuffer(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err = s.UpdateWorkerConfig(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var body map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Equal(t, true, body["success"])
		assert.Equal(t, float64(25), body["worker_concurrent_reviews"])

		// Verify database persistence deterministically
		var savedConcurrency int
		err = db.QueryRow("SELECT worker_concurrent_reviews FROM instance_details ORDER BY id ASC LIMIT 1").Scan(&savedConcurrency)
		require.NoError(t, err)
		assert.Equal(t, 25, savedConcurrency)
	})

	t.Run("UpdateWorkerConfig - Saves value successfully (second update)", func(t *testing.T) {
		setupCleanDB(t)

		e := echo.New()
		reqBody, err := json.Marshal(UpdateWorkerConfigRequest{WorkerConcurrentReviews: 25})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/worker-config", bytes.NewBuffer(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err = s.UpdateWorkerConfig(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var body map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Equal(t, true, body["success"])
		assert.Equal(t, float64(25), body["worker_concurrent_reviews"])

		// Verify database persistence deterministically
		var savedConcurrency int
		err = db.QueryRow("SELECT worker_concurrent_reviews FROM instance_details ORDER BY id ASC LIMIT 1").Scan(&savedConcurrency)
		require.NoError(t, err)
		assert.Equal(t, 25, savedConcurrency)
	})

	t.Run("GetWorkerConfig - Returns saved value", func(t *testing.T) {
		setupCleanDB(t)

		// Set value first
		_, err = db.Exec("UPDATE instance_details SET worker_concurrent_reviews = 20 WHERE id = 1")
		require.NoError(t, err)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/worker-config", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := s.GetWorkerConfig(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp WorkerConfigResponse
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 20, resp.WorkerConcurrentReviews)
	})

	t.Run("UpdateWorkerConfig - Rejects invalid low values", func(t *testing.T) {
		setupCleanDB(t)

		e := echo.New()
		reqBody, err := json.Marshal(UpdateWorkerConfigRequest{WorkerConcurrentReviews: 0})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/worker-config", bytes.NewBuffer(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err = s.UpdateWorkerConfig(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var body map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Contains(t, body["error"], "between 1 and 40")
	})

	t.Run("UpdateWorkerConfig - Rejects invalid high values", func(t *testing.T) {
		setupCleanDB(t)

		e := echo.New()
		reqBody, err := json.Marshal(UpdateWorkerConfigRequest{WorkerConcurrentReviews: 45})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/worker-config", bytes.NewBuffer(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err = s.UpdateWorkerConfig(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var body map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Contains(t, body["error"], "between 1 and 40")
	})
}
