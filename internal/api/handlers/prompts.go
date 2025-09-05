package handlers
package api

import (
    "context"
    "database/sql"
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
    // Auth required and org context already ensured by middleware
    pack := vendorpack.New()

    // Build entries from pack list; attempt to extract variable names non-invasively
    entries := []catalogEntry{}
    for _, t := range pack.List() {
        vars := []string{}
        // Best-effort: decrypt/load plaintext and parse placeholders to list variable names
        if body, err := pack.GetPlaintext(t.PromptKey, t.Provider); err == nil && len(body) > 0 {
            seen := map[string]bool{}
            for _, ph := range prompts.ParsePlaceholders(string(body)) {
                if !seen[ph.Name] {
                    seen[ph.Name] = true
                    vars = append(vars, ph.Name)
                }
            }
            sort.Strings(vars)
            // Zeroize buffer
            for i := range body {
                body[i] = 0
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
    ID            int64   `json:"id"`
    Type          string  `json:"type"`
    Title         string  `json:"title"`
    Body          string  `json:"body"`
    SequenceIndex int     `json:"sequence_index"`
    Enabled       bool    `json:"enabled"`
    AllowMarkdown bool    `json:"allow_markdown"`
    RedactOnLog   bool    `json:"redact_on_log"`
    CreatedBy     *int64  `json:"created_by,omitempty"`
    UpdatedBy     *int64  `json:"updated_by,omitempty"`
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
    // Load template body for variable discovery (safe; we only expose names)
    body, err := pack.GetPlaintext(key, provider)
    if err != nil {
        return c.JSON(http.StatusNotFound, map[string]string{"error": "template not found"})
    }
    placeholders := prompts.ParsePlaceholders(string(body))
    // Zeroize
    for i := range body {
        body[i] = 0
    }
    // Unique names
    nameSeen := map[string]bool{}
    varNames := []string{}
    for _, ph := range placeholders {
        if !nameSeen[ph.Name] {
            nameSeen[ph.Name] = true
            varNames = append(varNames, ph.Name)
        }
    }
    sort.Strings(varNames)

    // Resolve application context and list chunks per variable
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
    Type          string  `json:"type"`
    Title         string  `json:"title"`
    Body          string  `json:"body"`
    Enabled       *bool   `json:"enabled"`
    AllowMarkdown *bool   `json:"allow_markdown"`
    RedactOnLog   *bool   `json:"redact_on_log"`
    SequenceIndex *int    `json:"sequence_index"`
    // Optional context overrides (if not provided in query)
    AIConnectorID      *int64  `json:"ai_connector_id"`
    IntegrationTokenID *int64  `json:"integration_token_id"`
    Repository         *string `json:"repository"`
}

// POST /api/v1/prompts/:key/variables/:var/chunks
func (s *Server) CreatePromptChunk(c echo.Context) error {
    pc := auth.MustGetPermissionContext(c)
    key := c.Param("key")
    variable := c.Param("var")

    // Parse body
    var req createChunkRequest
    if err := c.Bind(&req); err != nil {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
    }
    // Basic validation
    if strings.TrimSpace(req.Body) == "" {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "body is required"})
    }
    // Default flags
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

    // Authorization: system chunks require org owner; user chunks allowed for owners/members
    if strings.ToLower(req.Type) == "system" {
        if !(pc.IsOwner || pc.IsSuperAdmin) {
            return c.JSON(http.StatusForbidden, map[string]string{"error": "owner or super admin required for system chunks"})
        }
    } else {
        // default to user chunk
        req.Type = "user"
        if !(pc.IsOwner || pc.IsMember || pc.IsSuperAdmin) {
            return c.JSON(http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
        }
    }

    // Resolve context (prefer query params then body overrides)
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

    // Determine sequence index if not provided — append after existing
    seq := 1000
    if req.SequenceIndex != nil {
        seq = *req.SequenceIndex
    } else {
        existing, err := mgr.ListChunks(c.Request().Context(), pc.OrgID, appCtxID, key, variable)
        if err == nil && len(existing) > 0 {
            maxSeq := existing[0].SequenceIndex
            for _, ch := range existing {
                if ch.SequenceIndex > maxSeq {
                    maxSeq = ch.SequenceIndex
                }
            }
            seq = maxSeq + 10
        }
    }

    // Create chunk
    createdBy := pc.GetUserID()
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
        // Surface unique constraint or other DB errors
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
    OrderedIDs []int64 `json:"ordered_ids"`
    // Optional context overrides
    AIConnectorID      *int64  `json:"ai_connector_id"`
    IntegrationTokenID *int64  `json:"integration_token_id"`
    Repository         *string `json:"repository"`
}

// POST /api/v1/prompts/:key/variables/:var/reorder
func (s *Server) ReorderPromptChunks(c echo.Context) error {
    pc := auth.MustGetPermissionContext(c)
    key := c.Param("key")
    variable := c.Param("var")

    // Require owner for reordering
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

// parsePromptContext parses context from query params and returns prompts.Context and provider (best effort)
func (s *Server) parsePromptContext(c echo.Context, orgID int64) (prompts.Context, string, error) {
    ctxSel := prompts.Context{OrgID: orgID}
    var provider string = "default"

    if v := strings.TrimSpace(c.QueryParam("ai_connector_id")); v != "" {
        if id, err := strconv.ParseInt(v, 10, 64); err == nil {
            ctxSel.AIConnectorID = &id
            // Best-effort provider lookup
            if p, err2 := s.lookupProvider(c.Request().Context(), id); err2 == nil && p != "" {
                provider = p
            }
        } else {
            return ctxSel, provider, err
        }
    }
    if v := strings.TrimSpace(c.QueryParam("integration_token_id")); v != "" {
        if id, err := strconv.ParseInt(v, 10, 64); err == nil {
            ctxSel.IntegrationTokenID = &id
        } else {
            return ctxSel, provider, err
        }
    }
    if v := strings.TrimSpace(c.QueryParam("repository")); v != "" {
        vv := v
        ctxSel.Repository = &vv
    }
    return ctxSel, provider, nil
}

// parsePromptContextWithBody merges query params and optional body-provided context fields
func (s *Server) parsePromptContextWithBody(c echo.Context, orgID int64, body any) (prompts.Context, string, error) {
    ctxSel, provider, err := s.parsePromptContext(c, orgID)
    if err != nil {
        return ctxSel, provider, err
    }
    // Merge from body via type switch for known request types
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
    // Minimal inline lookup to avoid exposing store; mirrors prompts.Store.providerFromAIConnector
    const q = `SELECT provider_name FROM ai_connectors WHERE id = $1`
    var provider string
    if err := s.db.QueryRowContext(ctx, q, aiConnectorID).Scan(&provider); err != nil {
        return "", err
    }
    return provider, nil
}

func ptrInt64(v int64) *int64 { return &v }
