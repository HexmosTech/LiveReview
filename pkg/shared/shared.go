package shared

// VCSCredentials holds the credentials for a version control system.
type VCSCredentials struct {
	Provider string
	Email    string
	Token    string
}
