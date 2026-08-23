package chatexport

import (
	"context"
	"fmt"

	"github.com/livereview/internal/vlrender"
	storagechat "github.com/livereview/storage/chat"
)

// BuildOptions controls what BuildDoc includes in the resulting ExportDoc.
type BuildOptions struct {
	// IncludeDebugArtifacts copies each message's DebugArtifacts into the
	// IR. Callers must derive this from the conversation's own stored
	// surface (never from client input) - see
	// internal/api.ExportConversation, which is the only caller today.
	IncludeDebugArtifacts bool
}

// BuildDoc loads conversationID - scoped to orgID/userID, matching every
// other storagechat.Store read - and renders each of its charts to PNG via
// the same vlrender path RenderChart already uses, producing one ExportDoc
// that RenderPDF/RenderHTML both consume unchanged.
func BuildDoc(ctx context.Context, store *storagechat.Store, orgID, userID, conversationID int64, opts BuildOptions) (*ExportDoc, error) {
	conv, err := store.GetConversation(ctx, orgID, userID, conversationID)
	if err != nil {
		return nil, err
	}
	messages, err := store.ListMessages(ctx, orgID, userID, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}

	doc := &ExportDoc{
		Conversation: ExportConversation{
			Title:     conv.Title,
			CreatedAt: conv.CreatedAt,
			UpdatedAt: conv.UpdatedAt,
		},
		Turns: make([]ExportTurn, 0, len(messages)),
	}

	for i, m := range messages {
		turn := ExportTurn{
			Seq:  i + 1,
			Role: m.Role,
			Text: m.Content,
		}
		if opts.IncludeDebugArtifacts {
			turn.DebugArtifacts = m.DebugArtifacts
		}
		for _, ch := range m.Charts {
			png, _, err := vlrender.ConvertVegaLiteToPNG(ctx, ch.VegaSpec, "2")
			if err != nil {
				return nil, fmt.Errorf("render chart %d: %w", ch.ID, err)
			}
			turn.Charts = append(turn.Charts, ExportChart{
				Title:       ch.Title,
				Description: ch.Description,
				PNG:         png,
			})
		}
		for _, f := range m.Files {
			turn.Files = append(turn.Files, ExportFile{
				Filename: f.Filename,
				Kind:     f.Kind,
				Rows:     f.Rows,
			})
		}
		doc.Turns = append(doc.Turns, turn)
	}

	return doc, nil
}
