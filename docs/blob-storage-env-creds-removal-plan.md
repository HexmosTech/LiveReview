# Removing cloud-storage credentials from the deploy env

Status: implemented
Related: `docs/deployment/env-verification.md`, `AGENTS.md` (Artifact sync channel), `docs/SECURITY_ROTATION.md`

## What was wrong

`make raw-deploy` failed in `check-env-prod` because `.env.prod` contained
`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `AWS_REGION` under a stale
"AWS Lambda Tools Execution Credentials" comment. The verifier rejects any key
that is not registered.

Those keys were unused:

- No Go code reads `AWS_*` environment variables.
- Blob storage reads its credentials from `system_settings.blob_storage`
  (`internal/blobstore`), which already contained a complete S3/B2 config.
- Bedrock AI connectors read their credentials from `ai_connectors`, not the
  env. There were no Bedrock connectors configured anyway.

## What changed

1. **Removed the keys** from `.env.prod`, `.env.staging`,
   `.env.prod.low-pricing`, `.env.local`, and the stale `.ghsm-backups/*`
   copies, then ran `make upload-secrets` so the GitHub secret-manager copies
   no longer carry them.
2. **Added a forbidden-keys guard** to `verify_deploy.py` /
   `deploy_env_keys.json`, so re-adding `AWS_*` fails the deploy with a clear
   message instead of silently drifting.
3. **Added `livereview storage ensure`** (`cmd/storage.go`):
   - Loads `DATABASE_URL` from `--env-file` (the deploy targets always pass it,
     so `main()`'s implicit `.env` load cannot silently target the wrong
     database) and prints the resolved host/database before doing anything.
   - If `system_settings.blob_storage` exists and is valid: read-only check,
     plus an optional `--test` write/read/delete marker.
   - If the row is missing or invalid: prompts for the values (masked secret
     input) and writes them, but only when stdin is a terminal. Otherwise it
     exits non-zero with instructions.
   - Never overwrites a valid row.
4. **Wired it into `raw-deploy`** (and the low-pricing/backend variants) after
   `db-ready.sh` and before `pm2 reload`. A broken or missing row stops the
   deploy before the new binary starts serving.
5. **Shared the validation/test code** between the HTTP settings API and the
   CLI via `blobstore.ValidateConfig` / `blobstore.CheckWritable`, so the
   "Test connection" button and the deploy check cannot drift.

Staging intentionally has no `blob_storage` row and keeps using the
filesystem backend; the staging deploy does not run the check.

## How to verify

```bash
# The original failing command, now green:
make check-env-prod

# Prove the DB credentials work with no AWS_* in the environment:
env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_REGION \
  ./livereview storage ensure --env-file .env.prod --no-interactive --test
```

Expected output:

```text
Target database: <host>:<port>/<db>
✅ blob_storage row exists and is valid.
  backend:           s3
  bucket/container:  livereview-private
  ...
Testing storage connection (write/read/delete)... ok
```

## Bootstrap / disaster recovery

There is no seed file and no env fallback. If the row is ever missing on a
fresh or restored database:

- Run `make raw-deploy` in a terminal and answer the prompt, or
- Set it once in **Settings → Storage** (works for GCS/Azure too).

The prompt supports `s3` and `filesystem`. Use the Settings UI for GCS/Azure.

## Remaining action: rotate the old key

The removed `AKIA…` AWS IAM key lived in several env files, local backups, and
GitHub repository variables. Revoke/rotate it. If the B2 key stored in
`blob_storage` was ever copied into an env file, rotate that too and re-enter
it via the prompt or Settings → Storage.
