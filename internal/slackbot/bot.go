package slackbot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/mcpagent"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const (
	maxConversations = 100
	agentTimeout     = 2 * time.Minute
)

// Bot is the Slack bot. It owns a single Socket Mode connection and
// dispatches events to per-org handlers based on the Slack team_id.
type Bot struct {
	socketClient  *socketmode.Client
	orgs          map[string]*orgHandler // teamID -> handler
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	appToken      string
	teamIDStored  func(orgID int64, teamID string) error
}

// orgHandler holds per-org state: its own Slack client, agent, and conversations.
type orgHandler struct {
	orgID         int64
	teamID        string
	slackClient   *slack.Client
	agent         *mcpagent.Agent
	conversations map[string]*conversation
	mu            sync.Mutex

	// lazy MCP init
	mcpServerURL  string
	mcpHeaders    map[string]string
	connector     *aiconnectors.Connector
	maxAgentSteps int
	agentMu       sync.Mutex
}

type conversation struct {
	history  []mcpagent.HistoryEntry
	lastUsed time.Time
}

// OrgConfig holds per-org configuration for the Slack bot.
type OrgConfig struct {
	OrgID         int64
	SlackBotToken string
	MCPServerURL  string
	MCPHeaders    map[string]string
	Connector     *aiconnectors.Connector
	MaxAgentSteps int
}

// Config holds configuration for the multi-org Slack bot.
type Config struct {
	SlackAppToken string
	Orgs          []OrgConfig
}

// New creates a new multi-org Slack bot. Performs auth test for each org
// to resolve the Slack workspace team_id, then connects to MCP for each.
// teamIDStored, if non-nil, is called after each org's team_id is resolved.
func New(cfg *Config, teamIDStored func(orgID int64, teamID string) error) (*Bot, error) {
	if cfg.SlackAppToken == "" {
		return nil, fmt.Errorf("SlackAppToken is required")
	}
	if len(cfg.Orgs) == 0 {
		return nil, fmt.Errorf("at least one org config is required")
	}

	orgs := make(map[string]*orgHandler, len(cfg.Orgs))

	for i := range cfg.Orgs {
		oc := &cfg.Orgs[i]
		if oc.SlackBotToken == "" {
			return nil, fmt.Errorf("org %d: SlackBotToken is required", oc.OrgID)
		}
		if oc.MCPServerURL == "" {
			return nil, fmt.Errorf("org %d: MCPServerURL is required", oc.OrgID)
		}
		if oc.Connector == nil {
			return nil, fmt.Errorf("org %d: Connector is required", oc.OrgID)
		}
		if oc.MaxAgentSteps <= 0 {
			oc.MaxAgentSteps = 8
		}

		for k, v := range oc.MCPHeaders {
			if isSensitiveHeader(k, v) {
				log.Printf("[SlackBot] Org %d: MCP header %q may contain a secret value", oc.OrgID, k)
			}
		}

		// Create per-org Slack client
		slackClient := slack.New(oc.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))

		// Auth test to resolve team_id
		authResp, err := slackClient.AuthTestContext(context.Background())
		if err != nil {
			log.Printf("[SlackBot] Org %d: auth test failed: %v — skipping", oc.OrgID, err)
			continue
		}
		log.Printf("[SlackBot] Org %d: authenticated as %s (%s), team=%s", oc.OrgID, authResp.User, authResp.UserID, authResp.TeamID)

		// Persist team_id if callback provided
		if teamIDStored != nil {
			if err := teamIDStored(oc.OrgID, authResp.TeamID); err != nil {
				log.Printf("[SlackBot] Org %d: failed to store team_id: %v", oc.OrgID, err)
			}
		}

		orgs[authResp.TeamID] = &orgHandler{
			orgID:         oc.OrgID,
			teamID:        authResp.TeamID,
			slackClient:   slackClient,
			conversations: make(map[string]*conversation),
			mcpServerURL:  oc.MCPServerURL,
			mcpHeaders:    oc.MCPHeaders,
			connector:     oc.Connector,
			maxAgentSteps: oc.MaxAgentSteps,
		}
	}

	if len(orgs) == 0 {
		return nil, fmt.Errorf("no orgs could be initialized (all auth tests failed)")
	}

	// Use the first org's slack client for the socket connection
	// (any will do — they all share the same app token)
	var firstClient *slack.Client
	for _, oh := range orgs {
		firstClient = oh.slackClient
		break
	}
	socketClient := socketmode.New(firstClient)

	return &Bot{
		socketClient:  socketClient,
		orgs:          orgs,
		appToken:      cfg.SlackAppToken,
		teamIDStored:  teamIDStored,
	}, nil
}

// Start starts the Socket Mode event loop and blocks until ctx is cancelled or an error occurs.
func (b *Bot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	handler := socketmode.NewSocketmodeHandler(b.socketClient)
	handler.Handle(socketmode.EventTypeEventsAPI, b.handleEvent)

	log.Printf("[SlackBot] Starting Socket Mode listener (%d orgs)", len(b.orgs))
	return handler.RunEventLoopContext(ctx)
}

// AddOrg dynamically registers a new org on a running bot.
func (b *Bot) AddOrg(oc OrgConfig) error {
	if oc.SlackBotToken == "" {
		return fmt.Errorf("org %d: SlackBotToken is required", oc.OrgID)
	}
	if oc.MCPServerURL == "" {
		return fmt.Errorf("org %d: MCPServerURL is required", oc.OrgID)
	}
	if oc.Connector == nil {
		return fmt.Errorf("org %d: Connector is required", oc.OrgID)
	}
	if oc.MaxAgentSteps <= 0 {
		oc.MaxAgentSteps = 8
	}

	slackClient := slack.New(oc.SlackBotToken, slack.OptionAppLevelToken(b.appToken))

	authResp, err := slackClient.AuthTestContext(context.Background())
	if err != nil {
		return fmt.Errorf("org %d: auth test failed: %w", oc.OrgID, err)
	}

	if b.teamIDStored != nil {
		if err := b.teamIDStored(oc.OrgID, authResp.TeamID); err != nil {
			log.Printf("[SlackBot] Org %d: failed to store team_id: %v", oc.OrgID, err)
		}
	}

	b.mu.Lock()
	b.orgs[authResp.TeamID] = &orgHandler{
		orgID:         oc.OrgID,
		teamID:        authResp.TeamID,
		slackClient:   slackClient,
		conversations: make(map[string]*conversation),
		mcpServerURL:  oc.MCPServerURL,
		mcpHeaders:    oc.MCPHeaders,
		connector:     oc.Connector,
		maxAgentSteps: oc.MaxAgentSteps,
	}
	b.mu.Unlock()

	log.Printf("[SlackBot] Org %d: added dynamically, team=%s", oc.OrgID, authResp.TeamID)
	return nil
}

func (b *Bot) handleEvent(evt *socketmode.Event, client *socketmode.Client) {
	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	client.Ack(*evt.Request)

	teamID := eventsAPIEvent.TeamID

	switch eventsAPIEvent.InnerEvent.Type {
	case "app_mention":
		b.handleAppMention(eventsAPIEvent.InnerEvent.Data, teamID)
	case "message":
		b.handleMessage(eventsAPIEvent.InnerEvent.Data, teamID)
	}
}

func (b *Bot) resolveTeam(teamID string) *orgHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.orgs[teamID]
}

func (b *Bot) handleAppMention(data any, teamID string) {
	mention, ok := data.(*slackevents.AppMentionEvent)
	if !ok {
		return
	}

	oh := b.resolveTeam(teamID)
	if oh == nil {
		log.Printf("[SlackBot] Unknown team %s for app_mention, skipping", teamID)
		return
	}

	text := strings.TrimSpace(mention.Text)
	text = stripMention(text, mention.User)

	oh.processMessage(mention.Channel, mention.TimeStamp, mention.ThreadTimeStamp, text)
}

func (b *Bot) handleMessage(data any, teamID string) {
	msg, ok := data.(*slackevents.MessageEvent)
	if !ok {
		return
	}

	if msg.BotID != "" {
		return
	}

	oh := b.resolveTeam(teamID)
	if oh == nil {
		return
	}

	// Only respond to DMs
	channelInfo, err := oh.slackClient.GetConversationInfo(&slack.GetConversationInfoInput{
		ChannelID: msg.Channel,
	})
	if err != nil || !channelInfo.IsIM {
		return
	}

	oh.processMessage(msg.Channel, msg.TimeStamp, msg.ThreadTimeStamp, msg.Text)
}

func (oh *orgHandler) ensureAgent() error {
	oh.agentMu.Lock()
	defer oh.agentMu.Unlock()
	if oh.agent != nil {
		return nil
	}
	mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer mcpCancel()
	mcpSession, err := mcpagent.ConnectMCP(mcpCtx, oh.mcpServerURL, oh.mcpHeaders)
	if err != nil {
		return fmt.Errorf("org %d: failed to connect to MCP: %w", oh.orgID, err)
	}
	provider := mcpagent.NewProvider(oh.connector)
	oh.agent = mcpagent.NewAgent(provider, mcpSession, oh.maxAgentSteps)
	log.Printf("[SlackBot] Org %d: connected to MCP lazily. Tools: %v", oh.orgID, toolNames(mcpSession.Tools))
	return nil
}

func (oh *orgHandler) processMessage(channel, ts, threadTS, text string) {
	key := channel + ":" + ts
	if threadTS != "" {
		key = channel + ":" + threadTS
	}

	oh.mu.Lock()
	conv, exists := oh.conversations[key]
	if !exists {
		conv = &conversation{}
		oh.conversations[key] = conv
		pruneConversationsLocked(oh.conversations)
	}
	history := conv.history
	oh.mu.Unlock()

	start := time.Now()

	if err := oh.ensureAgent(); err != nil {
		log.Printf("[SlackBot] Org %d: MCP not available: %s", oh.orgID, err)
		blocks := []slack.Block{
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", ":warning: Sorry, the backend is not ready yet. Please try again later.", false, false),
				nil, nil,
			),
		}
		oh.slackClient.PostMessage(channel, slack.MsgOptionBlocks(blocks...))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), agentTimeout)
	defer cancel()

	finalText, updatedHistory, err := oh.agent.RunTurn(ctx, history, text)
	if err != nil {
		log.Printf("[SlackBot] RunTurn error: %s", err)
		blocks := []slack.Block{
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", ":warning: Sorry, I ran into an error processing your request.", false, false),
				nil, nil,
			),
		}
		if _, _, err := oh.slackClient.PostMessage(channel, slack.MsgOptionBlocks(blocks...)); err != nil {
			log.Printf("[SlackBot] Failed to post error message: %s", err)
		}
		return
	}

	duration := time.Since(start)
	log.Printf("[SlackBot] Agent completed in %s, response length: %d", duration, len(finalText))

	oh.mu.Lock()
	conv.history = updatedHistory
	conv.lastUsed = time.Now()
	oh.mu.Unlock()

	if finalText == "" {
		finalText = "(no response)"
	}

	// Try rendering as a Vega-Lite chart report first
	if strings.Contains(finalText, `"$schema"`) || (strings.Contains(finalText, `"mark"`) && strings.Contains(finalText, `"encoding"`)) || (strings.Contains(finalText, `"title"`) && strings.Contains(finalText, `"spec"`)) {
		vlCtx, vlCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer vlCancel()
		if pngData, title, ok := parseAndRenderVegaLiteReport(vlCtx, finalText); ok {
			oh.uploadReportToSlack(channel, "", pngData, title)
			return
		}
		log.Printf("[SlackBot] Vega-Lite spec detected but rendering failed, falling back to text")
	}

	blocks := FormatSlackResponse(finalText)
	if _, _, err := oh.slackClient.PostMessage(channel, slack.MsgOptionBlocks(blocks...)); err != nil {
		log.Printf("[SlackBot] Failed to post response: %s", err)
	}
}

func pruneConversationsLocked(conversations map[string]*conversation) {
	if len(conversations) <= maxConversations {
		return
	}

	type kv struct {
		key string
		t   time.Time
	}
	var sorted []kv
	for k, v := range conversations {
		sorted = append(sorted, kv{k, v.lastUsed})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].t.Before(sorted[j].t)
	})

	toRemove := len(conversations) - maxConversations
	for i := 0; i < toRemove; i++ {
		delete(conversations, sorted[i].key)
	}
}

func isSensitiveHeader(key, value string) bool {
	if len(value) < 10 {
		return false
	}
	lower := strings.ToLower(key)
	if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "auth") || strings.Contains(lower, "secret") {
		return true
	}
	return strings.HasPrefix(value, "sk-") || strings.HasPrefix(value, "ghp_")
}

func stripMention(text, botUserID string) string {
	mention := fmt.Sprintf("<@%s>", botUserID)
	text = strings.ReplaceAll(text, mention, "")
	return strings.TrimSpace(text)
}

func toolNames(tools []mcpagent.MCPToolDef) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
