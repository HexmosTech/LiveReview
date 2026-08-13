package analytics

import (
	"database/sql"
	"encoding/json"
	"math"
	"strconv"
	"time"
)

// coerce converts a driver value into something json.Marshal can always encode
// and Vega-Lite can read. Every branch here exists because of a concrete way the
// naive version breaks:
//
//   - lib/pq hands back NUMERIC as []byte, so avg()/sum() would otherwise be
//     base64-encoded strings in the chart payload
//   - date_trunc buckets are timestamptz; Vega-Lite's "temporal" type wants
//     ISO-8601, and the printed date must stay the one the database computed,
//     not get shifted a day by an unwanted UTC conversion (see the time.Time
//     branch below)
//   - json.Marshal returns an error on NaN/Inf, which avg() over an empty group
//     and any division by zero will produce — one such value would fail the
//     entire response rather than one cell
func coerce(v any, ct *sql.ColumnType) any {
	switch val := v.(type) {
	case nil:
		return nil

	case time.Time:
		// Format in the value's own zone rather than converting to UTC.
		// date_trunc('month', ...) buckets in the database session's timezone
		// (e.g. IST): a bucket meaning "July 2026" is midnight July 1 in that
		// zone. Converting to UTC shifts it to 18:30 the previous day, which
		// then reads as June - the month itself becomes wrong, not just the
		// clock time. RFC3339 keeps the offset, so the printed date always
		// matches the wall-clock date the database computed.
		return val.Format(time.RFC3339)

	case []byte:
		return coerceBytes(val, ct)

	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil
		}
		return val

	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return f

	default:
		// int64, bool, string and anything else already marshal cleanly.
		return v
	}
}

func coerceBytes(b []byte, ct *sql.ColumnType) any {
	typeName := ""
	if ct != nil {
		typeName = ct.DatabaseTypeName()
	}

	switch typeName {
	case "JSON", "JSONB":
		var out any
		if err := json.Unmarshal(b, &out); err == nil {
			return out
		}
		return string(b)

	case "NUMERIC", "DECIMAL", "MONEY":
		if f, err := strconv.ParseFloat(string(b), 64); err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return nil
			}
			return f
		}
		return string(b)

	default:
		// Postgres arrays (review_feedback.tags is text[]) also land here, as
		// their literal "{a,b}" form. Left as a string deliberately: the schema
		// prompt tells the model not to select array columns directly, and
		// guessing at array parsing would be worse than showing the literal.
		return string(b)
	}
}
