package livisql

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/livereview/internal/database"
	"github.com/shrsv/dbctx"
)

func TestColumnsFor(t *testing.T) {
	cases := []struct {
		table string
		want  []string
	}{
		{"reviews", []string{"id", "repository", "branch", "commit_hash", "pr_mr_url", "connector_id",
			"status", "trigger_type", "user_email", "provider", "created_at", "started_at", "completed_at",
			"org_id", "mr_title", "author_name", "author_username", "friendly_name", "pull_request_id"}},
		{"orgs", []string{"id", "name", "description", "created_at", "updated_at", "is_active", "subscription_plan"}},
		{"users", []string{"id", "email", "first_name", "last_name", "is_active",
			"created_at", "last_login_at", "last_cli_used_at"}},
		{"loc_usage_ledger", []string{"id", "org_id", "review_id", "user_id", "operation_type", "trigger_source",
			"billable_loc", "accounted_at", "billing_period_start", "billing_period_end", "status",
			"created_at", "provider", "model", "input_tokens", "output_tokens", "llm_cost_usd",
			"actor_kind", "actor_email_snapshot"}},
		{"does_not_exist", nil},
	}
	for _, c := range cases {
		got := ColumnsFor(c.table)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ColumnsFor(%q) = %#v, want %#v", c.table, got, c.want)
		}
	}
}

// TestColumnsForNeverProjectsWithheldColumns asserts the columns this
// codebase has deliberately withheld - reviews.metadata (raw diff/PR
// content), users.password_hash and users.onboarding_api_key (secrets) -
// never show up as an allowed column name, for every table that has them.
// This is the property schema_render.go depends on to keep them out of an
// LLM prompt without a view in between.
func TestColumnsForNeverProjectsWithheldColumns(t *testing.T) {
	withheld := map[string][]string{
		"reviews": {"metadata"},
		"users":   {"password_hash", "onboarding_api_key"},
	}
	for table, cols := range withheld {
		got := ColumnsFor(table)
		for _, w := range cols {
			for _, g := range got {
				if g == w {
					t.Errorf("ColumnsFor(%q) includes withheld column %q: %v", table, w, got)
				}
			}
		}
	}
}

// TestColumnsForMatchesLiveSchema is the drift guard: every name ColumnsFor
// returns must exist as a real column on the real table, per dbctx's live
// introspection of the dev database (DATABASE_URL / .env). It catches a
// shadow typo or a renamed/dropped real column before it reaches an LLM
// prompt as either a silently-wrong or a silently-dropped name - see
// internal/mcpagent/dbctx_schema_plan.md.
//
// Skips if no database is available, matching this codebase's no-mocks,
// real-DB testing convention (see storage/analytics/adhoc_store_test.go).
func TestColumnsForMatchesLiveSchema(t *testing.T) {
	dsn, err := database.LoadDatabaseURL()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}

	ctx := context.Background()
	idx, err := dbctx.Build(ctx, dsn, nil)
	if err != nil {
		t.Skipf("skipping: dbctx could not connect: %v", err)
	}
	defer idx.Close()

	for table, cols := range shadowColumnCache {
		detail, err := idx.TableDetail(table)
		if err != nil {
			t.Errorf("TableDetail(%q): %v", table, err)
			continue
		}
		if detail == nil {
			t.Errorf("shadow %q has no matching real table in the live schema", table)
			continue
		}
		real := make(map[string]bool, len(detail.Columns))
		for _, c := range detail.Columns {
			real[c.Name] = true
		}
		for _, col := range cols {
			if !real[col] {
				t.Errorf("shadow %q projects column %q, which does not exist on the real table (real columns: %v)",
					table, col, realColumnNames(detail))
			}
		}
	}
}

func realColumnNames(d *dbctx.TableDetail) []string {
	out := make([]string, 0, len(d.Columns))
	for _, c := range d.Columns {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}
