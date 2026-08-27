package chatstats

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Chip is a label/value pair, same as the web UI's StatChip grid.
type Chip struct {
	Label string
	Value string
}

// FormatChips turns a chart's precomputed stats into the same chips the web
// UI shows, for bot surfaces. Trend charts default to "day" granularity.
func FormatChips(stats json.RawMessage) []Chip {
	if len(stats) == 0 {
		return nil
	}
	var all AllStats
	if err := json.Unmarshal(stats, &all); err != nil {
		return nil
	}
	switch all.Kind {
	case "trend":
		var s TrendStats
		if len(all.Day) == 0 || json.Unmarshal(all.Day, &s) != nil {
			return nil
		}
		chips := []Chip{
			{"Total", formatNumber(s.Total)},
			{"Avg per period", formatNumber(s.AvgPerPeriod)},
			{"Peak", fmt.Sprintf("%s (%s)", formatNumber(s.Peak.Value), s.Peak.Date)},
			{"Low", fmt.Sprintf("%s (%s)", formatNumber(s.Low.Value), s.Low.Date)},
		}
		if s.TrendPct != nil {
			dir := "down"
			if *s.TrendPct >= 0 {
				dir = "up"
			}
			chips = append(chips, Chip{"Trend", fmt.Sprintf("%s %s%% (%s → %s)", dir, formatNumber(math.Abs(*s.TrendPct)), s.FirstDate, s.LastDate)})
		}
		return chips

	case "multi_series_trend":
		var s MultiSeriesTrendStats
		if len(all.Day) == 0 || json.Unmarshal(all.Day, &s) != nil {
			return nil
		}
		return []Chip{
			{"Total", formatNumber(s.Total)},
			{"Series", strconv.Itoa(s.SeriesCount)},
			{"Top series", fmt.Sprintf("%s (%s)", s.TopSeries.Label, formatNumber(s.TopSeries.Value))},
			{"Date range", fmt.Sprintf("%s → %s", s.FirstDate, s.LastDate)},
		}

	case "band":
		var s BandStats
		if len(all.Stats) == 0 || json.Unmarshal(all.Stats, &s) != nil {
			return nil
		}
		return []Chip{
			{"Active users", formatNumber(s.TotalActive)},
			{"Largest band", fmt.Sprintf("%s (%s)", s.Largest.Label, formatNumber(s.Largest.Value))},
			{"Largest band's share", fmt.Sprintf("%d%%", s.LargestSharePct)},
		}

	case "heatmap":
		var s HeatmapStats
		if len(all.Stats) == 0 || json.Unmarshal(all.Stats, &s) != nil {
			return nil
		}
		return []Chip{
			{"Total", formatNumber(s.Total)},
			{"Active days", strconv.Itoa(s.ActiveDays)},
			{"Avg on active days", formatNumber(s.AvgOnActiveDays)},
			{"Busiest", fmt.Sprintf("%s (%s)", s.Busiest.Date, formatNumber(s.Busiest.Value))},
		}

	case "slope":
		var s SlopeStats
		if len(all.Stats) == 0 || json.Unmarshal(all.Stats, &s) != nil {
			return nil
		}
		return []Chip{
			{"Entities", strconv.Itoa(s.EntityCount)},
			{"Gained / Lost / Flat", fmt.Sprintf("%d / %d / %d", s.Gained, s.Lost, s.Flat)},
			{"Biggest gain", fmt.Sprintf("%s (+%s)", s.BiggestGain.Label, formatNumber(s.BiggestGain.Delta))},
			{"Biggest loss", fmt.Sprintf("%s (%s)", s.BiggestLoss.Label, formatNumber(s.BiggestLoss.Delta))},
		}

	case "category":
		var s CategoryStats
		if len(all.Stats) == 0 || json.Unmarshal(all.Stats, &s) != nil {
			return nil
		}
		return []Chip{
			{"Highest", fmt.Sprintf("%s (%s)", s.Highest.Label, formatNumber(s.Highest.Value))},
			{"Lowest", fmt.Sprintf("%s (%s)", s.Lowest.Label, formatNumber(s.Lowest.Value))},
			{"Top 3", formatCategoryList(s.Top3)},
			{"Bottom 3", formatCategoryList(s.Bottom3)},
		}

	case "generic":
		var s GenericStats
		if len(all.Stats) == 0 || json.Unmarshal(all.Stats, &s) != nil {
			return nil
		}
		return []Chip{
			{"Total", formatNumber(s.Total)},
			{"Count", strconv.Itoa(s.Count)},
			{"Highest", fmt.Sprintf("%s (%s)", s.Highest.Label, formatNumber(s.Highest.Value))},
			{"Lowest", fmt.Sprintf("%s (%s)", s.Lowest.Label, formatNumber(s.Lowest.Value))},
		}

	default:
		return nil
	}
}

func formatCategoryList(items []CategoryStat) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = fmt.Sprintf("%s (%s)", it.Label, formatNumber(it.Value))
	}
	return strings.Join(parts, ", ")
}

// formatNumber renders v the way JS's toLocaleString() does for the web UI's
// StatChips: thousands-grouped, no trailing zeros.
func formatNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, frac, hasFrac := strings.Cut(s, ".")

	var grouped strings.Builder
	n := len(intPart)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteByte(intPart[i])
	}
	out := grouped.String()
	if hasFrac {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}
