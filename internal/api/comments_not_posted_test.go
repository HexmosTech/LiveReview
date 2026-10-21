package api

import (
	"testing"

	"github.com/livereview/internal/githubapp"
)

func TestCommentsNotPostedNotice(t *testing.T) {
	s := &Server{}

	// GitHub PR, App configured: names the repo and offers the install.
	s.githubApp = &githubapp.Config{}
	n := s.commentsNotPostedNotice("https://github.com/HexmosTech/LiveReview/pull/50")
	if n["repository"] != "HexmosTech/LiveReview" || n["can_install"] != true {
		t.Fatalf("github with app configured: %+v", n)
	}
	if msg, _ := n["message"].(string); msg == "" || !contains(msg, "HexmosTech/LiveReview") {
		t.Fatalf("message must name the repository: %q", msg)
	}

	// GitHub PR, App not configured on this server: still explains, but no install button.
	s.githubApp = nil
	if n := s.commentsNotPostedNotice("https://github.com/HexmosTech/LiveReview/pull/50"); n["can_install"] != false {
		t.Fatalf("no app configured, install must not be offered: %+v", n)
	}

	// Other providers: generic message, never an install button (there is no GitHub App for them).
	s.githubApp = &githubapp.Config{}
	n = s.commentsNotPostedNotice("https://gitlab.com/group/project/-/merge_requests/3")
	if n["can_install"] != false || n["repository"] != nil {
		t.Fatalf("gitlab: %+v", n)
	}
}
