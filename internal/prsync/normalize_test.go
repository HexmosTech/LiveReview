package prsync

import "testing"

func TestNormalizeGitHubState(t *testing.T) {
	cases := []struct {
		name   string
		state  string
		merged bool
		want   string
	}{
		{"open", "open", false, "open"},
		{"closed_not_merged", "closed", false, "closed"},
		{"closed_and_merged", "closed", true, "merged"},
		{"open_but_merged_flag", "open", true, "merged"}, // defensive: merged always wins
		{"unknown_state", "weird", false, "open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeGitHubState(tc.state, tc.merged)
			if got != tc.want {
				t.Errorf("NormalizeGitHubState(%q, %v) = %q, want %q", tc.state, tc.merged, got, tc.want)
			}
		})
	}
}

func TestNormalizeGitLabState(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  string
	}{
		{"opened", "opened", "open"},
		{"open", "open", "open"},
		{"merged", "merged", "merged"},
		{"closed", "closed", "closed"},
		{"locked", "locked", "closed"},
		{"unknown", "weird", "open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeGitLabState(tc.state)
			if got != tc.want {
				t.Errorf("NormalizeGitLabState(%q) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}
