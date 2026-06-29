# LiveReview Staging Environment Guide

This guide details how to work with, build, deploy, and manage the LiveReview staging environment.

---

## 1. Staging Environment Details

- **Staging URL**: [https://livereview.hexmos.site](https://livereview.hexmos.site)
- **Deployment Path**: `/home/ubuntu/staging_lr`
- **Process Manager**: PM2 running `ecosystem.staging.config.js`
- **Configuration File**: `.env.staging`

---

## 2. Staging Make Commands

The `Makefile` defines several targets for staging development and operations:

### A. Build for Staging

```bash
make build-staging-with-ui
```

- **What it does**: Installs UI dependencies, injects `.env.staging` parameters, compiles the obfuscated UI bundle, and compiles the Go backend binary (`livereview`).
- **Mock AI**: Staging has `LIVEREVIEW_MOCK_AI=true` enabled to prevent generating actual OpenAI/Gemini costs during testing.

### B. Deploy to Staging

```bash
make raw-deploy-staging
```

- **What it does**:
  1. Compiles the latest staging build.
  2. Runs database migrations (`dbmate up` and `river migrate-up`) **directly from your local machine** against the staging PostgreSQL database (defined by `DATABASE_URL` in `.env.staging`).
  3. Uploads the compiled binary, config file, `.env.staging` configuration, and mock LLM settings to the staging host `nats03-do`.
  4. Reloads the PM2 staging daemon (`ecosystem.staging.config.js`).

### C. Stop Staging

```bash
make stop-staging
```

- **What it does**: Deletes the staging processes from PM2 on the staging host, stopping the services.

### D. Run Staging River UI (Local Dashboard)

```bash
make staging-river-ui
```

- **What it does**: Starts the River UI web server locally, connected to the staging database (via the database connection string defined in `.env.staging`). This allows you to inspect active, failed, or completed jobs on the staging queue from your local web browser.

---

## 3. Monitoring & Logs on Staging

To view logs and process status on the staging host, SSH into the server:

```bash
ssh server
cd /home/ubuntu/staging_lr

# Check active processes
pm2 status

# Monitor live logs
pm2 logs
```
