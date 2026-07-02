package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/mcpagent"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Bot is the Slack bot that listens for messages, runs the agent loop
// against an MCP server, and posts responses back to Slack.
type Bot struct {
	slackClient   *slack.Client
	socketClient  *socketmode.Client
	agent         *mcpagent.Agent
	conversations map[string][]mcpagent.HistoryEntry
	mu            sync.Mutex
	botUserID     string
	mcpServerURL  string
	mcpHeaders    map[string]string
	connector     *aiconnectors.Connector
	maxAgentSteps int
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

	slackClient := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	socketClient := socketmode.New(slackClient)

	return &Bot{
		slackClient:   slackClient,
		socketClient:  socketClient,
		conversations: make(map[string][]mcpagent.HistoryEntry),
		mcpServerURL:  cfg.MCPServerURL,
		mcpHeaders:    cfg.MCPHeaders,
		connector:     cfg.Connector,
		maxAgentSteps: cfg.MaxAgentSteps,
	}, nil
}

// Start connects to Slack Socket Mode and blocks forever handling events.
func (b *Bot) Start(ctx context.Context) error {
	// Connect to MCP server at startup
	log.Printf("[SlackBot] Connecting to MCP server at %s", b.mcpServerURL)
	mcpCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
	history := b.conversations[key]
	b.mu.Unlock()

	b.slackClient.PostMessage(channel, slack.MsgOptionText("_Thinking..._", false), slack.MsgOptionTS(ts))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	finalText, updatedHistory, err := b.agent.RunTurn(ctx, history, text)
	if err != nil {
		finalText = fmt.Sprintf("Sorry, I ran into an error: %s", err)
	}

	b.mu.Lock()
	b.conversations[key] = updatedHistory
	b.mu.Unlock()

	if finalText == "" {
		finalText = "(no response)"
	}

	b.slackClient.PostMessage(channel, slack.MsgOptionText(finalText, false), slack.MsgOptionTS(ts))
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
