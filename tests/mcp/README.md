# MCP Test Cases

This directory contains executable test cases for the LiveReview MCP server.

## Purpose
These markdown files are designed to be read and executed by an AI agent (like Claude with the LiveReview MCP server enabled) to verify that the server's tools are functioning correctly.

## Test Cases
- [List Tools](list_tools.md): Verifies tool discovery.
- [Basic Review](basic_review.md): Verifies the end-to-end review flow.

## Configuration
Before running the tests, create or update the `.env` file in this directory:
```bash
cp .env.example .env  # If you create an example file later, or just edit the .env I created
```
Fill in your `AI_TOKEN`, `GIT_TOKEN`, and `TEST_REPO_URL`.

## How to use
Tell the AI agent:
"Please execute the test cases in `tests/mcp/` using the credentials in `tests/mcp/.env` and report the results."
