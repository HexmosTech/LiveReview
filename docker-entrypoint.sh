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
    
    # Set the DATABASE_URL for dbmate if not already set
    if [ -z "$DATABASE_URL" ]; then
        echo "❌ DATABASE_URL environment variable is not set"
        exit 1
    fi
    
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
    echo "  - UI server will start on port 8081"
    echo "  - API server will start on port 8888"
    
    # Check if River UI should be started (based on environment variable)
    if [ "$ENABLE_RIVER_UI" = "true" ]; then
        echo "  - River UI will start on port 8080"
    fi
    
    # Determine API URL for UI configuration
    # Use environment variable if set, otherwise default to localhost
    API_URL="${LIVEREVIEW_API_URL:-http://localhost:8888}"
    echo "  - UI will be configured to use API at: $API_URL"
    
    # Start UI server in background with API URL configuration
    echo "🎨 Starting UI server..."
    ./livereview ui --port 8081 --api-url "$API_URL" &
    UI_PID=$!
    
    # Give UI server a moment to start
    sleep 2
    
    # Start API server in background
    echo "⚙️  Starting API server..."
    ./livereview api --port 8888 &
    API_PID=$!
    
    # Optionally start River UI
    RIVER_PID=""
    if [ "$ENABLE_RIVER_UI" = "true" ]; then
        echo "🌊 Starting River UI..."
        riverui &
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
    echo "🌐 UI available at: http://localhost:8081"
    echo "🔌 API available at: http://localhost:8888"
    
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
