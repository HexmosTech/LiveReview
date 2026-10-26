package auditlog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// Recorder writes audit events. It is safe for concurrent use.
type Recorder struct {
	db        *sql.DB
	hashChain bool
}

func NewRecorder(db *sql.DB) *Recorder {
	return &Recorder{db: db, hashChain: true}
}

// SetHashChain enables or disables the per-org hash chain at runtime. It is
// read from audit_settings by the maintenance manager.
func (r *Recorder) SetHashChain(enabled bool) {
	r.hashChain = enabled
}

// DB exposes the underlying handle for maintenance jobs.
func (r *Recorder) DB() *sql.DB { return r.db }

// Record writes an event on its own transaction (never inside a caller's tx).
// Use this from middleware, webhooks, and system jobs. Errors are returned so
// the caller can log them; they never roll back a business operation.
func (r *Recorder) Record(ctx context.Context, ev Event) error {
	if r == nil || r.db == nil {
		return errors.New("auditlog: recorder is not configured")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("auditlog: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.RecordTx(ctx, tx, ev); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordTx writes an event using the caller's transaction. For privileged
// actions this is what makes the change fail-closed: the handler returns the
// error and the whole business transaction rolls back.
//
// When called with an Echo context it also marks the request as "already
// audited", so the HTTP middleware does not write a duplicate fallback row.
func (r *Recorder) RecordTx(ctx context.Context, tx *sql.Tx, ev Event) error {
	if r == nil {
		return errors.New("auditlog: recorder is not configured")
	}
	if tx == nil {
		return errors.New("auditlog: nil transaction")
	}
	normalizeEvent(&ev)
	details, err := json.Marshal(SanitizeFields(ev.Details))
	if err != nil {
		return fmt.Errorf("auditlog: marshal details: %w", err)
	}
	ev.Details = nil

	var seq *int64
	var prevHash, hash *string
	if r.hashChain {
		s, p, h, err := r.nextChain(ctx, tx, ev.OrgID, ev, details)
		if err != nil {
			return err
		}
		seq, prevHash, hash = s, p, h
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_events (
			occurred_at, org_id, actor_user_id, actor_email, actor_name,
			actor_kind, actor_capacity, auth_method, api_key_id,
			action, category, resource_type, resource_id,
			outcome, status_code, severity,
			ip, user_agent, request_id, method, path,
			details, source, legacy_id, seq, prev_hash, hash
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
			$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27
		)`,
		ev.OccurredAt.UTC(), ev.OrgID, ev.ActorUserID, nullIfEmpty(ev.ActorEmail), nullIfEmpty(ev.ActorName),
		string(ev.ActorKind), string(ev.ActorCapacity), nullIfEmpty(ev.AuthMethod), ev.APIKeyID,
		string(ev.Action), string(ev.Category), nullIfEmpty(ev.ResourceType), nullIfEmpty(ev.ResourceID),
		string(ev.Outcome), ev.StatusCode, string(ev.Severity),
		nullIP(ev.IP), nullIfEmpty(ev.UserAgent), nullIfEmpty(ev.RequestID), nullIfEmpty(ev.Method), nullIfEmpty(ev.Path),
		details, string(ev.Source), ev.LegacyID, seq, prevHash, hash,
	)
	if err != nil {
		return fmt.Errorf("auditlog: insert: %w", err)
	}
	return nil
}

func normalizeEvent(ev *Event) {
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	// PostgreSQL timestamptz stores microseconds. Truncate before hashing and
	// inserting so the chain verifies on read-back.
	ev.OccurredAt = ev.OccurredAt.UTC().Truncate(time.Microsecond)
	if ev.Source == "" {
		ev.Source = SourceAPI
	}
	if ev.ActorKind == "" {
		ev.ActorKind = ActorKindSystem
	}
	if ev.ActorCapacity == "" {
		if ev.ActorKind == ActorKindSystem || ev.ActorKind == ActorKindWebhook {
			ev.ActorCapacity = ActorCapacitySystem
		} else {
			ev.ActorCapacity = ActorCapacityMember
		}
	}
	if ev.Outcome == "" {
		ev.Outcome = OutcomeSuccess
	}
	meta, known := LookupAction(ev.Action)
	if ev.Category == "" {
		if known {
			ev.Category = meta.Category
		} else {
			ev.Category = CategoryOther
		}
	}
	if ev.ResourceType == "" && known {
		ev.ResourceType = meta.ResourceType
	}
	if ev.Severity == "" {
		if known {
			ev.Severity = meta.Severity
		} else {
			ev.Severity = SeverityInfo
		}
	}
	if ev.IP != "" && net.ParseIP(ev.IP) == nil {
		ev.IP = ""
	}
	if len(ev.UserAgent) > 1024 {
		ev.UserAgent = ev.UserAgent[:1024]
	}
}

// hashPayload is the canonical input to the per-org chain. Field order is
// fixed, so the same event always produces the same digest.
type hashPayload struct {
	Seq           int64           `json:"seq"`
	OccurredAt    string          `json:"occurred_at"`
	OrgID         int64           `json:"org_id"`
	ActorUserID   *int64          `json:"actor_user_id,omitempty"`
	ActorEmail    string          `json:"actor_email,omitempty"`
	ActorName     string          `json:"actor_name,omitempty"`
	ActorKind     string          `json:"actor_kind"`
	ActorCapacity string          `json:"actor_capacity"`
	AuthMethod    string          `json:"auth_method,omitempty"`
	APIKeyID      *int64          `json:"api_key_id,omitempty"`
	Action        string          `json:"action"`
	Category      string          `json:"category"`
	ResourceType  string          `json:"resource_type,omitempty"`
	ResourceID    string          `json:"resource_id,omitempty"`
	Outcome       string          `json:"outcome"`
	StatusCode    *int            `json:"status_code,omitempty"`
	Severity      string          `json:"severity"`
	IP            string          `json:"ip,omitempty"`
	UserAgent     string          `json:"user_agent,omitempty"`
	RequestID     string          `json:"request_id,omitempty"`
	Method        string          `json:"method,omitempty"`
	Path          string          `json:"path,omitempty"`
	Source        string          `json:"source"`
	Details       json.RawMessage `json:"details"`
}

func (r *Recorder) nextChain(ctx context.Context, tx *sql.Tx, orgID int64, ev Event, details []byte) (*int64, *string, *string, error) {
	// Serialize chain appends per organization. The advisory lock is released
	// automatically when the transaction commits or rolls back.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, orgID); err != nil {
		return nil, nil, nil, fmt.Errorf("auditlog: chain lock: %w", err)
	}

	var lastSeq int64
	var lastHash sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT seq, hash FROM audit_events
		WHERE org_id = $1 AND seq IS NOT NULL
		ORDER BY seq DESC
		LIMIT 1`, orgID).Scan(&lastSeq, &lastHash)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, nil, fmt.Errorf("auditlog: chain tail: %w", err)
	}
	if err == sql.ErrNoRows {
		lastSeq = 0
	}

	seq := lastSeq + 1
	prev := lastHash.String
	h, err := HashEvent(seq, prev, ev, details)
	if err != nil {
		return nil, nil, nil, err
	}
	return &seq, &prev, &h, nil
}

// HashEvent computes the per-org chain digest for one event. It is exported so
// the integrity checker can recompute the same value.
func HashEvent(seq int64, prev string, ev Event, details []byte) (string, error) {
	payload := hashPayload{
		Seq:           seq,
		OccurredAt:    ev.OccurredAt.UTC().Format(time.RFC3339Nano),
		OrgID:         ev.OrgID,
		ActorUserID:   ev.ActorUserID,
		ActorEmail:    ev.ActorEmail,
		ActorName:     ev.ActorName,
		ActorKind:     string(ev.ActorKind),
		ActorCapacity: string(ev.ActorCapacity),
		AuthMethod:    ev.AuthMethod,
		APIKeyID:      ev.APIKeyID,
		Action:        string(ev.Action),
		Category:      string(ev.Category),
		ResourceType:  ev.ResourceType,
		ResourceID:    ev.ResourceID,
		Outcome:       string(ev.Outcome),
		StatusCode:    ev.StatusCode,
		Severity:      string(ev.Severity),
		IP:            ev.IP,
		UserAgent:     ev.UserAgent,
		RequestID:     ev.RequestID,
		Method:        ev.Method,
		Path:          ev.Path,
		Source:        string(ev.Source),
		Details:       json.RawMessage(details),
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("auditlog: chain encode: %w", err)
	}
	sum := sha256.Sum256(append([]byte(prev), canonical...))
	return hex.EncodeToString(sum[:]), nil
}

// System records a background/webhook event with no HTTP request.
func (r *Recorder) System(ctx context.Context, orgID int64, action Action, details Fields) error {
	ev := Event{
		OrgID:         orgID,
		ActorKind:     ActorKindSystem,
		ActorCapacity: ActorCapacitySystem,
		Action:        action,
		Details:       details,
		Source:        SourceSystem,
	}
	return r.Record(ctx, ev)
}

// Webhook records a provider-webhook-triggered event.
func (r *Recorder) Webhook(ctx context.Context, orgID int64, action Action, details Fields) error {
	ev := Event{
		OrgID:         orgID,
		ActorKind:     ActorKindWebhook,
		ActorCapacity: ActorCapacitySystem,
		Action:        action,
		Details:       details,
		Source:        SourceWebhook,
	}
	return r.Record(ctx, ev)
}

// AIEvent is the metadata captured for one model invocation. Raw prompts and
// responses are deliberately not fields: only a fingerprint and size.
type AIEvent struct {
	OrgID          int64
	ActorUserID    *int64
	ActorEmail     string
	ActorKind      ActorKind
	ActorCapacity  ActorCapacity
	ReviewID       *int64
	ConversationID *int64
	Provider       string
	Model          string
	Purpose        string
	InputTokens    int64
	OutputTokens   int64
	CostUSD        float64
	LatencyMS      int64
	PromptHash     string
	PromptBytes    int
	Outcome        Outcome
	Details        Fields
}

// AIInvocation records a model call. It is best-effort: callers should log and
// continue if it fails, because losing an audit row must not fail a review or
// a chat turn.
func (r *Recorder) AIInvocation(ctx context.Context, ai AIEvent) error {
	details := Fields{
		"provider":      ai.Provider,
		"model":         ai.Model,
		"purpose":       ai.Purpose,
		"input_tokens":  ai.InputTokens,
		"output_tokens": ai.OutputTokens,
		"cost_usd":      ai.CostUSD,
		"latency_ms":    ai.LatencyMS,
	}
	if ai.PromptHash != "" {
		details["prompt_hash"] = ai.PromptHash
	}
	if ai.PromptBytes > 0 {
		details["prompt_bytes"] = ai.PromptBytes
	}
	if ai.ReviewID != nil {
		details["review_id"] = *ai.ReviewID
	}
	if ai.ConversationID != nil {
		details["conversation_id"] = *ai.ConversationID
	}
	for k, v := range ai.Details {
		if _, exists := details[k]; !exists {
			details[k] = v
		}
	}
	outcome := ai.Outcome
	if outcome == "" {
		outcome = OutcomeSuccess
	}
	kind := ai.ActorKind
	if kind == "" {
		kind = ActorKindMember
	}
	capacity := ai.ActorCapacity
	if capacity == "" {
		capacity = ActorCapacityMember
	}
	return r.Record(ctx, Event{
		OrgID:         ai.OrgID,
		ActorUserID:   ai.ActorUserID,
		ActorEmail:    ai.ActorEmail,
		ActorKind:     kind,
		ActorCapacity: capacity,
		Action:        ActionAIInvocation,
		Category:      CategoryAI,
		ResourceType:  "model",
		ResourceID:    ai.Model,
		Outcome:       outcome,
		Details:       details,
		Source:        SourceSystem,
	})
}

// MarkRecorded tells the HTTP middleware that a handler already wrote the
// canonical domain event for this request.
func MarkRecorded(c echo.Context) {
	if c != nil {
		c.Set("audit_recorded", true)
	}
}

// WasRecorded reports whether the handler already wrote a domain event.
func WasRecorded(c echo.Context) bool {
	if c == nil {
		return false
	}
	v, _ := c.Get("audit_recorded").(bool)
	return v
}

// RequestID returns the request's audit correlation id, creating and setting
// one if necessary. Also sets the X-Request-Id response header.
func RequestID(c echo.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get("audit_request_id").(string); ok && v != "" {
		return v
	}
	id := newRequestID()
	c.Set("audit_request_id", id)
	c.Response().Header().Set("X-Request-Id", id)
	return id
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullIP(s string) any {
	if net.ParseIP(s) == nil {
		return nil
	}
	return s
}

// LogRecordError is the standard way middleware reports a failed audit write.
func LogRecordError(action Action, err error) {
	if err == nil {
		return
	}
	log.Error().Err(err).Str("audit_action", string(action)).Msg("audit event could not be written")
}
