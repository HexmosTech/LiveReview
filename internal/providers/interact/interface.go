package interact

// Minimal, provider-agnostic interaction layer for threaded replies and reactions.
// Hosts (GitLab/GitHub/Bitbucket) will implement this interface.

// Commit represents a commit associated with an MR iteration.
type Commit struct {
	SHA        string
	ParentSHAs []string
	Title      string
	Message    string
	Author     string
	CreatedAt  string // ISO8601
}

// ThreadSummary captures a thread anchor and host identifiers.
type ThreadSummary struct {
	HostThreadID string
	AnchorType   string // "general" | "line"
	FilePath     string
	Line         int
	BaseSHA      string
	HeadSHA      string
	State        string // open | resolved
}

// HostMessage represents a single host comment message.
type HostMessage struct {
	HostMessageID    string
	HostThreadID     string
	ActorHandle      string
	ActorType        string // human | ai | system
	Body             string
	InReplyToHostID  string
	Kind             string // general | line_comment | system
	CreatedAtISO8601 string
	UpdatedAtISO8601 string
}

// PostedMessage is returned after successfully posting/replying.
type PostedMessage struct {
	HostMessageID string
	HostThreadID  string
}

// InteractionAdapter defines cross-host reply and reaction capabilities.
type InteractionAdapter interface {
	// Listing
	ListCommits(mrID string) ([]Commit, error)
	ListThreads(mrID string) ([]ThreadSummary, error)
	ListMessages(threadHostID string) ([]HostMessage, error)

	// Posting
	ReplyToMessage(mrID string, inReplyToHostMessageID string, body string) (PostedMessage, error)
	AddReaction(hostMessageID string, reaction string) error
	MarkResolved(threadHostID string) error
}
