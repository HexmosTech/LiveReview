# Deployment Configuration

This directory contains deployment-related configuration files.

## Files

### livereview.hexmos.com
Production Nginx configuration file for the LiveReview service at hexmos.com.

### env-verification.md
Pre-deployment environment variable verification guide for `scripts/deployment_checks/verify_deploy.py`.
Documents why the check exists, how it protects `.env`, and how to add/remove deployment variables.

**Deployment Instructions:**
1. Copy this file to your Nginx sites-available directory:
   ```bash
   sudo cp livereview.hexmos.com /etc/nginx/sites-available/
   ```
2. Create a symbolic link in sites-enabled:
   ```bash
   sudo ln -s /etc/nginx/sites-available/livereview.hexmos.com /etc/nginx/sites-enabled/
   ```
3. Test Nginx configuration:
   ```bash
   sudo nginx -t
   ```
4. Reload Nginx:
   ```bash
   sudo systemctl reload nginx
   ```
