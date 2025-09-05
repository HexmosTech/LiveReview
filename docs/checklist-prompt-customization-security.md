## Prompt customization & protection — execution checklist

This is a phased, executable checklist to implement the spec in `docs/prompt-customize-and-protect.md`. Each task calls out the files to create/edit and a one-line brief. At the end of each phase, there are a few minimal spot checks you can run to prove it’s complete.

Core objectives
- Protect vendor-supplied prompts from easy extraction or reverse engineering (at-rest and static analysis hardening; minimize runtime exposure).
- Allow customers to customize prompts safely via DB-stored chunks with clear targeting and ordering.

Legend:
- [ ] = todo, [x] = done
- Paths are relative to repo root

---

### Phase 0 — Prep and guardrails

Objective: Ensure the repo builds as-is; add ignore rules for vendor artifacts; accept plaintext templates in repo, but ensure release artifacts and runtime images ship binaries only (no source files).

Tasks
- [x] Update `.gitignore` — add `internal/prompts/vendor/keyring_gen.go` and `internal/prompts/vendor/templates/*.enc` (local vendor dev artifacts; must not be committed).
- [ ] Inventory plaintext vendor templates — canonical source is the existing `internal/prompts/templates.go` (and related files). It is acceptable to keep plaintext templates in the repo; ensure release artifacts (and runtime Docker images) ship binaries only and not source files.

Spot checks (evidence of completion)
- [x] `go build` succeeds with no new warnings (default stub path only; we haven’t added stubs yet, so this validates the baseline).
- [x] `git status` shows a clean tree after a no-op build.
- [ ] Release packaging ships binaries only (no Go source files). For container builds, the runtime stage must not include the source tree (including `internal/prompts/*.go`).

---

### Phase 1 — Database schema (context + chunks)

Objective: Create tables for application context and prompt chunks as per the spec; indexes and constraints included. Use dbmate for migrations, and include seed data in the migration itself.

Tasks
- [x] Ensure dbmate is available
	- Use `./pgctl.sh migrations` (installs dbmate) if not already installed.
- [x] Create migration via dbmate
	- Run: `dbmate new create_prompt_context_and_chunks` (this creates timestamped `.up.sql` and `.down.sql` files under `db/migrations/`).
	- Edit the generated `.up.sql` to:
		- [x] CREATE TABLE `prompt_application_context` and indexes (`idx_pac_org`, `idx_pac_targeting`).
		- [x] CREATE TABLE `prompt_chunks` and indexes (`idx_chunks_prompt_var`, `idx_chunks_appctx`) and the UNIQUE constraint.
		- [x] Seed default context per existing org: `INSERT INTO prompt_application_context (org_id) SELECT id FROM public.orgs;` (ensure idempotency with `ON CONFLICT DO NOTHING` if you add a unique key later).
	- Edit the generated `.down.sql` to drop in reverse order: indexes, `prompt_chunks`, `prompt_application_context`.
- [x] Apply migration
	- Run: `dbmate up` (or `./pgctl.sh reset` which also applies migrations, if you want a clean reset).

Spot checks
- [x] Verify seeded rows exist:
	- Using `./pgctl.sh shell -c "SELECT count(*) FROM prompt_application_context;"` returns ≥ number of rows in `public.orgs`.
- [x] Verify indexes exist:
	- `./pgctl.sh shell -c "\di+ public.idx_pac_org"`
	- `./pgctl.sh shell -c "\di+ public.idx_pac_targeting"`
	- `./pgctl.sh shell -c "\di+ public.idx_chunks_prompt_var"`
	- `./pgctl.sh shell -c "\di+ public.idx_chunks_appctx"`

---

### Phase 2 — Go types and manager scaffolding

Objective: Introduce core types and a Manager interface with a no-op/stub implementation so the app compiles and can be wired later.

Tasks
- [x] Create `internal/prompts/models.go` — define `TemplateDescriptor`, `Context`, and `Chunk` structs per spec (no DB or rendering logic yet).
- [x] Create `internal/prompts/manager.go` — define `Manager` interface with methods: `GetTemplateDescriptor`, `Render`, `ResolveApplicationContext`, `ListChunks`, `CreateChunk`, `UpdateChunk`, `DeleteChunk`, `ReorderChunks`.
- [x] Create `internal/prompts/manager_stub.go` — a stub `Manager` implementation returning unimplemented errors (or minimal happy-path for listing empty chunks) to keep `go build` green.

Spot checks
- [x] `go build` still succeeds; no runtime changes.
- [ ] `go vet`/linters (if configured) pass for new files.

---

### Phase 3 — Data access layer (store) for context and chunks

Objective: Implement SQL accessors for `prompt_application_context` resolution and chunk CRUD with ordering.

Tasks
- [x] Create `internal/prompts/store.go` — DB queries for:
	- [x] `ResolveApplicationContext` implementing precedence: repo > group > ai+git > ai only > git only > org.
	- [x] `ListChunks(orgID, applicationContextID, promptKey, variableName)` ordered by `sequence_index ASC, created_at ASC`.
	- [x] `Create/Update/Delete/Reorder` chunk operations with unique constraint handling.
- [ ] Create `internal/prompts/store_sql.go` or `internal/prompts/sql/` — keep SQL texts readable and unit-testable.
- [x] Add small helper: `providerFromAIConnector(id)` — maps `ai_connectors.id` → provider name (cached) for template selection.

Spot checks
- [ ] Manual DB insert of a few chunks shows expected ordering from the store list call.
- [ ] Optional (later): add `internal/prompts/store_test.go` unit tests once UI is working.

---

### Phase 4 — Vendor pack skeleton (stub and real via build tags)

Objective: Provide a stub vendor pack for default builds and a real pack path for CI releases (encrypted blobs + keyring).

 - [x] Create `internal/prompts/vendor/pack.go` — define `VendorPack` interface used by `Manager` to fetch descriptors and encrypted blobs.
 - [x] Create `internal/prompts/vendor/pack_stub.go` — `//go:build !vendor_prompts` — implements `VendorPack` with safe stub strings; registers default descriptors (no sensitive content).
 - [x] Create `internal/prompts/vendor/pack_real.go` — `//go:build vendor_prompts` — uses `//go:embed internal/prompts/vendor/templates/*.enc` and a generated `keyring_gen.go`; fails fast if artifacts are missing.
 - [x] Expose registry of plaintext templates for tooling — edit `internal/prompts/templates.go` or add `internal/prompts/registry.go` to export `PlaintextTemplates()` (or equivalent) returning a list/map of `{prompt_key, provider(optional), body}`.
 - [x] Create CLI tool `internal/prompts/vendor/cmd/prompts-encrypt/main.go` — encrypt plaintext templates into `internal/prompts/vendor/templates/*.enc` and write `keyring_gen.go` with `{key_id -> BEK}` and `build_id`.
	- Source defaults to `internal/prompts/templates.go` via the exported registry; optionally support a plaintext folder via `$VENDOR_PROMPT_SRC`.
 - [x] Update `.gitignore` — ensure `internal/prompts/vendor/keyring_gen.go` and `internal/prompts/vendor/templates/*.enc` are ignored.

- Spot checks
- [x] `go build` (no tag) succeeds and the app reports “stub vendor pack active” at init (log line).
- [x] `go build -tags vendor_prompts` fails with a clear message if no `.enc`/keyring present (intentional until CI path is wired).
- [x] Vendor build (with assets) succeeds locally; encrypted blobs and manifest embedded.
- [ ] Verify a vendor-tagged release build artifact does not contain any Go source files (e.g., `internal/prompts/*.go`).
- [x] Early static scan (vendor build): built a local binary with `-tags vendor_prompts` and spot-checked strings; no plaintext vendor templates found.
	- Example checks: `strings -a ./livereview | grep -E "(\{\{VAR:|some_known_vendor_snippet)"` should return no plaintext vendor body lines.

---

### Phase 5 — Render engine and variable substitution

Objective: Implement rendering: JIT decrypt vendor template (real pack) or load stub (stub pack), scan variables, resolve chunks, and substitute with join/default options.

Tasks
- [x] Create `internal/prompts/manager_impl.go` — implement `Manager.Render`:
	- [x] Derive provider via `providerFromAIConnector` and select `TemplateDescriptor`.
	- [x] Decrypt template (real pack) / load stub; zeroize buffers after use.
	- [x] Discover variables from template if not declared in descriptor.
	- [x] For each variable occurrence: resolve app context, fetch chunks, filter `enabled`, order by `sequence_index, created_at`, join bodies; honor `join` and `default` inline options.
	- [x] Replace placeholders; return final string.
- [x] Create `internal/prompts/vars.go` — variable scanner for `{{VAR:name|join=...|default="..."}}` with simple parser.
- [x] Implement `ResolveApplicationContext` in `manager` via store.

Spot checks
- [x] With stub pack active, a minimal call to `Manager.Render` returns a prompt containing joined chunk text for a variable (verified via dev fallback path and DB list logic).
 - [x] Add `internal/prompts/render_test.go` tests for happy path and option parsing. Note: run scoped tests `go test ./internal/prompts` to avoid unrelated package failures.
 - [x] Early runtime memory-dump sanity check (after render path exists): used `make vendor-memdump-check` to capture cores (e.g., `core_render_smoke.42152`, `core_render_smoke.43749`, `core_render_smoke.44180`) and grep for `{{VAR:` and specific placeholders (`style_guide`, `security_guidelines`). No raw vendor template placeholders found; one generic `{{VAR:` hit corresponds to the in-binary regex pattern (acceptable false-positive). See `docs/memory_dump_check.md`.

---

### Phase 6 — Wire new prompt manager into the builder (no legacy fallback)

Objective: Replace the old prompt assembly entirely with the new manager-driven render path. No legacy fallback to avoid silent divergence.

Tasks
- [x] Edit `internal/prompts/builder.go` — delegate to `Manager.Render("code_review"/"summary")` and append sections via helpers; remove legacy assembly path.
- [x] Keep `internal/prompts/templates.go` constants as plaintext sources (for encryption and dev-only stub), but do not use them to assemble prompts at runtime.
- [x] Identify and migrate call sites: Gemini, LangChain (LLM abstraction), AI Connectors adapter now use `Manager.Render` + code/summary section helpers.
- [x] Startup/DI checks — real vendor builds fail fast if assets are missing; render errors propagate (no silent fallback to legacy).

- [ ] Vendor E2E test (encrypted prompts, full path)
	- Goal: Prove the real vendor pack works end-to-end with encrypted templates.
	- Steps (local dev):
		1) Build vendor assets and binary
			 - Simple one-step rebuild:
				 - make vendor-prompts-rebuild
			 - Optional: pin build ID or key material (otherwise auto-generated by the tool):
				 - make vendor-prompts-encrypt ARGS="--build-id $(date +%Y%m%d%H%M%S)"
				 - make vendor-prompts-build
		2) Start backend API with the vendor binary
			 - Ensure DB is up and migrated (dbmate/pgctl), and required API/AI keys exist in environment or `livereview.toml`.
			 - If you keep env in `.env`, source it first:
				 - source .env
			 - Then run the API:
				 - ./livereview_vendor api
		3) Trigger a review from the frontend (manual)
			 - You will initiate a MR review from the UI and verify responses.
	- Notes
		- The vendor binary name is `./livereview_vendor` (created by the Makefile target).
		- If step (1) fails, check that `internal/prompts/vendor/templates/*.enc` and `internal/prompts/vendor/keyring_gen.go` were generated; delete them and retry `make vendor-prompts-rebuild` if needed.
		- To cleanly switch back to stub (non-vendor) dev, just build/run `./livereview api` without the vendor tag.

Spot checks
- [x] End-to-end review (stub pack): review triggers successfully; prompt contains expected base sections and code changes. Errors from render propagate.
- [x] Vendor build path: `pack_real.New()` panics when manifest/keyring or `.enc` assets are absent (fail fast), ensuring no implicit fallback.
- [ ] Vendor E2E (this phase): With `./livereview_vendor api` running, triggering a review from the frontend completes successfully; backend logs indicate real vendor pack active and no plaintext vendor templates appear in logs.

---

### Phase 7 — REST endpoints (required for admin UX + UI)

Objective: Expose endpoints to manage chunks and preview renders. The UI in Phase 8 consumes these APIs.

Tasks
- [ ] Create `internal/api/handlers/prompts.go` — handlers for:
	- [ ] `GET /api/prompts/catalog` — list descriptors (prompt_key, build_id, variables, provider info if applicable).
	- [ ] `GET /api/prompts/{key}/render` — preview for provided context params (redacted output in logs).
	- [ ] `GET /api/prompts/{key}/variables` — list variables and chunks.
	- [ ] `POST /api/prompts/{key}/variables/{var}/chunks` — create.
	- [ ] `POST /api/prompts/{key}/variables/{var}/reorder` — reorder by IDs.
- [ ] Wire routes in the API router (e.g., `cmd/api.go` or `internal/api/router.go`).
- [ ] Add AuthZ checks: org admin for system chunks; project/repo admin for user chunks.

Spot checks
- [ ] `curl` GET catalog returns entries even with stub pack.
- [ ] Create a user chunk via POST, then GET variables shows it in ordered list.
- [ ] Minimal variables sanity: create chunks for variables `style_guide` and `security_guidelines` under a chosen `prompt_key`; confirm they appear in the variables list and render preview (Phase 8) reflects them.

---

### Phase 8 — UI entry points (MVP)

Objective: Provide a working admin UI to manage chunks and preview prompts. This must ship.

Tasks
- [ ] Minimal MVP (customer-supplied sections): surface two simple editors for optional sections.
	- Sections: "Style guide" → variable `style_guide`; "Security guidelines" → variable `security_guidelines`.
	- Behavior: if user saves content, create/update a single user chunk per section for the selected `prompt_key` and resolved context; if empty/not saved, the section doesn’t exist (no chunk created).
	- Files to edit: incorporate into `ui/src/pages/Prompts/index.tsx` as two cards with a textarea and Save button (use existing API client).
- [ ] Navigation: add a "Settings → Prompts" link in the app nav.
	- Files to edit: `ui/src/App.tsx` (or router), `ui/src/components/*Nav*` if present — add route `/settings/prompts` and nav item.
- [ ] API client: create typed client for prompts endpoints.
	- New file: `ui/src/services/prompts.ts` — functions: `getCatalog()`, `getVariables(key, ctx)`, `createChunk(...)`, `reorderChunks(...)`, `renderPreview(key, ctx)`.
	- New file: `ui/src/types/prompts.ts` — TS types for descriptors, variables, chunks, context.
- [ ] State management: add a small store slice or React context.
	- New file: `ui/src/store/prompts.slice.ts` (if Redux), or `ui/src/contexts/promptsContext.ts` (if Context API) — holds selected prompt, variables, chunks, loading state.
- [ ] Pages and components:
	- New file: `ui/src/pages/Prompts/index.tsx` — top-level page with prompt selector and preview pane.
	- New file: `ui/src/pages/Prompts/VariableEditor.tsx` — list chunks for a variable with add/edit/delete and enable toggle.
	- New file: `ui/src/components/SortableList.tsx` — simple drag-handle reorder (use HTML5 drag & drop or an existing lib in repo; fallback to up/down controls if no lib).
	- New file: `ui/src/components/PromptContextSelector.tsx` — pick AI connector, Git connector, repo.
- [ ] Config: ensure API base URL is set (same origin or proxy).
	- Files to edit: `ui/src/constants/index.ts` (or similar) — set `API_BASE`.
	- If needed: dev proxy in `ui/server.js` or webpack devServer to `/api`.
- [ ] AuthZ integration: reuse existing auth guards to gate access to admin-only actions (system-chunk ops) vs project/repo admins.
	- Files to edit: `ui/src/contexts/auth` or route guards.
- [ ] Tests: add a minimal jest smoke test for the page (optional for first working build).

Spot checks
- [ ] Run UI dev server; navigate to Settings → Prompts. Catalog loads and variables list appears for a chosen prompt.
- [ ] Create a user chunk, reorder chunks, and see the order reflected immediately and after refresh.
- [ ] Render Preview shows joined chunk text for the selected context (with stub vendor pack in dev).
- [ ] AuthZ: non-admin accounts cannot perform system-chunk operations; user-chunk operations respect scope.
- [ ] Minimal sections behavior: with no content saved, the preview does not include style/security sections; after adding content to either section, the preview includes its text in the appropriate place (assuming the template references `{{VAR:style_guide}}` / `{{VAR:security_guidelines}}`).

---

### Phase 9 — Docker build integration (Makefile + lrops.py + Dockerfile)

Objective: Integrate vendor prompt encryption and embedding into the actual build flow used in production: Makefile targets that call `scripts/lrops.py` to produce multi-arch Docker images. Ensure the Docker build path uses `-tags vendor_prompts`, runs the encrypt step, and ships binaries only.

Tasks
- [ ] Edit `scripts/lrops.py` — integrate vendor encryption before Docker build:
	- [ ] In `cmd_build` (docker path) and `cmd_docker` flows, add a pre-build step that:
		- Generates BEK/key_id (random per build) and computes build_id (commit or tag).
		- Invokes the encrypt CLI: `go run ./internal/prompts/vendor/cmd/prompts-encrypt` to write `internal/prompts/vendor/templates/*.enc` and `internal/prompts/vendor/keyring_gen.go`.
		- Sets an environment or build-arg to signal vendor build (e.g., `GO_BUILD_TAGS=vendor_prompts`).
	- [ ] Ensure cleanup (optional): after successful build, delete generated `.enc`/`keyring_gen.go` in working tree (CI workspaces only; avoid deleting developer local files).
- [ ] Edit `Dockerfile.crosscompile` — ensure Go build uses vendor prompts:
	- [ ] Add `ARG GO_BUILD_TAGS` defaulting to empty.
	- [ ] Pass `-tags "$GO_BUILD_TAGS"` to `go build` so `vendor_prompts` can be injected from lrops.py.
	- [ ] Confirm multi-stage build keeps plaintext source out of final stage; only the binary is copied.
- [ ] Edit `.dockerignore` — exclude repo files from final context if desired (optional), but ensure plaintext does not land in runtime layer; multi-stage separation is sufficient.
- [ ] Edit `Makefile` (if needed) — no functional change required since it already delegates to `lrops.py`; optionally add a `vendor-build` alias that sets ARGS for vendor mode.
- [ ] Update CI pipeline — remove separate encryption steps; rely on `lrops.py` so local/dev/CI paths are consistent.

Spot checks
- [ ] Dry run: `make docker-multiarch-dry` prints a plan that includes the vendor encrypt step and `GO_BUILD_TAGS=vendor_prompts`.
- [ ] Build (single-arch ok for local): `make docker-build-dry` followed by real build produces an image where the binary runs and logs indicate real vendor pack active.
- [ ] Inspect image (or SBOM): no Go source files present; plaintext templates from `internal/prompts` are not in the final runtime layer.
- [ ] Optional: verify that removing the generated `.enc`/`keyring_gen.go` causes vendor-tag builds to fail fast (as expected), confirming the integration is effective.

---

### Phase 10 — Security, logging, and observability

Objective: Enforce redaction, avoid logging plaintext templates, and add minimal metrics.

Tasks
- [ ] Edit `internal/logging/` — add a redact helper and ensure render paths never log decrypted template data; respect `redact_on_log` for chunks.
- [ ] Add metrics in render path: counts, decrypt duration, resolve failures, orphaned chunks (via variable scan vs DB contents).
- [ ] Add basic doc section to `internal/prompts/README.md` — notes on redaction, keys, and operational pitfalls.
  

Spot checks
- [ ] Grep logs during render: no plaintext vendor template content; redaction in place where applicable.
- [ ] Metrics surface in whatever collector is available (or at least exported counters/histograms are incremented in tests).

---

### Phase 11 — Tests and hardening

Objective: Solidify with unit/integration tests and cover edge cases; ensure default dev experience stays smooth.

Tasks
- [ ] Unit tests: precedence selection (org-only, AI-only, Git-only, AI+Git, Group, Repo).
- [ ] Unit tests: variable parsing (join/default), chunk ordering, disabled chunks filtered, empty variable falls back to default or empty.
- [ ] Integration test (stub pack): full render of a prompt_key with variables and chunks.
- [ ] Optional integration test (vendor tag): exercise decrypt path if secrets are available in CI.

Spot checks
- [ ] `go test ./...` passes locally without vendor artifacts or tags.
- [ ] Optional: vendor-tagged tests pass in CI when enabled.

---

### Appendix — File map (quick reference)

New files (planned)
- `db/migrations/*create_prompt_context_and_chunks*.up.sql` — DDL for context + chunks tables + indexes (generated by dbmate).
- `db/migrations/*create_prompt_context_and_chunks*.down.sql` — drop tables/indexes (generated by dbmate).
- `internal/prompts/models.go` — core types: TemplateDescriptor, Context, Chunk.
- `internal/prompts/manager.go` — Manager interface.
- `internal/prompts/manager_stub.go` — stub implementation.
- `internal/prompts/store.go` — data access functions.
- `internal/prompts/store_test.go` — precedence and CRUD tests.
- `internal/prompts/vendor/pack.go` — VendorPack interface.
- `internal/prompts/vendor/pack_stub.go` — stub vendor pack (default build).
- `internal/prompts/vendor/pack_real.go` — real vendor pack (vendor_prompts tag).
- `internal/prompts/vendor/cmd/prompts-encrypt/main.go` — encrypt CLI tool (reads from exported registry in `internal/prompts/templates.go` or `internal/prompts/registry.go`).
- `internal/prompts/render.go` — Render implementation.
- `internal/prompts/vars.go` — variable parsing.
- `internal/prompts/render_test.go` — rendering tests.
- `internal/api/handlers/prompts.go` — REST endpoints.
- `internal/prompts/README.md` — developer notes.
 - `ui/src/services/prompts.ts` — UI API client.
 - `ui/src/types/prompts.ts` — UI types.
 - `ui/src/store/prompts.slice.ts` or `ui/src/contexts/promptsContext.ts` — UI state.
 - `ui/src/pages/Prompts/index.tsx` and `ui/src/pages/Prompts/VariableEditor.tsx` — UI pages.
 - `ui/src/components/SortableList.tsx` and `ui/src/components/PromptContextSelector.tsx` — UI components.

Existing files (to edit)
- `.gitignore` — ignore vendor artifacts.
- `Makefile` — vendor targets and dependencies.
- `cmd/api.go` or router file — route registration for prompts endpoints.
- `internal/prompts/builder.go` — wire manager-based render.
- Possibly org creation path (to seed default context row) or add a backfill script.
 - `ui/src/App.tsx` (or router) and nav component — add route and menu entry for Settings → Prompts.

Notes
- Provider handling is derived from `ai_connectors` (no new provider tables required).
- No token/length validation needed; keep lightweight `allow_markdown`, `redact_on_log` flags.
- Variables are names only; there is no separate variables table.

