# Deployment Configuration

This directory contains deployment-related configuration files.

## Files

### livereview.hexmos.com
Production Nginx configuration file for the LiveReview service at hexmos.com.

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
