package tools

import (
	"context"
	"database/sql"
	"encoding/json"
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

// defaultCatalog provides fallback tool definitions if system_settings tools_catalog row is not yet set.
// LambdaARNs are managed in the lr-tools repository and configured dynamically via system_settings/database.
var defaultCatalog = []AvailableTool{
	{ID: 1, Name: "ruff", Description: "Fast Python linter and formatter", LambdaARN: "", Multiplier: 1.0, UseCase: "Python lint/format"},
	{ID: 2, Name: "openapi", Description: "OpenAPI and YAML specification validation", LambdaARN: "", Multiplier: 1.0, UseCase: "OpenAPI/YAML validation"},
	{ID: 3, Name: "osv", Description: "Google open-source vulnerability scanner", LambdaARN: "", Multiplier: 1.0, UseCase: "Dependencies & Supply Chain"},
	{ID: 4, Name: "gitleaks", Description: "Git history secret and credential scanner", LambdaARN: "", Multiplier: 12.0, UseCase: "Secrets & Security"},
	{ID: 5, Name: "trivy", Description: "Container and SCA vulnerability scanner", LambdaARN: "", Multiplier: 4.0, UseCase: "Dependencies & Supply Chain"},
	{ID: 6, Name: "trufflehog", Description: "Deep entropy-based secret detection", LambdaARN: "", Multiplier: 12.0, UseCase: "Secrets & Security"},
	{ID: 7, Name: "tfsec", Description: "Terraform security and misconfiguration scanner", LambdaARN: "", Multiplier: 1.0, UseCase: "Infrastructure & Config"},
	{ID: 8, Name: "hadolint", Description: "Dockerfile best practices linter", LambdaARN: "", Multiplier: 1.0, UseCase: "Containers & Docker"},
	{ID: 9, Name: "actionlint", Description: "GitHub Actions workflow linter", LambdaARN: "", Multiplier: 1.0, UseCase: "CI/CD & Workflow"},
	{ID: 10, Name: "golangci-lint", Description: "Go meta-linter suite", LambdaARN: "", Multiplier: 12.0, UseCase: "Go Code Quality"},
	{ID: 11, Name: "spectral", Description: "API style guide enforcement linter", LambdaARN: "", Multiplier: 3.0, UseCase: "API Schema"},
	{ID: 12, Name: "bandit", Description: "Python security scanner (SAST)", LambdaARN: "", Multiplier: 12.0, UseCase: "Python SAST"},
	{ID: 13, Name: "detect-secrets", Description: "Baseline pattern-based secrets detection", LambdaARN: "", Multiplier: 12.0, UseCase: "Secrets & Security"},
	{ID: 14, Name: "semgrep", Description: "Fast multi-language SAST analyzer", LambdaARN: "", Multiplier: 12.0, UseCase: "Security & Code Quality"},
	{ID: 15, Name: "eslint", Description: "JavaScript / TypeScript linter", LambdaARN: "", Multiplier: 12.0, UseCase: "JS/TS Code Quality"},
}

// LoadCatalog loads tools catalog from system_settings row 'tools_catalog', falling back to defaultCatalog.
func (s *ToolsStore) LoadCatalog(ctx context.Context) ([]AvailableTool, error) {
	tools := make([]AvailableTool, len(defaultCatalog))
	copy(tools, defaultCatalog)

	if s.db == nil {
		return tools, nil
	}

	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM public.system_settings WHERE name = 'tools_catalog'").Scan(&data)
	if err == sql.ErrNoRows {
		return tools, nil
	}
	if err != nil {
		return tools, nil // Fail open to default catalog if table/row error
	}

	// Unmarshal overrides
	var overrides []AvailableTool
	if unmarshalErr := json.Unmarshal(data, &overrides); unmarshalErr == nil && len(overrides) > 0 {
		return overrides, nil
	}

	// Map of name -> AvailableTool shape
	var mapOverrides map[string]AvailableTool
	if unmarshalErr := json.Unmarshal(data, &mapOverrides); unmarshalErr == nil && len(mapOverrides) > 0 {
		for i, t := range tools {
			if o, ok := mapOverrides[t.Name]; ok {
				if o.LambdaARN != "" {
					tools[i].LambdaARN = o.LambdaARN
				}
				if o.Multiplier > 0 {
					tools[i].Multiplier = o.Multiplier
				}
				if o.Description != "" {
					tools[i].Description = o.Description
				}
				if o.UseCase != "" {
					tools[i].UseCase = o.UseCase
				}
			}
		}
	}

	return tools, nil
}

type OrgToolConfig struct {
	Enabled    bool            `json:"enabled"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`
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
		lowerName := strings.ToLower(t.Name)
		cfg, ok := toolSettings[lowerName]
		if !ok {
			cfg = toolSettings[fmt.Sprintf("%d", t.ID)]
		}
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
	}
	return settingsMap
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

	if strings.HasPrefix(trimmedJSON, "[") {
		if err := json.Unmarshal(resultJSON, &parsedFindings); err != nil {
			log.Printf("[WARN] StoreToolResultEvent: failed to parse array findings for tool %d: %v", toolID, err)
			parsedFindings = []ToolFinding{}
			stderr = fmt.Sprintf("failed to parse array findings: %v", err)
			exitCode = -1
		} else if len(parsedFindings) > 0 {
			exitCode = 1
		} else {
			exitCode = 0
		}
	} else {
		type ToolLambdaResponse struct {
			ExitCode    int             `json:"exit_code"`
			Findings    json.RawMessage `json:"findings"`
			LinesOfCode int             `json:"lines_of_code"`
			Stderr      string          `json:"stderr"`
		}

		var resp ToolLambdaResponse
		if err := json.Unmarshal(resultJSON, &resp); err != nil {
			stderr = fmt.Sprintf("failed to parse lambda response: %v. Raw: %s", err, string(resultJSON))
			exitCode = -1
		} else {
			exitCode = resp.ExitCode
			stderr = resp.Stderr
			loc = resp.LinesOfCode
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
		ToolID:      toolID,
		ToolName:    toolName,
		ExitCode:    exitCode,
		Findings:    parsedFindings,
		LinesOfCode: loc,
		Stderr:      stderr,
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
	ToolID      int64         `json:"tool_id"`
	ToolName    string        `json:"tool_name"`
	ExitCode    int           `json:"exit_code"`
	Findings    []ToolFinding `json:"findings"`
	LinesOfCode int           `json:"lines_of_code"`
	Stderr      string        `json:"stderr"`
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
