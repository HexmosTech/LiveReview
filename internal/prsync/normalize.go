package prsync

// NormalizeGitHubState maps GitHub's PR state fields to the canonical
// "open" | "closed" | "merged" values stored in pull_requests.state.
// GitHub reports state as "open"/"closed" plus a separate merged boolean;
// merged always wins regardless of the state field.
func NormalizeGitHubState(state string, merged bool) string {
	if merged {
		return "merged"
	}
	if state == "closed" {
		return "closed"
	}
	return "open"
}

// NormalizeGitLabState maps GitLab's MR state field to the canonical
// "open" | "closed" | "merged" values stored in pull_requests.state.
// "locked" is not a terminal state in GitLab (it's an orthogonal flag on
// newer payloads) but is treated conservatively as closed if ever seen as
// a bare state value.
func NormalizeGitLabState(state string) string {
	switch state {
	case "opened", "open":
		return "open"
	case "merged":
		return "merged"
	case "closed", "locked":
		return "closed"
	default:
		return "open"
	}
}
