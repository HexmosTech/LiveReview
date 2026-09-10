#!/bin/sh
set -e

echo "🚀 Starting LiveReview application..."

# Function to load .env file
load_env() {
    if [ -f ".env" ]; then
        echo "📋 Loading environment variables from .env file..."
        # Export variables from .env, handling variable substitution
        set -a
        . ./.env
        set +a
        echo "✅ Environment variables loaded"
    else
        echo "⚠️  No .env file found, using existing environment variables"
    fi
}

# Function to extract database connection details
extract_db_details() {
    if [ -z "$DATABASE_URL" ]; then
        echo "❌ DATABASE_URL not set"
        exit 1
    fi
    
    # Parse DATABASE_URL to extract components
    # Format: postgres://user:password@host:port/database?params
    DB_USER=$(echo "$DATABASE_URL" | sed -n 's#.*://\([^:]*\):.*#\1#p')
    DB_PASS=$(echo "$DATABASE_URL" | sed -n 's#.*://[^:]*:\([^@]*\)@.*#\1#p')
    DB_HOST=$(echo "$DATABASE_URL" | sed -n 's#.*@\([^:]*\):.*#\1#p')
    DB_PORT=$(echo "$DATABASE_URL" | sed -n 's#.*:\([0-9]*\)/.*#\1#p')
    DB_NAME=$(echo "$DATABASE_URL" | sed -n 's#.*/\([^?]*\).*#\1#p')
    
    echo "📊 Database connection details:"
    echo "  - Host: $DB_HOST"
    echo "  - Port: $DB_PORT"
    echo "  - User: $DB_USER"
    echo "  - Database: $DB_NAME"
    
    export DB_USER DB_PASS DB_HOST DB_PORT DB_NAME
}

# Function to wait for PostgreSQL to be ready
wait_for_postgres() {
    echo "⏳ Waiting for PostgreSQL server to be ready..."
    
    until pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER"; do
        echo "  PostgreSQL is not ready yet. Waiting 2 seconds..."
        sleep 2
    done
    
    echo "✅ PostgreSQL server is ready!"
}

# Function to check if database exists and create if needed
ensure_database_exists() {
    echo "🔍 Checking if database '$DB_NAME' exists..."
    
    # Try to connect to the target database
    if PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" > /dev/null 2>&1; then
        echo "✅ Database '$DB_NAME' exists"
        return 0
    fi
    
    echo "⚠️  Database '$DB_NAME' does not exist, creating it..."
    
    # Connect to postgres database to create the target database
    if PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "postgres" -c "CREATE DATABASE $DB_NAME;" > /dev/null 2>&1; then
        echo "✅ Database '$DB_NAME' created successfully"
    else
        echo "❌ Failed to create database '$DB_NAME'"
        exit 1
    fi
}

# Function to run database migrations
run_migrations() {
    echo "🔄 Running database migrations..."
    echo "🗄  Using DATABASE_URL: $DATABASE_URL"
    
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
        # In production mode with reverse proxy, do NOT set API_URL
        # Let the Go server auto-detect from the frontend URL
        echo "  - Production mode: API behind reverse proxy (auto-detect from frontend URL)"
        unset API_URL  # Ensure no API_URL is set
    else
        API_URL="http://localhost:$BACKEND_PORT"
        echo "  - Demo mode: Direct API access at $API_URL"
        export API_URL="$API_URL"
    fi
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
    
    # Step 1: Load environment variables from .env
    load_env
    
    # Step 2: Extract database connection details
    extract_db_details
    
    # Step 3: Wait for PostgreSQL server
    wait_for_postgres
    
    # Step 4: Ensure database exists (create if needed)
    ensure_database_exists
    
    # Step 5: Run migrations
    run_migrations
    
}

# Execute main function
main "$@"
