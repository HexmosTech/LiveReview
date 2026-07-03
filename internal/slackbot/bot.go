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
	maxConversations    = 100
	agentTimeout        = 2 * time.Minute
)

// Bot is the Slack bot that listens for messages, runs the agent loop
// against an MCP server, and posts responses back to Slack.
type Bot struct {
	slackClient   *slack.Client
	socketClient  *socketmode.Client
	agent         *mcpagent.Agent
	conversations map[string]*conversation
	mu            sync.Mutex
	botUserID     string
	mcpServerURL  string
	mcpHeaders    map[string]string
	connector     *aiconnectors.Connector
	maxAgentSteps int
	ctx           context.Context
	cancel        context.CancelFunc
}

type conversation struct {
	history  []mcpagent.HistoryEntry
	lastUsed time.Time
}

// Config holds pre-resolved configuration for the Slack bot.
type Config struct {
	SlackBotToken string
	SlackAppToken string
	MCPServerURL  string
	MCPHeaders    map[string]string
	Connector     *aiconnectors.Connector
	MaxAgentSteps int
}

// New creates a new Slack bot. Config must already have a resolved Connector.
func New(cfg *Config) (*Bot, error) {
	if cfg.SlackBotToken == "" {
		return nil, fmt.Errorf("SlackBotToken is required")
	}
	if cfg.SlackAppToken == "" {
		return nil, fmt.Errorf("SlackAppToken is required")
	}
	if cfg.MCPServerURL == "" {
		return nil, fmt.Errorf("MCPServerURL is required")
	}
	if cfg.Connector == nil {
		return nil, fmt.Errorf("Connector is required")
	}
	if cfg.MaxAgentSteps <= 0 {
		cfg.MaxAgentSteps = 8
	}

	for k, v := range cfg.MCPHeaders {
		if isSensitiveHeader(k, v) {
			log.Printf("[SlackBot] Warning: MCP header %q may contain a secret value", k)
		}
	}

	slackClient := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	socketClient := socketmode.New(slackClient)

	return &Bot{
		slackClient:   slackClient,
		socketClient:  socketClient,
		conversations: make(map[string]*conversation),
		mcpServerURL:  cfg.MCPServerURL,
		mcpHeaders:    cfg.MCPHeaders,
		connector:     cfg.Connector,
		maxAgentSteps: cfg.MaxAgentSteps,
	}, nil
}

// Start connects to Slack Socket Mode and blocks forever handling events.
func (b *Bot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Connect to MCP server at startup
	log.Printf("[SlackBot] Connecting to MCP server at %s", b.mcpServerURL)
	mcpCtx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()

	mcpSession, err := mcpagent.ConnectMCP(mcpCtx, b.mcpServerURL, b.mcpHeaders)
	if err != nil {
		return fmt.Errorf("failed to connect to MCP server: %w", err)
	}
	log.Printf("[SlackBot] Connected to MCP server. Tools: %v", toolNames(mcpSession.Tools))

	provider := mcpagent.NewProvider(b.connector)
	b.agent = mcpagent.NewAgent(provider, mcpSession, b.maxAgentSteps)

	// Get bot user ID
	authResp, err := b.slackClient.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to auth test: %w", err)
	}
	b.botUserID = authResp.UserID
	log.Printf("[SlackBot] Authenticated as %s (%s)", authResp.User, authResp.UserID)

	// Set up Socket Mode handler
	handler := socketmode.NewSocketmodeHandler(b.socketClient)
	handler.Handle(socketmode.EventTypeEventsAPI, b.handleEvent)

	log.Printf("[SlackBot] Starting Socket Mode listener")
	return handler.RunEventLoopContext(ctx)
}

func (b *Bot) handleEvent(evt *socketmode.Event, client *socketmode.Client) {
	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	client.Ack(*evt.Request)

	switch eventsAPIEvent.InnerEvent.Type {
	case "app_mention":
		b.handleAppMention(eventsAPIEvent.InnerEvent.Data)
	case "message":
		b.handleMessage(eventsAPIEvent.InnerEvent.Data)
	}
}

func (b *Bot) handleAppMention(data any) {
	mention, ok := data.(*slackevents.AppMentionEvent)
	if !ok {
		return
	}

	text := strings.TrimSpace(mention.Text)
	text = stripMention(text, b.botUserID)

	b.processMessage(mention.Channel, mention.TimeStamp, mention.ThreadTimeStamp, text)
}

func (b *Bot) handleMessage(data any) {
	msg, ok := data.(*slackevents.MessageEvent)
	if !ok {
		return
	}

	if msg.BotID != "" || msg.User == b.botUserID {
		return
	}

	channelInfo, err := b.slackClient.GetConversationInfo(&slack.GetConversationInfoInput{
		ChannelID: msg.Channel,
	})
	if err != nil || !channelInfo.IsIM {
		return
	}

	b.processMessage(msg.Channel, msg.TimeStamp, msg.ThreadTimeStamp, msg.Text)
}

func (b *Bot) processMessage(channel, ts, threadTS, text string) {
	key := channel + ":" + ts
	if threadTS != "" {
		key = channel + ":" + threadTS
	}

	b.mu.Lock()
	conv, exists := b.conversations[key]
	if !exists {
		conv = &conversation{}
		b.conversations[key] = conv
		b.pruneConversationsLocked()
	}
	history := conv.history
	b.mu.Unlock()

	// Post a placeholder message
	b.slackClient.PostMessage(channel, slack.MsgOptionText("_Thinking..._", false), slack.MsgOptionTS(ts))

	start := time.Now()

	ctx, cancel := context.WithTimeout(b.ctx, agentTimeout)
	defer cancel()

	finalText, updatedHistory, err := b.agent.RunTurn(ctx, history, text)
	if err != nil {
		log.Printf("[SlackBot] RunTurn error: %s", err)
		blocks := []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", "🤖 LiveReview Assistant", false, false)),
			slack.NewDividerBlock(),
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", ":warning: Sorry, I ran into an error processing your request.", false, false),
				nil, nil,
			),
			slack.NewDividerBlock(),
			slack.NewContextBlock("",
				slack.NewTextBlockObject("mrkdwn", ":zap: *LiveReview* — AI-powered code review", false, false),
			),
		}
		if _, _, err := b.slackClient.PostMessage(channel, slack.MsgOptionBlocks(blocks...), slack.MsgOptionTS(ts)); err != nil {
			log.Printf("[SlackBot] Failed to post error message: %s", err)
		}
		return
	}

	duration := time.Since(start)
	log.Printf("[SlackBot] Agent completed in %s, response length: %d", duration, len(finalText))

	b.mu.Lock()
	conv.history = updatedHistory
	conv.lastUsed = time.Now()
	b.mu.Unlock()

	if finalText == "" {
		finalText = "(no response)"
	}

	// Try rendering as a Vega-Lite chart report first
	if strings.Contains(finalText, `"$schema"`) || (strings.Contains(finalText, `"mark"`) && strings.Contains(finalText, `"encoding"`)) || (strings.Contains(finalText, `"title"`) && strings.Contains(finalText, `"spec"`)) {
		vlCtx, vlCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer vlCancel()
		if pngData, title, ok := parseAndRenderVegaLiteReport(vlCtx, finalText); ok {
			b.uploadReportToSlack(channel, ts, pngData, title)
			return
		}
		log.Printf("[SlackBot] Vega-Lite spec detected but rendering failed, falling back to text")
	}

	blocks := FormatSlackResponse(finalText, duration)
	if _, _, err := b.slackClient.PostMessage(channel, slack.MsgOptionBlocks(blocks...), slack.MsgOptionTS(ts)); err != nil {
		log.Printf("[SlackBot] Failed to post response: %s", err)
	}
}

func (b *Bot) pruneConversationsLocked() {
	if len(b.conversations) <= maxConversations {
		return
	}

	type kv struct {
		key string
		t   time.Time
	}
	var sorted []kv
	for k, v := range b.conversations {
		sorted = append(sorted, kv{k, v.lastUsed})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].t.Before(sorted[j].t)
	})

	toRemove := len(b.conversations) - maxConversations
	for i := 0; i < toRemove; i++ {
		delete(b.conversations, sorted[i].key)
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
