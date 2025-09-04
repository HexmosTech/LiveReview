#!/bin/sh
set -e

echo "🚀 Starting LiveReview application..."

# Function to wait for PostgreSQL to be ready
wait_for_postgres() {
    echo "⏳ Waiting for PostgreSQL to be ready..."
    
    until pg_isready -h livereview-db -p 5432 -U livereview; do
        echo "  PostgreSQL is not ready yet. Waiting 2 seconds..."
        sleep 2
    done
    
    echo "✅ PostgreSQL is ready!"
}

# Function to run database migrations
run_migrations() {
    echo "🔄 Running database migrations..."
    
    # Build DATABASE_URL from parts if not provided
    if [ -z "$DATABASE_URL" ]; then
        if [ -z "$DB_PASSWORD" ]; then
            echo "❌ DB_PASSWORD not provided; cannot construct DATABASE_URL"
            exit 1
        fi
        DATABASE_URL="postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable"
        export DATABASE_URL
    fi
    echo "🗄  Using DATABASE_URL host=$(echo "$DATABASE_URL" | sed 's#.*@##' | cut -d'?' -f1)"
    
    # Run dbmate migrations first
    if dbmate up; then
        echo "✅ Database migrations completed successfully!"
    else
        echo "❌ Database migrations failed!"
        exit 1
    fi
    
    # Run River migrations
    echo "🌊 Running River queue migrations..."
    if river migrate-up --database-url "$DATABASE_URL"; then
        echo "✅ River migrations completed successfully!"
    else
        echo "❌ River migrations failed!"
        exit 1
    fi
}

# Function to start all services (UI, API, and optionally River UI)
start_servers() {
    echo "🚀 Starting LiveReview servers..."
    
    # Read port configuration from environment variables
    BACKEND_PORT="${LIVEREVIEW_BACKEND_PORT:-8888}"
    FRONTEND_PORT="${LIVEREVIEW_FRONTEND_PORT:-8081}"
    REVERSE_PROXY="${LIVEREVIEW_REVERSE_PROXY:-false}"
    
    echo "📊 Configuration detected:"
    echo "  - Backend port: $BACKEND_PORT"
    echo "  - Frontend port: $FRONTEND_PORT"  
    echo "  - Reverse proxy mode: $REVERSE_PROXY"
    
    # Check if River UI should be started (based on environment variable)
    if [ "$ENABLE_RIVER_UI" = "true" ]; then
        echo "  - River UI will start on port 8080"
    fi
    
    # Auto-generate API URL based on reverse proxy setting
    if [ "$REVERSE_PROXY" = "true" ]; then
        API_URL="http://localhost/api"
        echo "  - Production mode: API behind reverse proxy at /api"
    else
        API_URL="http://localhost:$BACKEND_PORT"
        echo "  - Demo mode: Direct API access"
    fi
    echo "  - UI will be configured to use API at: $API_URL"
    
    # Auto-generate framework-specific environment variables for the UI build process
    # Note: We keep .env minimal for customers and derive these at runtime here.
    export API_URL="$API_URL"
    export VITE_API_URL="$API_URL"
    export REACT_APP_API_URL="$API_URL"
    export NEXT_PUBLIC_API_URL="$API_URL"
    export LIVEREVIEW_API_URL="$API_URL"  # Legacy support
    
    # Also export the standard port variables for legacy compatibility
    export BACKEND_PORT="$BACKEND_PORT"
    export FRONTEND_PORT="$FRONTEND_PORT"
    export LIVEREVIEW_BACKEND_PORT="$BACKEND_PORT"
    export LIVEREVIEW_FRONTEND_PORT="$FRONTEND_PORT"
    
    # Start UI server in background with API URL configuration
    echo "🎨 Starting UI server..."
    # Forward child process output explicitly to container stdout/stderr for docker logs
    ./livereview ui --port "$FRONTEND_PORT" --api-url "$API_URL" \
        >> /proc/1/fd/1 2>> /proc/1/fd/2 &
    UI_PID=$!
    
    # Give UI server a moment to start
    sleep 2
    
    # Start API server in background
    echo "⚙️  Starting API server..."
    ./livereview api --port "$BACKEND_PORT" \
        >> /proc/1/fd/1 2>> /proc/1/fd/2 &
    API_PID=$!
    
    # Optionally start River UI
    RIVER_PID=""
    if [ "$ENABLE_RIVER_UI" = "true" ]; then
    echo "🌊 Starting River UI..."
    riverui >> /proc/1/fd/1 2>> /proc/1/fd/2 &
        RIVER_PID=$!
    fi
    
    # Function to cleanup on exit
    cleanup() {
        echo "🛑 Shutting down servers..."
        if [ -n "$RIVER_PID" ]; then
            kill $UI_PID $API_PID $RIVER_PID 2>/dev/null || true
            wait $UI_PID $API_PID $RIVER_PID 2>/dev/null || true
        else
            kill $UI_PID $API_PID 2>/dev/null || true
            wait $UI_PID $API_PID 2>/dev/null || true
        fi
        echo "✅ Servers stopped"
    }
    
    # Set trap to cleanup on exit
    trap cleanup TERM INT
    
    echo "✅ Servers are starting up..."
    echo "🌐 UI available at: http://localhost:$FRONTEND_PORT"
    echo "🔌 API available at: http://localhost:$BACKEND_PORT"
    
    if [ "$ENABLE_RIVER_UI" = "true" ]; then
        echo "🌊 River UI available at: http://localhost:8080"
    fi
    
    # Wait for all processes
    if [ -n "$RIVER_PID" ]; then
        wait $UI_PID $API_PID $RIVER_PID
    else
        wait $UI_PID $API_PID
    fi
}

# Main execution flow
main() {
    echo "📋 LiveReview Startup Sequence"
    echo "=============================="
    
    # Step 1: Wait for PostgreSQL
    wait_for_postgres
    
    # Step 2: Run migrations
    run_migrations
    
    # Step 3: Start servers
    start_servers
}

# Execute main function
main "$@"
