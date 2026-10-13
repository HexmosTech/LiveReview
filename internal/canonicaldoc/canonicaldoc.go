// Package canonicaldoc builds the stable, provider-agnostic "canonical review
// document" every jq ruleset is evaluated against, and evaluates jq boolean
// expressions against it. It is shared by the CI/CD gate API
// (internal/api/ci_rulesets_handler.go) and the risk-policy engine's static
// gate stage (internal/policyengine), both of which reuse the same
// build/eval primitives so there is exactly one rule engine.
//
// A truthy jq result (anything except false/null) means "block".
package canonicaldoc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/livereview/pkg/models"
	storagereviews "github.com/livereview/storage/reviews"
)

// Finding is one row of the "findings" array in the canonical doc.
type Finding struct {
	Severity    string `json:"severity"`
	Confidence  string `json:"confidence"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Type        string `json:"type"`
	FilePath    string `json:"file_path,omitempty"`
	LineNumber  *int   `json:"line_number,omitempty"`
}

// Counts carries pre-aggregated severity/category counts, zero-filled for the
// known severities so jq expressions like `.counts.by_severity.critical > 0`
// never fail with "null cannot be compared" on a clean review.
type Counts struct {
	BySeverity map[string]int `json:"by_severity"`
	ByCategory map[string]int `json:"by_category"`
	Total      int            `json:"total"`
}

// ReviewDoc is the canonical document shape jq rulesets evaluate against.
type ReviewDoc struct {
	ReviewID   int64    `json:"review_id"`
	OrgID      int64    `json:"org_id"`
	Repository string   `json:"repository"`
	Provider   string   `json:"provider"`
	Status     string   `json:"status"`
	Findings   []Finding `json:"findings"`
	Counts     Counts    `json:"counts"`
	// DiffClassification is surfaced (when present) so jq rules can reference the
	// diff-level semantic classification produced by the review LLM
	// (see docs/risk-policy-engine.md §5). Absent for CLI/legacy reviews.
	DiffClassification *models.DiffClassification `json:"diff_classification,omitempty"`
}

// KnownSeverities is the fixed 3-value severity model the AI review prompt emits.
var KnownSeverities = []string{"critical", "warning", "info"}

// header is the minimal review row needed to build the canonical doc's header.
type header struct {
	ID         int64
	Repository string
	Provider   string
	Status     string
}

// Build constructs the canonical review document for a review scoped strictly
// to orgID - never trust a caller-supplied review_id without confirming org
// ownership first.
func Build(ctx context.Context, db *sql.DB, orgID, reviewID int64) (*ReviewDoc, error) {
	var h header
	var provider sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT id, repository, provider, status
		FROM reviews
		WHERE id = $1 AND org_id = $2`, reviewID, orgID).Scan(&h.ID, &h.Repository, &provider, &h.Status)
	if err != nil {
		return nil, err
	}
	h.Provider = provider.String

	doc := &ReviewDoc{
		ReviewID:   h.ID,
		OrgID:      orgID,
		Repository: h.Repository,
		Provider:   h.Provider,
		Status:     h.Status,
		Findings:   []Finding{},
		Counts: Counts{
			BySeverity: map[string]int{},
			ByCategory: map[string]int{},
		},
	}
	for _, sev := range KnownSeverities {
		doc.Counts.BySeverity[sev] = 0
	}

	taxonomy := storagereviews.NewTaxonomyReportStore(db)
	rows, _, err := taxonomy.ListFindings(ctx, storagereviews.TaxonomyFilter{OrgID: orgID, ReviewID: reviewID}, 500, 0, storagereviews.TaxonomyFindingsOptions{})
	if err != nil {
		return nil, fmt.Errorf("loading findings: %w", err)
	}
	for _, r := range rows {
		sev := strings.ToLower(r.Severity)
		cat := strings.ToLower(r.Category)
		f := Finding{
			Severity:    sev,
			Confidence:  strings.ToLower(r.Confidence),
			Category:    cat,
			Subcategory: strings.ToLower(r.Subcategory),
			Type:        strings.ToLower(r.IssueType),
			LineNumber:  r.LineNumber,
		}
		if r.FilePath != nil {
			f.FilePath = *r.FilePath
		}
		doc.Findings = append(doc.Findings, f)
		doc.Counts.BySeverity[sev]++
		if cat != "" {
			doc.Counts.ByCategory[cat]++
		}
		doc.Counts.Total++
	}

	// Surface the diff-level classification if the review carried one.
	doc.DiffClassification = loadDiffClassification(ctx, db, orgID, reviewID)

	return doc, nil
}

// loadDiffClassification reads the diff-level classification out of the review's
// persisted review_result metadata (written by the review LLM), if present.
func loadDiffClassification(ctx context.Context, db *sql.DB, orgID, reviewID int64) *models.DiffClassification {
	var raw []byte
	err := db.QueryRowContext(ctx,
		`SELECT metadata->'review_result'->'diffClassification' FROM reviews WHERE id = $1 AND org_id = $2`,
		reviewID, orgID).Scan(&raw)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var dc models.DiffClassification
	if err := json.Unmarshal(raw, &dc); err != nil {
		return nil
	}
	return &dc
}

// RunJQBool evaluates expr against doc and returns whether the first produced
// value is truthy (jq truthiness: everything except false/null), plus the raw
// first result for display/debugging.
func RunJQBool(expr string, doc interface{}) (blocked bool, result interface{}, err error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return false, nil, fmt.Errorf("invalid jq expression: %w", err)
	}

	// Round-trip through JSON so the doc matches what gojq expects
	// (map[string]interface{}/[]interface{}/plain scalars).
	raw, err := json.Marshal(doc)
	if err != nil {
		return false, nil, fmt.Errorf("marshaling document: %w", err)
	}
	// nosemgrep: go.lang.security.deserialization.unsafe-deserialization-interface.go-unsafe-deserialization-interface -- raw is our own json.Marshal output from just above; gojq requires generic map/slice/scalar input, and encoding/json into interface{} can only produce those, never arbitrary types.
	var input interface{}
	if err := json.Unmarshal(raw, &input); err != nil {
		return false, nil, fmt.Errorf("unmarshaling document: %w", err)
	}

	iter := query.Run(input)
	v, ok := iter.Next()
	if !ok {
		return false, nil, fmt.Errorf("jq expression produced no output")
	}
	if e, ok := v.(error); ok {
		return false, nil, fmt.Errorf("jq evaluation error: %w", e)
	}
	switch t := v.(type) {
	case bool:
		blocked = t
	case nil:
		blocked = false
	default:
		blocked = true
	}
	return blocked, v, nil
}
