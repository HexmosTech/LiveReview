package teamsbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/mcpagent"
)

const (
	agentTimeout   = 5 * time.Minute
	maxHistorySize = 100
)

type conversation struct {
	history  []mcpagent.HistoryEntry
	threadID string
}

type orgHandler struct {
	orgID         int64
	botAppID      string
	botPassword   string
	agent         *mcpagent.Agent
	conversations map[string]*conversation
	mu            sync.Mutex
	agentMu       sync.Mutex
	mcpServerURL  string
	mcpHeaders    map[string]string
	connector     *aiconnectors.Connector
	maxSteps      int
}

type Bot struct {
	orgs    map[int64]*orgHandler
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	appID   string
	baseURL string
}

type BotConfig struct {
	OrgID        int64
	BotAppID     string
	BotPassword  string
	MCPServerURL string
	MCPHeaders   map[string]string
	Connector    *aiconnectors.Connector
	MaxSteps     int
}

func NewBot(ctx context.Context, configs []BotConfig, baseURL string) *Bot {
	ctx, cancel := context.WithCancel(ctx)
	b := &Bot{
		orgs:    make(map[int64]*orgHandler),
		ctx:     ctx,
		cancel:  cancel,
		baseURL: baseURL,
	}
	for _, cfg := range configs {
		oh := &orgHandler{
			orgID:         cfg.OrgID,
			botAppID:      cfg.BotAppID,
			botPassword:   cfg.BotPassword,
			conversations: make(map[string]*conversation),
			mcpServerURL:  cfg.MCPServerURL,
			mcpHeaders:    cfg.MCPHeaders,
			connector:     cfg.Connector,
			maxSteps:      cfg.MaxSteps,
		}
		b.orgs[cfg.OrgID] = oh
		if len(b.orgs) == 1 {
			b.appID = cfg.BotAppID
		}
	}
	return b
}

func (b *Bot) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (b *Bot) Stop() {
	b.cancel()
}

func (b *Bot) AddOrg(cfg BotConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	oh := &orgHandler{
		orgID:         cfg.OrgID,
		botAppID:      cfg.BotAppID,
		botPassword:   cfg.BotPassword,
		conversations: make(map[string]*conversation),
		mcpServerURL:  cfg.MCPServerURL,
		mcpHeaders:    cfg.MCPHeaders,
		connector:     cfg.Connector,
		maxSteps:      cfg.MaxSteps,
	}
	b.orgs[cfg.OrgID] = oh
	if b.appID == "" {
		b.appID = cfg.BotAppID
	}
}

func (b *Bot) GetAppID() string {
	return b.appID
}

func (b *Bot) UpdateBotToken(orgID int64, appID, password string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if oh, ok := b.orgs[orgID]; ok {
		oh.botAppID = appID
		oh.botPassword = password
	}
}

// HandleActivity processes an incoming Bot Framework activity and sends replies
// to the serviceUrl via the Connector API (async protocol).
func (b *Bot) HandleActivity(ctx context.Context, activity *Activity, authHeader string) error {
	switch activity.Type {
	case ActivityTypeMessage:
		return b.handleMessage(ctx, activity)
	case ActivityTypeConversationUpdate:
		b.handleConversationUpdate(ctx, activity)
		return nil
	default:
		return nil
	}
}

func (b *Bot) handleMessage(ctx context.Context, activity *Activity) error {
	convID := activity.Conversation.ID
	if convID == "" || activity.Text == "" {
		return nil
	}

	text := activity.Text

	isDM := activity.Conversation.ConversationType == ConversationTypePersonal
	if !isDM {
		mentioned := false
		botID := ""
		if activity.Recipient != nil {
			botID = activity.Recipient.ID
		}
		for _, entity := range activity.Entities {
			if entity.Type == EntityTypeMention && entity.Mentioned != nil && entity.Mentioned.ID == botID {
				mentioned = true
				text = strings.ReplaceAll(text, entity.Text, "")
				text = strings.TrimSpace(text)
				break
			}
		}
		if !mentioned {
			return nil
		}
	}

	oh := b.findOrg()
	if oh == nil {
		reply := b.buildReply("Teams bot is not fully configured yet. Please contact your admin.", activity, nil)
		return b.postReply(ctx, activity, reply)
	}

	if oh.connector != nil {
		log.Printf("[TeamsBot] Using connector: %s / model: %s", oh.connector.GetProvider(), oh.connector.ModelConfig().Model)
	}

	oh.mu.Lock()
	conv, ok := oh.conversations[convID]
	if !ok {
		conv = &conversation{threadID: convID}
		oh.conversations[convID] = conv
	}
	if len(oh.conversations) > maxHistorySize {
		oh.pruneConversationsLocked()
	}
	oh.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()

	if err := oh.ensureAgent(ctx); err != nil {
		log.Printf("[TeamsBot] Org %d: failed to initialize agent: %s", oh.orgID, err)
		reply := b.buildReply("Sorry, I'm having trouble connecting. Please try again later.", activity, nil)
		return b.postReply(ctx, activity, reply)
	}

	tStart := time.Now()
	response, history, err := oh.agent.RunTurn(ctx, conv.history, text)
	elapsed := time.Since(tStart)
	if err != nil {
		log.Printf("[TeamsBot] Org %d: agent error after %s: %s", oh.orgID, elapsed, err)
		reply := b.buildReply("Sorry, I encountered an error processing your request.", activity, nil)
		return b.postReply(ctx, activity, reply)
	}
	log.Printf("[TeamsBot] Org %d: agent responded in %s (response len=%d)", oh.orgID, elapsed, len(response))

	conv.history = history

	if hasVegaLiteSpec(response) {
		vlCtx, vlCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer vlCancel()
		attachments, replyText := buildAttachmentsFromVegaLite(vlCtx, b.baseURL, response)
		if len(attachments) > 0 {
			log.Printf("[TeamsBot] Rendered %d Vega-Lite charts for Teams", len(attachments))
			if replyText != "" {
				b.postReply(ctx, activity, b.buildReply(replyText, activity, nil))
			}
			for _, att := range attachments {
				if err := b.postReply(ctx, activity, b.buildReply("", activity, []Attachment{att})); err != nil {
					log.Printf("[TeamsBot] Failed to send chart image (413), sending text fallback")
				}
			}
			return nil
		}
	}

	reply := b.buildReply(response, activity, nil)
	return b.postReply(ctx, activity, reply)
}

func (b *Bot) handleConversationUpdate(ctx context.Context, activity *Activity) {
	if activity.MembersAdded == nil {
		return
	}

	botID := ""
	if activity.Recipient != nil {
		botID = activity.Recipient.ID
	}

	added := false
	for _, member := range activity.MembersAdded {
		if member.ID == botID {
			added = true
			break
		}
	}

	if !added {
		return
	}

	welcome := `Hi! I'm the LiveReview bot. I can help you review code, check billing, and more.`

	if activity.Conversation.ConversationType == ConversationTypePersonal {
		reply := b.buildReply(welcome, activity, nil)
		if err := b.postReply(context.Background(), activity, reply); err != nil {
			log.Printf("[TeamsBot] Failed to send welcome: %s", err)
		}
	}
}

// formatForTeams strips raw Vega-Lite JSON blocks that weren't caught by the
// rendering pipeline, keeping any surrounding text and descriptions.
func formatForTeams(text string) string {
	var parts []string
	remaining := text
	for {
		start := strings.Index(remaining, "```json")
		if start < 0 {
			if trimmed := strings.TrimSpace(remaining); trimmed != "" {
				parts = append(parts, trimmed)
			}
			break
		}
		if start > 0 {
			if trimmed := strings.TrimSpace(remaining[:start]); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		blockStart := start + len("```json")
		end := strings.Index(remaining[blockStart:], "```")
		if end < 0 {
			break
		}
		remaining = remaining[blockStart+end+3:]
	}
	return strings.Join(parts, "\n\n")
}

func (b *Bot) buildReply(text string, orig *Activity, attachments []Attachment) *Activity {
	id := make([]byte, 16)
	rand.Read(id)
	return &Activity{
		Type:         ActivityTypeMessage,
		ID:           hex.EncodeToString(id),
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Text:         formatForTeams(text),
		TextFormat:   "markdown",
		Attachments:  attachments,
		Conversation: orig.Conversation,
		Recipient:    orig.From,
		From:         orig.Recipient,
		ReplyToID:    orig.ID,
	}
}

// postReply sends an Activity to the Bot Framework Connector API via
// POST {serviceUrl}/v3/conversations/{conversationId}/activities
func (b *Bot) postReply(ctx context.Context, orig *Activity, reply *Activity) error {
	if orig.ServiceURL == "" {
		return fmt.Errorf("no serviceUrl on incoming activity")
	}

	u := fmt.Sprintf("%s/v3/conversations/%s/activities",
		strings.TrimRight(orig.ServiceURL, "/"),
		orig.Conversation.ID)

	body, err := json.Marshal(reply)
	if err != nil {
		return fmt.Errorf("marshal reply: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post reply: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("connector returned status %d", resp.StatusCode)
	}

	log.Printf("[TeamsBot] Reply posted to %s (status=%d)", u, resp.StatusCode)
	return nil
}

func (b *Bot) findOrg() *orgHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, oh := range b.orgs {
		return oh
	}
	return nil
}

func (oh *orgHandler) ensureAgent(ctx context.Context) error {
	oh.agentMu.Lock()
	defer oh.agentMu.Unlock()
	if oh.agent != nil {
		return nil
	}

	mcpSession, err := mcpagent.ConnectMCP(ctx, oh.mcpServerURL, oh.mcpHeaders)
	if err != nil {
		return fmt.Errorf("failed to connect to MCP server: %w", err)
	}

	provider := mcpagent.NewProvider(oh.connector)
	agent := mcpagent.NewAgent(provider, mcpSession, oh.maxSteps)

	oh.agent = agent
	return nil
}

func (oh *orgHandler) pruneConversationsLocked() {
	count := len(oh.conversations)
	if count <= maxHistorySize {
		return
	}
	remove := count - maxHistorySize
	for id := range oh.conversations {
		if remove <= 0 {
			break
		}
		delete(oh.conversations, id)
		remove--
	}
}
