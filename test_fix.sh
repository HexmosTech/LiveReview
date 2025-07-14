#!/bin/bash
# Simple test script to verify the line comment fix

# Build the binary
echo "Building LiveReview..."
make build

# Test specific lines
echo -e "\n=== Testing Line 160 (Deleted Line) ==="
./livereview review --mr-url "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/403" \
  --comment "TEST: This is a test comment for line 160 (deleted line: defer client.Close())" \
  --file "liveapi-backend/gatekeeper/gk_input_handler.go" \
  --line 160

echo -e "\n=== Testing Line 44 (Added Line) ==="
./livereview review --mr-url "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/403" \
  --comment "TEST: This is a test comment for line 44 (added line: TopK *float64)" \
  --file "liveapi-backend/gatekeeper/gk_input_handler.go" \
  --line 44
