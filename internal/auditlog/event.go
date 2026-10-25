package auditlog

import (
	"encoding/json"
	"reflect"
	"time"
)

// Event is one audit record. It is used both for writing and for reads.
//
// OccurredAt is stored in UTC. The API renders local time in the UI and
// ISO-8601 in exports.
type Event struct {
	ID            int64          `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	OrgID         int64          `json:"org_id,omitempty"`
	ActorUserID   *int64         `json:"actor_user_id,omitempty"`
	ActorEmail    string         `json:"actor_email,omitempty"`
	ActorName     string         `json:"actor_name,omitempty"`
	ActorKind     ActorKind      `json:"actor_kind"`
	ActorCapacity ActorCapacity  `json:"actor_capacity,omitempty"`
	AuthMethod    string         `json:"auth_method,omitempty"`
	APIKeyID      *int64         `json:"api_key_id,omitempty"`
	Action        Action         `json:"action"`
	Category      Category       `json:"category"`
	ResourceType  string         `json:"resource_type,omitempty"`
	ResourceID    string         `json:"resource_id,omitempty"`
	Outcome       Outcome        `json:"outcome"`
	StatusCode    *int           `json:"status_code,omitempty"`
	Severity      Severity       `json:"severity"`
	IP            string         `json:"ip,omitempty"`
	UserAgent     string         `json:"user_agent,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	Method        string         `json:"method,omitempty"`
	Path          string         `json:"path,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
	Source        Source         `json:"source,omitempty"`
	LegacyID      *int64         `json:"legacy_id,omitempty"`
	Seq           *int64         `json:"seq,omitempty"`
	PrevHash      string         `json:"prev_hash,omitempty"`
	Hash          string         `json:"hash,omitempty"`

	// Derived display fields, not stored.
	ActionLabel   string `json:"action_label,omitempty"`
	CategoryLabel string `json:"category_label,omitempty"`
	ActorIP       string `json:"-"`
}

// FillDerived populates human labels from the catalog. It never changes stored
// data, so it is safe to call on read.
func (e *Event) FillDerived() {
	if meta, ok := LookupAction(e.Action); ok {
		e.ActionLabel = meta.Label
		if e.Category == "" {
			e.Category = meta.Category
		}
	}
	if e.Category != "" {
		e.CategoryLabel = CategoryLabel(e.Category)
	}
}

// Fields is a details payload.
type Fields map[string]any

// Details builds a Fields map from alternating key/value pairs:
//
//	audit.Details("role", "owner", "is_active", true)
//
// It panics on an odd number of arguments: that is a programmer error, caught
// by tests long before production.
func Details(kv ...any) Fields {
	if len(kv)%2 != 0 {
		panic("auditlog.Details: expected an even number of key/value arguments")
	}
	out := make(Fields, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok || key == "" {
			panic("auditlog.Details: keys must be non-empty strings")
		}
		out[key] = kv[i+1]
	}
	return out
}

// Changes returns a before/after diff for the given fields. Only changed keys
// are included, so audit payloads stay small and readable.
func Changes(before, after Fields) Fields {
	changes := Fields{}
	for k, oldVal := range before {
		newVal, exists := after[k]
		if !exists {
			changes[k] = Fields{"from": oldVal, "to": nil}
			continue
		}
		if !reflect.DeepEqual(oldVal, newVal) {
			changes[k] = Fields{"from": oldVal, "to": newVal}
		}
	}
	for k, newVal := range after {
		if _, exists := before[k]; !exists {
			changes[k] = Fields{"from": nil, "to": newVal}
		}
	}
	if len(changes) == 0 {
		return Fields{}
	}
	return Fields{"changes": changes}
}

// MarshalDetails sanitizes and marshals a details payload for storage.
func MarshalDetails(f Fields) ([]byte, error) {
	sanitized := SanitizeFields(f)
	return json.Marshal(sanitized)
}
