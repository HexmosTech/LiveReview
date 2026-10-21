package github

import (
	"strings"

	coreprocessor "github.com/livereview/internal/core_processor"
)

// DetectDirectMention returns true when the comment body contains a direct mention
// of the bot's username. GitHub treats mentions as case-insensitive @username tokens.
func DetectDirectMention(commentBody string, botInfo *coreprocessor.UnifiedBotUserInfoV2) bool {
	if botInfo == nil {
		return false
	}

	username := strings.TrimSpace(botInfo.Username)
	if username == "" {
		return false
	}

	body := strings.ToLower(commentBody)
	if strings.Contains(body, "@"+strings.ToLower(username)) {
		return true
	}
	// A GitHub App's bot account is "<slug>[bot]", but people mention it as plain "@<slug>".
	if slug := strings.TrimSuffix(strings.ToLower(username), "[bot]"); slug != strings.ToLower(username) && slug != "" {
		return strings.Contains(body, "@"+slug)
	}
	return false
}
