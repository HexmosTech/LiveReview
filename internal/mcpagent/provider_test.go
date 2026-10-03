package mcpagent

import (
	"testing"

	"github.com/livereview/internal/aiconnectors"
)

func TestLLMErrorTemplates(t *testing.T) {
	tests := []struct {
		name     string
		category aiconnectors.LLMErrorCategory
		want     string
	}{
		{
			name:     "Auth Error",
			category: aiconnectors.ErrCategoryAuth,
			want:     "**Action Required: AI Provider Issue**\n\nThe AI Provider's API key is invalid or the model is missing. Please configure a valid provider in settings to continue.",
		},
		{
			name:     "Overload Error",
			category: aiconnectors.ErrCategoryOverload,
			want:     "**AI Provider is Busy**\n\nThe selected model is currently experiencing high traffic. If you continue to see this issue, please edit your configuration to change to a different model or provider.",
		},
		{
			name:     "Timeout Error",
			category: aiconnectors.ErrCategoryTimeout,
			want:     "**Analysis Took Too Long**\n\nThe AI took too long to generate your response and timed out. Try asking a narrower or more specific question.",
		},
		{
			name:     "Unknown Error",
			category: aiconnectors.ErrCategoryUnknown,
			want:     "",
		},
		{
			name:     "Unmapped Category",
			category: aiconnectors.LLMErrorCategory("something_else"),
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			if tpl, ok := LLMErrorTemplates[tt.category]; ok {
				got = tpl.Message
			}
			if got != tt.want {
				t.Errorf("LLMErrorTemplates[%v].Message = %q, want %q", tt.category, got, tt.want)
			}
		})
	}
}
