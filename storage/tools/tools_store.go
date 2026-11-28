package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
)

type AvailableTool struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	LambdaARN   string  `json:"lambda_arn"`
	Multiplier  float64 `json:"multiplier"`
	UseCase     string  `json:"use_case"`
	Category    string  `json:"category"`
	DocsURL     string  `json:"docs_url"`
}

type OrgToolView struct {
	AvailableTool
	Enabled    bool            `json:"enabled"`
	ConfigJSON json.RawMessage `json:"config_json"`
}

type OrgToolRow struct {
	OrgID      int64           `json:"org_id"`
	ToolID     int64           `json:"tool_id"`
	Enabled    bool            `json:"enabled"`
	ConfigJSON json.RawMessage `json:"config_json"`
}

type ToolsStore struct {
	db *sql.DB
}

func NewToolsStore(db *sql.DB) *ToolsStore {
	return &ToolsStore{db: db}
}

// LoadCatalog loads the tools catalog entirely from the database.
// All tool definitions (names, ARNs, multipliers) are managed by lr-tools deployer
// and stored in the available_tools table. No hardcoded catalog exists in Go source.
//
// Resolution order:
//  1. system_settings row 'tools_catalog' (admin override) — highest priority
//  2. available_tools table (populated by lr-tools deployer with real Lambda ARNs)
//  3. Empty list — no tools available until registered via lr-tools
func (s *ToolsStore) LoadCatalog(ctx context.Context) ([]AvailableTool, error) {
	if s.db == nil {
		return nil, nil
	}

	// --- Tier 1: system_settings admin override ---
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM public.system_settings WHERE name = 'tools_catalog'").Scan(&data)
	if err == nil && len(data) > 0 {
		var overrides []AvailableTool
		if unmarshalErr := json.Unmarshal(data, &overrides); unmarshalErr == nil && len(overrides) > 0 {
			return overrides, nil
		}
	}

	// --- Tier 2: available_tools table (populated by lr-tools deployer) ---
	rows, dbErr := s.db.QueryContext(ctx, "SELECT id, name, description, lambda_arn, multiplier, COALESCE(use_case, ''), COALESCE(category, ''), COALESCE(docs_url, '') FROM public.available_tools ORDER BY id")
	if dbErr == nil {
		defer rows.Close()
		var dbTools []AvailableTool
		for rows.Next() {
			var t AvailableTool
			if scanErr := rows.Scan(&t.ID, &t.Name, &t.Description, &t.LambdaARN, &t.Multiplier, &t.UseCase, &t.Category, &t.DocsURL); scanErr == nil {
				dbTools = append(dbTools, t)
			}
		}
		if len(dbTools) > 0 {
			return dbTools, nil
		}
	}

	// No tools registered — return empty
	return nil, nil
}

const (
	RepoToolModeInherit = "inherit"
	RepoToolModeCustom  = "custom"
)

type OrgToolConfig struct {
	Enabled    bool            `json:"enabled"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`
}

// RepoToolRuleConfig represents per-tool file filter rules stored inside OrgToolConfig.ConfigJSON.
type RepoToolRuleConfig struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type RepoToolSettings struct {
	Mode  string                   `json:"mode"`
	Tools map[string]OrgToolConfig `json:"tools,omitempty"`
}

type RepoToolView struct {
	AvailableTool
	Enabled     bool            `json:"enabled"`
	OrgEnabled  bool            `json:"org_enabled"`
	RepoEnabled *bool           `json:"repo_enabled,omitempty"`
	ConfigJSON  json.RawMessage `json:"config_json"`
}

func parseToolConfigMap(rawMap map[string]interface{}) map[string]OrgToolConfig {
	settingsMap := make(map[string]OrgToolConfig, len(rawMap))
	for k, v := range rawMap {
		lowerK := strings.ToLower(k)
		if b, ok := v.(bool); ok {
			settingsMap[lowerK] = OrgToolConfig{
				Enabled:    b,
				ConfigJSON: json.RawMessage("{}"),
			}
		} else if m, ok := v.(map[string]interface{}); ok {
			cfg := OrgToolConfig{ConfigJSON: json.RawMessage("{}")}
			if eb, ok := m["enabled"].(bool); ok {
				cfg.Enabled = eb
			}
			if cj, ok := m["config_json"]; ok {
				if cjBytes, err := json.Marshal(cj); err == nil {
					cfg.ConfigJSON = json.RawMessage(cjBytes)
				}
			}
			settingsMap[lowerK] = cfg
		}
	}
	return settingsMap
}

func lookupToolConfig(settingsMap map[string]OrgToolConfig, t AvailableTool) (OrgToolConfig, bool) {
	return LookupToolConfig(settingsMap, t)
}

// LookupToolConfig resolves a tool entry in a repo/org settings map by lowercase name or numeric ID.
func LookupToolConfig(settingsMap map[string]OrgToolConfig, t AvailableTool) (OrgToolConfig, bool) {
	if len(settingsMap) == 0 {
		return OrgToolConfig{}, false
	}
	if cfg, ok := settingsMap[strings.ToLower(t.Name)]; ok {
		return cfg, true
	}
	cfg, ok := settingsMap[fmt.Sprintf("%d", t.ID)]
	return cfg, ok
}

// GetAvailableToolsForOrg lists all available tools from the catalog, annotated with the org's enabling configuration from orgs.settings.
func (s *ToolsStore) GetAvailableToolsForOrg(ctx context.Context, orgID int64) ([]OrgToolView, error) {
	catalog, err := s.LoadCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load tools catalog: %w", err)
	}

	toolSettings := s.getOrgToolSettings(ctx, orgID)

	views := make([]OrgToolView, len(catalog))
	for i, t := range catalog {
		cfg, _ := lookupToolConfig(toolSettings, t)
		if len(cfg.ConfigJSON) == 0 {
			cfg.ConfigJSON = json.RawMessage("{}")
		}

		views[i] = OrgToolView{
			AvailableTool: t,
			Enabled:       cfg.Enabled,
			ConfigJSON:    cfg.ConfigJSON,
		}
	}
	return views, nil
}

func (s *ToolsStore) getOrgToolSettings(ctx context.Context, orgID int64) map[string]OrgToolConfig {
	settingsMap := make(map[string]OrgToolConfig)
	if s.db == nil {
		return settingsMap
	}

	var settingsJSON []byte
	err := s.db.QueryRowContext(ctx, "SELECT settings->'tools' FROM public.orgs WHERE id = $1", orgID).Scan(&settingsJSON)
	if err != nil || len(settingsJSON) == 0 {
		return settingsMap
	}

	var rawMap map[string]interface{}
	if err := json.Unmarshal(settingsJSON, &rawMap); err == nil {
		return parseToolConfigMap(rawMap)
	}
	return settingsMap
}

func parseRepoToolSettingsJSON(settingsJSON []byte) (RepoToolSettings, error) {
	res := RepoToolSettings{
		Mode:  RepoToolModeInherit,
		Tools: map[string]OrgToolConfig{},
	}
	if len(settingsJSON) == 0 {
		return res, nil
	}

	var raw struct {
		Mode  string                 `json:"mode"`
		Tools map[string]interface{} `json:"tools"`
	}
	if err := json.Unmarshal(settingsJSON, &raw); err != nil {
		return RepoToolSettings{}, fmt.Errorf("unmarshal repository tool settings: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(raw.Mode), RepoToolModeCustom) {
		res.Mode = RepoToolModeCustom
	}
	if len(raw.Tools) > 0 {
		res.Tools = parseToolConfigMap(raw.Tools)
	}
	return res, nil
}

// GetRepoToolSettings loads the repository-level static analysis override settings
// from repositories.settings->'tools', scoped strictly to orgID.
func (s *ToolsStore) GetRepoToolSettings(ctx context.Context, orgID, repoID int64) (RepoToolSettings, error) {
	if s.db == nil || repoID <= 0 {
		return RepoToolSettings{
			Mode:  RepoToolModeInherit,
			Tools: map[string]OrgToolConfig{},
		}, nil
	}

	var settingsJSON []byte
	err := s.db.QueryRowContext(
		ctx,
		`SELECT settings->'tools' FROM public.repositories WHERE id = $1 AND org_id = $2`,
		repoID, orgID,
	).Scan(&settingsJSON)
	if err != nil {
		return RepoToolSettings{}, err
	}
	return parseRepoToolSettingsJSON(settingsJSON)
}

// GetAvailableToolsForRepo returns the catalog annotated with both org default state
// and repository-level overrides for a given repository in orgID.
func (s *ToolsStore) GetAvailableToolsForRepo(ctx context.Context, orgID, repoID int64) (string, []RepoToolView, error) {
	repoSettings, err := s.GetRepoToolSettings(ctx, orgID, repoID)
	if err != nil {
		return "", nil, err
	}

	catalog, err := s.LoadCatalog(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to load tools catalog: %w", err)
	}

	orgSettings := s.getOrgToolSettings(ctx, orgID)

	views := make([]RepoToolView, len(catalog))
	for i, t := range catalog {
		orgCfg, _ := lookupToolConfig(orgSettings, t)
		repoCfg, hasRepo := lookupToolConfig(repoSettings.Tools, t)

		effective := orgCfg.Enabled
		if repoSettings.Mode == RepoToolModeCustom {
			effective = hasRepo && repoCfg.Enabled
		}

		var repoEnabled *bool
		if hasRepo {
			v := repoCfg.Enabled
			repoEnabled = &v
		}

		cfgJSON := orgCfg.ConfigJSON
		if hasRepo && len(repoCfg.ConfigJSON) > 0 {
			cfgJSON = repoCfg.ConfigJSON
		}
		if len(cfgJSON) == 0 {
			cfgJSON = json.RawMessage("{}")
		}

		views[i] = RepoToolView{
			AvailableTool: t,
			Enabled:       effective,
			OrgEnabled:    orgCfg.Enabled,
			RepoEnabled:   repoEnabled,
			ConfigJSON:    cfgJSON,
		}
	}
	return repoSettings.Mode, views, nil
}

func cleanGlobs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, g := range in {
		if trimmed := strings.TrimSpace(g); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// UpsertRepoToolSettings persists per-repository tool mode ('inherit' or 'custom')
// and optional explicit tool map into repositories.settings->'tools', scoped to orgID.
func (s *ToolsStore) UpsertRepoToolSettings(ctx context.Context, orgID, repoID int64, mode string, toolStates map[string]bool) (RepoToolSettings, error) {
	return s.UpsertRepoToolSettingsWithConfigs(ctx, orgID, repoID, mode, toolStates, nil)
}

// UpsertRepoToolSettingsWithConfigs persists per-repository tool mode, enabled map,
// and optional per-tool filter configs (include/exclude) into repositories.settings->'tools'.
func (s *ToolsStore) UpsertRepoToolSettingsWithConfigs(ctx context.Context, orgID, repoID int64, mode string, toolStates map[string]bool, toolConfigs map[string]RepoToolRuleConfig) (RepoToolSettings, error) {
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if normalizedMode != RepoToolModeInherit && normalizedMode != RepoToolModeCustom {
		return RepoToolSettings{}, fmt.Errorf("invalid mode %q: must be %q or %q", mode, RepoToolModeInherit, RepoToolModeCustom)
	}
	if s.db == nil || repoID <= 0 {
		return RepoToolSettings{}, errors.New("valid repository and database connection are required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RepoToolSettings{}, fmt.Errorf("begin repository tool settings transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var settingsJSON []byte
	if err := tx.QueryRowContext(
		ctx,
		`SELECT settings->'tools' FROM public.repositories WHERE id = $1 AND org_id = $2 FOR UPDATE`,
		repoID, orgID,
	).Scan(&settingsJSON); err != nil {
		return RepoToolSettings{}, err
	}
	existing, err := parseRepoToolSettingsJSON(settingsJSON)
	if err != nil {
		return RepoToolSettings{}, err
	}

	nextTools := make(map[string]OrgToolConfig, len(existing.Tools))
	for k, v := range existing.Tools {
		nextTools[k] = v
	}

	var normalizedConfigs map[string]RepoToolRuleConfig
	if toolConfigs != nil {
		normalizedConfigs = make(map[string]RepoToolRuleConfig, len(toolConfigs))
		for k, v := range toolConfigs {
			normalizedConfigs[strings.ToLower(strings.TrimSpace(k))] = RepoToolRuleConfig{
				Include: cleanGlobs(v.Include),
				Exclude: cleanGlobs(v.Exclude),
			}
		}
	}

	if toolStates != nil || normalizedConfigs != nil {
		catalog, catErr := s.LoadCatalog(ctx)
		if catErr != nil {
			return RepoToolSettings{}, fmt.Errorf("failed to load tools catalog: %w", catErr)
		}
		normalizedInput := make(map[string]bool, len(toolStates))
		for k, v := range toolStates {
			normalizedInput[strings.ToLower(strings.TrimSpace(k))] = v
		}
		// Rebuild when custom states are explicitly supplied so tools omitted from
		// the custom selection snapshot do not keep stale enabled flags.
		if normalizedMode == RepoToolModeCustom || len(normalizedInput) > 0 {
			nextTools = make(map[string]OrgToolConfig, len(catalog))
			for _, t := range catalog {
				lowerName := strings.ToLower(t.Name)
				idKey := fmt.Sprintf("%d", t.ID)
				prevCfg, _ := lookupToolConfig(existing.Tools, t)
				cfgJSON := prevCfg.ConfigJSON
				if len(cfgJSON) == 0 {
					cfgJSON = json.RawMessage("{}")
				}
				if normalizedConfigs != nil {
					rc, hasRC := normalizedConfigs[lowerName]
					if !hasRC {
						rc, hasRC = normalizedConfigs[idKey]
					}
					if hasRC && (len(rc.Include) > 0 || len(rc.Exclude) > 0) {
						if b, mErr := json.Marshal(rc); mErr == nil {
							cfgJSON = b
						}
					} else {
						cfgJSON = json.RawMessage("{}")
					}
				}
				enabled := prevCfg.Enabled
				if toolStates != nil {
					var ok bool
					enabled, ok = normalizedInput[lowerName]
					if !ok {
						enabled = normalizedInput[idKey]
					}
				}
				nextTools[lowerName] = OrgToolConfig{
					Enabled:    enabled,
					ConfigJSON: cfgJSON,
				}
			}
		}
	}

	updated := RepoToolSettings{
		Mode:  normalizedMode,
		Tools: nextTools,
	}
	payloadBytes, err := json.Marshal(updated)
	if err != nil {
		return RepoToolSettings{}, fmt.Errorf("marshal repo tool settings: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE public.repositories
		SET settings = jsonb_set(
			COALESCE(settings, '{}'::jsonb),
			'{tools}',
			$3::jsonb
		),
		updated_at = now()
		WHERE id = $1 AND org_id = $2
	`, repoID, orgID, payloadBytes)
	if err != nil {
		return RepoToolSettings{}, fmt.Errorf("failed to update repository tool settings: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return RepoToolSettings{}, fmt.Errorf("check rows affected for repository tool settings: %w", err)
	}
	if n == 0 {
		return RepoToolSettings{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return RepoToolSettings{}, fmt.Errorf("commit repository tool settings: %w", err)
	}
	return updated, nil
}

// UpsertOrgTool inserts or updates the enabling configuration of a tool for a specific organization in orgs.settings.
func (s *ToolsStore) UpsertOrgTool(ctx context.Context, orgID, toolID int64, enabled bool) (OrgToolRow, error) {
	catalog, err := s.LoadCatalog(ctx)
	if err != nil {
		return OrgToolRow{}, err
	}

	var toolName string
	for _, t := range catalog {
		if t.ID == toolID {
			toolName = t.Name
			break
		}
	}
	if toolName == "" {
		return OrgToolRow{}, sql.ErrNoRows
	}

	lowerName := strings.ToLower(toolName)
	toolSettings := s.getOrgToolSettings(ctx, orgID)
	existingCfg := toolSettings[lowerName]
	if len(existingCfg.ConfigJSON) == 0 {
		existingCfg.ConfigJSON = json.RawMessage("{}")
	}
	existingCfg.Enabled = enabled

	cfgBytes, err := json.Marshal(existingCfg)
	if err != nil {
		return OrgToolRow{}, fmt.Errorf("marshal tool settings: %w", err)
	}

	query := `
		UPDATE public.orgs
		SET settings = jsonb_set(
			COALESCE(settings, '{}'::jsonb),
			'{tools}',
			COALESCE(settings->'tools', '{}'::jsonb) || jsonb_build_object($2::text, $3::jsonb)
		)
		WHERE id = $1
	`
	_, err = s.db.ExecContext(ctx, query, orgID, lowerName, cfgBytes)
	if err != nil {
		return OrgToolRow{}, fmt.Errorf("failed to update org tool settings: %w", err)
	}

	return OrgToolRow{
		OrgID:      orgID,
		ToolID:     toolID,
		Enabled:    enabled,
		ConfigJSON: existingCfg.ConfigJSON,
	}, nil
}

// GetEnabledToolsForOrg returns the catalog details of all tools that have been explicitly enabled by the org.
func (s *ToolsStore) GetEnabledToolsForOrg(ctx context.Context, orgID int64) ([]AvailableTool, error) {
	catalog, err := s.LoadCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load tools catalog: %w", err)
	}

	toolSettings := s.getOrgToolSettings(ctx, orgID)
	var enabledTools []AvailableTool
	for _, t := range catalog {
		lowerName := strings.ToLower(t.Name)
		cfg, ok := toolSettings[lowerName]
		if !ok {
			cfg = toolSettings[fmt.Sprintf("%d", t.ID)]
		}
		if cfg.Enabled {
			enabledTools = append(enabledTools, t)
		}
	}
	return enabledTools, nil
}

// GetAvailableToolByName fetches a tool from the catalog by name.
func (s *ToolsStore) GetAvailableToolByName(ctx context.Context, name string) (*AvailableTool, error) {
	catalog, err := s.LoadCatalog(ctx)
	if err != nil {
		return nil, err
	}

	target := strings.ToLower(strings.TrimSpace(name))
	for _, t := range catalog {
		if strings.ToLower(t.Name) == target {
			toolCopy := t
			return &toolCopy, nil
		}
	}
	return nil, nil
}

// InsertToolResultEvent wraps raw Lambda response and logs it in review_events table.
func (s *ToolsStore) InsertToolResultEvent(ctx context.Context, reviewID, orgID, toolID int64, toolName string, resultJSON []byte) error {
	var parsedFindings []ToolFinding
	trimmedJSON := strings.TrimSpace(string(resultJSON))
	var exitCode int
	var stderr string
	var loc int
	var failureReason string

	if strings.HasPrefix(trimmedJSON, "[") {
		if err := json.Unmarshal(resultJSON, &parsedFindings); err != nil {
			log.Printf("[WARN] StoreToolResultEvent: failed to parse array findings for tool %d: %v", toolID, err)
			parsedFindings = []ToolFinding{}
			stderr = fmt.Sprintf("failed to parse array findings: %v", err)
			exitCode = -1
			failureReason = stderr
		} else if len(parsedFindings) > 0 {
			exitCode = 1
		} else {
			exitCode = 0
		}
	} else {
		type ToolLambdaResponse struct {
			ExitCode      int             `json:"exit_code"`
			Findings      json.RawMessage `json:"findings"`
			LinesOfCode   int             `json:"lines_of_code"`
			Stderr        string          `json:"stderr"`
			FailureReason string          `json:"failure_reason,omitempty"`
		}

		var resp ToolLambdaResponse
		if err := json.Unmarshal(resultJSON, &resp); err != nil {
			stderr = fmt.Sprintf("failed to parse lambda response: %v. Raw: %s", err, string(resultJSON))
			exitCode = -1
			failureReason = stderr
		} else {
			exitCode = resp.ExitCode
			stderr = resp.Stderr
			loc = resp.LinesOfCode
			failureReason = resp.FailureReason
			if len(resp.Findings) > 0 {
				if errFindings := json.Unmarshal(resp.Findings, &parsedFindings); errFindings != nil {
					log.Printf("[WARN] StoreToolResultEvent: failed to unmarshal findings for tool %d: %v", toolID, errFindings)
				}
			}
		}
	}

	// Redact plaintext secrets across all finding fields before persisting to database event log
	for idx := range parsedFindings {
		parsedFindings[idx].Secret = "[REDACTED]"
		parsedFindings[idx].CodeSnippet = "[REDACTED]"

		parsedFindings[idx].Message = redactMatchDetails(parsedFindings[idx].Message)
		parsedFindings[idx].Extra.Message = redactMatchDetails(parsedFindings[idx].Extra.Message)
	}

	eventData := ToolResultEventData{
		ToolID:        toolID,
		ToolName:      toolName,
		ExitCode:      exitCode,
		Findings:      parsedFindings,
		LinesOfCode:   loc,
		Stderr:        stderr,
		FailureReason: failureReason,
	}

	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal tool result event data: %w", err)
	}

	query := `
		INSERT INTO public.review_events (review_id, org_id, event_type, level, data)
		VALUES ($1, $2, 'tool_result', 'info', $3)
	`
	_, err = s.db.ExecContext(ctx, query, reviewID, orgID, eventDataBytes)
	if err != nil {
		return fmt.Errorf("failed to insert tool result review event: %w", err)
	}
	return nil
}

func redactMatchDetails(msg string) string {
	msg = strings.TrimSpace(msg)
	for _, pattern := range []string{" (Match:", "(Match:", " Match:"} {
		if idx := strings.Index(msg, pattern); idx != -1 {
			msg = strings.TrimSpace(msg[:idx])
		}
	}
	// Redact plaintext token matches if embedded in message
	if strings.Contains(msg, "AKIA") || strings.Contains(msg, "ghp_") || strings.Contains(msg, "sk_") {
		msg = strings.TrimSpace(msg) + " [REDACTED]"
	}
	return msg
}

type ToolFinding struct {
	File        string `json:"file"`
	FilePath    string `json:"file_path"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	LineNumber  int    `json:"line_number"`
	Start       struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"start"`
	Col         int    `json:"col"`
	Rule        string `json:"rule"`
	RuleID      string `json:"rule_id"`
	CheckID     string `json:"check_id"`
	Message     string `json:"message"`
	Extra       struct {
		Message  string `json:"message"`
		Severity string `json:"severity"`
	} `json:"extra"`
	Secret      string `json:"secret,omitempty"`
	CodeSnippet string `json:"code_snippet,omitempty"`
}

type ToolResultEventData struct {
	ToolID        int64         `json:"tool_id"`
	ToolName      string        `json:"tool_name"`
	ExitCode      int           `json:"exit_code"`
	Findings      []ToolFinding `json:"findings"`
	LinesOfCode   int           `json:"lines_of_code"`
	Stderr        string        `json:"stderr"`
	FailureReason string        `json:"failure_reason,omitempty"`
}

func (s *ToolsStore) GetToolResultsForReview(ctx context.Context, reviewID int64) ([]ToolResultEventData, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT data 
		FROM public.review_events 
		WHERE review_id = $1 AND event_type = 'tool_result'
	`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ToolResultEventData
	for rows.Next() {
		var rawData []byte
		if err := rows.Scan(&rawData); err != nil {
			return nil, err
		}
		var data ToolResultEventData
		if err := json.Unmarshal(rawData, &data); err != nil {
			return nil, err
		}
		results = append(results, data)
	}
	return results, nil
}
