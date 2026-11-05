package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/license"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffReview_ToolsOnly(t *testing.T) {
	t.Run("returns 403 when req.ToolsOnly is true and deploymentConfig.IsCloud is false", func(t *testing.T) {
		e := echo.New()
		reqBody := `{"tools_only": true, "diff_zip_base64": "dummy"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/diff-review", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		c.Set("org_id", int64(1))
		c.Set("user_id", int64(0))

		s := &Server{
			deploymentConfig: &DeploymentConfig{IsCloud: false},
		}

		err := s.DiffReview(c)
		require.NoError(t, err)

		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "Third-party tools are only available in cloud mode", resp["error"])
		assert.NotNil(t, resp["envelope"])

		envelope, ok := resp["envelope"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "v1", envelope["envelope_version"])
	})

	t.Run("verifies JSON envelope structure with PlanContext attached", func(t *testing.T) {
		e := echo.New()
		reqBody := `{"tools_only": true, "diff_zip_base64": "dummy"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/diff-review", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		c.Set("org_id", int64(42))
		c.Set("user_id", int64(0))
		c.Set(apimiddleware.PlanContextKey, apimiddleware.PlanContext{
			PlanType: license.PlanTeam,
			Limits:   license.PlanTeam.GetLimits(),
		})

		s := &Server{
			deploymentConfig: &DeploymentConfig{IsCloud: false},
		}

		err := s.DiffReview(c)
		require.NoError(t, err)

		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "Third-party tools are only available in cloud mode", resp["error"])
		assert.NotNil(t, resp["envelope"])

		envelope, ok := resp["envelope"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "v1", envelope["envelope_version"])
		assert.Equal(t, license.PlanTeam.String(), envelope["plan_code"])
	})

	t.Run("tools_only false passes cloud gate and proceeds to diff validation", func(t *testing.T) {
		e := echo.New()
		reqBody := `{"tools_only": false}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/diff-review", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		c.Set("org_id", int64(1))
		c.Set("user_id", int64(0))

		s := &Server{
			deploymentConfig: &DeploymentConfig{IsCloud: false},
		}

		err := s.DiffReview(c)
		require.NoError(t, err)

		// Should pass the cloud-gate and fail on missing diff_zip_base64 (400 Bad Request)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "diff_zip_base64 is required", resp["error"])
	})
}
