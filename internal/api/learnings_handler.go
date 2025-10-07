package api

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/learnings"
)

type LearningsHandler struct {
	store   learnings.Store
	service *learnings.Service
}

func NewLearningsHandler(db *sql.DB) *LearningsHandler {
	ps := learnings.NewPostgresStore(db)
	svc := learnings.NewService(ps)
	return &LearningsHandler{store: ps, service: svc}
}

func (h *LearningsHandler) List(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	items, err := h.store.ListByOrg(c.Request().Context(), orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

func (h *LearningsHandler) Get(c echo.Context) error {
	id := c.Param("id")
	l, err := h.store.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, l)
}

func (h *LearningsHandler) Upsert(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	var body struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Tags   []string `json:"tags"`
		Scope  string   `json:"scope_kind"`
		RepoID string   `json:"repo_id"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	scope := learnings.ScopeKind(body.Scope)
	id, shortID, action, err := h.service.UpsertFromMetadata(c.Request().Context(), orgID, learnings.Draft{Title: body.Title, Body: body.Body, Tags: body.Tags, Scope: scope, RepoID: body.RepoID}, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": id, "short_id": shortID, "action": action})
}

func (h *LearningsHandler) Update(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	id := c.Param("id")
	// fetch to get short_id
	l, err := h.store.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	var body struct {
		Title  *string   `json:"title"`
		Body   *string   `json:"body"`
		Tags   *[]string `json:"tags"`
		Scope  *string   `json:"scope_kind"`
		RepoID *string   `json:"repo_id"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	var scopePtr *learnings.ScopeKind
	if body.Scope != nil {
		sv := learnings.ScopeKind(*body.Scope)
		scopePtr = &sv
	}
	_, _, err2 := h.service.UpdateFromMetadata(c.Request().Context(), orgID, l.ShortID, learnings.Deltas{Title: body.Title, Body: body.Body, Tags: body.Tags, Scope: scopePtr, RepoID: body.RepoID})
	if err2 != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err2.Error())
	}
	l2, _ := h.store.GetByID(c.Request().Context(), id)
	return c.JSON(http.StatusOK, l2)
}

func (h *LearningsHandler) Delete(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	id := c.Param("id")
	l, err := h.store.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if err := h.service.DeleteByShortID(c.Request().Context(), orgID, l.ShortID, nil); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *LearningsHandler) ApplyActionFromReply(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	var body struct {
		Metadata struct {
			Action  string   `json:"action"`
			ShortID string   `json:"short_id"`
			Title   string   `json:"title"`
			Body    string   `json:"body"`
			Scope   string   `json:"scope_kind"`
			RepoID  string   `json:"repo_id"`
			Tags    []string `json:"tags"`
		} `json:"metadata"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	switch body.Metadata.Action {
	case "add":
		id, shortID, action, err := h.service.UpsertFromMetadata(c.Request().Context(), orgID, learnings.Draft{Title: body.Metadata.Title, Body: body.Metadata.Body, Tags: body.Metadata.Tags, Scope: learnings.ScopeKind(body.Metadata.Scope), RepoID: body.Metadata.RepoID}, nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"action": action, "id": id, "short_id": shortID})
	case "update":
		sid := body.Metadata.ShortID
		_, action, err := h.service.UpdateFromMetadata(c.Request().Context(), orgID, sid, learnings.Deltas{Title: &body.Metadata.Title, Body: &body.Metadata.Body, Tags: &body.Metadata.Tags})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"action": action, "short_id": sid})
	case "delete":
		sid := body.Metadata.ShortID
		if err := h.service.DeleteByShortID(c.Request().Context(), orgID, sid, nil); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]interface{}{"action": "deleted", "short_id": sid})
	default:
		return c.JSON(http.StatusOK, map[string]string{"action": "none"})
	}
}
