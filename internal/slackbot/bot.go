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

// Bot is the Slack bot manager. It owns one Socket Mode connection per distinct
// Slack app-level token, since a Socket Mode socket can only authenticate with a
// single app. Events are dispatched to per-org handlers based on the team_id.
type Bot struct {
	runners      []*socketRunner // one runner per app token
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	teamIDStored func(orgID int64, teamID string) error
}

// socketRunner owns a single Socket Mode connection (one app token) and the
// per-org handlers that share that app token.
type socketRunner struct {
	bot          *Bot
	appToken     string
	socketClient *socketmode.Client
	orgs         map[string]*orgHandler // teamID -> handler
	mu           sync.RWMutex
}

// orgHandler holds per-org state: its own Slack client, agent, and conversations.
type orgHandler struct {
	orgID         int64
	orgName       string
	teamID        string
	botUserID     string
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
	OrgName       string
	SlackBotToken string
	SlackAppToken string
	MCPServerURL  string
	MCPHeaders    map[string]string
	Connector     *aiconnectors.Connector
	MaxAgentSteps int
}

// Config holds configuration for the multi-org Slack bot.
type Config struct {
	Orgs []OrgConfig
}

// New creates a new multi-org Slack bot manager. Orgs are grouped by app token,
// with one Socket Mode connection per distinct app token. Performs an auth test
// for each org to resolve the Slack workspace team_id. teamIDStored, if non-nil,
// is called after each org's team_id is resolved.
func New(cfg *Config, teamIDStored func(orgID int64, teamID string) error) (*Bot, error) {
	if len(cfg.Orgs) == 0 {
		return nil, fmt.Errorf("at least one org config is required")
	}

	// Group orgs by app token, preserving first-seen order.
	appGroups := make(map[string][]*OrgConfig)
	var appOrder []string
	for i := range cfg.Orgs {
		oc := &cfg.Orgs[i]
		if oc.SlackBotToken == "" {
			return nil, fmt.Errorf("org %d: SlackBotToken is required", oc.OrgID)
		}
		if oc.SlackAppToken == "" {
			return nil, fmt.Errorf("org %d: SlackAppToken is required", oc.OrgID)
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

		if _, ok := appGroups[oc.SlackAppToken]; !ok {
			appOrder = append(appOrder, oc.SlackAppToken)
		}
		appGroups[oc.SlackAppToken] = append(appGroups[oc.SlackAppToken], oc)
	}

	bot := &Bot{teamIDStored: teamIDStored}

	for _, appToken := range appOrder {
		runner := &socketRunner{bot: bot, appToken: appToken, orgs: make(map[string]*orgHandler)}
		var firstClient *slack.Client
		for _, oc := range appGroups[appToken] {
			handler, err := runner.addOrg(oc, teamIDStored)
			if err != nil {
				log.Printf("[SlackBot] Org %d: skipping: %v", oc.OrgID, err)
				continue
			}
			if firstClient == nil {
				firstClient = handler.slackClient
			}
		}
		if len(runner.orgs) == 0 {
			log.Printf("[SlackBot] App token %s: no orgs could be initialized — skipping runner", maskAppToken(appToken))
			continue
		}
		runner.socketClient = socketmode.New(firstClient)
		bot.runners = append(bot.runners, runner)
	}

	if len(bot.runners) == 0 {
		return nil, fmt.Errorf("no orgs could be initialized (all auth tests failed)")
	}

	return bot, nil
}

// Start starts the Socket Mode event loop for every runner and blocks until ctx
// is cancelled or all runners exit.
func (b *Bot) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.ctx = ctx
	b.cancel = cancel
	runners := make([]*socketRunner, len(b.runners))
	copy(runners, b.runners)
	b.mu.Unlock()

	if len(runners) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(runners))
	for _, r := range runners {
		wg.Add(1)
		go func(r *socketRunner) {
			defer wg.Done()
			if err := r.start(ctx); err != nil {
				errCh <- err
			}
		}(r)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *socketRunner) start(ctx context.Context) error {
	handler := socketmode.NewSocketmodeHandler(r.socketClient)
	handler.Handle(socketmode.EventTypeEventsAPI, r.handleEvent)
	log.Printf("[SlackBot] Starting Socket Mode listener (%d orgs, app token %s)", len(r.orgs), maskAppToken(r.appToken))
	return handler.RunEventLoopContext(ctx)
}

// addOrg resolves an org's team_id and registers it on this runner. The returned
// handler's slackClient can be used to derive the runner's socket client.
func (r *socketRunner) addOrg(oc *OrgConfig, teamIDStored func(orgID int64, teamID string) error) (*orgHandler, error) {
	slackClient := slack.New(oc.SlackBotToken, slack.OptionAppLevelToken(oc.SlackAppToken))

	// Auth test to resolve team_id
	authResp, err := slackClient.AuthTestContext(context.Background())
	if err != nil {
		return nil, err
	}
	log.Printf("[SlackBot] Org %d: authenticated as %s (%s), team=%s", oc.OrgID, authResp.User, authResp.UserID, authResp.TeamID)

	// Persist team_id if callback provided
	if teamIDStored != nil {
		if err := teamIDStored(oc.OrgID, authResp.TeamID); err != nil {
			log.Printf("[SlackBot] Org %d: failed to store team_id: %v", oc.OrgID, err)
		}
	}

	handler := &orgHandler{
		orgID:         oc.OrgID,
		orgName:       oc.OrgName,
		teamID:        authResp.TeamID,
		botUserID:     authResp.UserID,
		slackClient:   slackClient,
		conversations: make(map[string]*conversation),
		mcpServerURL:  oc.MCPServerURL,
		mcpHeaders:    oc.MCPHeaders,
		connector:     oc.Connector,
		maxAgentSteps: oc.MaxAgentSteps,
	}

	r.mu.Lock()
	if _, exists := r.orgs[authResp.TeamID]; exists {
		log.Printf("[SlackBot] Org %d: replacing existing handler for team %s", oc.OrgID, authResp.TeamID)
	}
	r.orgs[authResp.TeamID] = handler
	r.mu.Unlock()

	return handler, nil
}

// UpdateBotToken immediately swaps the Slack API client for an org to a new token.
// The app token is used to keep the org attached to the correct socket runner.
func (b *Bot) UpdateBotToken(orgID int64, newToken, appToken string) {
	if newToken == "" {
		log.Printf("[SlackBot] Org %d: bot token empty, cannot update token", orgID)
		return
	}
	if appToken == "" {
		log.Printf("[SlackBot] Org %d: app token empty, cannot update token", orgID)
		return
	}
	slackClient := slack.New(newToken, slack.OptionAppLevelToken(appToken))

	b.mu.RLock()
	for _, r := range b.runners {
		r.mu.Lock()
		for _, oh := range r.orgs {
			if oh.orgID == orgID {
				if r.appToken != appToken {
					log.Printf("[SlackBot] Org %d: app token changed (%s -> %s), will be re-registered via AddOrg", orgID, maskAppToken(r.appToken), maskAppToken(appToken))
					r.mu.Unlock()
					b.mu.RUnlock()
					b.removeOrg(orgID)
					return
				}
				oh.slackClient = slackClient
				log.Printf("[SlackBot] Org %d: bot token updated immediately", orgID)
				r.mu.Unlock()
				b.mu.RUnlock()
				return
			}
		}
		r.mu.Unlock()
	}
	b.mu.RUnlock()
	log.Printf("[SlackBot] Org %d: not found for immediate token update, will be set during AddOrg", orgID)
}

func (b *Bot) removeOrg(orgID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := 0; i < len(b.runners); {
		r := b.runners[i]
		r.mu.Lock()
		for teamID, oh := range r.orgs {
			if oh.orgID == orgID {
				delete(r.orgs, teamID)
			}
		}
		empty := len(r.orgs) == 0
		r.mu.Unlock()
		if empty {
			b.runners = append(b.runners[:i], b.runners[i+1:]...)
		} else {
			i++
		}
	}
}

// AddOrg dynamically registers a new org on a running (or not-yet-started) bot.
func (b *Bot) AddOrg(oc OrgConfig) error {
	if oc.SlackBotToken == "" {
		return fmt.Errorf("org %d: SlackBotToken is required", oc.OrgID)
	}
	if oc.SlackAppToken == "" {
		return fmt.Errorf("org %d: SlackAppToken is required", oc.OrgID)
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

	b.mu.RLock()
	var runner *socketRunner
	for _, r := range b.runners {
		if r.appToken == oc.SlackAppToken {
			runner = r
			break
		}
	}
	b.mu.RUnlock()

	if runner != nil {
		handler, err := runner.addOrg(&oc, b.teamIDStored)
		if err != nil {
			return err
		}
		log.Printf("[SlackBot] Org %d: added dynamically, team=%s", oc.OrgID, handler.teamID)
		return nil
	}

	// New app token — spin up a fresh runner.
	runner = &socketRunner{bot: b, appToken: oc.SlackAppToken, orgs: make(map[string]*orgHandler)}
	handler, err := runner.addOrg(&oc, b.teamIDStored)
	if err != nil {
		return err
	}
	runner.socketClient = socketmode.New(handler.slackClient)

	b.mu.Lock()
	b.runners = append(b.runners, runner)
	ctx := b.ctx
	b.mu.Unlock()

	if ctx != nil {
		go func() {
			if err := runner.start(ctx); err != nil {
				log.Printf("[SlackBot] Org %d: dynamically-started runner exited: %v", oc.OrgID, err)
			}
		}()
	}

	log.Printf("[SlackBot] Org %d: added dynamically on new app token, team=%s", oc.OrgID, handler.teamID)
	return nil
}

func (r *socketRunner) handleEvent(evt *socketmode.Event, client *socketmode.Client) {
	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	client.Ack(*evt.Request)

	teamID := eventsAPIEvent.TeamID

	switch eventsAPIEvent.InnerEvent.Type {
	case "app_mention":
		r.bot.handleAppMention(eventsAPIEvent.InnerEvent.Data, teamID)
	case "message":
		r.bot.handleMessage(eventsAPIEvent.InnerEvent.Data, teamID)
	}
}

func (b *Bot) resolveTeam(teamID string) *orgHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, r := range b.runners {
		r.mu.RLock()
		oh := r.orgs[teamID]
		r.mu.RUnlock()
		if oh != nil {
			return oh
		}
	}
	return nil
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
	text = stripMention(text, oh.botUserID)

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
	if oh.orgName != "" {
		mcpSession.OrgName = oh.orgName
	}
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

	// Try rendering as one or more Vega-Lite chart reports
	if strings.Contains(finalText, `"$schema"`) ||
		(strings.Contains(finalText, `"mark"`) && strings.Contains(finalText, `"encoding"`)) ||
		(strings.Contains(finalText, `"title"`) && strings.Contains(finalText, `"spec"`)) ||
		strings.Contains(finalText, `"reports"`) {
		vlCtx, vlCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer vlCancel()
		if reports, ok := parseAndRenderVegaLiteReports(vlCtx, finalText); ok {
			oh.uploadReportsToSlack(channel, "", reports)
			return
		}
		log.Printf("[SlackBot] Vega-Lite render failed after retries, sending friendly error")
		if _, _, err := oh.slackClient.PostMessage(channel, slack.MsgOptionText("Having an issue generating the data, please try again.", false)); err != nil {
			log.Printf("[SlackBot] Failed to post response: %s", err)
		}
		return
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

// maskAppToken shortens an app-level token for safe logging, keeping the prefix
// readable while hiding the secret portion.
func maskAppToken(token string) string {
	if len(token) <= 12 {
		return "***"
	}
	return token[:6] + "..."
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
