package api

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/prompts"
	vendorpack "github.com/livereview/internal/prompts/vendor"
)

// Prompts API — Phase 7 endpoints

type catalogEntry struct {
	PromptKey string   `json:"prompt_key"`
	Provider  string   `json:"provider"`
	BuildID   string   `json:"build_id"`
	Variables []string `json:"variables"`
}

// GET /api/v1/prompts/catalog
func (s *Server) GetPromptsCatalog(c echo.Context) error {
	pack := vendorpack.New()
	entries := []catalogEntry{}
	listed := pack.List()
	// In dev builds without vendor pack, add registry_stub templates to catalog
	if len(listed) == 0 {
		for _, pt := range prompts.PlaintextTemplates() {
			listed = append(listed, vendorpack.TemplateInfo{PromptKey: pt.PromptKey, Provider: pt.Provider, BuildID: "dev"})
		}
	}
	for _, t := range listed {
		vars := []string{}
		// Try pack; if not found, fallback to plaintext registry (dev)
		if body, err := pack.GetPlaintext(t.PromptKey, t.Provider); err == nil && len(body) > 0 {
			seen := map[string]bool{}
			for _, ph := range prompts.ParsePlaceholders(string(body)) {
				if !seen[ph.Name] {
					seen[ph.Name] = true
					vars = append(vars, ph.Name)
				}
			}
			sort.Strings(vars)
			for i := range body {
				body[i] = 0
			}
		} else {
			// fallback for dev: scan plaintext registry
			for _, pt := range prompts.PlaintextTemplates() {
				prov := pt.Provider
				if prov == "" {
					prov = "default"
				}
				wantProv := t.Provider
				if wantProv == "" {
					wantProv = "default"
				}
				if pt.PromptKey == t.PromptKey && prov == wantProv {
					seen := map[string]bool{}
					for _, ph := range prompts.ParsePlaceholders(pt.Body) {
						if !seen[ph.Name] {
							seen[ph.Name] = true
							vars = append(vars, ph.Name)
						}
					}
					sort.Strings(vars)
					break
				}
			}
		}
		entries = append(entries, catalogEntry{
			PromptKey: t.PromptKey,
			Provider:  t.Provider,
			BuildID:   t.BuildID,
			Variables: vars,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"catalog": entries})
}

type renderPreviewResponse struct {
	Prompt   string `json:"prompt"`
	BuildID  string `json:"build_id"`
	Provider string `json:"provider"`
}

// GET /api/v1/prompts/:key/render
// Query: ai_connector_id, integration_token_id, repository
func (s *Server) RenderPromptPreview(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	key := c.Param("key")
	ctxSel, provider, err := s.parsePromptContext(c, pc.OrgID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	pack := vendorpack.New()
	store := prompts.NewStore(s.db)
	mgr := prompts.NewManager(store, pack)

	out, err := mgr.Render(c.Request().Context(), ctxSel, key, nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, renderPreviewResponse{
		Prompt:   out,
		BuildID:  pack.ActiveBuildID(),
		Provider: provider,
	})
}

type chunkDTO struct {
	ID            int64  `json:"id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	SequenceIndex int    `json:"sequence_index"`
	Enabled       bool   `json:"enabled"`
	AllowMarkdown bool   `json:"allow_markdown"`
	RedactOnLog   bool   `json:"redact_on_log"`
	CreatedBy     *int64 `json:"created_by,omitempty"`
	UpdatedBy     *int64 `json:"updated_by,omitempty"`
}

type variableEntry struct {
	Name   string     `json:"name"`
	Chunks []chunkDTO `json:"chunks"`
}

// GET /api/v1/prompts/:key/variables
func (s *Server) GetPromptVariables(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	key := c.Param("key")
	ctxSel, provider, err := s.parsePromptContext(c, pc.OrgID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	pack := vendorpack.New()
	var body []byte
	body, err = pack.GetPlaintext(key, provider)
	var placeholders []prompts.Placeholder
	if err == nil {
		placeholders = prompts.ParsePlaceholders(string(body))
		for i := range body {
			body[i] = 0
		}
	} else {
		// Dev fallback: search plaintext registry
		found := false
		for _, pt := range prompts.PlaintextTemplates() {
			prov := pt.Provider
			if prov == "" {
				prov = "default"
			}
			if pt.PromptKey == key && prov == provider {
				placeholders = prompts.ParsePlaceholders(pt.Body)
				found = true
				break
			}
		}
		if !found {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "template not found"})
		}
	}
	nameSeen := map[string]bool{}
	varNames := []string{}
	for _, ph := range placeholders {
		if !nameSeen[ph.Name] {
			nameSeen[ph.Name] = true
			varNames = append(varNames, ph.Name)
		}
	}
	sort.Strings(varNames)

	store := prompts.NewStore(s.db)
	mgr := prompts.NewManager(store, pack)
	appCtxID, err := mgr.ResolveApplicationContext(c.Request().Context(), ctxSel)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	varsOut := make([]variableEntry, 0, len(varNames))
	for _, v := range varNames {
		chunks, err := mgr.ListChunks(c.Request().Context(), pc.OrgID, appCtxID, key, v)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		dto := make([]chunkDTO, 0, len(chunks))
		for _, ch := range chunks {
			dto = append(dto, chunkDTO{
				ID:            ch.ID,
				Type:          ch.Type,
				Title:         ch.Title,
				Body:          ch.Body,
				SequenceIndex: ch.SequenceIndex,
				Enabled:       ch.Enabled,
				AllowMarkdown: ch.AllowMarkdown,
				RedactOnLog:   ch.RedactOnLog,
				CreatedBy:     ch.CreatedBy,
				UpdatedBy:     ch.UpdatedBy,
			})
		}
		varsOut = append(varsOut, variableEntry{Name: v, Chunks: dto})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"prompt_key": key,
		"provider":   provider,
		"variables":  varsOut,
	})
}

type createChunkRequest struct {
	Type               string  `json:"type"`
	Title              string  `json:"title"`
	Body               string  `json:"body"`
	Enabled            *bool   `json:"enabled"`
	AllowMarkdown      *bool   `json:"allow_markdown"`
	RedactOnLog        *bool   `json:"redact_on_log"`
	SequenceIndex      *int    `json:"sequence_index"`
	AIConnectorID      *int64  `json:"ai_connector_id"`
	IntegrationTokenID *int64  `json:"integration_token_id"`
	Repository         *string `json:"repository"`
}

// POST /api/v1/prompts/:key/variables/:var/chunks
func (s *Server) CreatePromptChunk(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	key := c.Param("key")
	variable := c.Param("var")

	var req createChunkRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Body) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "body is required"})
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	allowMD := true
	if req.AllowMarkdown != nil {
		allowMD = *req.AllowMarkdown
	}
	redact := true
	if req.RedactOnLog != nil {
		redact = *req.RedactOnLog
	}

	if strings.ToLower(req.Type) == "system" {
		if !(pc.IsOwner || pc.IsSuperAdmin) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "owner or super admin required for system chunks"})
		}
	} else {
		req.Type = "user"
		if !(pc.IsOwner || pc.IsMember || pc.IsSuperAdmin) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
		}
	}

	ctxSel, _, err := s.parsePromptContextWithBody(c, pc.OrgID, &req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	store := prompts.NewStore(s.db)
	pack := vendorpack.New()
	mgr := prompts.NewManager(store, pack)
	appCtxID, err := mgr.ResolveApplicationContext(c.Request().Context(), ctxSel)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Enforce single-value semantics: if a chunk already exists for this (org, context, prompt_key, variable), update it in-place.
	existing, err := mgr.ListChunks(c.Request().Context(), pc.OrgID, appCtxID, key, variable)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	createdBy := pc.GetUserID()
	if len(existing) > 0 {
		ch := existing[0]
		// Update mutable fields
		ch.Title = req.Title
		ch.Body = req.Body
		ch.UpdatedBy = ptrInt64(createdBy)
		// Preserve sequence_index & flags (could be toggled later via dedicated UI)
		if err := mgr.UpdateChunk(c.Request().Context(), ch); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]any{"chunk": chunkDTO{
			ID:            ch.ID,
			Type:          ch.Type,
			Title:         ch.Title,
			Body:          ch.Body,
			SequenceIndex: ch.SequenceIndex,
			Enabled:       ch.Enabled,
			AllowMarkdown: ch.AllowMarkdown,
			RedactOnLog:   ch.RedactOnLog,
			CreatedBy:     ch.CreatedBy,
			UpdatedBy:     ch.UpdatedBy,
		}})
	}

	// No existing chunk — create a new base entry (sequence index fixed at 1000 unless provided)
	seq := 1000
	if req.SequenceIndex != nil {
		seq = *req.SequenceIndex
	}
	ch := prompts.Chunk{
		OrgID:                pc.OrgID,
		ApplicationContextID: appCtxID,
		PromptKey:            key,
		VariableName:         variable,
		Type:                 strings.ToLower(req.Type),
		Title:                req.Title,
		Body:                 req.Body,
		SequenceIndex:        seq,
		Enabled:              enabled,
		AllowMarkdown:        allowMD,
		RedactOnLog:          redact,
		CreatedBy:            ptrInt64(createdBy),
		UpdatedBy:            ptrInt64(createdBy),
	}
	id, err := mgr.CreateChunk(c.Request().Context(), ch)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	ch.ID = id
	return c.JSON(http.StatusOK, map[string]any{"chunk": chunkDTO{
		ID:            ch.ID,
		Type:          ch.Type,
		Title:         ch.Title,
		Body:          ch.Body,
		SequenceIndex: ch.SequenceIndex,
		Enabled:       ch.Enabled,
		AllowMarkdown: ch.AllowMarkdown,
		RedactOnLog:   ch.RedactOnLog,
		CreatedBy:     ch.CreatedBy,
		UpdatedBy:     ch.UpdatedBy,
	}})
}

type reorderRequest struct {
	OrderedIDs         []int64 `json:"ordered_ids"`
	AIConnectorID      *int64  `json:"ai_connector_id"`
	IntegrationTokenID *int64  `json:"integration_token_id"`
	Repository         *string `json:"repository"`
}

// POST /api/v1/prompts/:key/variables/:var/reorder
func (s *Server) ReorderPromptChunks(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	key := c.Param("key")
	variable := c.Param("var")

	if !(pc.IsOwner || pc.IsSuperAdmin) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "owner or super admin required"})
	}

	var req reorderRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if len(req.OrderedIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ordered_ids is required"})
	}

	ctxSel, _, err := s.parsePromptContextWithBody(c, pc.OrgID, &req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	store := prompts.NewStore(s.db)
	pack := vendorpack.New()
	mgr := prompts.NewManager(store, pack)
	appCtxID, err := mgr.ResolveApplicationContext(c.Request().Context(), ctxSel)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if err := mgr.ReorderChunks(c.Request().Context(), pc.OrgID, appCtxID, key, variable, req.OrderedIDs); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Helpers
func (s *Server) parsePromptContext(c echo.Context, orgID int64) (prompts.Context, string, error) {
	ctxSel := prompts.Context{OrgID: orgID}
	provider := "default"
	if v := strings.TrimSpace(c.QueryParam("ai_connector_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return ctxSel, provider, err
		}
		ctxSel.AIConnectorID = &id
		if p, err2 := s.lookupProvider(c.Request().Context(), id); err2 == nil && p != "" {
			provider = p
		}
	}
	if v := strings.TrimSpace(c.QueryParam("integration_token_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return ctxSel, provider, err
		}
		ctxSel.IntegrationTokenID = &id
	}
	if v := strings.TrimSpace(c.QueryParam("repository")); v != "" {
		vv := v
		ctxSel.Repository = &vv
	}
	return ctxSel, provider, nil
}

func (s *Server) parsePromptContextWithBody(c echo.Context, orgID int64, body any) (prompts.Context, string, error) {
	ctxSel, provider, err := s.parsePromptContext(c, orgID)
	if err != nil {
		return ctxSel, provider, err
	}
	switch t := body.(type) {
	case *createChunkRequest:
		if t.AIConnectorID != nil {
			ctxSel.AIConnectorID = t.AIConnectorID
			if p, err2 := s.lookupProvider(c.Request().Context(), *t.AIConnectorID); err2 == nil && p != "" {
				provider = p
			}
		}
		if t.IntegrationTokenID != nil {
			ctxSel.IntegrationTokenID = t.IntegrationTokenID
		}
		if t.Repository != nil {
			ctxSel.Repository = t.Repository
		}
	case *reorderRequest:
		if t.AIConnectorID != nil {
			ctxSel.AIConnectorID = t.AIConnectorID
			if p, err2 := s.lookupProvider(c.Request().Context(), *t.AIConnectorID); err2 == nil && p != "" {
				provider = p
			}
		}
		if t.IntegrationTokenID != nil {
			ctxSel.IntegrationTokenID = t.IntegrationTokenID
		}
		if t.Repository != nil {
			ctxSel.Repository = t.Repository
		}
	}
	return ctxSel, provider, nil
}

func (s *Server) lookupProvider(ctx context.Context, aiConnectorID int64) (string, error) {
	const q = `SELECT provider_name FROM ai_connectors WHERE id = $1`
	var provider string
	if err := s.db.QueryRowContext(ctx, q, aiConnectorID).Scan(&provider); err != nil {
		return "", err
	}
	return provider, nil
}

func ptrInt64(v int64) *int64 { return &v }
