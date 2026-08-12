package livisql

import (
	"sort"
	"strings"
)

// Role is the caller's permission level. CatalogFor does not take one -
// member and owner see the same table set (explicit decision - the
// previous owner-only gate on billing/quota/license tables was dropped
// along with the hand-curated table list). Kept only for callers outside
// this package that still log/normalize a role for diagnostics (mcpagent's
// analyticsRole, chat debug log lines) - it has no bearing on table
// visibility.
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
type shadow struct {
	name string
	body string
}

// specialShadows are tables that cannot be templated by AutoOrgScopedShadow:
// orgs is the tenant row itself, keyed by id rather than org_id; users has no
// org_id at all - membership lives in user_roles, so a join is what scopes
// it, and the hand-written projection is what keeps password_hash and
// onboarding_api_key structurally unreachable (secretColumns can't help here
// since these two are excluded by never being in the projection to begin
// with, not by being filtered out of one).
var specialShadows = []shadow{
	{"orgs", `SELECT id, name, description, created_at, updated_at, is_active, subscription_plan
		FROM public.orgs WHERE id = $1`},

	{"users", `SELECT DISTINCT u.id, u.email, u.first_name, u.last_name, u.is_active,
		u.created_at, u.last_login_at, u.last_cli_used_at
		FROM public.users u
		JOIN public.user_roles ur ON ur.user_id = u.id
		WHERE ur.org_id = $1`},
}

// secretColumns is an exact-name denylist applied when auto-generating a
// shadow for an org_id-scoped table (see AutoOrgScopedShadow) - real
// credentials found in this schema's org_id-having tables (checked against
// the live DB, not guessed). Exact names, not substrings: several legitimate
// analytics columns contain "token" (loc_usage_ledger.input_tokens,
// quota_batch_settlements.context_tokens_batch, ...) without being secrets,
// so a substring match would wrongly exclude real analytics data.
var secretColumns = map[string]bool{
	"password_hash":      true,
	"onboarding_api_key": true,
	"api_key":            true,
	"master_api_key":     true,
	"access_token":       true,
	"refresh_token":      true,
	"pat_token":          true,
	"bot_token":          true,
	"client_secret":      true,
	"webhook_secret":     true,
	"bot_password":       true,
	"admin_password":     true,
	"token_hash":         true,
	"key_hash":           true,
}

// IsSecretColumn reports whether name is on the secretColumns denylist -
// exported so mcpagent's schema-text renderer (schema_render.go) can keep
// secret column NAMES out of the LLM prompt too, using the same list the SQL
// guard already uses to keep their VALUES out of query results, rather than
// maintaining a second copy of it.
func IsSecretColumn(name string) bool {
	return secretColumns[name]
}

// AutoOrgScopedShadow builds a shadow for a table that carries org_id
// directly: every column except secretColumns, from the live schema (see
// mcpagent's orgScopedColumns, sourced from the dbctx index) rather than a
// hand-written per-table list. columns is expected to include org_id - the
// caller is responsible for only calling this for tables that actually have
// that column (a table without one would produce an invalid WHERE clause).
// Returns false if every column was denylisted, so a real table never ends
// up with an empty, useless shadow.
func AutoOrgScopedShadow(table string, columns []string) (shadow, bool) {
	kept := make([]string, 0, len(columns))
	for _, c := range columns {
		if !secretColumns[c] {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		return shadow{}, false
	}
	return shadow{
		name: table,
		body: "SELECT " + strings.Join(kept, ", ") + " FROM public." + table + " WHERE org_id = $1",
	}, true
}

// deniedTables are excluded from CatalogFor entirely, regardless of what
// orgScopedColumns supplies - not visible to the SQL guard (no shadow, so
// any query referencing them is rejected as CodeUnknownTable) and, since
// mcpagent's schema-text renderer sources its table list from this same
// Catalog, not visible in the LLM prompt either. One list, one place.
var deniedTables = map[string]bool{
	"upgrade_requests":             true,
	"webhook_registry":             true,
	"integration_tokens":           true,
	"org_slack_configs":            true,
	"org_discord_configs":          true,
	"org_teams_configs":            true,
	"api_keys":                     true,
	"reviews_backup_20260806":      true,
	"prompt_chunks":                true,
	"prompt_application_context":   true,
	"dashboard_cache":              true,
	"upgrade_request_events":       true,
	"upgrade_replacement_cutovers": true,
	"upgrade_payment_attempts":     true,
	"user_management_audit":        true,
	"user_role_history":            true,
}

// Catalog is the set of relations that may be referenced in a generated
// query, plus the shadow bodies used to rewrite them.
type Catalog struct {
	shadows map[string]string
}

// CatalogFor returns the relations visible for this turn: the two
// hand-written specials (orgs, users) plus an auto-generated shadow for
// every table in orgScopedColumns (table name -> its column list) that
// isn't on deniedTables. There is no role input - see the Role doc comment.
func CatalogFor(orgScopedColumns map[string][]string) Catalog {
	c := Catalog{shadows: make(map[string]string, len(specialShadows)+len(orgScopedColumns))}
	for _, s := range specialShadows {
		c.shadows[s.name] = s.body
	}
	for table, cols := range orgScopedColumns {
		if deniedTables[table] {
			continue
		}
		if s, ok := AutoOrgScopedShadow(table, cols); ok {
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
