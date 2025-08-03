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
    
    # Run migrations
    if dbmate up; then
        echo "✅ Database migrations completed successfully!"
    else
        echo "❌ Database migrations failed!"
        exit 1
    fi
}

# Function to start both UI and API servers
start_servers() {
    echo "🚀 Starting LiveReview servers..."
    echo "  - UI server will start on port 8081"
    echo "  - API server will start on port 8888"
    
    # Start UI server in background
    echo "🎨 Starting UI server..."
    ./livereview ui --port 8081 &
    UI_PID=$!
    
    # Give UI server a moment to start
    sleep 2
    
    # Start API server in background
    echo "⚙️  Starting API server..."
    ./livereview api --port 8888 &
    API_PID=$!
    
    # Function to cleanup on exit
    cleanup() {
        echo "🛑 Shutting down servers..."
        kill $UI_PID $API_PID 2>/dev/null || true
        wait $UI_PID $API_PID 2>/dev/null || true
        echo "✅ Servers stopped"
    }
    
    # Set trap to cleanup on exit
    trap cleanup TERM INT
    
    echo "✅ Both servers are starting up..."
    echo "🌐 UI available at: http://localhost:8081"
    echo "🔌 API available at: http://localhost:8888"
    
    # Wait for both processes
    wait $UI_PID $API_PID
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
