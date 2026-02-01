#!/bin/bash
# GitHub Container Registry Login Script
# Requires GITHUB_TOKEN environment variable to be set

if [ -z "$GITHUB_TOKEN" ]; then
    echo "Error: GITHUB_TOKEN environment variable is not set"
    echo "Please set it with: export GITHUB_TOKEN=ghp_your_token_here"
    exit 1
fi

echo "$GITHUB_TOKEN" | docker login ghcr.io -u shrsv --password-stdin
