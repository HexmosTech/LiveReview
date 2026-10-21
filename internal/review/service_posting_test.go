package review

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsPostingPermissionError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"github 403", fmt.Errorf("failed to post summary comment: GitHub general comment failed: 403 Forbidden"), true},
		{"github app token on a repo it is not installed on", errors.New("Resource not accessible by integration"), true},
		{"server error", errors.New("GitHub general comment failed: 502 Bad Gateway"), false},
		{"network", errors.New("dial tcp: i/o timeout"), false},
		{"not found", errors.New("GitHub general comment failed: 404 Not Found"), false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPostingPermissionError(tc.err); got != tc.want {
				t.Errorf("isPostingPermissionError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
