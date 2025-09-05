package prompts

import (
	"regexp"
	"strings"
)

// Placeholder represents a single {{VAR:...}} occurrence with parsed options.
type Placeholder struct {
	Raw     string
	Name    string
	Options map[string]string // e.g., join, default
}

var (
	// Matches {{VAR:name|key=value|key2="quoted value"}}
	// Capture 1 = name, Capture 2 = options (may be empty)
	varPattern = regexp.MustCompile(`\{\{VAR:([a-zA-Z0-9_\-]+)((?:\|[^}]+)?)}}`)
	optPattern = regexp.MustCompile(`\|([^=|]+)=([^|]+)`) // key=value segments
)

// ParsePlaceholders returns all placeholder occurrences in order of appearance.
func ParsePlaceholders(body string) []Placeholder {
	matches := varPattern.FindAllStringSubmatchIndex(body, -1)
	out := make([]Placeholder, 0, len(matches))
	for _, idx := range matches {
		raw := body[idx[0]:idx[1]]
		name := body[idx[2]:idx[3]]
		optsRaw := ""
		// FindAllStringSubmatchIndex returns indices as:
		// [fullStart, fullEnd, group1Start, group1End, group2Start, group2End, ...]
		// Our regex has two groups: name (group1) and options (group2).
		if len(idx) >= 6 && idx[4] != -1 {
			optsRaw = body[idx[4]:idx[5]]
		}
		opts := map[string]string{}
		if optsRaw != "" {
			sub := optPattern.FindAllStringSubmatch(optsRaw, -1)
			for _, seg := range sub {
				key := strings.TrimSpace(seg[1])
				val := strings.TrimSpace(seg[2])
				// Trim surrounding quotes if present
				if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
					val = val[1 : len(val)-1]
				}
				// Decode basic escapes
				val = decodeEscapes(val)
				opts[strings.ToLower(key)] = val
			}
		}
		out = append(out, Placeholder{Raw: raw, Name: name, Options: opts})
	}
	return out
}

func decodeEscapes(s string) string {
	// Minimal decoding: \n, \t, \r, \\; leave others as-is
	b := strings.Builder{}
	b.Grow(len(s))
	esc := false
	for _, r := range s {
		if !esc {
			if r == '\\' {
				esc = true
				continue
			}
			b.WriteRune(r)
			continue
		}
		switch r {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteRune(r)
		}
		esc = false
	}
	if esc {
		b.WriteByte('\\')
	}
	return b.String()
}
