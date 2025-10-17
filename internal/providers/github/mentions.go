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

	mentionPattern := "@" + strings.ToLower(username)
	return strings.Contains(strings.ToLower(commentBody), mentionPattern)
}
