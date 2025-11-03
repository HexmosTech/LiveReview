package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	lic "github.com/livereview/internal/license"
)

// attachLicenseRoutes registers license endpoints under /api/v1
func (s *Server) attachLicenseRoutes(v1 *echo.Group) {
	// Public endpoints for status? For now require auth to avoid leaking metadata.
	group := v1.Group("/license")
	group.GET("/status", s.handleLicenseStatus)
	group.POST("/update", s.handleLicenseUpdate)
	group.POST("/refresh", s.handleLicenseRefresh)
	group.DELETE("/delete", s.handleLicenseDelete)
}

func (s *Server) licenseService() *lic.Service {
	if s._licenseSvc == nil {
		cfg := lic.LoadConfig()
		svc := lic.NewService(cfg, s.db)
		s._licenseSvc = svc
		return svc
	}
	if v, ok := s._licenseSvc.(*lic.Service); ok {
		return v
	}
	// fallback recreate if type mismatch
	cfg := lic.LoadConfig()
	svc := lic.NewService(cfg, s.db)
	s._licenseSvc = svc
	return svc
}

func (s *Server) handleLicenseStatus(c echo.Context) error {
	svc := s.licenseService()
	st, err := svc.LoadOrInit(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	resp := toStatusResponse(st)
	return c.JSON(http.StatusOK, resp)
}

func (s *Server) handleLicenseUpdate(c echo.Context) error {
	var body struct {
		Token string `json:"token"`
	}
	if err := c.Bind(&body); err != nil || body.Token == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_token_required"})
	}
	svc := s.licenseService()
	st, err := svc.EnterLicense(c.Request().Context(), body.Token)
	if err != nil {
		return classifyLicenseError(c, err)
	}
	return c.JSON(http.StatusOK, toStatusResponse(st))
}

func (s *Server) handleLicenseRefresh(c echo.Context) error {
	svc := s.licenseService()
	st, err := svc.PerformOnlineValidation(c.Request().Context(), true)
	if err != nil && err != lic.ErrLicenseMissing { // still return status even if missing
		return classifyLicenseError(c, err)
	}
	return c.JSON(http.StatusOK, toStatusResponse(st))
}

func toStatusResponse(st *lic.LicenseState) *LicenseStatusResponse {
	if st == nil {
		return &LicenseStatusResponse{Status: lic.StatusMissing}
	}
	var expStr, lastValStr *string
	if st.ExpiresAt != nil {
		s := st.ExpiresAt.UTC().Format(time.RFC3339)
		expStr = &s
	}
	if st.LastValidatedAt != nil {
		s := st.LastValidatedAt.UTC().Format(time.RFC3339)
		lastValStr = &s
	}
	return &LicenseStatusResponse{
		Status:             st.Status,
		Subject:            st.Subject,
		AppName:            st.AppName,
		SeatCount:          st.SeatCount,
		Unlimited:          st.Unlimited,
		ExpiresAt:          expStr,
		LastValidatedAt:    lastValStr,
		LastValidationCode: st.LastValidationErrCode,
	}
}

func (s *Server) handleLicenseDelete(c echo.Context) error {
	svc := s.licenseService()
	if err := svc.DeleteLicense(c.Request().Context()); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "license_deleted_successfully"})
}

func classifyLicenseError(c echo.Context, err error) error {
	switch err {
	case lic.ErrLicenseMissing:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_missing"})
	case lic.ErrLicenseExpired:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_expired"})
	case lic.ErrLicenseInvalid:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_invalid"})
	}
	if _, ok := err.(lic.NetworkError); ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "license_network"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "license_internal"})
}

// Extend Server with license service pointer (added here to avoid circular edits above)
type licenseBackref interface{}

// _licenseSvc is added dynamically.
func (s *Server) setLicenseSvc(svc *lic.Service) { s._licenseSvc = svc }

// Add field via embedding pattern (can't alter original struct here directly with patch limitations); we rely on a pointer stored via this variable name.
// In practice you'd add this to the Server struct, but to minimize unrelated modifications we'll attach dynamically.
var _ = sql.ErrNoRows // silence import if unused
