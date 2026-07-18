package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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

// GetAvailableToolsForOrg lists all available tools from the catalog, annotated with the org's enabling configuration.
func (s *ToolsStore) GetAvailableToolsForOrg(ctx context.Context, orgID int64) ([]OrgToolView, error) {
	query := `
		SELECT
			t.id,
			t.name,
			t.description,
			t.lambda_arn,
			t.multiplier,
			t.use_case,
			COALESCE(ot.enabled, false) AS enabled,
			COALESCE(ot.config_json, '{}'::jsonb) AS config_json
		FROM public.available_tools t
		LEFT JOIN public.org_tools ot ON t.id = ot.tool_id AND ot.org_id = $1
		ORDER BY t.name
	`
	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query available tools for org %d: %w", orgID, err)
	}
	defer rows.Close()

	var views []OrgToolView
	for rows.Next() {
		var v OrgToolView
		var configBytes []byte
		err := rows.Scan(
			&v.ID,
			&v.Name,
			&v.Description,
			&v.LambdaARN,
			&v.Multiplier,
			&v.UseCase,
			&v.Enabled,
			&configBytes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan org tool view: %w", err)
		}
		v.ConfigJSON = json.RawMessage(configBytes)
		views = append(views, v)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating org tool views: %w", err)
	}
	return views, nil
}

// UpsertOrgTool inserts or updates the enabling configuration of a tool for a specific organization.
func (s *ToolsStore) UpsertOrgTool(ctx context.Context, orgID, toolID int64, enabled bool) (OrgToolRow, error) {
	// First check if the tool actually exists in available_tools
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM public.available_tools WHERE id = $1)", toolID).Scan(&exists)
	if err != nil {
		return OrgToolRow{}, fmt.Errorf("failed to check tool existence: %w", err)
	}
	if !exists {
		return OrgToolRow{}, sql.ErrNoRows
	}

	query := `
		INSERT INTO public.org_tools (org_id, tool_id, enabled, config_json, updated_at)
		VALUES ($1, $2, $3, '{}'::jsonb, NOW())
		ON CONFLICT (org_id, tool_id) DO UPDATE
		  SET enabled = EXCLUDED.enabled,
		      updated_at = NOW()
		RETURNING org_id, tool_id, enabled, config_json
	`
	var r OrgToolRow
	var configBytes []byte
	err = s.db.QueryRowContext(ctx, query, orgID, toolID, enabled).Scan(
		&r.OrgID,
		&r.ToolID,
		&r.Enabled,
		&configBytes,
	)
	if err != nil {
		return OrgToolRow{}, fmt.Errorf("failed to upsert org tool: %w", err)
	}
	r.ConfigJSON = json.RawMessage(configBytes)
	return r, nil
}

// GetEnabledToolsForOrg returns the catalog details of all tools that have been explicitly enabled by the org.
func (s *ToolsStore) GetEnabledToolsForOrg(ctx context.Context, orgID int64) ([]AvailableTool, error) {
	query := `
		SELECT
			t.id,
			t.name,
			t.description,
			t.lambda_arn,
			t.multiplier,
			t.use_case
		FROM public.available_tools t
		JOIN public.org_tools ot ON t.id = ot.tool_id
		WHERE ot.org_id = $1 AND ot.enabled = true
		ORDER BY t.name
	`
	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled tools for org %d: %w", orgID, err)
	}
	defer rows.Close()

	var tools []AvailableTool
	for rows.Next() {
		var t AvailableTool
		err := rows.Scan(
			&t.ID,
			&t.Name,
			&t.Description,
			&t.LambdaARN,
			&t.Multiplier,
			&t.UseCase,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan enabled tool: %w", err)
		}
		tools = append(tools, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating enabled tools: %w", err)
	}
	return tools, nil
}

// InsertToolResultEvent wraps raw Lambda response and logs it in review_events table.
func (s *ToolsStore) InsertToolResultEvent(ctx context.Context, reviewID, orgID, toolID int64, toolName string, resultJSON []byte) error {
	type ToolLambdaResponse struct {
		ExitCode    int             `json:"exit_code"`
		Findings    json.RawMessage `json:"findings"`
		LinesOfCode int             `json:"lines_of_code"`
		Stderr      string          `json:"stderr"`
	}

	var resp ToolLambdaResponse
	// If unmarshaling fails or findings is nil, initialize it with empty array
	if err := json.Unmarshal(resultJSON, &resp); err != nil {
		resp.Stderr = fmt.Sprintf("failed to parse lambda response: %v. Raw: %s", err, string(resultJSON))
		resp.ExitCode = -1
	}
	if len(resp.Findings) == 0 {
		resp.Findings = json.RawMessage("[]")
	}

	var parsedFindings []ToolFinding
	if err := json.Unmarshal(resp.Findings, &parsedFindings); err != nil {
		// If parsing fails, store an empty slice rather than corrupting the event
		log.Printf("[WARN] StoreToolResultEvent: failed to parse findings for tool %d: %v", toolID, err)
		parsedFindings = []ToolFinding{}
	}

	eventData := ToolResultEventData{
		ToolID:      toolID,
		ToolName:    toolName,
		ExitCode:    resp.ExitCode,
		Findings:    parsedFindings,
		LinesOfCode: resp.LinesOfCode,
		Stderr:      resp.Stderr,
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

type ToolFinding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
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
