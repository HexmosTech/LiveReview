package mcpagent

import (
	"fmt"
	"strings"

	"github.com/livereview/internal/aiconnectors"
)

// LLMErrorTemplate defines the UI representation of an LLM error,
// including both the chat message text and the Action Card to render.
type LLMErrorTemplate struct {
	Message    string
	ActionCard ActionCard
}

// LLMErrorTemplates stores the fallback configurations for various LLM errors.
// This allows the frontend to automatically render the correct box without
// hardcoded display logic.
var LLMErrorTemplates = map[aiconnectors.LLMErrorCategory]LLMErrorTemplate{
	aiconnectors.ErrCategoryAuth: {
		Message: "> **Action Required: AI Provider Issue**\n> \n> The AI Provider's API key is invalid or the model is missing. Please configure a valid provider in settings to continue.",
		ActionCard: ActionCard{
			Title:       "Configuration Required",
			Description: "Please edit the current AI provider configuration to continue.",
			ButtonText:  "Configure AI Provider",
			ActionURL:   "/ai",
		},
	},
	aiconnectors.ErrCategoryOverload: {
		Message: "> **AI Provider is Busy**\n> \n> The selected model is currently experiencing high traffic. If you continue to see this issue, please edit your configuration to change to a different model or provider.",
		ActionCard: ActionCard{
			Title:       "High Traffic Detected",
			Description: "Please select a different model in settings.",
			ButtonText:  "Configure AI Provider",
			ActionURL:   "/ai",
		},
	},
	aiconnectors.ErrCategoryDeprecated: {
		Message: "> **Model No Longer Available**\n> \n> The selected AI model has been deprecated or removed by the provider. Please update your AI provider configuration to use a different model.",
		ActionCard: ActionCard{
			Title:       "Model No Longer Available",
			Description: "The selected model has been deprecated. Please update your AI provider configuration.",
			ButtonText:  "Configure AI Provider",
			ActionURL:   "/ai",
		},
	},
	aiconnectors.ErrCategoryTimeout: {
		Message: "> **Analysis Took Too Long**\n> \n> The AI took too long to generate your response and timed out. Try asking a narrower or more specific question.",
		ActionCard: ActionCard{
			Title:       "Timeout Error",
			Description: "The request took too long to complete.",
			ButtonText:  "Try Again",
			ActionURL:   "#retry",
		},
	},
	aiconnectors.ErrCategoryUnknown: {
		Message: "> **Unexpected Error**\n> \n> An unexpected error occurred while communicating with the AI provider. The technical details have been attached below for troubleshooting.",
		ActionCard: ActionCard{
			Title:       "Unexpected Error",
			Description: "An unknown error occurred during processing.",
			ButtonText:  "Try Again",
			ActionURL:   "#retry",
		},
	},
}

const ModelOutputErrorText = "Model Output Error"

// LookupErrorTemplate returns the error template for a given category.
func LookupErrorTemplate(category aiconnectors.LLMErrorCategory) (LLMErrorTemplate, bool) {
	tpl, ok := LLMErrorTemplates[category]
	return tpl, ok
}

// MatchErrorTemplateByText returns the error template matching the provided response text.
func MatchErrorTemplateByText(text string) (LLMErrorTemplate, bool) {
	for _, tpl := range LLMErrorTemplates {
		if strings.HasPrefix(text, tpl.Message) {
			return tpl, true
		}
	}
	return LLMErrorTemplate{}, false
}

// NewMCPOfflineError generates the error response components when the MCP server is unreachable.
func NewMCPOfflineError(err error, mcpURL string) (string, ActionCard, *DebugArtifacts) {
	msg := "> **Something went wrong on our end**\n> \n> I am having trouble connecting to my internal data tools right now. The technical details have been attached below for troubleshooting. Please try your request again in a few moments."
	
	debugLog := fmt.Sprintf("MCP Connection Failed: %s\nURL: %s", err.Error(), mcpURL)
	debugArt := &DebugArtifacts{RawLLMError: debugLog}
	
	card := ActionCard{
		Title:       "MCP Server Offline",
		Description: "The internal data server is unreachable (502 Bad Gateway).",
		ButtonText:  "Try Again",
		ActionURL:   "#retry",
	}

	return msg, card, debugArt
}

// InjectErrorState parses the text and populates the HistoryEntry with the appropriate ActionCard,
// DebugArtifacts, SuggestedQuestions, and sets is_error to true if it matches a known error state.
func InjectErrorState(entry HistoryEntry, text string, debugArt *DebugArtifacts) {
	if tpl, ok := MatchErrorTemplateByText(text); ok {
		entry["suggested_questions"] = DefaultAIErrorSuggestedQuestions
		card := tpl.ActionCard // copy
		entry["action_card"] = &card
		entry["debug_artifacts"] = debugArt
		entry["is_error"] = true
	} else if strings.Contains(text, ModelOutputErrorText) {
		// Also check for format errors, which are hallucinations, not provider errors
		entry["suggested_questions"] = DefaultAIErrorSuggestedQuestions
		entry["action_card"] = &ActionCard{
			Title:       "Formatting Error",
			Description: "The model failed to format the response correctly.",
			ButtonText:  "Try Again",
			ActionURL:   "#retry",
		}
		entry["debug_artifacts"] = debugArt
		entry["is_error"] = true
	}
}
