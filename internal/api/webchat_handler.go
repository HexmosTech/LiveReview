package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/chatstats"
	"github.com/livereview/internal/livisql"
	"github.com/livereview/internal/mcpagent"
	"github.com/livereview/internal/vlrender"
	storageanalytics "github.com/livereview/storage/analytics"
	storagechat "github.com/livereview/storage/chat"
	"github.com/rs/zerolog/log"
)

type WebChatRequest struct {
	Message        string `json:"message"`
	ConversationID *int64 `json:"conversationId,omitempty"`
}

// WebChatChart is a chart report the frontend renders itself with
// react-vega/vega-embed: unlike the old PNG path, Spec is the normalized
// Vega-Lite spec verbatim, so the browser gets full interactivity (tooltips,
// hover, legend filtering) instead of a flat image.
type WebChatChart struct {
	Title       string                `json:"title,omitempty"`
	Description string                `json:"description,omitempty"`
	Query       string                `json:"query,omitempty"`
	TimeRange   string                `json:"time_range,omitempty"`
	Granularity string                `json:"granularity,omitempty"`
	Context     vlrender.ChartContext `json:"context"`
	Spec        json.RawMessage       `json:"spec"`
	// Stats holds the precomputed KPI chips for this chart (Total/Avg per
	// period/Peak/Low/Trend etc.) - see internal/chatstats.ComputeAllStats.
	// Nil when the spec doesn't match any of the shapes chatstats knows how
	// to summarize.
	Stats json.RawMessage `json:"stats,omitempty"`
	// ID is only set for a chart loaded back from persistence (GetConversation,
	// RenderChart) - a chart freshly produced within a live turn has none yet.
	ID int64 `json:"id,omitempty"`
}

type WebChatResponse struct {
	Response           string                            `json:"response"`
	Charts             []WebChatChart                    `json:"charts,omitempty"`
	Files              []WebChatFile                     `json:"files,omitempty"`
	SuggestedQuestions []mcpagent.SuggestedQuestionCategory `json:"suggested_questions,omitempty"`
	DebugArtifacts     json.RawMessage                   `json:"debug_artifacts,omitempty"`
	SessionID          string                            `json:"sessionId,omitempty"`
	ConversationID     int64                             `json:"conversationId"`
}

// analyticsRoleFor maps permissions onto the SQL catalog's roles. Super admins
// and owners see billing tables; everyone else sees only review data.
func analyticsRoleFor(pc *auth.PermissionContext) string {
	switch {
	case pc.IsSuperAdmin:
		return string(livisql.RoleSuperAdmin)
	case pc.IsOwner:
		return string(livisql.RoleOwner)
	default:
		return string(livisql.RoleMember)
	}
}

// newChatSessionID mints a random session id for correlating debug log
// lines across one chat conversation's turns (see internal/logging.ChatTurnLogger).
func newChatSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) HandleWebChat(c echo.Context) error {
	var req WebChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Message == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "message is required"})
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil || pc.User == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	orgID := pc.OrgID
	userID := pc.User.ID
	ctx := c.Request().Context()
	chatStore := storagechat.NewStore(s.db)

	// Resolve (or create) the conversation this turn belongs to, and load its
	// prior turns to feed the agent — the client no longer round-trips history.
	var convID int64
	var sessionID string
	var err error
	if req.ConversationID != nil {
		conv, convErr := chatStore.GetConversation(ctx, orgID, userID, *req.ConversationID)
		if convErr != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
		}
		convID = conv.ID
		sessionID = conv.SessionID
	} else {
		sessionID = newChatSessionID()
		convID, err = chatStore.CreateConversation(ctx, orgID, userID, titleFromFirstMessage(req.Message), sessionID, storagechat.SurfaceChat)
		if err != nil {
			log.Error().Err(err).Msg("WebChat: failed to create conversation")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start conversation"})
		}
	}
	if sessionID == "" {
		sessionID = newChatSessionID()
	}

	priorMessages, err := chatStore.ListMessages(ctx, orgID, userID, convID)
	if err != nil {
		log.Error().Err(err).Msg("WebChat: failed to load conversation history")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
	}
	var history []mcpagent.HistoryEntry
	for _, m := range priorMessages {
		history = append(history, m.RawHistoryEntries...)
	}

	// Persist the user's question the moment it reaches this handler -
	// before the connector, MCP session, or agent loop get a chance to
	// fail, and before a slow turn gives the client a chance to disconnect.
	// A turn can take 20-40s end to end; without this, a client that gives
	// up (or a downstream failure) before the reply was ready lost the
	// question too, since the old path only persisted user+assistant
	// together in one transaction at the very end. Best-effort: a failure
	// here is logged but does not block the turn, since the reply is still
	// worth attempting even if this write had trouble.
	if _, _, err := chatStore.AppendUserMessage(ctx, convID, storagechat.MessageInput{
		Role:              "user",
		Content:           req.Message,
		RawHistoryEntries: []mcpagent.HistoryEntry{{"role": "user", "content": req.Message}},
	}); err != nil {
		log.Error().Err(err).Int64("conversation_id", convID).Msg("WebChat: failed to persist user message")
	}

	// persistReply saves the assistant's side of this turn - success or
	// error text alike - so reopening the conversation later always shows
	// what happened, never a question with no visible outcome. Best-effort
	// past this point: a persistence failure is logged, never surfaced to
	// the user as a turn failure, since the reply already computed (or the
	// error already known) is the thing that matters to return now.
	persistReply := func(assistantText string, charts []WebChatChart, artifacts []mcpagent.Artifact, turnEntries []mcpagent.HistoryEntry, rawLLMOutput string, debugArt *mcpagent.DebugArtifacts) []int64 {
		var debugJSON json.RawMessage
		if debugArt != nil {
			debugJSON, _ = json.Marshal(debugArt)
		}
		fileIDs, err := persistAssistantMessage(ctx, chatStore, convID, req.Message, assistantText, turnEntries, charts, artifacts, rawLLMOutput, debugJSON)
		if err != nil {
			log.Error().Err(err).Int64("conversation_id", convID).Msg("WebChat: failed to persist assistant message")
		}
		return fileIDs
	}

	connector, err := s.resolveOrgConnector(ctx, orgID)
	if err != nil {
		userFriendlyErr := "No active AI Connector configured for your organization. Please add or enable an AI Model Connector in Settings → AI Settings."
		if !strings.Contains(err.Error(), "no AI connectors") {
			userFriendlyErr = fmt.Sprintf("AI Connector error: %s. Please check your AI model settings.", err.Error())
		}
		persistReply(userFriendlyErr, nil, nil, nil, "", nil)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": userFriendlyErr})
	}

	mcpURL := resolveMCPBaseURL(s.db)
	maxSteps := 20

	authHeader := c.Request().Header.Get("Authorization")
	mcpHeaders := map[string]string{}
	if authHeader != "" {
		mcpHeaders["Authorization"] = authHeader
	}
	if orgCtx := c.Request().Header.Get("X-Org-Context"); orgCtx != "" {
		mcpHeaders["X-Org-Context"] = orgCtx
	}

	mcpSession, err := mcpagent.ConnectMCP(ctx, mcpURL, mcpHeaders)
	if err != nil {
		log.Error().Err(err).Str("url", mcpURL).Msg("WebChat: failed to connect to MCP server")
		errText := fmt.Sprintf("Internal AI Service Error: Unable to connect to MCP tools (%s).", err.Error())
		persistReply(errText, nil, nil, nil, "", nil)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": errText})
	}

	if pc.CurrentOrg != nil && pc.CurrentOrg.Name != "" {
		mcpSession.OrgName = pc.CurrentOrg.Name
	}
	if pc.User != nil {
		mcpSession.UserName = pc.User.FullName()
	}
	// Scope for generated SQL. Derived from the permission booleans rather than
	// pc.Role, which is a free-form label; getting this wrong would let a member
	// read billing data that the REST tools gate behind ownership.
	mcpSession.OrgID = orgID
	mcpSession.UserRole = analyticsRoleFor(pc)

	provider := mcpagent.NewProvider(connector)
	agent := mcpagent.NewAgent(provider, mcpSession, maxSteps).
		WithAnalytics(storageanalytics.NewAdHocStore(s.db))

	responseText, updatedHistory, artifacts, debugArt, err := agent.RunTurnWithArtifacts(ctx, history, req.Message, sessionID, "livi")
	if err != nil {
		log.Error().Err(err).Msg("WebChat: agent loop failed")
		errText := fmt.Sprintf("Agent loop failed: %s", err.Error())
		persistReply(errText, nil, nil, nil, "", nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": errText})
	}
	// Whatever RunTurnWithArtifacts appended on top of the history we fed it
	// (possibly including a freshly swapped-in system prompt entry - see
	// mcpagent/agent.go's per-turn prompt selection) is this turn's new
	// content. The user entry within it was already persisted above by
	// AppendUserMessage before the call; only the assistant entries get
	// saved below.
	turnEntries := updatedHistory[len(history):]

	resp := WebChatResponse{
		Response:       responseText,
		SessionID:      sessionID,
		ConversationID: convID,
	}

	for _, entry := range turnEntries {
		if sq, ok := entry["suggested_questions"].([]mcpagent.SuggestedQuestionCategory); ok && len(sq) > 0 {
			resp.SuggestedQuestions = sq
			break
		} else if rawSq, ok := entry["suggested_questions"]; ok && rawSq != nil {
			if b, err := json.Marshal(rawSq); err == nil {
				var sq []mcpagent.SuggestedQuestionCategory
				if err := json.Unmarshal(b, &sq); err == nil && len(sq) > 0 {
					resp.SuggestedQuestions = sq
					break
				}
			}
		}
	}

	if vlrender.HasVegaLiteSpec(responseText) {
		charts, cleanText, err := extractChartsFromVega(responseText)
		if err == nil && len(charts) > 0 {
			resp.Charts = charts
			// The Vega-Lite JSON must never surface in the chat. If stripping
			// removes everything, keep only the surrounding text (or empty).
			cleanText = strings.TrimSpace(cleanText)
			resp.Response = cleanText
		} else if errors.Is(err, vlrender.ErrTrivialSpec) {
			// A single value/bar (1 row) is a real answer, not "no data" —
			// show its description/query as plain text instead of a chart.
			desc, query, timeRange, granularity, chartContext, ok := vlrender.TrivialDescription(responseText)
			text := desc
			if query != "" || timeRange != "" || granularity != "" || chartContext != "" {
				if text != "" {
					text += "\n\n"
				}
				detail := "Query: " + query
				if timeRange != "" {
					detail += "\nTime range: " + timeRange
				}
				if granularity != "" {
					detail += "\nGranularity: " + granularity
				}
				if chartContext != "" {
					detail += "\nContext: " + chartContext
				}
				text += detail
			}
			if !ok || text == "" {
				text = mcpagent.NoDataAnalyticsResponseText
				if len(resp.SuggestedQuestions) == 0 {
					resp.SuggestedQuestions = mcpagent.DefaultNoDataSuggestedQuestions
				}
			}
			resp.Response = text
		} else {
			// Extraction failed: never show the raw JSON.
			resp.Response = "Having an issue generating the data, please try again."
		}
	} else if looksLikeTruncatedVegaSpec(responseText) {
		resp.Response = extractDescriptionFromVegaSpec(responseText)
	}

	// Include debug artifacts in the live response for /chat-debug.
	if debugArt != nil {
		if debugJSON, err := json.Marshal(debugArt); err == nil {
			resp.DebugArtifacts = debugJSON
		}
	}

	// Persist the reply (messages + charts + file exports) so the file
	// exports get durable DB ids we can build stable download URLs from.
	// Charts and exports are surfaced on the response regardless, since the
	// files field is driven by the persisted ids below. The user message
	// was already saved by AppendUserMessage before the agent ran.
	fileIDs := persistReply(resp.Response, resp.Charts, artifacts, turnEntries, responseText, debugArt)

	// File downloads are served from the DB, so only files that were actually
	// persisted (got a real id back) are offered. A persistence failure loses
	// the download rather than advertising a link that 404s - the message text
	// stays honest because it is stored alongside, so the two can never drift.
	resp.Files = make([]WebChatFile, 0, len(artifacts))
	for i, art := range artifacts {
		if i >= len(fileIDs) {
			break
		}
		resp.Files = append(resp.Files, chatFileFromArtifact(art, fileIDs[i]))
	}

	return c.JSON(http.StatusOK, resp)
}

// titleFromFirstMessage derives a v1 conversation title by truncating the
// first user message - no extra LLM call. Runes, not bytes, so multi-byte
// text isn't cut mid-character.
func titleFromFirstMessage(message string) string {
	const maxRunes = 80
	trimmed := strings.TrimSpace(message)
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

// persistAssistantMessage saves the reply half of a turn (plus any chart and
// file-export artifacts) via AppendAssistantMessage - the user half was
// already saved by AppendUserMessage before the agent ran. It returns the
// persisted file ids, one per artifact entry, so the caller can build stable
// download URLs - or nil if a file was not persisted.
//
// turnEntries is nil on an early-failure path (connector/MCP/agent-loop
// error, before any LLM history exists) - in that case a single synthetic
// assistant entry is stored so the conversation still replays as valid
// history, matching what a real assistant turn's shape would have been. The
// system-prompt entry mcpagent/agent.go may have prepended ahead of the user
// entry, when turnEntries is populated, is intentionally dropped:
// RunTurnWithArtifacts idempotently re-derives/swaps it on every call
// (agent.go:148-166), so replaying persisted history without it is safe, and
// simpler than tracking which prompt variant was live on a given turn.
func persistAssistantMessage(ctx context.Context, chatStore *storagechat.Store, convID int64, userText, assistantText string, turnEntries []mcpagent.HistoryEntry, charts []WebChatChart, artifacts []mcpagent.Artifact, rawLLMOutput string, debugArtifacts json.RawMessage) ([]int64, error) {
	assistantEntries := []mcpagent.HistoryEntry{{"role": "assistant", "content": assistantText}}
	for i, e := range turnEntries {
		if role, _ := e["role"].(string); role == "user" {
			assistantEntries = turnEntries[i+1:]
			break
		}
	}

	chartInputs := make([]storagechat.ChartInput, 0, len(charts))
	for _, ch := range charts {
		chartInputs = append(chartInputs, storagechat.ChartInput{
			Title:                 ch.Title,
			Description:           ch.Description,
			Query:                 ch.Query,
			TimeRange:             ch.TimeRange,
			Granularity:           ch.Granularity,
			Context:               ch.Context,
			TriggeringUserMessage: userText,
			VegaSpec:              ch.Spec,
			Stats:                 ch.Stats,
			RawLLMOutput:          rawLLMOutput,
		})
	}

	fileInputs := make([]storagechat.FileInput, 0, len(artifacts))
	for _, art := range artifacts {
		fileInputs = append(fileInputs, toFileInput(art))
	}

	if strings.TrimSpace(assistantText) == "" {
		if len(charts) > 0 {
			var titles []string
			for _, c := range charts {
				if c.Title != "" {
					titles = append(titles, c.Title)
				}
			}
			if len(titles) > 0 {
				assistantText = "Rendered charts for: " + strings.Join(titles, ", ") + "."
			} else {
				assistantText = "Rendered analytics charts."
			}
		} else if len(artifacts) > 0 {
			assistantText = "I've put the results in a file you can download."
		}
	}

	for idx, entry := range assistantEntries {
		if role, _ := entry["role"].(string); role == "assistant" {
			content, _ := entry["content"].(string)
			text, _ := entry["text"].(string)
			if strings.TrimSpace(content) == "" && strings.TrimSpace(text) == "" {
				assistantEntries[idx]["content"] = assistantText
				assistantEntries[idx]["text"] = assistantText
			} else if strings.TrimSpace(content) == "" {
				assistantEntries[idx]["content"] = text
			}
		}
	}

	_, fileIDs, err := chatStore.AppendAssistantMessage(ctx, convID,
		storagechat.MessageInput{
			Role:              "assistant",
			Content:           assistantText,
			RawHistoryEntries: assistantEntries,
			DebugArtifacts:    debugArtifacts,
		},
		chartInputs,
		fileInputs,
	)
	return fileIDs, err
}

// extractChartsFromVega pulls the normalized Vega-Lite spec(s) out of the raw
// agent text, for the frontend to render directly (see WebChatChart). This is
// the same shape-detection this package used to hand off to vl-convert for
// PNG rendering, minus the rendering: the browser does that now.
func extractChartsFromVega(text string) ([]WebChatChart, string, error) {
	body := vlrender.ExtractJSONBlock(text)

	var multi struct {
		Reports []vlrender.VegaLiteReport `json:"reports"`
	}
	if err := json.Unmarshal([]byte(body), &multi); err == nil && len(multi.Reports) > 0 {
		charts, err := reportsToCharts(multi.Reports)
		// stripVegaBlocks' brace-scanning heuristic expects prose before the
		// JSON payload and can mismatch on a nested object when the response
		// IS the JSON (no leading prose, as here). We already know the exact
		// substring that parsed as the reports payload, so remove that
		// directly instead of re-scanning for it.
		return charts, strings.TrimSpace(strings.Replace(text, body, "", 1)), err
	}

	var wrapped vlrender.VegaLiteReport
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Spec) > 0 {
		charts, err := reportsToCharts([]vlrender.VegaLiteReport{wrapped})
		if err != nil {
			return nil, text, err
		}
		return charts, stripVegaBlocks(text), nil
	}

	var rawMap map[string]any
	if err := json.Unmarshal([]byte(body), &rawMap); err != nil {
		return nil, text, nil
	}
	if _, ok := rawMap["$schema"]; !ok && rawMap["mark"] == nil && rawMap["layer"] == nil && rawMap["vconcat"] == nil && rawMap["hconcat"] == nil {
		return nil, text, nil
	}
	spec, err := vlrender.NormalizeVegaLiteSpec([]byte(body))
	if err != nil {
		return nil, text, nil
	}
	if vlrender.SpecIsTrivial(spec) {
		return nil, text, vlrender.ErrTrivialSpec
	}
	stats, statsErr := chatstats.ComputeAllStats(spec)
	if statsErr != nil {
		log.Warn().Err(statsErr).Msg("chart stats computation failed for text-extracted spec, continuing without stats")
	}
	return []WebChatChart{{Title: "LiveReview Chart", Spec: json.RawMessage(spec), Stats: stats}}, stripVegaBlocks(text), nil
}

func reportsToCharts(reports []vlrender.VegaLiteReport) ([]WebChatChart, error) {
	var charts []WebChatChart
	trivialOnly := true
	for _, r := range reports {
		spec, err := vlrender.NormalizeVegaLiteSpec(r.Spec)
		if err != nil {
			trivialOnly = false
			continue
		}
		if vlrender.SpecIsTrivial(spec) {
			continue
		}
		trivialOnly = false
		charts = append(charts, WebChatChart{
			Title:       vlrender.FriendlyTitle(r.Title, r.Subtitle),
			Description: r.Description,
			Query:       r.Query,
			TimeRange:   r.TimeRange,
			Granularity: r.Granularity,
			Context:     r.Context,
			Spec:        json.RawMessage(spec),
			Stats:       r.Stats,
		})
	}
	if len(charts) == 0 {
		if trivialOnly {
			return nil, vlrender.ErrTrivialSpec
		}
		return nil, errors.New("no charts could be extracted")
	}
	return charts, nil
}

// stripVegaBlocks recursively removes leading Vega-Lite JSON objects and any
// leftover code fences from the text, returning the remaining prose.
func stripVegaBlocks(raw string) string {
	idx := strings.Index(raw, "\n{")
	if idx < 0 {
		idx = strings.Index(raw, "{")
	}
	if idx < 0 {
		return strings.TrimSpace(removeEmptyCodeFences(raw))
	}

	depth := 0
	inStr := false
	esc := false
	start := -1

	for i := idx; i < len(raw); i++ {
		ch := raw[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
			} else if ch == '"' {
				inStr = false
			}
			continue
		}
		if ch == '"' {
			inStr = true
			esc = false
			continue
		}
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start >= 0 {
				if vlrender.IsVegaJSON([]byte(raw[start : i+1])) {
					raw = strings.TrimSpace(raw[:start] + raw[i+1:])
					return stripVegaBlocks(raw)
				}
				start = -1
			}
		}
	}
	return strings.TrimSpace(removeEmptyCodeFences(raw))
}

// removeEmptyCodeFences removes any leftover ```json / ``` fence markers that
// may remain after a Vega-Lite JSON block was stripped.
func removeEmptyCodeFences(raw string) string {
	if !strings.Contains(raw, "```") {
		return raw
	}
	var lines []string
	for _, ln := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(ln)
		if t == "```" || t == "```json" || t == "```python" || t == "```json ```" {
			continue
		}
		lines = append(lines, ln)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// looksLikeTruncatedVegaSpec returns true if text looks like a Vega-Lite
// report wrapper that was cut off before the "spec" field was emitted.
func looksLikeTruncatedVegaSpec(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || trimmed[0] != '{' || trimmed[len(trimmed)-1] == '}' {
		return false
	}
	return strings.Contains(text, `"title"`) && strings.Contains(text, `"description"`) && !strings.Contains(text, `"spec"`)
}

// extractDescriptionFromVegaSpec tries to pull the "description" value out of
// a truncated Vega-Lite JSON string using a simple scan (json.Unmarshal cannot
// parse incomplete JSON). Returns a user-friendly fallback on failure.
func extractDescriptionFromVegaSpec(text string) string {
	idx := strings.Index(text, `"description"`)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(`"description"`):]
	// skip optional whitespace + colon
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ':' {
		return ""
	}
	rest = strings.TrimSpace(rest[1:])
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	// scan to matching unescaped closing quote
	var desc strings.Builder
	inEscape := false
	for i := 1; i < len(rest); i++ {
		ch := rest[i]
		if inEscape {
			switch ch {
			case 'n':
				desc.WriteByte('\n')
			case 't':
				desc.WriteByte('\t')
			case 'r':
				desc.WriteByte('\r')
			case 'b':
				desc.WriteByte('\b')
			case 'f':
				desc.WriteByte('\f')
			case '/':
				desc.WriteByte('/')
			case '\\':
				desc.WriteByte('\\')
			case '"':
				desc.WriteByte('"')
			case 'u':
				// Decode \uXXXX unicode escapes.
				if i+4 < len(rest) {
					// Bound-check r (still uint64) against utf8.MaxRune before
					// converting to rune, so the conversion itself is never
					// reached with an out-of-range value.
					r, err := strconv.ParseUint(rest[i+1:i+5], 16, 32)
					if err == nil && r <= utf8.MaxRune && utf8.ValidRune(rune(r)) {
						desc.WriteRune(rune(r))
						i += 4
					} else {
						desc.WriteString(`\u`)
					}
				} else {
					desc.WriteString(`\u`)
				}
			default:
				desc.WriteByte('\\')
				desc.WriteByte(ch)
			}
			inEscape = false
			continue
		}
		if ch == '\\' {
			inEscape = true
			continue
		}
		if ch == '"' {
			break
		}
		desc.WriteByte(ch)
	}
	result := strings.TrimSpace(desc.String())
	if result == "" {
		return ""
	}
	return result
}

func (s *Server) resolveOrgConnector(ctx context.Context, orgID int64) (*aiconnectors.Connector, error) {
	storage := aiconnectors.NewStorage(s.db)
	connectors, err := storage.GetAllConnectors(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query AI connectors: %w", err)
	}
	if len(connectors) == 0 {
		return nil, fmt.Errorf("no AI connectors configured in this organization")
	}
	for _, record := range connectors {
		options := storage.GetConnectorOptions(ctx, record)
		c, err := aiconnectors.NewConnector(ctx, options)
		if err != nil {
			continue
		}
		return c, nil
	}
	return nil, fmt.Errorf("all AI connectors failed to initialize")
}
