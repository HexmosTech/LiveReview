package chatexport

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/livereview/internal/chatstats"
	"github.com/livereview/internal/vlrender"
	storagechat "github.com/livereview/storage/chat"
	"github.com/rs/zerolog/log"
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
			Seq:       i + 1,
			Role:      m.Role,
			CreatedAt: m.CreatedAt,
			Text:      m.Content,
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
				Stats:       resolveExportStats(ch.Stats),
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

// BuildCompiledDoc loads and combines several conversations - each scoped
// to orgID/userID like BuildDoc - into one CompiledDoc, in the given order.
// The whole request fails if any id isn't found/owned, or if the
// conversations don't all share the same stored surface: since
// opts.IncludeDebugArtifacts applies uniformly to the whole compiled
// document, mixing surfaces could otherwise let a crafted request smuggle
// one chat_debug conversation's debug data into what looks like a plain
// compiled export.
func BuildCompiledDoc(ctx context.Context, store *storagechat.Store, orgID, userID int64, conversationIDs []int64, title, subtitle string, opts BuildOptions) (*CompiledDoc, error) {
	if len(conversationIDs) == 0 {
		return nil, fmt.Errorf("no conversations selected")
	}

	compiled := &CompiledDoc{
		Title:         title,
		Subtitle:      subtitle,
		Conversations: make([]ExportDoc, 0, len(conversationIDs)),
	}

	var surface string
	for i, id := range conversationIDs {
		conv, err := store.GetConversation(ctx, orgID, userID, id)
		if err != nil {
			return nil, fmt.Errorf("conversation %d: %w", id, err)
		}
		if i == 0 {
			surface = conv.Surface
		} else if conv.Surface != surface {
			return nil, fmt.Errorf("conversation %d has a different surface than the rest of the selection", id)
		}

		doc, err := BuildDoc(ctx, store, orgID, userID, id, opts)
		if err != nil {
			return nil, err
		}
		compiled.Conversations = append(compiled.Conversations, *doc)
	}

	return compiled, nil
}

// resolveExportStats flattens a chart's stored chatstats.AllStats blob into
// the label/value chips both renderers show, mirroring the chips
// ChatConversation.tsx renders for the same "kind" (see StatChip usage
// there). A trend-shaped chart exports its day-granularity view - an export
// is a static snapshot, not an interactive toggle, so day (matching what a
// reader sees opening the chat fresh) is the one view worth freezing. Returns
// nil when statsJSON is empty or its granularity has no data (e.g. a chart
// with less than a day of history).
func resolveExportStats(statsJSON []byte) []StatLine {
	if len(statsJSON) == 0 {
		return nil
	}
	var all chatstats.AllStats
	if err := json.Unmarshal(statsJSON, &all); err != nil {
		log.Warn().Err(err).Msg("resolveExportStats: failed to unmarshal stored AllStats blob, exporting without stats")
		return nil
	}

	switch all.Kind {
	case "trend":
		var s chatstats.TrendStats
		if len(all.Day) == 0 {
			return nil
		}
		if err := json.Unmarshal(all.Day, &s); err != nil {
			log.Warn().Err(err).Msg("resolveExportStats: failed to unmarshal trend day stats")
			return nil
		}
		trend := "n/a"
		if s.TrendPct != nil {
			direction := "up"
			pct := *s.TrendPct
			if pct < 0 {
				direction = "down"
				pct = -pct
			}
			trend = fmt.Sprintf("%s %s%% (%s → %s)", direction, formatNum(pct), formatExportDate(s.FirstDate), formatExportDate(s.LastDate))
		}
		return []StatLine{
			{"Total", formatNum(s.Total)},
			{"Avg per period", formatNum(s.AvgPerPeriod)},
			{"Peak", fmt.Sprintf("%s (%s)", formatNum(s.Peak.Value), formatExportDate(s.Peak.Date))},
			{"Low", fmt.Sprintf("%s (%s)", formatNum(s.Low.Value), formatExportDate(s.Low.Date))},
			{"Trend", trend},
		}
	case "multi_series_trend":
		var s chatstats.MultiSeriesTrendStats
		if len(all.Day) == 0 {
			return nil
		}
		if err := json.Unmarshal(all.Day, &s); err != nil {
			log.Warn().Err(err).Msg("resolveExportStats: failed to unmarshal multi-series trend day stats")
			return nil
		}
		return []StatLine{
			{"Total", formatNum(s.Total)},
			{"Series", strconv.Itoa(s.SeriesCount)},
			{"Top series", fmt.Sprintf("%s (%s)", s.TopSeries.Label, formatNum(s.TopSeries.Value))},
			{"Range", fmt.Sprintf("%s → %s", formatExportDate(s.FirstDate), formatExportDate(s.LastDate))},
		}
	case "band":
		var s chatstats.BandStats
		if len(all.Stats) == 0 {
			return nil
		}
		if err := json.Unmarshal(all.Stats, &s); err != nil {
			log.Warn().Err(err).Msg("resolveExportStats: failed to unmarshal band stats")
			return nil
		}
		return []StatLine{
			{"Active users", formatNum(s.TotalActive)},
			{"Largest band", fmt.Sprintf("%s (%s)", s.Largest.Label, formatNum(s.Largest.Value))},
			{"Largest band's share", fmt.Sprintf("%d%%", s.LargestSharePct)},
		}
	case "heatmap":
		var s chatstats.HeatmapStats
		if len(all.Stats) == 0 {
			return nil
		}
		if err := json.Unmarshal(all.Stats, &s); err != nil {
			log.Warn().Err(err).Msg("resolveExportStats: failed to unmarshal heatmap stats")
			return nil
		}
		return []StatLine{
			{"Total", formatNum(s.Total)},
			{"Active days", strconv.Itoa(s.ActiveDays)},
			{"Avg on active days", formatNum(s.AvgOnActiveDays)},
			{"Busiest day", fmt.Sprintf("%s (%s)", formatExportDate(s.Busiest.Date), formatNum(s.Busiest.Value))},
		}
	case "slope":
		var s chatstats.SlopeStats
		if len(all.Stats) == 0 {
			return nil
		}
		if err := json.Unmarshal(all.Stats, &s); err != nil {
			log.Warn().Err(err).Msg("resolveExportStats: failed to unmarshal slope stats")
			return nil
		}
		return []StatLine{
			{"Entities", strconv.Itoa(s.EntityCount)},
			{"Gained / Lost / Flat", fmt.Sprintf("%d / %d / %d", s.Gained, s.Lost, s.Flat)},
			{"Biggest gain", fmt.Sprintf("%s (+%s)", s.BiggestGain.Label, formatNum(s.BiggestGain.Delta))},
			{"Biggest loss", fmt.Sprintf("%s (%s)", s.BiggestLoss.Label, formatNum(s.BiggestLoss.Delta))},
		}
	case "category":
		var s chatstats.CategoryStats
		if len(all.Stats) == 0 {
			return nil
		}
		if err := json.Unmarshal(all.Stats, &s); err != nil {
			log.Warn().Err(err).Msg("resolveExportStats: failed to unmarshal category stats")
			return nil
		}
		return []StatLine{
			{"Highest", fmt.Sprintf("%s (%s)", s.Highest.Label, formatNum(s.Highest.Value))},
			{"Lowest", fmt.Sprintf("%s (%s)", s.Lowest.Label, formatNum(s.Lowest.Value))},
			{"Top 3", formatCategoryList(s.Top3)},
			{"Bottom 3", formatCategoryList(s.Bottom3)},
		}
	default:
		return nil
	}
}

func formatCategoryList(stats []chatstats.CategoryStat) string {
	parts := make([]string, len(stats))
	for i, s := range stats {
		parts[i] = fmt.Sprintf("%s (%s)", s.Label, formatNum(s.Value))
	}
	return strings.Join(parts, ", ")
}

// formatNum matches JS's toLocaleString() for the plain numbers these stats
// ever produce: thousands separators, no trailing ".00" for whole numbers.
func formatNum(f float64) string {
	if f == float64(int64(f)) {
		return addThousands(strconv.FormatInt(int64(f), 10))
	}
	return addThousands(strconv.FormatFloat(f, 'f', 2, 64))
}

func addThousands(s string) string {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, frac, hasFrac := s, "", false
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		intPart, frac, hasFrac = s[:idx], s[idx+1:], true
	}
	var out []byte
	for i, c := range []byte(intPart) {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	result := string(out)
	if hasFrac {
		result += "." + frac
	}
	if neg {
		result = "-" + result
	}
	return result
}

// formatExportDate matches the frontend's formatAxisDate (rebucketChart.ts)
// for the plain "YYYY-MM-DD" bucket dates chatstats produces, so the export
// and the live chat UI read the same date format.
func formatExportDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Warn().Err(err).Str("date", dateStr).Msg("formatExportDate: failed to parse bucket date, using raw string")
		return dateStr
	}
	return t.Format("Jan 02, 2006")
}
