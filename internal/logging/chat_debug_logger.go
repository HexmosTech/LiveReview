package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Debug log for the Livi/Slack/Discord/Teams chat agent (see
// internal/mcpagent.Agent.RunTurn). Unlike ReviewLogger (one file per review),
// this is a single shared file so every session can be tailed/grepped
// together - lines are tagged [session_id][HH:MM:SS][source] for filtering.
//
// Enabled only when LIVI_DEBUG_LOG=true (off by default: full chat/tool
// content, including review comments, would otherwise sit in plaintext on
// disk for every prod conversation). The file is wiped at process boot via
// InitChatDebugLog, which must be called exactly once - only the process
// that owns the chat/bot code (the "livereview-api" pm2 app) should call it,
// since other pm2 processes (workers) never touch this file.

const chatDebugLogPath = "chat_debug_logs/chat_debug.log"

var (
	chatLogEnabled bool
	chatLogFile    *os.File
	chatLogMu      sync.Mutex
	chatLogOnce    sync.Once
)

// InitChatDebugLog enables and wipes the shared chat debug log if
// LIVI_DEBUG_LOG is truthy. Safe to call multiple times; only the first call
// takes effect. Call once at process startup, before any chat traffic.
func InitChatDebugLog() {
	chatLogOnce.Do(func() {
		val := strings.ToLower(strings.TrimSpace(os.Getenv("LIVI_DEBUG_LOG")))
		if val != "true" && val != "1" {
			return
		}
		dir := filepath.Dir(chatDebugLogPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("[ChatDebugLog] failed to create log dir %s: %v\n", dir, err)
			return
		}
		// os.Create truncates an existing file - this is the boot-wipe.
		f, err := os.Create(chatDebugLogPath)
		if err != nil {
			fmt.Printf("[ChatDebugLog] failed to create log file %s: %v\n", chatDebugLogPath, err)
			return
		}
		chatLogFile = f
		chatLogEnabled = true
		fmt.Printf("[ChatDebugLog] enabled, writing to %s\n", chatDebugLogPath)
	})
}

// ChatTurnLogger writes correlated debug lines for one chat session. Agents
// are reused across many sessions/conversations (cached per org by the
// bots), so the logger - not the agent - carries the session identity.
type ChatTurnLogger struct {
	sessionID string
	source    string
}

// NewChatTurnLogger returns a logger for one session. source should be one
// of "livi", "slack", "discord", "teams". Safe to construct even when
// LIVI_DEBUG_LOG is off - all methods become no-ops.
func NewChatTurnLogger(sessionID, source string) *ChatTurnLogger {
	return &ChatTurnLogger{sessionID: sessionID, source: source}
}

func (l *ChatTurnLogger) Enabled() bool {
	return l != nil && chatLogEnabled
}

func (l *ChatTurnLogger) write(label, format string, args ...interface{}) {
	if !l.Enabled() {
		return
	}
	chatLogMu.Lock()
	defer chatLogMu.Unlock()
	line := fmt.Sprintf("[%s][%s][%s] %s: %s\n",
		l.sessionID, time.Now().Format("15:04:05"), l.source, label, fmt.Sprintf(format, args...))
	chatLogFile.WriteString(line)
	chatLogFile.Sync()
}

func (l *ChatTurnLogger) Context(orgName, userEmail, providerModel string) {
	l.write("Context", "org=%q user=%q ai=%q", orgName, userEmail, providerModel)
}

func (l *ChatTurnLogger) UserInput(text string) {
	l.write("User Input", "%s", text)
}

func (l *ChatTurnLogger) AIRequest(step int, historyJSON string) {
	l.write("AI Request", "step=%d payload=%s", step, redactSecrets(historyJSON))
}

func (l *ChatTurnLogger) AIResponse(step int, elapsed time.Duration, response string) {
	l.write("AI Response", "step=%d elapsed=%s %s", step, elapsed.Round(time.Millisecond), response)
}

func (l *ChatTurnLogger) AIError(step int, elapsed time.Duration, err error) {
	l.write("AI Error", "step=%d elapsed=%s error=%v", step, elapsed.Round(time.Millisecond), err)
}

func (l *ChatTurnLogger) ToolCall(step int, name, argsJSON string) {
	l.write("Tool Call", "step=%d %s %s", step, name, redactSecrets(argsJSON))
}

func (l *ChatTurnLogger) ToolResponse(step int, name string, elapsed time.Duration, rawLen int, truncated bool, content string) {
	l.write("Tool Response", "step=%d %s elapsed=%s len=%d truncated=%v %s",
		step, name, elapsed.Round(time.Millisecond), rawLen, truncated, content)
}

func (l *ChatTurnLogger) ToolError(step int, name string, elapsed time.Duration, err error) {
	l.write("Tool Error", "step=%d %s elapsed=%s error=%v", step, name, elapsed.Round(time.Millisecond), err)
}

func (l *ChatTurnLogger) FinalResponse(text string) {
	l.write("Final Response To User", "%s", text)
}

func (l *ChatTurnLogger) StepLimitHit(maxSteps int) {
	l.write("Error", "hit step limit (%d steps) without a final answer", maxSteps)
}

var secretPattern = regexp.MustCompile(`(?i)"(authorization|bearer|api[_-]?key|token|password|secret)"\s*:\s*"([^"]{0,4})[^"]*"`)

// redactSecrets masks likely-sensitive values in JSON-ish text before it hits
// disk in plaintext. Best-effort, not a substitute for not logging secrets.
func redactSecrets(s string) string {
	return secretPattern.ReplaceAllString(s, `"$1":"$2***"`)
}
