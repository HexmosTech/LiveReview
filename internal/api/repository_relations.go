package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
)

// RelatedRepository is one row of the Related Repos tab: the linked
// repository's own identity plus the relation's metadata.
type RelatedRepository struct {
	RelationID   int64     `json:"relation_id"`
	RepositoryID int64     `json:"repository_id"`
	Provider     string    `json:"provider"`
	FullName     string    `json:"full_name"`
	WebURL       string    `json:"web_url"`
	CreatedAt    time.Time `json:"created_at"`
}

// orgScopedRepo confirms repoID belongs to orgID, returning ErrNoRows-mapped
// 404 otherwise. Call this for every repository ID taken from a path
// parameter or request body before it's used in a query - IDs are never
// trusted blindly (see CLAUDE.md's tenant-isolation rules).
func (s *Server) orgScopedRepoExists(c echo.Context, orgID, repoID int64) error {
	var exists bool
	err := s.db.QueryRowContext(c.Request().Context(),
		`SELECT EXISTS(SELECT 1 FROM repositories WHERE id = $1 AND org_id = $2)`, repoID, orgID,
	).Scan(&exists)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to verify repository")
	}
	if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "Repository not found")
	}
	return nil
}

// ListRelatedRepositories handles GET /api/v1/repositories/:repoId/related.
// Relations are stored directionally but read symmetrically: a repo related
// FROM or TO the requested one both show up here.
func (s *Server) ListRelatedRepositories(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}
	if err := s.orgScopedRepoExists(c, pc.OrgID, repoID); err != nil {
		return err
	}

	rows, err := s.db.QueryContext(c.Request().Context(), `
		SELECT rr.id, r.id, r.provider, r.full_name, r.web_url, rr.created_at
		FROM repository_relations rr
		JOIN repositories r ON r.id = CASE WHEN rr.repository_id = $1 THEN rr.related_repository_id ELSE rr.repository_id END
		WHERE rr.org_id = $2 AND (rr.repository_id = $1 OR rr.related_repository_id = $1)
		ORDER BY rr.created_at DESC`, repoID, pc.OrgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to list related repositories")
	}
	defer rows.Close()

	related := []RelatedRepository{}
	for rows.Next() {
		var rr RelatedRepository
		if err := rows.Scan(&rr.RelationID, &rr.RepositoryID, &rr.Provider, &rr.FullName, &rr.WebURL, &rr.CreatedAt); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to read related repositories")
		}
		related = append(related, rr)
	}
	return c.JSON(http.StatusOK, related)
}

// AddRelatedRepositoryRequest is the POST body for linking a related repo.
type AddRelatedRepositoryRequest struct {
	RelatedRepositoryID int64 `json:"related_repository_id"`
}

// AddRelatedRepository handles POST /api/v1/repositories/:repoId/related
// (org owners and super admins only). Both repos must belong to the caller's
// org - never trust the body's related_repository_id without checking.
func (s *Server) AddRelatedRepository(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if !pc.IsSuperAdmin && !pc.IsOwner {
		return echo.NewHTTPError(http.StatusForbidden, "Only organization owners can manage related repos")
	}
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}
	var req AddRelatedRepositoryRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}
	if req.RelatedRepositoryID == repoID {
		return echo.NewHTTPError(http.StatusBadRequest, "A repository cannot be related to itself")
	}
	if err := s.orgScopedRepoExists(c, pc.OrgID, repoID); err != nil {
		return err
	}
	if err := s.orgScopedRepoExists(c, pc.OrgID, req.RelatedRepositoryID); err != nil {
		return err
	}

	var createdBy any
	if pc.User != nil {
		createdBy = pc.User.ID
	}
	_, err = s.db.ExecContext(c.Request().Context(), `
		INSERT INTO repository_relations (org_id, repository_id, related_repository_id, created_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (repository_id, related_repository_id) DO NOTHING`,
		pc.OrgID, repoID, req.RelatedRepositoryID, createdBy)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to add related repository")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Related repository added"})
}

// RemoveRelatedRepository handles DELETE /api/v1/repositories/:repoId/related/:relationId
// (org owners and super admins only). Deletes by relation ID, scoped to the
// org, regardless of which side of the pair :repoId names - the tab only
// ever shows relations already confirmed to involve :repoId via the list
// endpoint, but the org_id check here is what actually enforces isolation.
func (s *Server) RemoveRelatedRepository(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if !pc.IsSuperAdmin && !pc.IsOwner {
		return echo.NewHTTPError(http.StatusForbidden, "Only organization owners can manage related repos")
	}
	relationID, err := strconv.ParseInt(c.Param("relationId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid relation ID")
	}

	res, err := s.db.ExecContext(c.Request().Context(),
		`DELETE FROM repository_relations WHERE id = $1 AND org_id = $2`, relationID, pc.OrgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to remove related repository")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "Relation not found")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Related repository removed"})
}
