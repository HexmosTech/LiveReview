package main

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
	}{
		{"trailing_slash", "https://github.com/acme/repo/pull/1", "https://github.com/acme/repo/pull/1/"},
		{"case_insensitive_host", "https://GitHub.com/acme/repo/pull/1", "https://github.com/acme/repo/pull/1"},
		{"dot_git_suffix", "https://gitlab.com/group/repo.git", "https://gitlab.com/group/repo"},
		{"query_string_ignored", "https://github.com/acme/repo/pull/1?tab=files", "https://github.com/acme/repo/pull/1"},
		{"fragment_ignored", "https://github.com/acme/repo/pull/1#discussion_r1", "https://github.com/acme/repo/pull/1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			na, nb := normalizeURL(tc.a), normalizeURL(tc.b)
			if na != nb {
				t.Errorf("normalizeURL(%q)=%q != normalizeURL(%q)=%q, expected equal", tc.a, na, tc.b, nb)
			}
		})
	}
}

func TestNormalizeURL_DistinctURLsStayDistinct(t *testing.T) {
	a := normalizeURL("https://github.com/acme/repo/pull/1")
	b := normalizeURL("https://github.com/acme/repo/pull/2")
	if a == b {
		t.Errorf("expected different PR URLs to normalize differently, both got %q", a)
	}
}

func TestNormalizeURL_Empty(t *testing.T) {
	if got := normalizeURL(""); got != "" {
		t.Errorf("normalizeURL(\"\") = %q, want empty string", got)
	}
	if got := normalizeURL("   "); got != "" {
		t.Errorf("normalizeURL(whitespace) = %q, want empty string", got)
	}
}
