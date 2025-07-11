#!/bin/bash

# Test script for GitLab line comments
# This script verifies that line comments can be created properly

echo "GitLab Line Comment Test"
echo "========================"

# Check if GitLab URL and token are provided
if [ -z "$GITLAB_URL" ] || [ -z "$GITLAB_TOKEN" ]; then
    echo "Error: GITLAB_URL and GITLAB_TOKEN environment variables must be set"
    echo "Example:"
    echo "  export GITLAB_URL=https://git.example.com"
    echo "  export GITLAB_TOKEN=your_access_token"
    exit 1
fi

# Check if MR URL is provided
if [ -z "$1" ]; then
    echo "Error: Merge Request URL required"
    echo "Usage: $0 <merge_request_url> [file_path] [line_number]"
    echo "Example: $0 https://git.example.com/group/project/-/merge_requests/123 src/main.go 42"
    exit 1
fi

MR_URL="$1"
FILE_PATH="${2:-README.md}"  # Default to README.md if not provided
LINE_NUM="${3:-1}"           # Default to line 1 if not provided

echo "Using GitLab URL: $GITLAB_URL"
echo "Using MR URL: $MR_URL"
echo "File path: $FILE_PATH"
echo "Line number: $LINE_NUM"
echo

# Extract project path and MR ID from the URL
PROJECT_PATH=$(echo "$MR_URL" | sed -E 's|.*/([^/]+/[^/]+)/-/merge_requests/.*|\1|')
MR_ID=$(echo "$MR_URL" | sed -E 's|.*/-/merge_requests/([0-9]+).*|\1|')

echo "Extracted project path: $PROJECT_PATH"
echo "Extracted MR ID: $MR_ID"
echo

# URL encode the project path
PROJECT_PATH_ENCODED=$(echo "$PROJECT_PATH" | sed 's|/|%2F|g')

echo "URL-encoded project path: $PROJECT_PATH_ENCODED"
echo

# Get merge request versions to obtain required SHAs
echo "Fetching MR versions..."
VERSIONS_RESPONSE=$(curl -s -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
    "$GITLAB_URL/api/v4/projects/$PROJECT_PATH_ENCODED/merge_requests/$MR_ID/versions")

# Extract the SHAs from the first version (latest)
BASE_SHA=$(echo "$VERSIONS_RESPONSE" | grep -o '"base_commit_sha":"[^"]*' | head -1 | cut -d'"' -f4)
HEAD_SHA=$(echo "$VERSIONS_RESPONSE" | grep -o '"head_commit_sha":"[^"]*' | head -1 | cut -d'"' -f4)
START_SHA=$(echo "$VERSIONS_RESPONSE" | grep -o '"start_commit_sha":"[^"]*' | head -1 | cut -d'"' -f4)

echo "Base SHA: $BASE_SHA"
echo "Head SHA: $HEAD_SHA"
echo "Start SHA: $START_SHA"
echo

if [ -z "$BASE_SHA" ] || [ -z "$HEAD_SHA" ] || [ -z "$START_SHA" ]; then
    echo "Error: Failed to retrieve SHAs from MR versions"
    echo "Response: $VERSIONS_RESPONSE"
    exit 1
fi

# Create a line comment using the discussions API
echo "Creating line comment..."
COMMENT_TEXT="Test line comment created at $(date) via script"

# Create JSON data for the request
JSON_DATA=$(cat <<EOF
{
  "body": "$COMMENT_TEXT",
  "position": {
    "position_type": "text",
    "base_sha": "$BASE_SHA",
    "head_sha": "$HEAD_SHA",
    "start_sha": "$START_SHA",
    "new_path": "$FILE_PATH",
    "old_path": "$FILE_PATH",
    "new_line": $LINE_NUM
  }
}
EOF
)

# Post the comment
echo "Posting comment with data:"
echo "$JSON_DATA"
echo

RESPONSE=$(curl -s -X POST \
    -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$JSON_DATA" \
    "$GITLAB_URL/api/v4/projects/$PROJECT_PATH_ENCODED/merge_requests/$MR_ID/discussions")

echo "Response:"
echo "$RESPONSE" | grep -v "^\s*$" | head -20
echo

# Check if the comment was created successfully
if echo "$RESPONSE" | grep -q '"id"'; then
    DISCUSSION_ID=$(echo "$RESPONSE" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)
    echo "Success! Line comment created with discussion ID: $DISCUSSION_ID"
    echo "View the comment in the merge request: $MR_URL"
else
    echo "Error: Failed to create line comment"
    echo "Full response:"
    echo "$RESPONSE"
    exit 1
fi

echo
echo "Test completed successfully!"
