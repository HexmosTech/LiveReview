package bitbucket

import (
	"strings"
	"unicode"

	coreprocessor "github.com/livereview/internal/core_processor"
)

// DetectDirectMention returns true when the comment body contains a direct mention of the bot
// taking into account Bitbucket's account ID and UUID mention formats in addition to usernames.
func DetectDirectMention(commentBody string, botInfo *coreprocessor.UnifiedBotUserInfoV2) bool {
	if botInfo == nil {
		return false
	}

	commentLower := strings.ToLower(commentBody)
	if username := strings.TrimSpace(botInfo.Username); username != "" {
		mentionPattern := "@" + strings.ToLower(username)
		if strings.Contains(commentLower, mentionPattern) {
			return true
		}
	}

	normalizedIdentifiers := make(map[string]string)
	if normalized := normalizeIdentifier(botInfo.UserID); normalized != "" {
		normalizedIdentifiers[normalized] = botInfo.UserID
	}

	if botInfo.Metadata != nil {
		if accountIDRaw, ok := botInfo.Metadata["account_id"].(string); ok {
			if normalized := normalizeIdentifier(accountIDRaw); normalized != "" {
				normalizedIdentifiers[normalized] = accountIDRaw
			}
		}
		if uuidRaw, ok := botInfo.Metadata["uuid"].(string); ok {
			if normalized := normalizeIdentifier(uuidRaw); normalized != "" {
				normalizedIdentifiers[normalized] = uuidRaw
			}
		}
	}

	if len(normalizedIdentifiers) > 0 {
		for _, mention := range extractNormalizedMentions(commentBody) {
			if mention == "" {
				continue
			}
			if _, ok := normalizedIdentifiers[mention]; ok {
				return true
			}
		}
	}

	if botInfo.Metadata != nil {
		if accountIDRaw, ok := botInfo.Metadata["account_id"].(string); ok && accountIDRaw != "" {
			accountIDPattern := strings.ToLower("@{" + strings.Trim(accountIDRaw, "{}") + "}")
			if strings.Contains(commentLower, accountIDPattern) {
				return true
			}
		}
	}

	return false
}

func normalizeIdentifier(value string) string {
	if value == "" {
		return ""
	}
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, "{}")
	return strings.ToLower(trimmed)
}

func extractNormalizedMentions(commentBody string) []string {
	runes := []rune(commentBody)
	length := len(runes)
	mentions := make([]string, 0)
	seen := make(map[string]struct{})

	isIdentifierRune := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ':' || r == '.'
	}

	for i := 0; i < length; i++ {
		if runes[i] != '@' {
			continue
		}

		if i+1 >= length {
			continue
		}

		var identifier string
		if runes[i+1] == '{' {
			start := i + 2
			j := start
			for j < length && runes[j] != '}' {
				j++
			}
			identifier = string(runes[start:j])
			i = j
		} else {
			start := i + 1
			j := start
			for j < length && isIdentifierRune(runes[j]) {
				j++
			}
			identifier = string(runes[start:j])
			i = j - 1
		}

		normalized := normalizeIdentifier(identifier)
		if normalized == "" {
			continue
		}

		if _, exists := seen[normalized]; exists {
			continue
		}

		seen[normalized] = struct{}{}
		mentions = append(mentions, normalized)
	}

	return mentions
}
