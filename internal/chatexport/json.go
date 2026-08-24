package chatexport

import (
	"encoding/json"
	"time"

	"github.com/livereview/internal/vlrender"
)

// CompileJSONExport is the top-level structure for a multi-conversation
// JSON debug export. It mirrors the CompiledDoc but adds user/org context
// and db field annotations, and omits chart PNGs in favor of Vega specs.
type CompileJSONExport struct {
	Schema        string               `json:"$schema"`
	FormatVersion string               `json:"format_version"`
	ExportedAt    time.Time            `json:"exported_at"`
	Title         string               `json:"title"`
	Subtitle      string               `json:"subtitle,omitempty"`
	Context       ExportContext         `json:"context"`
	Conversations []CompileJSONConvDoc `json:"conversations"`
}

// ExportContext captures the user, org, and role context at export time.
type ExportContext struct {
	User ExportedUser   `json:"user"`
	Org  ExportedOrg    `json:"org"`
	Role ExportedRole   `json:"role"`
}

// ExportedUser is the user who triggered the export.
type ExportedUser struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	FullName  string `json:"full_name"`
	DBTable   string `json:"db_table"`
	DBFields  []string `json:"db_fields"`
}

// ExportedOrg is the organization the export is scoped to.
type ExportedOrg struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Description      string  `json:"description,omitempty"`
	IsActive         bool    `json:"is_active"`
	SubscriptionPlan *string `json:"subscription_plan,omitempty"`
	DBTable          string  `json:"db_table"`
	DBFields         []string `json:"db_fields"`
}

// ExportedRole captures the user's role in the org at export time.
type ExportedRole struct {
	Role         string `json:"role"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	IsOwner      bool   `json:"is_owner"`
	IsMember     bool   `json:"is_member"`
	DBTable      string `json:"db_table"`
	DBFields     []string `json:"db_fields"`
	RoleLookup   string `json:"role_lookup"`
}

// CompileJSONConvDoc is one conversation within a compile export.
type CompileJSONConvDoc struct {
	Conversation ExportConversation `json:"conversation"`
	Turns        []ExportJSONTurn   `json:"turns"`
}

// ExportJSONTurn is one turn in the JSON export. Unlike ExportTurn, it
// carries Vega specs instead of PNGs, and includes file CSV data.
type ExportJSONTurn struct {
	Seq            int               `json:"seq"`
	Role           string            `json:"role"`
	Text           string            `json:"text"`
	CreatedAt      time.Time         `json:"created_at"`
	Charts         []ExportJSONChart `json:"charts"`
	Files          []ExportJSONFile  `json:"files"`
	DebugArtifacts json.RawMessage   `json:"debug_artifacts,omitempty"`
	DBTable        string            `json:"db_table"`
	DBFields       []string          `json:"db_fields"`
}

// ExportJSONChart carries the Vega spec and query metadata instead of PNG.
type ExportJSONChart struct {
	Title         string            `json:"title,omitempty"`
	Description   string            `json:"description,omitempty"`
	Query         string            `json:"query,omitempty"`
	TimeRange     string            `json:"time_range,omitempty"`
	Granularity   string            `json:"granularity,omitempty"`
	Context       vlrender.ChartContext `json:"context,omitempty"`
	VegaSpec      json.RawMessage   `json:"vega_spec"`
	RawLLMOutput  string            `json:"raw_llm_output,omitempty"`
	DBTable       string            `json:"db_table"`
	DBFields      []string          `json:"db_fields"`
}

// ExportJSONFile carries file metadata plus CSV data.
type ExportJSONFile struct {
	Filename    string            `json:"filename"`
	Kind        string            `json:"kind"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Query       string            `json:"query,omitempty"`
	TimeRange   string            `json:"time_range,omitempty"`
	Granularity string            `json:"granularity,omitempty"`
	Context     vlrender.ChartContext `json:"context,omitempty"`
	Rows        int               `json:"rows"`
	CSVData     string            `json:"csv_data,omitempty"`
	DBTable     string            `json:"db_table"`
	DBFields    []string          `json:"db_fields"`
}

// BuildCompileJSON converts a CompiledDoc into the JSON export structure.
// The caller provides user/org/role context from the PermissionContext.
func BuildCompileJSON(
	doc *CompiledDoc,
	userID int64, userEmail, userFirstName, userLastName, userFullName string,
	orgID int64, orgName, orgDescription string, orgIsActive bool, orgSubPlan *string,
	role string, isSuperAdmin, isOwner, isMember bool,
) *CompileJSONExport {
	out := &CompileJSONExport{
		Schema:        "livereview/chat-debug-compile-export/v1",
		FormatVersion: "1.0.0",
		ExportedAt:    time.Now().UTC(),
		Title:         doc.Title,
		Subtitle:      doc.Subtitle,
		Context: ExportContext{
			User: ExportedUser{
				ID:        userID,
				Email:     userEmail,
				FirstName: userFirstName,
				LastName:  userLastName,
				FullName:  userFullName,
				DBTable:   "users",
				DBFields:  []string{"id", "email", "first_name", "last_name", "is_active", "created_at", "updated_at"},
			},
			Org: ExportedOrg{
				ID:               orgID,
				Name:             orgName,
				Description:      orgDescription,
				IsActive:         orgIsActive,
				SubscriptionPlan: orgSubPlan,
				DBTable:          "organizations",
				DBFields:         []string{"id", "name", "description", "is_active", "subscription_plan", "subscription_status", "max_users", "created_by_user_id", "settings"},
			},
			Role: ExportedRole{
				Role:         role,
				IsSuperAdmin: isSuperAdmin,
				IsOwner:      isOwner,
				IsMember:     isMember,
				DBTable:      "user_roles",
				DBFields:     []string{"user_id", "role_id", "org_id"},
				RoleLookup:   "roles.name via user_roles.role_id = roles.id",
			},
		},
	}

	out.Conversations = make([]CompileJSONConvDoc, 0, len(doc.Conversations))
	for _, conv := range doc.Conversations {
		jc := CompileJSONConvDoc{
			Conversation: conv.Conversation,
		}
		jc.Turns = make([]ExportJSONTurn, 0, len(conv.Turns))
		for _, turn := range conv.Turns {
			jt := ExportJSONTurn{
				Seq:            turn.Seq,
				Role:           turn.Role,
				Text:           turn.Text,
				CreatedAt:      turn.CreatedAt,
				DebugArtifacts: turn.DebugArtifacts,
				DBTable:        "chat_messages",
				DBFields:       []string{"id", "conversation_id", "role", "content", "turn_seq", "created_at"},
			}
			jt.Charts = make([]ExportJSONChart, 0, len(turn.Charts))
			for _, ch := range turn.Charts {
				jt.Charts = append(jt.Charts, ExportJSONChart{
					Title:        ch.Title,
				Description:  ch.Description,
					Query:        ch.Query,
					TimeRange:    ch.TimeRange,
					Granularity:  ch.Granularity,
					Context:      ch.Context,
					VegaSpec:     ch.VegaSpec,
					RawLLMOutput: ch.RawLLMOutput,
					DBTable:      "chat_charts",
					DBFields:     []string{"id", "message_id", "title", "description", "query", "time_range", "granularity", "context", "triggering_user_message", "vega_spec", "raw_llm_output", "created_at"},
				})
			}
			jt.Files = make([]ExportJSONFile, 0, len(turn.Files))
			for _, f := range turn.Files {
				jt.Files = append(jt.Files, ExportJSONFile{
					Filename:    f.Filename,
					Kind:        f.Kind,
					Title:       f.Title,
					Description: f.Description,
					Query:       f.Query,
					TimeRange:   f.TimeRange,
					Granularity: f.Granularity,
					Context:     f.Context,
					Rows:        f.Rows,
					CSVData:     f.CSVData,
					DBTable:     "chat_files",
					DBFields:    []string{"id", "message_id", "kind", "filename", "title", "description", "query", "time_range", "granularity", "context", "rows", "created_at"},
				})
			}
			jc.Turns = append(jc.Turns, jt)
		}
		out.Conversations = append(out.Conversations, jc)
	}

	return out
}
