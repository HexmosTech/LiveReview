# Security Credentials Rotation Checklist

This document lists all credentials that were previously committed to git history and **MUST be rotated** before making the repository public.

> ⚠️ **CRITICAL**: Do not make this repository public until ALL high-priority credentials have been rotated.

## Credentials to Rotate

### 🔴 CRITICAL Priority (Rotate Immediately)

| Secret | Description | How to Rotate |
|--------|-------------|---------------|
| **Production DATABASE_URL** | PostgreSQL credentials for live database (IP: REDACTED_DB_HOST) | 1. Generate new password on DB server<br>2. Update connection string in deployment environment<br>3. Restart application |
| **RAZORPAY_LIVE_KEY** | Production payment gateway key | Regenerate in [Razorpay Dashboard](https://dashboard.razorpay.com/) → Settings → API Keys |
| **RAZORPAY_LIVE_SECRET** | Production payment gateway secret | Same as above - regenerate together with key |

### 🔴 HIGH Priority (Rotate Before Public Release)

| Secret | Description | How to Rotate |
|--------|-------------|---------------|
| **JWT_SECRET** | Signs all authentication tokens | Generate new: `openssl rand -hex 32`<br>Update in deployment `.env`<br>⚠️ All users will need to re-login |
| **CLOUD_JWT_SECRET** | Cloud mode authentication | Generate new UUID: `uuidgen`<br>Update in deployment `.env` |
| **FW_PARSE_ADMIN_SECRET** | Parse Server admin access | Generate new: `openssl rand -base64 32`<br>Update in Parse Server config |
| **RAZORPAY_WEBHOOK_SECRET** | Validates Razorpay webhook signatures | Regenerate in Razorpay Dashboard → Webhooks |

### 🟡 MEDIUM Priority (Rotate Soon After)

| Secret | Description | How to Rotate |
|--------|-------------|---------------|
| **RAZORPAY_TEST_KEY/SECRET** | Test payment keys | Regenerate in Razorpay Dashboard (test mode) |
| **LR_DISCORD_WEBHOOK_URL** | Signup notifications webhook | Create new webhook in Discord server settings, delete old one |

### 🟢 LOW Priority (Rotate When Convenient)

| Secret | Description | Notes |
|--------|-------------|-------|
| **VITE_PUBLIC_POSTHOG_KEY** | Analytics tracking | Low risk - client-side key. Can regenerate in PostHog project settings |
| **LR_CLARITY_ID** | Microsoft Clarity analytics | Low risk - can create new project if desired |
| **LR_LISTMONK_**** | Mailing list config | Internal service, rotate if exposed externally |

## Rotation Procedure

### Before Making Public

1. [ ] Create backup of current production `.env`
2. [ ] Rotate all CRITICAL credentials
3. [ ] Rotate all HIGH priority credentials
4. [ ] Test application functionality after rotation
5. [ ] Verify webhooks still work (Razorpay, etc.)
6. [ ] Confirm users can still authenticate (or plan for re-login)

### After Making Public

1. [ ] Rotate MEDIUM priority credentials
2. [ ] Monitor for any unauthorized access attempts
3. [ ] Rotate LOW priority credentials as convenient

## Verification

After rotation, verify:
- [ ] Application starts without errors
- [ ] Users can log in
- [ ] Payment processing works (test mode first, then live)
- [ ] Webhooks receive and process events
- [ ] Database connections succeed

## Notes

- Never commit rotated credentials to git
- Store production credentials in secure secret management (environment variables, vault, etc.)
- Consider implementing secret scanning in CI/CD to prevent future commits
