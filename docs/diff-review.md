## Plan: Minimal diff-review for CLI prototype

Create `/api/v1/diff-review` that bypasses auth, accepts base64-encoded diff ZIP in JSON, reuses existing diff parser from [cmd/mrmodel/lib/diff_parser.go](cmd/mrmodel/lib/diff_parser.go), feeds into review pipeline, and returns structured JSON with comments embedded in diffs.

### Steps

1. ✅ Added public routes for POST/GET diff-review with bypass key validation in [internal/api/server.go](internal/api/server.go#L443).

2. ✅ Created handler in [internal/api/diff_review.go](internal/api/diff_review.go) (parse base64-zip via `parseDiffZipBase64`, convert `LocalCodeDiff` to `CodeDiff`, create review record, kick off async review).

3. ✅ Added `PreloadedChanges` plumbing in [internal/review/service.go](internal/review/service.go) to skip provider fetch/post for CLI and feed preloaded diffs into AI review.

4. ✅ Added polling endpoint in `diff_review.go` to return processing/completed status and embed comments into the original hunks.

5. ✅ Added contract-focused unit test that builds a real base64 zip input and prints the canonical response structure in [internal/api/diff_review_test.go](internal/api/diff_review_test.go#L150-L236).

### Further Considerations

1. ✅ `convertLocalToModelDiff` and hunk formatting implemented in handler helpers (with unit coverage).
2. ✅ Comment line validation logs out-of-range scenarios while preserving comments.
3. ✅ Review result and preloaded diffs persisted in metadata for polling.

### Next Steps (detailed)

1) **Client usage (customer machine)**
- Capture diff: `git diff --staged` (default) or `git diff HEAD~1..HEAD` (flag) or `--diff-file` for custom input.
- Write diff to temp file, zip it, base64 it.
- POST JSON `{ "diff_zip_base64": "...", "repo_name": "<cwd basename or --repo-name>" }` to `/api/v1/diff-review` with header `X-API-Key: <personal-key>` (env/flag in CLI).
- Read `review_id`, poll GET `/api/v1/diff-review/:id` until `status == completed`.
- Render locally: pretty table or JSON showing summary + comments grouped by file/hunk.

2) **Inline sample request/response**
- Example request (truncated payload):

	```json
	{
		"diff_zip_base64": "<base64 zip, len≈276>",
		"repo_name": "my-repo"
	}
	```

- Example response (from contract test):

	```json
	{
		"status": "completed",
		"summary": "Example summary for contract test",
		"files": [
			{
				"file_path": "foo.txt",
				"hunks": [
					{"old_start_line":0,"old_line_count":0,"new_start_line":1,"new_line_count":2,"content":"@@ -0,0 +1,2 @@\n+hello\n+world\n "}
				],
				"comments": [
					{"line":1,"content":"Example review note","severity":"info","category":"example"}
				]
			}
		]
	}
	```

- Expected HTTP codes: 200 on accepted/processing/completed, 401 on bad key, 400 on malformed payload.

3) ✅ **Implement `lrc` CLI (new binary under `cmd/lrc`)**
- Flags: `--repo-name` (default cwd basename), `--diff-source` (staged|working|range|file), `--range` (for range mode), `--diff-file`, `--api-url`, `--api-key`, `--poll-interval`, `--timeout`, `--output` (json|pretty), `--verbose`.
- Flow: collect diff → zip+base64 → POST → poll → render → exit non-zero on HTTP/contract failures or review status `failed`.
- Dependencies: reuse existing diff parser structs if needed for rendering; avoid server imports where possible.
- Implementation: placed CLI code in [cmd/lrc/main.go](cmd/lrc/main.go), using `github.com/urfave/cli/v2` for argument parsing (consistent with existing binaries).

4) ✅ **Handler-level unit test (no DB)**
- Added tests that stub ReviewManager methods (create, update status, merge metadata, get) to assert:
	- `preloaded_changes` stored on POST.
	- `review_result` stored on completion path.
	- Correct status responses for processing/completed.
- Tests in [internal/api/diff_review_test.go](internal/api/diff_review_test.go).

5) ✅ **Key model, configurability, and security**
- Shifted from single global bypass key to per-user personal API keys.
- Database schema: [db/migrations/20251219135906_create_api_keys_table.sql](db/migrations/20251219135906_create_api_keys_table.sql)
- Server implementation:
	- API key generation/hashing/validation: [internal/api/api_keys.go](internal/api/api_keys.go)
	- CRUD endpoints: [internal/api/api_key_handlers.go](internal/api/api_key_handlers.go)
	- Middleware for authentication: `APIKeyAuthMiddleware` validates keys and sets user/org context
	- Keys are SHA-256 hashed, include prefix for display, track last-used timestamp
	- Scoped to user and organization, support expiration and revocation
- Updated [internal/api/diff_review.go](internal/api/diff_review.go) to use API key auth from middleware context instead of bypass key
- Updated [internal/api/server.go](internal/api/server.go) to register API key routes under org context and protect diff-review endpoints with API key middleware
- Client: `lrc` accepts `--api-key` (or env `LRC_API_KEY`). Request/response structure unchanged.

6) ✅ **Optional polish**
- Added `make lrc` target to build the CLI in [Makefile](Makefile).
- Added comprehensive README in [cmd/lrc/README.md](cmd/lrc/README.md) with usage examples, flag documentation, and troubleshooting.
- Future enhancement: Consider a `--no-poll` mode that just returns `review_id` for external orchestration.
