package github

import (
	"testing"

	coreprocessor "github.com/livereview/internal/core_processor"
)

func TestDetectDirectMention(t *testing.T) {
	cases := []struct {
		name     string
		username string
		body     string
		want     bool
	}{
		{"plain user", "livereview-bot", "hey @LiveReview-Bot please look", true},
		{"no mention", "livereview-bot", "nothing here", false},
		{"app bot mentioned by slug", "livereview[bot]", "@livereview can you review this?", true},
		{"app bot mentioned with suffix", "livereview[bot]", "@livereview[bot] review", true},
		{"app bot not mentioned", "livereview[bot]", "@someone else", false},
		{"empty username", "", "@anything", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectDirectMention(tc.body, &coreprocessor.UnifiedBotUserInfoV2{Username: tc.username})
			if got != tc.want {
				t.Fatalf("DetectDirectMention(%q, %q) = %v, want %v", tc.body, tc.username, got, tc.want)
			}
		})
	}
	if DetectDirectMention("@x", nil) {
		t.Fatal("nil bot info must not match")
	}
}
