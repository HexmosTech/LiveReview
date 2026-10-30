package pushnotify

import "testing"

func TestShortRepoLabel(t *testing.T) {
	cases := map[string]string{
		"https://github.com/HexmosTech/LiveReview":          "HexmosTech/LiveReview",
		"https://github.com/HexmosTech/LiveReview/pull/150": "HexmosTech/LiveReview",
		"git@github.com:HexmosTech/LiveReview.git":          "HexmosTech/LiveReview",
		"HexmosTech/LiveReview":                             "HexmosTech/LiveReview",
		"":                                                  "",
	}
	for input, want := range cases {
		if got := shortRepoLabel(input); got != want {
			t.Errorf("shortRepoLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short string should be unchanged, got %q", got)
	}
	long := "this is a much longer string than the limit allows"
	got := truncate(long, 10)
	if len([]rune(got)) > 11 { // 10 runes + the ellipsis rune
		t.Errorf("truncate(%q, 10) = %q, too long", long, got)
	}
}

func TestBuildPayloadUsesReviewSpecifics(t *testing.T) {
	info := &reviewInfo{
		repository: "https://github.com/HexmosTech/LiveReview",
		mrTitle:    "Stream findings live, and notify the triggering user via browser push",
	}

	p := buildPayload(KindFirstFinding, 13524, info, "https://livereview.hexmos.com")
	if p.URL != "https://livereview.hexmos.com/#/reviews/13524" {
		t.Errorf("unexpected URL: %q", p.URL)
	}
	if p.Tag != "review-13524" {
		t.Errorf("unexpected tag: %q", p.Tag)
	}
	wantSubstrings := []string{"#13524", "HexmosTech/LiveReview", "Stream findings live"}
	for _, want := range wantSubstrings {
		if !contains(p.Body, want) {
			t.Errorf("body %q missing %q", p.Body, want)
		}
	}

	failed := buildPayload(KindFailed, 13524, info, "")
	if failed.Title == "Review failed" {
		t.Errorf("expected a softer failure title, got %q", failed.Title)
	}
	if failed.Title != "Review completed with errors" {
		t.Errorf("unexpected failed title: %q", failed.Title)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
