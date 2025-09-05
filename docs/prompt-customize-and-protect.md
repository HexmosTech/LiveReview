## Prompt customization and protection (spec v0.1)

This spec defines how LiveReview will protect vendor-supplied prompt templates by keeping them encrypted at rest and in transit internally, while enabling orgs to customize prompts via database-stored chunks with context-based application and ordering. A template assembles chunks, possibly by referencing variables (named groups of chunks). A prompt builder composes a final prompt string by rendering a template with resolved variables.

### Scope and goals

- Vendor templates are proprietary and must remain encrypted 99% of the time; decrypt only just-in-time for rendering to an AI backend, then wipe.
- Users/orgs customize prompts via chunks stored in the DB (plaintext by default), in two types:
	- System-generated chunks (managed by LiveReview)
	- User-generated chunks (editable via UI/API)
- Chunks can be targeted by “application context” (ai connector, git connector, repo, etc.) with wildcard default “*”, and an explicit order.
- Vendor templates are shipped encrypted with the application (not stored in DB). Registration happens in Go code; variables used by templates are declared in code or discovered at runtime. Git history serves as the source of truth for template evolution.
- Provide a small Go API and optional REST endpoints to manage chunks and to compose final prompts.

---

## Core concepts

### 1) Encrypted vendor templates (not in DB)

- Storage: shipped with the binary/release as encrypted blobs (e.g., embedded files via Go `//go:embed` or sidecar files under `internal/prompts/vendor/`).
- Encryption: AES-256-GCM (or equivalent) envelope encryption.
		- File format: header contains algo, key_id, build_id, nonce, AAD; body is ciphertext. Example AAD: `prompt_key|build_id|checksum`.
	- Build-embedded encryption key (BEK): generated per release by CI and embedded into the binary; not persisted in DB or logs. Rotation happens per release; optionally embed a small keyring for phased transitions.
	- Rotation: support key_id in header; re-encrypt blobs offline during releases; runtime supports multiple active key_ids for phased upgrades.

	Build-time key generation and embedding (CI):
	- On each release build, generate a random 256-bit BEK and a `key_id` (e.g., `hkdf(sha256(BEK))[:16]`).
	- Use the BEK to encrypt all vendor templates into encrypted blobs committed into the artifact (not the repo).
	- Emit a generated Go file (e.g., `internal/prompts/vendor/keyring_gen.go`) with a small in-memory keyring: `{key_id -> BEK}`. Optionally include the previous release key to allow reading prior-format blobs if needed.
	- Alternatively, derive BEK as `HKDF(commit_sha || build_ts, CI_SECRET_PEPPER)` to avoid storing the raw key in CI artifacts while still embedding only the derived bytes in the binary.
		- Harden embedding: split constant material across multiple packages and combine at init. Additional code obfuscation can be evaluated later if needed.
- Decryption: performed only in memory at render time. Decrypted bytes are:
	- held for the minimum time necessary,
	- never logged,
	- immediately zeroized after composing the final string.
- Code registration (no external manifest): each vendor template is registered in Go code with non-sensitive metadata. The encrypted body remains encrypted until render.
	- prompt_key (e.g., `code_review`), build_id (e.g., git commit or semver tag), cipher checksum and key_id from the encrypted header.
	- Variables are either:
		- discovered at runtime by scanning the decrypted template for tokens like `{{VAR:...}}`, or
		- declared alongside the template in Go (for stricter validation), but still compiled into code—not a separate manifest file.
	- Optional compatibility mappings (variable renames across builds) can be expressed in Go code as a map.

### 2) Chunks (DB)

Chunks are named text blocks that can be injected into vendor templates via variables. Two types:

- System-generated chunks: created and updated by LiveReview (e.g., org guidelines harvested, dynamic facts). Editable by admins only, or programmatically.
- User-generated chunks: created/edited via UI/API by org/project/repo admins.

Characteristics:
- Plaintext by default (not encrypted), because they are customer-provided; optionally allow per-chunk encryption if storing sensitive data later.
- Ordered: each chunk has a `sequence_index` integer to define its relative position within a variable for the selected application context.
- Contextual: each chunk can declare applicability rules by “application context” (see next section). Defaults are wildcards `*` for global applicability.
- Validation: lightweight only (e.g., markdown allowed flag, URL policy). No token-based validation and no length-based tests are required.

### 3) Application context (targeting)

Defines where a chunk applies. Initial dimensions (future-ready):
- org_id (mandatory owner),
- ai_connector_id (nullable FK to ai_connectors),
- integration_token_id (nullable FK to integration_tokens; i.e., git connector),
- group_identifier (nullable TEXT; future: provider-specific group/namespace path or ID),
- repository (nullable TEXT; provider-specific repo identifier),

Future-ready: branch, language, file path patterns, provider model, etc.

Precedence (detailed → general): the single most detailed context wins.
1) Repository
2) Group
3) AI connector AND Git connector (both specified)
4) AI connector only
5) Git connector only
6) Org (global)

Resolution rule:
- Find an existing prompt_application_context row for the org that exactly matches the request at the highest possible precedence level (repository > group > both connectors > AI-only > Git-only > org). If none found at a level, fall back to the next level. Iteration 1 uses the org-only default row.
- Once a single context row is selected, use only its chunks for the render. Within that context, chunks for a variable are ordered by sequence_index asc, then created_at asc.

### 4) Templates and variables

Vendor templates are encrypted strings that may include variable references. A variable is a named group that resolves to zero or more chunks at render time.

- Syntax inside decrypted template: `{{VAR:name}}` for variables; optional `{{VAR:name|join=\n\n|default="..."}}` controls joining and default fallback.
- At render time, for each variable occurrence, LiveReview resolves applicable chunks by name and concatenates them in the resolved order. If no chunks match, the variable resolves to an empty string (or an inline default if specified).
- Variables may also be used inside chunk bodies; evaluation order is single pass by template variables only (nesting is not expanded in v0.1 to avoid recursion), or we can explicitly disallow nested variable expansion in chunk bodies for now.

Note: A variable is just a name that refers to chunks; there is no separate variable entity/table. Multiple chunks can share the same variable name within a context to support ordered composition.

Provider-specific templates: Each AI connector is tied to a provider (see `ai_connectors` in the schema). Template registration can include provider-specific variants for the same `prompt_key`. At render time, the provider is inferred from the selected AI connector, and the manager chooses the best-matching vendor template variant; if a provider-specific variant is missing, it falls back to a default template for that `prompt_key`.

---

## Rendering pipeline

When a review is triggered:
1) Determine context: org_id, ai_connector, git_connector, repo_id, and any other attributes.
2) Select template by `prompt_key` from the in-code registry (not DB), validate checksum/key_id/build_id; obtain the set of variables referenced either by declaration in code or by scanning the decrypted template.
3) Decrypt vendor template to memory (just-in-time).
4) Build the variable environment (system vars + request vars). Resolve one application context row for this request using the precedence order above. Fetch chunks for that context and each variable.
5) Render:
	 - Substitute `{{VAR:...}}` placeholders using the variable environment.
	- Replace each `{{VAR:name}}` with the joined chunk bodies (after variable substitution within chunks), using ordering and context rules.
6) Return the final prompt string to the AI connector.
7) Zeroize decrypted buffers; avoid logging any plaintext template content. Respect redaction rules for logs.

Final prompt = vendor template (encrypted → decrypted JIT) + resolved system chunks + resolved user chunks.


## Data model (PostgreSQL)

Note: Vendor template bodies are not stored in DB. We only track chunk data and a separate application context. Iteration 1 uses a single global context per org; more specific targeting (AI connector, git connector, repository) is modeled but may remain NULL (wildcard) initially.

Tables (initial):

```sql
-- Application context (target where chunks apply)
CREATE TABLE IF NOT EXISTS prompt_application_context (
	id                      BIGSERIAL   PRIMARY KEY,
	org_id                  BIGINT      NOT NULL REFERENCES public.orgs(id),
	-- Optional specific targeting (NULL means wildcard "*")
	ai_connector_id         INTEGER              REFERENCES public.ai_connectors(id),
	integration_token_id    BIGINT               REFERENCES public.integration_tokens(id), -- git connector
	group_identifier        TEXT,                -- e.g., GitLab group path, GitHub org/owner; NULL means wildcard
	repository              TEXT,   -- optional identifier for repo; NULL means wildcard
	-- Ordering scope (sequence applied when resolving variables within this context)
	created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pac_org ON prompt_application_context (org_id);
CREATE INDEX IF NOT EXISTS idx_pac_targeting ON prompt_application_context (org_id, ai_connector_id, integration_token_id, group_identifier, repository);

-- Chunks: user or system; ordered within a variable for a given application context
CREATE TABLE IF NOT EXISTS prompt_chunks (
	id                      BIGSERIAL   PRIMARY KEY,
	org_id                  BIGINT      NOT NULL REFERENCES public.orgs(id), -- owner org (redundant but useful for RLS)
	application_context_id  BIGINT      NOT NULL REFERENCES prompt_application_context(id) ON DELETE CASCADE,
	prompt_key              TEXT        NOT NULL,     -- which template this chunk is for
	variable_name           TEXT        NOT NULL,     -- which variable this chunk contributes to
	chunk_type              TEXT        NOT NULL,     -- 'system' | 'user'
	title                   TEXT,                    -- optional descriptive title
	body                    TEXT        NOT NULL,     -- plaintext by default
	sequence_index          INTEGER     NOT NULL DEFAULT 1000, -- order within (prompt_key, variable_name, application_context)
	enabled                 BOOLEAN     NOT NULL DEFAULT TRUE,
	-- validation/meta (lightweight; no token/length checks)
	allow_markdown          BOOLEAN     NOT NULL DEFAULT TRUE,
	redact_on_log           BOOLEAN     NOT NULL DEFAULT FALSE,
	created_by              BIGINT,                   -- user id
	updated_by              BIGINT,
	created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (application_context_id, prompt_key, variable_name, sequence_index)
);

CREATE INDEX IF NOT EXISTS idx_chunks_prompt_var ON prompt_chunks (prompt_key, variable_name);
CREATE INDEX IF NOT EXISTS idx_chunks_appctx ON prompt_chunks (application_context_id);
```

Notes:
- No template content or catalog is stored in DB.
- A default `prompt_application_context` row should be created per org (org-only, other fields NULL) and used for iteration 1.
- Multiple rows per variable are allowed because order matters; order is defined by `sequence_index` within a specific application context.
- If later we support per-chunk encryption, add fields `ciphertext, nonce, aad_hash, content_hash` and migrate select chunks.

---

## Go package and API surface

Package: `internal/prompts` will gain a small manager that composes with the existing builder logic.

Essential interfaces (sketch):

```go
// Template descriptor available in plaintext Go code (metadata only)
type TemplateDescriptor struct {
	PromptKey    string
	BuildID      string // git commit or tag
	CipherChecksum string
	Variables    []string // variable names used by the template (optional; if empty, discover by scanning)
	Provider     string // optional provider name for provider-specific variants; empty means default
}

// No explicit Anchor type; templates reference variables directly.

// Resolver context
type Context struct {
	OrgID               int64
	AIConnectorID       *int32  // matches ai_connectors.id (nullable); determines provider
	IntegrationTokenID  *int64  // matches integration_tokens.id (nullable)
	Repository          *string // nullable wildcard
}

// Chunk CRUD (DB)
type Chunk struct {
	ID                   int64
	OrgID                int64
	ApplicationContextID int64
	PromptKey            string
	VariableName         string
	Type                 string // system|user
	Title                string
	Body                 string
	SequenceIndex        int
	Enabled              bool
	AllowMarkdown        bool
	RedactOnLog          bool
	CreatedBy            *int64
	UpdatedBy            *int64
}

type Manager interface {
	// Vendor template runtime
	GetTemplateDescriptor(promptKey string, provider string) (TemplateDescriptor, error) // provider derived from AI connector if empty
	Render(ctx context.Context, c Context, promptKey string, vars map[string]string) (string, error)

		// Chunks
	ResolveApplicationContext(ctx context.Context, c Context) (applicationContextID int64, err error)
	ListChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string) ([]Chunk, error)
	CreateChunk(ctx context.Context, ch Chunk) (int64, error)
	UpdateChunk(ctx context.Context, ch Chunk) error
	DeleteChunk(ctx context.Context, orgID, chunkID int64) error
	ReorderChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string, orderedIDs []int64) error
}
```

Render algorithm (pseudocode):

```
provider := providerFromAIConnector(c.AIConnectorID) // lookup in DB or cache
desc := GetTemplateDescriptor(promptKey, provider)
tpl := decryptTemplateBlob(promptKey)         // JIT decrypt
defer zeroize(tpl)

for each scalar placeholder (simple ${NAME} if supported), replace from vars[name]

// Determine variables to resolve
varNames := desc.Variables
if len(varNames) == 0 { varNames = discoverVariablesFromTemplate(tpl) }

for each varName in varNames:
	appCtxID := ResolveApplicationContext(context)
	chunks := queryChunks(appCtxID, prompt_key, varName)
	chunks := filter enabled
	sort by sequence_index asc, created_at asc
	body := join(chunks.body, "\n\n")
	replace all "{{VAR:"+varName+"}}" with body (honor inline join/default options if specified)

return tpl
```

Backwards compatibility: if `Manager` isn’t configured, fall back to current static prompt assembly.

---

## REST endpoints (optional)

- GET /api/prompts/catalog -> list descriptors (prompt_key, build_id, variables)
- GET /api/prompts/{key}/render?org=...&ai=...&git=...&repo=... -> preview final prompt (with redaction)
- GET /api/prompts/{key}/variables -> list variables and existing chunks
- POST /api/prompts/{key}/variables/{var}/chunks -> create chunk (body, type, context, order)
- POST /api/prompts/{key}/variables/{var}/reorder -> reorder by id list

AuthZ:
- Only org admins can manage system chunks; project/repo admins can manage user chunks within their scope.
- All operations are constrained to the caller’s org.

---

## Security model

- Vendor templates: AES-256-GCM encryption; content never stored in DB; JIT decryption; zeroization; strict log redaction. Use a minimal in-binary keyring with key_id-based rotation; key-sharding is optional for added friction. Obfuscation can be added later if required.
- Chunks: plaintext by default; application-level validation; optional per-chunk encryption in future. Redact-on-log flag respected.
- Variables: assembled per-request in memory; do not persist rendered final prompt. If persisted for audit, store with redactions.

Hardening notes:
- Do not reuse BEK for any database encryption should that be added later; DB encryption (if needed) must use a different key source (env/KMS) and rotation policy.
- Bind ciphertext with strong AAD (`prompt_key|build_id|checksum`) and verify checks prior to decryption to prevent misuse.
- Keep decrypted buffers on the stack or short-lived heap slices; explicitly zeroize after use and avoid copying.

---

## Evolution (via Git/builds)

- Vendor templates evolve with Git. The encrypted blob is tied to a build_id (commit/tag). Variables used by templates may change; code can supply a mapping from old variable names to new ones to preserve chunk eligibility across refactors.
- Upgrades are delivered with builds. DB chunks persist and re-apply so long as variable names are compatible.
- If a variable is removed or renamed without a mapping, affected chunks remain in DB but won’t render; the UI should flag them as "orphaned" with guidance.

Pinning:
- Allow an org-level configuration to select which build_id to use for a given `prompt_key` (optional). In the simple case, the app uses the build’s current descriptor by default.

---

## UI outline (MVP)

- Settings -> Prompts
	- Select prompt (e.g., Code Review).
	- Show variables with preview of resolved content.
	- For each variable: list chunks (system first, then user), with context chips and order drag-handle.
	- Add/Edit/Delete user chunks; set context (ai connector, git connector, repo), order, enable/disable.
	- Preview final prompt for a given context (org, ai connector, git connector, repo).
	- Warnings for orphaned chunks when variables are removed/renamed across builds.

---

## Operational notes

- Observability: metrics for render counts, decrypt time/failures, chunk counts, orphaned chunks. Logs redacted.
- Testing: unit tests for matching order/specificity; golden tests for rendering with sample manifests; property tests for wildcard matching; fuzz tests for placeholder substitution.
- Performance: decrypt templates once per request; optionally cache decrypted templates for milliseconds within the same request scope if multiple renders occur, but do not cache across requests.

---

## Developer and build experience

Goals:
- Keep plain `go build` working for all contributors without access to proprietary templates or keys.
- Make release/CI builds automatically encrypt and embed vendor templates with a per-build key.
- Allow authorized developers to locally test with real vendor templates when needed, without committing secrets.

Key ideas:
- Build tags select the vendor pack implementation:
	- Default (no tag): StubVendorPack is compiled. Templates render with safe, non-proprietary placeholders; chunk logic is fully exercised.
	- `-tags vendor_prompts`: RealVendorPack is compiled, which embeds encrypted blobs and the keyring.
- Encrypted blobs (`internal/prompts/vendor/*.enc`) and the generated keyring file (`internal/prompts/vendor/keyring_gen.go`) are not required for default builds.

File layout (example):
- internal/prompts/vendor/
	- pack_stub.go           // +build !vendor_prompts — provides StubVendorPack
	- pack_real.go           // +build vendor_prompts — uses go:embed to load *.enc and keyring
	- keyring_gen.go         // generated by CI or local tool when building with vendor_prompts
	- templates/*.enc        // encrypted blobs generated at build time for release/vendor dev

Local development (no secrets):
- Run `go build` or `make build` (default). This uses StubVendorPack; everything compiles and runs.
- Prompt rendering substitutes variables to chunks normally; vendor template body is replaced by a stub string noting “vendor template unavailable in dev”. This keeps flows testable.

Authorized vendor development (opt-in):
- Pre-req: access to plaintext vendor templates in a secure location outside the repo (e.g., $VENDOR_PROMPT_SRC).
- Steps (suggested tooling):
	1) Generate a temporary build-embedded key (BEK) and key_id.
	2) Encrypt plaintext templates into `internal/prompts/vendor/templates/*.enc` using the BEK.
	3) Generate `internal/prompts/vendor/keyring_gen.go` containing the BEK keyed by key_id.
	4) Build with `-tags vendor_prompts`.
- None of these artifacts should be committed; add `internal/prompts/vendor/keyring_gen.go` and `internal/prompts/vendor/templates/*.enc` to `.gitignore` for local vendor dev. Release builds will manage their own copies in CI artifacts.

CI / release builds:
- Build script performs:
	1) Determine build_id from git (commit or tag).
	2) Generate a per-build 256-bit BEK and key_id.
	3) Encrypt vendor templates to `internal/prompts/vendor/templates/*.enc`.
	4) Emit `keyring_gen.go` with a small in-memory keyring map `{key_id -> BEK}` and embed build_id.
	5) `go build -tags vendor_prompts` (or build via Docker). Artifacts include only encrypted blobs; BEK bytes are inside the binary.
	6) Clean up generated sources post-build (if building in a shared workspace).

Makefile integration (suggested):
- keep: `make build` -> plain `go build` (stub vendor pack, no secrets required).
- add: `make prompts-encrypt` -> runs a small Go tool to encrypt plaintext templates into vendor/*.enc (using a provided BEK).
- add: `make prompts-keyring` -> generates vendor/keyring_gen.go with BEK/key_id/build_id.
- add: `make build-with-vendor` -> depends on prompts-encrypt + prompts-keyring, then `go build -tags vendor_prompts`.
- existing: `make build-versioned` and Docker workflows can call the above steps automatically in CI.

Failure modes and fallbacks:
- If `-tags vendor_prompts` is used but encrypted blobs or keyring are missing, build fails fast with a clear error from pack_real.go (init-time check).
- If running a binary with StubVendorPack, rendering will still work using DB chunks and stub templates; logs should mark that vendor templates are not present. This is suitable for feature development and tests.

Testing:
- Unit tests run without vendor templates (stub pack). This ensures contributors can run `go test ./...` without secrets.
- Integration tests that verify vendor templates can be placed under a `vendor_prompts` build tag or skipped unless an env var is present.

Security notes:
- Do not commit plaintext vendor templates.
- For local vendor dev, ensure generated `.enc` and `keyring_gen.go` are gitignored.
- CI should generate BEK per build and never log key material. Use short-lived CI workspaces.

Bottom line: `go build` remains untouched and fast for everyday development. The encryption/embedding path is opt-in via a build tag and is automated in CI for releases.

---

## Decisions incorporated

- Provider variants: Supported via AI connector, which implies a provider. Vendor templates may register provider-specific variants under the same `prompt_key`; resolution uses the provider from the selected AI connector with default fallback.
- Validation: No token-based or length-based validation is required. Keep lightweight controls only (e.g., markdown flag, URL policy, redaction flag).
- Variables: No separate variable storage/table. A variable is simply the name by which chunks are grouped and resolved.

---

## Requirements coverage

- Encrypted vendor templates not in DB; decrypted JIT only: Done by “Encrypted vendor templates” and “Rendering pipeline”.
- System and user chunks stored in DB with CRUD, context, ordering: Done by “Chunks (DB)”, “Application context”, schema, and API.
- Easy API/Go package to manage prompts and compose final output: Done by “Go package and API surface”.
- Schema for prompt chunk management and context rules: Done by “Data model (PostgreSQL)”.
- Evolution/upgrade of encrypted prompts without DB base: Done by “Evolution (via Git/builds)”, with code-based registration and variable mappings.

