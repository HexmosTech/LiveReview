#!/bin/bash
#
# Local Development Setup Script for LiveReview
# This script sets up a local development environment using nginx and ngrok
# to allow external access to your local development server.
#
# Prerequisites:
#   - nginx installed (sudo apt install nginx)
#   - ngrok installed and configured (https://ngrok.com/download)
#
# What this script does:
#   1. Creates a temporary nginx configuration file
#   2. Restarts nginx to apply the configuration
#   3. Starts ngrok to expose the local server to the internet
#

# Exit on any error
set -e

echo "=== LiveReview Local Development Setup ==="
echo ""

# -------------------------------------------------------------------------
# 1. Create nginx configuration
# -------------------------------------------------------------------------
echo "Setting up nginx configuration..."

# Create a temporary nginx config file
NGINX_CONFIG_FILE="/tmp/livereview_nginx.conf"

cat > "$NGINX_CONFIG_FILE" << 'EOF'
server {
    listen       80;
    server_name  _;

    # Route /api/* to port 8888 (backend API)
    location ^~ /api/ {
        proxy_pass http://127.0.0.1:8888;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Everything else to port 8081 (frontend)
    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

# Check if we have permission to modify nginx configs
if [ -w /etc/nginx/conf.d/ ]; then
    sudo cp "$NGINX_CONFIG_FILE" /etc/nginx/conf.d/livereview.conf
    echo "✅ Nginx configuration created at /etc/nginx/conf.d/livereview.conf"
else
    echo "⚠️  Unable to automatically copy nginx config. Please run:"
    echo "    sudo cp $NGINX_CONFIG_FILE /etc/nginx/conf.d/livereview.conf"
    echo "    sudo nginx -t"
    echo "    sudo systemctl restart nginx"
    read -p "Press Enter to continue after you've manually configured nginx..."
fi

# Restart nginx if we have permission
if command -v systemctl &> /dev/null && [ -w /etc/nginx/conf.d/ ]; then
    echo "Restarting nginx..."
    sudo systemctl restart nginx
    echo "✅ Nginx restarted"
fi

# -------------------------------------------------------------------------
# 2. Start ngrok to expose the local server
# -------------------------------------------------------------------------
echo ""
echo "Starting ngrok to expose local server..."
echo "NOTE: Keep this terminal window open while developing!"
echo ""

# Check if ngrok is installed
if ! command -v ngrok &> /dev/null; then
    echo "❌ ngrok is not installed. Please install it from https://ngrok.com/download"
    exit 1
fi

# The ngrok command with your specific parameters
echo "Running ngrok with your specified configuration..."
echo "ngrok http --url=talented-manually-turkey.ngrok-free.app 80 --host-header=\"localhost:80\""
echo ""
echo "Your application will be accessible at: https://talented-manually-turkey.ngrok-free.app"
echo ""
echo "Press Ctrl+C to stop the services when you're done"
echo "-------------------------------------------------------------------------"

# Run ngrok
ngrok http --url=talented-manually-turkey.ngrok-free.app 80 --host-header="localhost:80"

# This part won't execute until ngrok is stopped (Ctrl+C)
echo ""
echo "Cleaning up..."

# Remove nginx config if we have permission
if [ -w /etc/nginx/conf.d/ ]; then
    sudo rm -f /etc/nginx/conf.d/livereview.conf
    sudo systemctl restart nginx
    echo "✅ Nginx configuration removed"
else
    echo "⚠️  Please manually remove the nginx configuration:"
    echo "    sudo rm /etc/nginx/conf.d/livereview.conf"
    echo "    sudo systemctl restart nginx"
fi

echo ""
echo "Local development environment has been shut down."
