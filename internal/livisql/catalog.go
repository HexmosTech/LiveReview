package livisql

import "sort"

// Role is the caller's permission level. Raw SQL bypasses the per-endpoint
// authorization the REST/MCP tools used to apply, so the visible table set has
// to be re-derived from the role here — otherwise a member could query billing
// tables that GET_api_v1_billing_usage_members gates behind ownership.
type Role string

const (
	RoleMember     Role = "member"
	RoleOwner      Role = "owner"
	RoleSuperAdmin Role = "super_admin"
)

// shadow is the body of the CTE that stands in for a real table. Every shadow
// applies the org filter itself, so a generated query cannot widen its scope:
// it only ever sees rows that already passed this WHERE clause. $1 is the org
// id, bound by the executor.
//
// Two projection rules matter:
//   - secrets are never projected (users.password_hash, users.onboarding_api_key)
//   - large free-text jsonb is dropped (reviews.metadata holds diff/PR content,
//     which has no analytics value and would bloat a CSV export)
type shadow struct {
	name string
	body string
}

// orgScoped tables carry org_id directly, so the shadow is a plain filter.
var memberShadows = []shadow{
	{"reviews", `SELECT id, repository, branch, commit_hash, pr_mr_url, connector_id,
		status, trigger_type, user_email, provider, created_at, started_at, completed_at,
		org_id, mr_title, author_name, author_username, friendly_name, pull_request_id
		FROM public.reviews WHERE org_id = $1`},

	{"repositories", `SELECT id, org_id, connector_id, provider, provider_repo_id,
		full_name, name, web_url, default_branch, is_private, description,
		last_synced_at, last_sync_status, created_at, updated_at
		FROM public.repositories WHERE org_id = $1`},

	{"pull_requests", `SELECT id, repository_id, org_id, provider, provider_pr_id, number,
		title, state, author_id, author_username, author_name, source_branch, target_branch,
		web_url, provider_created_at, provider_updated_at, created_at, updated_at
		FROM public.pull_requests WHERE org_id = $1`},

	{"ai_comments", `SELECT id, review_id, comment_type, file_path, line_number, created_at, org_id
		FROM public.ai_comments WHERE org_id = $1`},

	{"review_events", `SELECT id, review_id, org_id, ts, event_type, level, batch_id
		FROM public.review_events WHERE org_id = $1`},

	{"review_feedback", `SELECT id, org_id, review_id, ai_comment_id, vote_type, severity,
		source_type, lrc_version, created_at, retracted_at
		FROM public.review_feedback WHERE org_id = $1`},

	// orgs is the tenant row itself, keyed by id rather than org_id. settings
	// (jsonb) is withheld — it holds org configuration, not analytics data.
	{"orgs", `SELECT id, name, description, created_at, updated_at, is_active, subscription_plan
		FROM public.orgs WHERE id = $1`},

	// users has no org_id at all: membership lives in user_roles. The join is
	// what scopes it, and the projection is what keeps password_hash and
	// onboarding_api_key structurally unreachable.
	{"users", `SELECT DISTINCT u.id, u.email, u.first_name, u.last_name, u.is_active,
		u.created_at, u.last_login_at, u.last_cli_used_at
		FROM public.users u
		JOIN public.user_roles ur ON ur.user_id = u.id
		WHERE ur.org_id = $1`},

	{"user_roles", `SELECT user_id, role_id, org_id, created_at, updated_at
		FROM public.user_roles WHERE org_id = $1`},
}

// Billing and cost data was owner-gated at the REST layer; keep it that way.
var ownerShadows = []shadow{
	{"loc_usage_ledger", `SELECT id, org_id, review_id, user_id, operation_type, trigger_source,
		billable_loc, accounted_at, billing_period_start, billing_period_end, status,
		created_at, provider, model, input_tokens, output_tokens, llm_cost_usd,
		actor_kind, actor_email_snapshot
		FROM public.loc_usage_ledger WHERE org_id = $1`},

	{"org_billing_state", `SELECT id, org_id, current_plan_code, billing_period_start,
		billing_period_end, loc_used_month, loc_blocked, trial_started_at, trial_ends_at,
		created_at, updated_at
		FROM public.org_billing_state WHERE org_id = $1`},
}

// Catalog is the set of relations a given role may reference, plus the shadow
// bodies used to rewrite them.
type Catalog struct {
	shadows map[string]string
}

// CatalogFor returns the relations visible to role. Unknown roles get the
// member catalog: surfaces that authenticate an organization rather than a
// user (the Slack/Discord/Teams bots) must not silently gain owner visibility.
func CatalogFor(role Role) Catalog {
	c := Catalog{shadows: make(map[string]string, len(memberShadows)+len(ownerShadows))}
	for _, s := range memberShadows {
		c.shadows[s.name] = s.body
	}
	if role == RoleOwner || role == RoleSuperAdmin {
		for _, s := range ownerShadows {
			c.shadows[s.name] = s.body
		}
	}
	return c
}

// Allows reports whether relname may appear in a generated query.
func (c Catalog) Allows(relname string) bool {
	_, ok := c.shadows[relname]
	return ok
}

// Tables lists the visible relation names, sorted for stable prompts and tests.
func (c Catalog) Tables() []string {
	out := make([]string, 0, len(c.shadows))
	for name := range c.shadows {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// allowedFunctions is an allowlist rather than a denylist: a denylist can be
// outrun by any function nobody thought to block (pg_read_file, dblink,
// query_to_xml, lo_import, ...), and new ones ship with every Postgres release.
var allowedFunctions = map[string]bool{
	// aggregates
	"count": true, "sum": true, "avg": true, "min": true, "max": true,
	"stddev": true, "variance": true, "percentile_cont": true, "percentile_disc": true,
	"bool_or": true, "bool_and": true, "every": true,
	// window
	"rank": true, "dense_rank": true, "row_number": true, "lag": true, "lead": true,
	"first_value": true, "last_value": true, "ntile": true,
	// numeric
	"round": true, "abs": true, "ceil": true, "ceiling": true, "floor": true,
	"trunc": true, "mod": true, "power": true, "sqrt": true, "div": true,
	// null handling
	"coalesce": true, "nullif": true, "greatest": true, "least": true,
	// date/time
	"date_trunc": true, "date_part": true, "extract": true, "to_char": true,
	"to_date": true, "to_timestamp": true, "age": true, "now": true,
	"current_date": true, "current_timestamp": true, "make_interval": true,
	"justify_interval": true, "date_bin": true,
	// text
	"lower": true, "upper": true, "initcap": true, "trim": true, "btrim": true,
	"ltrim": true, "rtrim": true, "concat": true, "concat_ws": true,
	"substring": true, "substr": true, "length": true, "char_length": true,
	"split_part": true, "replace": true, "left": true, "right": true, "lpad": true, "rpad": true,
	// jsonb (reviews.metadata is withheld, but review payloads elsewhere are jsonb)
	"jsonb_array_length": true, "jsonb_extract_path_text": true, "jsonb_typeof": true,
	// set returning
	"generate_series": true, "unnest": true,
	// casts/misc
	"cast": true, "int4": true, "int8": true, "float8": true, "numeric": true, "text": true,
}

func functionAllowed(name string) bool {
	return allowedFunctions[name]
}
