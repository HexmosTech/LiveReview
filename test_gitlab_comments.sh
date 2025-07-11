#!/bin/bash

# Check if a GitLab token was provided
if [ -z "$1" ]; then
  echo "Usage: $0 <gitlab_token>"
  echo "Please provide your GitLab token as an argument"
  exit 1
fi

# Edit the test file to replace the token
GITLAB_TOKEN="$1"
TEST_FILE="./tests/gitlab_comment_test.go"

# Create a backup
cp $TEST_FILE ${TEST_FILE}.bak

# Replace the token placeholder with the actual token
sed -i "s/Token: \"YOUR_GITLAB_TOKEN\"/Token: \"$GITLAB_TOKEN\"/" $TEST_FILE

# Run the test with verbose output
go test -v ./tests -run=TestGitLabLineCommentMethods

# Restore the backup to avoid committing the token
mv ${TEST_FILE}.bak $TEST_FILE

echo "Test completed. Check the GitLab merge request to see which comment methods worked!"
