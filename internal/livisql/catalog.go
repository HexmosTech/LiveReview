package livisql

import "sort"

// Role is the caller's permission level. Catalog does not take one - member
// and owner see the same table set. Kept only for callers outside this
// package that still log/normalize a role for diagnostics (mcpagent's
// analyticsRole, chat debug log lines) - it has no bearing on table
// visibility.
type Role string

const (
	RoleMember     Role = "member"
	RoleOwner      Role = "owner"
	RoleSuperAdmin Role = "super_admin"
)

// secretColumns is an exact-name denylist of credential columns. Guard uses
// it to refuse any query that references one of these by name (see
// checkColumnRef in guard.go); schema_render.go uses IsSecretColumn to keep
// the same names out of the LLM-facing schema text. Exact names, not
// substrings: several legitimate analytics columns contain "token"
// (loc_usage_ledger.input_tokens, quota_batch_settlements.context_tokens_batch,
// ...) without being secrets, so a substring match would wrongly exclude
// real analytics data.
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
// guard uses to refuse queries that reference them.
func IsSecretColumn(name string) bool {
	return secretColumns[name]
}

// deniedTables are excluded from Catalog entirely - not visible to the SQL
// guard (any query referencing them is rejected as CodeUnknownTable) and,
// since mcpagent's schema-text renderer sources its table list from this
// same Catalog, not visible in the LLM prompt either. One list, one place.
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
	"ai_comments":                  true,
}

// Catalog is the set of real table names a generated query may reference.
// There is no per-table shadow/rewrite anymore - the guard validates the
// query's shape (read-only, table denylist, secret-column denylist, an
// org_id filter present, no constant-vs-constant tautology) and then runs
// the model's own SQL essentially as written. See guard.go's package doc for
// what that trades away.
type Catalog struct {
	tables map[string]bool
}

// CatalogFor returns the relations visible for this turn: every name in
// allTables that isn't on deniedTables. allTables is expected to be every
// real table name the live schema (dbctx) reports - see mcpagent's
// schemaIndex/idx.Tables(), which needs no per-table column introspection
// now that there is nothing to auto-generate a shadow body from.
func CatalogFor(allTables []string) Catalog {
	c := Catalog{tables: make(map[string]bool, len(allTables))}
	for _, t := range allTables {
		if !deniedTables[t] {
			c.tables[t] = true
		}
	}
	return c
}

// Allows reports whether relname may appear in a generated query.
func (c Catalog) Allows(relname string) bool {
	return c.tables[relname]
}

// Tables lists the visible relation names, sorted for stable prompts and tests.
func (c Catalog) Tables() []string {
	out := make([]string, 0, len(c.tables))
	for name := range c.tables {
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
	// pg_trgm - ranking repository-name matches by closeness
	"similarity": true,
	// set returning
	"generate_series": true, "unnest": true,
	"jsonb_array_elements": true, "jsonb_array_elements_text": true,
	// casts/misc
	"cast": true, "int4": true, "int8": true, "float8": true, "numeric": true, "text": true,
}

func functionAllowed(name string) bool {
	return allowedFunctions[name]
}
