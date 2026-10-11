package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/blastradius/repocache"
	"github.com/rs/zerolog/log"
)

// repoCacheRepo loads the org-scoped repository row and maps it to its repo-cache entry.
func (s *Server) repoCacheRepo(c echo.Context) (repocache.Repo, error) {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return repocache.Repo{}, echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return repocache.Repo{}, echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}
	r := repocache.Repo{OrgID: orgID}
	err = s.db.QueryRowContext(c.Request().Context(),
		`SELECT connector_id, web_url FROM repositories WHERE id = $1 AND org_id = $2`, repoID, orgID,
	).Scan(&r.ConnectorID, &r.CloneURL)
	if err == sql.ErrNoRows {
		return r, echo.NewHTTPError(http.StatusNotFound, "Repository not found")
	}
	if err != nil {
		return r, echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch repository")
	}
	return r, nil
}

// GetRepositoryCache handles GET /api/v1/repositories/:repoId/cache.
func (s *Server) GetRepositoryCache(c echo.Context) error {
	r, err := s.repoCacheRepo(c)
	if err != nil {
		return err
	}
	settings, err := repocache.LoadSettings(c.Request().Context(), s.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to load repo cache settings")
	}
	return c.JSON(http.StatusOK, struct {
		repocache.CachedRepo
		Enabled bool `json:"enabled"`
	}{repocache.Inspect(repocache.DefaultRoot, r), settings.Enabled})
}

// ClearRepositoryCache handles DELETE /api/v1/repositories/:repoId/cache (org owners and super admins).
func (s *Server) ClearRepositoryCache(c echo.Context) error {
	if authMethod, _ := c.Get(authMethodContextKey).(string); authMethod == authMethodAPIKey {
		return echo.NewHTTPError(http.StatusForbidden, "Clearing the repo cache requires a signed-in session, not an API key")
	}
	if pc := auth.MustGetPermissionContext(c); !pc.IsSuperAdmin && !pc.IsOwner {
		return echo.NewHTTPError(http.StatusForbidden, "Only organization owners can clear the repo cache")
	}
	r, err := s.repoCacheRepo(c)
	if err != nil {
		return err
	}
	ok, err := repocache.Clear(repocache.DefaultRoot, r)
	if err != nil {
		log.Warn().Err(err).Int64("org_id", r.OrgID).Msg("[repo_cache] clear failed")
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to clear the repo cache")
	}
	if !ok {
		return echo.NewHTTPError(http.StatusConflict, "A review is using this repo right now; try again shortly")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Repo cache cleared"})
}
