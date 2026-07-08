package teamsbot

// Activity represents a Bot Framework Activity (the message format used by Teams).
type Activity struct {
	Type                 string                 `json:"type"`
	ID                   string                 `json:"id,omitempty"`
	Timestamp            string                 `json:"timestamp,omitempty"`
	ServiceURL           string                 `json:"serviceUrl,omitempty"`
	ChannelID            string                 `json:"channelId,omitempty"`
	From                 *ChannelAccount        `json:"from,omitempty"`
	Conversation         *ConversationAccount   `json:"conversation,omitempty"`
	Recipient            *ChannelAccount        `json:"recipient,omitempty"`
	TextFormat           string                 `json:"textFormat,omitempty"`
	Text                 string                 `json:"text,omitempty"`
	Attachments          []Attachment           `json:"attachments,omitempty"`
	Entities             []Entity               `json:"entities,omitempty"`
	ReplyToID            string                 `json:"replyToId,omitempty"`
	Action               string                 `json:"action,omitempty"`
	MembersAdded         []ChannelAccount       `json:"membersAdded,omitempty"`
	MembersRemoved       []ChannelAccount       `json:"membersRemoved,omitempty"`
}

type ChannelAccount struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	AADObjectID    string `json:"aadObjectId,omitempty"`
	Role           string `json:"role,omitempty"`
}

type ConversationAccount struct {
	ID                string `json:"id"`
	Name              string `json:"name,omitempty"`
	ConversationType   string `json:"conversationType,omitempty"`
	TenantID           string `json:"tenantId,omitempty"`
}

type Attachment struct {
	ContentType string `json:"contentType"`
	ContentURL  string `json:"contentUrl,omitempty"`
	Content     any    `json:"content,omitempty"`
	Name        string `json:"name,omitempty"`
}

type Entity struct {
	Type       string           `json:"type"`
	Text       string           `json:"text,omitempty"`
	Mentioned  *ChannelAccount  `json:"mentioned,omitempty"`
}

type AdaptiveCard struct {
	Type    string           `json:"type"`
	Version string           `json:"version"`
	Body    []AdaptiveElement `json:"body"`
}

type AdaptiveElement struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	Size   string `json:"size,omitempty"`
	Weight string `json:"weight,omitempty"`
	Wrap   bool   `json:"wrap,omitempty"`
	URL    string `json:"url,omitempty"`
	AltText string `json:"altText,omitempty"`
}

const (
	ActivityTypeMessage           = "message"
	ActivityTypeConversationUpdate = "conversationUpdate"
	ConversationTypePersonal      = "personal"
	ConversationTypeChannel       = "channel"
	EntityTypeMention             = "mention"
	ContentTypeAdaptiveCard       = "application/vnd.microsoft.card.adaptive"
)
