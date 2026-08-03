package discordbot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/mcpagent"
)

const (
	maxConversations = 100
	agentTimeout     = 2 * time.Minute
	discordColor     = 0x5865F2
)

type conversation struct {
	history  []mcpagent.HistoryEntry
	lastUsed time.Time
}

type orgHandler struct {
	orgID         int64
	orgName       string
	session       *discordgo.Session
	agent         *mcpagent.Agent
	conversations map[string]*conversation
	mu            sync.Mutex
	agentMu       sync.Mutex
	mcpServerURL  string
	mcpHeaders    map[string]string
	connector     *aiconnectors.Connector
	maxAgentSteps int
	botUserID     string
}

type Bot struct {
	orgs     map[int64]*orgHandler // orgID -> handler
	guildMap map[string]int64      // guildID -> orgID
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

type OrgConfig struct {
	OrgID         int64
	OrgName       string
	BotToken      string
	MCPServerURL  string
	MCPHeaders    map[string]string
	Connector     *aiconnectors.Connector
	MaxAgentSteps int
}

// NewOrg creates a new orgHandler for a single org and returns the handler
// plus all guild IDs the bot was found in.
func NewOrg(oc OrgConfig) (*orgHandler, []string, error) {
	if oc.BotToken == "" {
		return nil, nil, fmt.Errorf("org %d: BotToken is required", oc.OrgID)
	}
	if oc.MCPServerURL == "" {
		return nil, nil, fmt.Errorf("org %d: MCPServerURL is required", oc.OrgID)
	}
	if oc.Connector == nil {
		return nil, nil, fmt.Errorf("org %d: Connector is required", oc.OrgID)
	}
	if oc.MaxAgentSteps <= 0 {
		oc.MaxAgentSteps = 8
	}

	session, err := discordgo.New("Bot " + oc.BotToken)
	if err != nil {
		return nil, nil, fmt.Errorf("org %d: failed to create discord session: %w", oc.OrgID, err)
	}
	session.Identify.Intents = discordgo.IntentGuildMessages |
		discordgo.IntentDirectMessages |
		discordgo.IntentMessageContent |
		discordgo.IntentGuilds

	guildsList, err := session.UserGuilds(200, "", "", false)
	if err != nil {
		return nil, nil, fmt.Errorf("org %d: failed to list guilds: %w", oc.OrgID, err)
	}
	if len(guildsList) == 0 {
		return nil, nil, fmt.Errorf("org %d: bot is not in any guild", oc.OrgID)
	}

	guildIDs := make([]string, 0, len(guildsList))
	for _, g := range guildsList {
		guildIDs = append(guildIDs, g.ID)
	}
	log.Printf("[DiscordBot] Org %d: registered in %d guild(s)", oc.OrgID, len(guildIDs))

	botUser := ""
	if u, err := session.User("@me"); err == nil {
		botUser = u.ID
		log.Printf("[DiscordBot] Org %d: authenticated as %s (%s)", oc.OrgID, u.Username, u.ID)
	}

	return &orgHandler{
		orgID:         oc.OrgID,
		orgName:       oc.OrgName,
		session:       session,
		conversations: make(map[string]*conversation),
		mcpServerURL:  oc.MCPServerURL,
		mcpHeaders:    oc.MCPHeaders,
		connector:     oc.Connector,
		maxAgentSteps: oc.MaxAgentSteps,
		botUserID:     botUser,
	}, guildIDs, nil
}

// New creates a multi-org Discord bot. Each org gets its own session.
// Sessions are NOT opened here — call Start to connect them.
func New(cfgs []OrgConfig, guildIDStored func(orgID int64, guildID string) error) (*Bot, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("at least one org config is required")
	}

	bot := &Bot{
		orgs:     make(map[int64]*orgHandler),
		guildMap: make(map[string]int64),
	}

	for _, oc := range cfgs {
		oh, guildIDs, err := NewOrg(oc)
		if err != nil {
			log.Printf("[DiscordBot] Org %d: failed to initialize: %v — skipping", oc.OrgID, err)
			continue
		}
		bot.orgs[oh.orgID] = oh
		for _, gid := range guildIDs {
			bot.guildMap[gid] = oh.orgID
		}
		// Store the primary guild ID (first one)
		if guildIDStored != nil && len(guildIDs) > 0 {
			if err := guildIDStored(oc.OrgID, guildIDs[0]); err != nil {
				log.Printf("[DiscordBot] Org %d: failed to store guild_id: %v", oc.OrgID, err)
			}
		}
	}

	if len(bot.orgs) == 0 {
		return nil, fmt.Errorf("no orgs could be initialized for Discord bot")
	}

	return bot, nil
}

func (b *Bot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	var wg sync.WaitGroup
	errCh := make(chan error, len(b.orgs))

	b.mu.RLock()
	for _, oh := range b.orgs {
		wg.Add(1)
		oh := oh
		go func() {
			defer wg.Done()
			oh.session.AddHandler(b.makeHandler())
			oh.session.AddHandler(b.makeReadyHandler())
			if err := oh.session.Open(); err != nil {
				errCh <- fmt.Errorf("org %d: failed to open session: %w", oh.orgID, err)
				return
			}
			log.Printf("[DiscordBot] Org %d: connected to gateway", oh.orgID)
		}()
	}
	b.mu.RUnlock()

	wg.Wait()
	close(errCh)

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("discord bot errors: %s", strings.Join(errs, "; "))
	}

	log.Printf("[DiscordBot] All orgs connected (%d total)", len(b.orgs))

	<-b.ctx.Done()

	b.mu.RLock()
	for _, oh := range b.orgs {
		oh.session.Close()
	}
	b.mu.RUnlock()

	return b.ctx.Err()
}

func (b *Bot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

func (b *Bot) UpdateBotToken(orgID int64, newToken string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if oh, ok := b.orgs[orgID]; ok {
		oc := OrgConfig{
			OrgID:         oh.orgID,
			OrgName:       oh.orgName,
			BotToken:      newToken,
			MCPServerURL:  oh.mcpServerURL,
			MCPHeaders:    oh.mcpHeaders,
			Connector:     oh.connector,
			MaxAgentSteps: oh.maxAgentSteps,
		}
		newOh, newGuildIDs, err := NewOrg(oc)
		if err != nil {
			log.Printf("[DiscordBot] Org %d: failed to update bot token: %v", orgID, err)
			return
		}
		oh.session.Close()

		// Remove old guild entries from guildMap
		for gid, oid := range b.guildMap {
			if oid == orgID {
				delete(b.guildMap, gid)
			}
		}
		// Add new guild entries
		for _, gid := range newGuildIDs {
			b.guildMap[gid] = orgID
		}

		b.orgs[orgID] = newOh
		newOh.session.AddHandler(b.makeHandler())
		newOh.session.AddHandler(b.makeReadyHandler())
		if b.ctx != nil {
			go func() {
				if err := newOh.session.Open(); err != nil {
					log.Printf("[DiscordBot] Org %d: failed to reconnect with new token: %v", orgID, err)
				}
			}()
		}
		log.Printf("[DiscordBot] Org %d: bot token updated and reconnected", orgID)
	}
}

func (b *Bot) AddOrg(oc OrgConfig) error {
	oh, guildIDs, err := NewOrg(oc)
	if err != nil {
		return fmt.Errorf("failed to add org %d: %w", oc.OrgID, err)
	}

	b.mu.Lock()
	b.orgs[oh.orgID] = oh
	for _, gid := range guildIDs {
		b.guildMap[gid] = oh.orgID
	}
	b.mu.Unlock()

	if b.ctx != nil {
		oh.session.AddHandler(b.makeHandler())
		oh.session.AddHandler(b.makeReadyHandler())
		go func() {
			if err := oh.session.Open(); err != nil {
				log.Printf("[DiscordBot] Org %d: failed to open session: %v", oh.orgID, err)
			}
		}()
	}

	log.Printf("[DiscordBot] Org %d: added dynamically (%d guilds)", oh.orgID, len(guildIDs))
	return nil
}

// makeHandler returns a closure that routes messageCreate events to the correct
// orgHandler by looking up the guild in the guildMap.
func (b *Bot) makeHandler() func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.Bot {
			return
		}
		if len(m.Content) == 0 && len(m.Attachments) == 0 {
			return
		}

		isDM := m.GuildID == ""

	// nosemgrep: trailofbits.go.missing-runlock-on-rwmutex -- RLock/RUnlock are correctly paired around the goroutine-spawn loop below; the flagged returns are inside goroutines running after RUnlock.
	b.mu.RLock()
		defer b.mu.RUnlock()

		var oh *orgHandler
		if isDM {
			// DM — route to first available org (same session)
			for _, handler := range b.orgs {
				if handler.session == s {
					oh = handler
					break
				}
			}
			if oh == nil {
				return
			}
			oh.processMessage(m.ChannelID, m.ID, "", m.Content)
			return
		}

		// Guild message — look up by guildID
		orgID, ok := b.guildMap[m.GuildID]
		if !ok {
			return
		}
		oh = b.orgs[orgID]
		if oh == nil {
			return
		}

		// @mention
		for _, mention := range m.Mentions {
			if mention.ID == oh.botUserID {
				text := strings.ReplaceAll(m.Content, fmt.Sprintf("<@%s>", oh.botUserID), "")
				text = strings.ReplaceAll(text, fmt.Sprintf("<@!%s>", oh.botUserID), "")
				text = strings.TrimSpace(text)
				oh.processMessage(m.ChannelID, m.ID, "", text)
				return
			}
		}
	}
}

func (b *Bot) makeReadyHandler() func(*discordgo.Session, *discordgo.Ready) {
	return func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("[DiscordBot] Session ready — bot user: %s, guilds: %d", r.User.Username, len(r.Guilds))
	}
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
	log.Printf("[DiscordBot] Org %d: connected to MCP. Tools: %v", oh.orgID, toolNames(mcpSession.Tools))
	return nil
}

func (oh *orgHandler) processMessage(channelID, messageID, threadID, text string) {
	if text == "" {
		return
	}

	key := channelID + ":" + messageID

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
		log.Printf("[DiscordBot] Org %d: MCP not available: %s", oh.orgID, err)
		oh.session.ChannelMessageSend(channelID, "⚠️ Sorry, the backend is not ready yet. Please try again later.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), agentTimeout)
	defer cancel()

	oh.session.ChannelTyping(channelID)

	finalText, updatedHistory, err := oh.agent.RunTurn(ctx, history, text)
	if err != nil {
		log.Printf("[DiscordBot] RunTurn error: %s", err)
		oh.session.ChannelMessageSend(channelID, "⚠️ Sorry, I ran into an error processing your request.")
		return
	}

	duration := time.Since(start)
	log.Printf("[DiscordBot] Agent completed in %s, response length: %d", duration, len(finalText))

	oh.mu.Lock()
	conv.history = updatedHistory
	conv.lastUsed = time.Now()
	oh.mu.Unlock()

	if finalText == "" {
		finalText = "(no response)"
	}

	if hasVegaLiteSpec(finalText) {
		vlCtx, vlCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer vlCancel()
		if reports, ok := parseAndRenderVegaLiteReports(vlCtx, finalText); ok {
			oh.uploadReportsToDiscord(channelID, reports, finalText)
			return
		}
		log.Printf("[DiscordBot] Vega-Lite render failed after retries, sending friendly error")
		if _, err := oh.session.ChannelMessageSend(channelID, "Having an issue generating the data, please try again."); err != nil {
			log.Printf("[DiscordBot] Failed to send message: %s", err)
		}
		return
	}

	formatted := FormatDiscordResponse(finalText)
	if len(formatted) > discordMaxMessageLen {
		parts := splitMessage(formatted, discordMaxMessageLen)
		for _, part := range parts {
			if _, err := oh.session.ChannelMessageSend(channelID, part); err != nil {
				log.Printf("[DiscordBot] Failed to send message: %s", err)
				break
			}
		}
	} else {
		if _, err := oh.session.ChannelMessageSend(channelID, formatted); err != nil {
			log.Printf("[DiscordBot] Failed to send message: %s", err)
		}
	}
}

func (oh *orgHandler) uploadReportsToDiscord(channelID string, reports []renderedReport, originalText string) {
	for _, r := range reports {
		if len(r.PNGData) == 0 {
			continue
		}
		msg := r.Description
		if msg == "" {
			msg = r.Title
		}
		if r.Query != "" {
			if msg != "" {
				msg += "\n\n"
			}
			msg += fmt.Sprintf("Query used: %s", r.Query)
		}
		reader := bytes.NewReader(r.PNGData)
		name := "report.png"
		if _, err := oh.session.ChannelFileSend(channelID, name, reader); err != nil {
			log.Printf("[DiscordBot] Failed to upload report: %s", err)
			continue
		}
		if msg != "" {
			oh.session.ChannelMessageSend(channelID, msg)
		}
	}

	cleanText := originalText
	for {
		start := strings.Index(cleanText, "```json")
		if start < 0 {
			break
		}
		end := strings.Index(cleanText[start+len("```json"):], "```")
		if end < 0 {
			break
		}
		cleanText = cleanText[:start] + cleanText[start+end+len("```json")+3:]
	}
	cleanText = stripTopLevelVegaJSON(cleanText)
	cleanText = strings.TrimSpace(cleanText)
	if cleanText != "" {
		formatted := FormatDiscordResponse(cleanText)
		if len(formatted) > discordMaxMessageLen {
			parts := splitMessage(formatted, discordMaxMessageLen)
			for _, part := range parts {
				oh.session.ChannelMessageSend(channelID, part)
			}
		} else {
			oh.session.ChannelMessageSend(channelID, formatted)
		}
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

func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var parts []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			parts = append(parts, text)
			break
		}
		cut := strings.LastIndex(text[:maxLen], "\n")
		if cut < 0 {
			cut = strings.LastIndex(text[:maxLen], " ")
		}
		if cut < 0 {
			cut = maxLen
		}
		parts = append(parts, text[:cut])
		text = text[cut:]
	}
	return parts
}

func parseAndRenderVegaLiteReports(ctx context.Context, text string) ([]renderedReport, bool) {
	reports, err := renderVegaLiteReports(ctx, text)
	if err != nil {
		log.Printf("[DiscordBot] Vega-Lite render failed: %s", err)
		return nil, false
	}
	return reports, true
}

func toolNames(tools []mcpagent.MCPToolDef) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
