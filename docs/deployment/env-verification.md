# Pre-Deployment Environment Verification

This document explains the pre-deployment environment check implemented in `scripts/deployment_checks/verify_deploy.py` and hooked into `make raw-deploy`, `make raw-deploy-staging`, and `make build-with-ui`.

---

## 1. Why Was This Check Added?

### The Incident
During a past production deployment, users reported that the GitHub App integration ("Continue with GitHub") completely stopped functioning in production.

### Root Cause
- LiveReview was deployed using `make raw-deploy`.
- The local `.env.prod` on the deploying machine was missing the GitHub App environment variables (`GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY`, `GITHUB_APP_CLIENT_ID`, etc.).
- When `make raw-deploy` ran, it copied `.env.prod` directly to the production server's `.env`, overwriting the existing production configuration with an incomplete file.
- In `internal/githubapp/config.go`, LiveReview does not crash if `GITHUB_APP_ID` is empty—it silently treats it as "feature disabled" and starts up normally. As a result, the deployment succeeded without errors, but the GitHub App was dead in production.

To prevent this from ever happening again, **every deployment target now enforces strict key verification** before any build or rsync can occur.

---

## 2. Critical Safety Rule: Local `.env` Is Never Touched

- **Local `.env` is strictly for workstation development.**
- The verification script and deployment targets **only** operate on `.env.prod`, `.env.staging`, and `.env.prod.low-pricing`.
- Any attempt to run `scripts/deployment_checks/verify_deploy.py` against `.env` will immediately abort with an error to protect your local environment.
- Deployment targets never overwrite or modify your local `.env`.

---

## 3. How the Check Works

When you run `make raw-deploy` (or `make build-with-ui`, `make raw-deploy-staging`, `make check-env-prod`):

1. The script `scripts/deployment_checks/verify_deploy.py` is invoked with the target environment file and mode (`prod`, `staging`, or `prod-low-pricing`).
2. It verifies:
   - **Key Presence**: Every required key (Database, JWT, GitHub App, Razorpay, etc.) must exist in the file and have a non-empty value.
   - **Mode Validity**: In production, `RAZORPAY_MODE` must be `live` and `LIVEREVIEW_PRICING_PROFILE` must be `actual`.
   - **No Unregistered Keys**: If someone added a new environment variable to `.env.prod` that is not registered in the script, the check halts immediately. This prevents accidental configuration drift.

---

## 4. Troubleshooting & How to Fix Errors (For Developers and AI Agents)

### Scenario A: Missing or Empty Variable
**Error message:**
```text
❌ ERROR: Missing or empty required environment variable(s) in .env.prod:
   - GITHUB_APP_ID
   - GITHUB_APP_PRIVATE_KEY
```

**How to fix:**
1. Check if the variable was accidentally cleared in `.env.prod`.
2. If this is a required key, fill in the correct value in `.env.prod` and sync it to GitHub Secret Manager:
   ```bash
   make upload-secrets
   ```
3. If this variable is deprecated and has been removed from the Go codebase:
   - Open [`scripts/deployment_checks/deploy_env_keys.json`](file:///home/gk/hex/LiveReview/scripts/deployment_checks/deploy_env_keys.json).
   - Remove the key name from `base_keys` or the specific mode list.
   - Run `make check-env-prod` to verify the fix.

---

### Scenario B: Unregistered New Variable Found
**Error message:**
```text
❌ ERROR: Unregistered new environment variable(s) found in .env.prod:
   - NEW_SERVICE_API_KEY
```

**How to fix:**
This happens when someone introduces a new configuration option to `.env.prod` or `.env.staging`. To maintain deployment safety, all deployment variables must be registered.

1. **If this variable is required for production/staging:**
   - Open [`scripts/deployment_checks/deploy_env_keys.json`](file:///home/gk/hex/LiveReview/scripts/deployment_checks/deploy_env_keys.json).
   - Add `"NEW_SERVICE_API_KEY"` to `base_keys` (or the specific mode `add` list).
   - Test the check:
     ```bash
     make check-env-prod
     ```
   - Upload the updated secret bundle to GitHub repository variables:
     ```bash
     make upload-secrets
     ```
2. **If this variable was added by mistake or is unused:**
   - Remove the line from `.env.prod`.

---

## 5. Verification Commands

You can run these checks anytime without deploying:

```bash
# Check production environment file (.env.prod)
make check-env-prod

# Check staging environment file (.env.staging)
make check-env-staging

# Check low-pricing environment file (.env.prod.low-pricing)
make check-env-low-pricing
```

The script supports both `python3` and `python` and can also be run directly:
```bash
python3 scripts/deployment_checks/verify_deploy.py .env.prod --mode prod
```

