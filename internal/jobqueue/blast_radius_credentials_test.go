package jobqueue

import "testing"

// GitHub App connectors store their installation token in pat_token and leave access_token empty, so the clone
// credentials must come from pat_token for them just as for a PAT (an empty token made every clone fail).
func TestGitCredentials_GitHubAppUsesCachedInstallationToken(t *testing.T) {
	for _, tc := range []struct {
		name, tokenType, access, pat, want string
	}{
		{"github app", "GITHUB_APP", "", "ghs_install", "ghs_install"},
		{"pat", "PAT", "", "ghp_pat", "ghp_pat"},
		{"oauth", "oauth", "gho_oauth", "", "gho_oauth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user, token := gitCredentials("github", tc.access, tc.tokenType, tc.pat)
			if token != tc.want {
				t.Errorf("token %q, want %q", token, tc.want)
			}
			if user != "" {
				t.Errorf("username %q, want empty so repocache uses its GitHub default", user)
			}
		})
	}
}
