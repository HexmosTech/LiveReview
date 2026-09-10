#!/bin/bash

set -e

echo "Starting MCP server..."

nohup ./livereview api > server.log 2>&1 &
SERVER_PID=$!

echo $SERVER_PID > server.pid

cleanup() {
    echo "Cleaning up..."
    kill $SERVER_PID || true
}

trap cleanup EXIT

echo "Waiting for server startup..."

for i in {1..30}; do
    response=$(curl -s http://localhost:8888/health || true)

    if echo "$response" | grep -q '"status":"healthy"\|"status": "healthy"'; then
        echo "Server is healthy"
        break
    fi

    echo "Waiting for server to start..."
    sleep 2
done

response=$(curl -s http://localhost:8888/health || true)

if ! echo "$response" | grep -q '"status":"healthy"\|"status": "healthy"'; then
    echo "Server failed to become healthy"
    cat server.log
    exit 1
fi

echo "Running MCP test script..."

python3 tests/mcp/mcp-testcase.py

echo "Checking MCP test result..."

python3 <<EOF
import json
import sys

with open("test_results.json") as f:
    result = json.load(f)

status = result.get("suite_summary", {}).get("overall_status")

print(f"Overall Status: {status}")

if status == "PASSED":
    print("MCP test passed")
    sys.exit(0)

print("MCP test failed")
print(json.dumps(result, indent=2))
sys.exit(1)
EOF