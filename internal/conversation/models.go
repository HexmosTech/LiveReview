package conversation

// Minimal domain models for conversation graph (threads, messages, reactions).
// Flesh out as we implement the reply capability.

type ActorType string

const (
	ActorHuman  ActorType = "human"
	ActorAI     ActorType = "ai"
	ActorSystem ActorType = "system"
)

type Conversation struct {
	ID            int64
	ProviderType  string
	ProviderMRURL string
	RepoFullName  string
	SourceBranch  string
	TargetBranch  string
	State         string
}

type Thread struct {
	ID           int64
	Conversation int64
	HostThreadID string
	AnchorType   string // general | line
	FilePath     string
	Line         int
	BaseSHA      string
	HeadSHA      string
	State        string // open | resolved
}

type Message struct {
	ID              int64
	ThreadID        int64
	HostMessageID   string
	ActorType       ActorType
	ActorHandle     string
	Body            string
	Kind            string // general | line_comment | system
	InReplyToHostID string
}

type Reaction struct {
	ID          int64
	MessageID   int64
	ActorHandle string
	Type        string
}
