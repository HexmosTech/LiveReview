# LiveReview Testing Guide

This directory contains tests for the LiveReview pipeline. The testing approach is to break down the pipeline into separate testable components:

1. **Getting MR Context:** Tests in `internal/providers/gitlab/tests/merge_request_test.go`
2. **Constructing Prompts:** Tests in `internal/ai/gemini/tests/prompt_test.go`
3. **Submitting Prompts to AI Provider:** Tests in `internal/ai/gemini/tests/api_test.go`
4. **Parsing AI Responses:** Tests in `internal/ai/gemini/tests/parser_test.go`
5. **Posting Comments:** Tests in `internal/providers/gitlab/tests/post_comments_test.go`

## Running Tests

You can run individual component tests or all tests using the provided Makefile targets:

```bash
# Run specific component tests
make -f Makefile.test test-parser
make -f Makefile.test test-prompt
make -f Makefile.test test-api
make -f Makefile.test test-gitlab-mr
make -f Makefile.test test-gitlab-comments
make -f Makefile.test test-pipeline

# Run all tests
make -f Makefile.test test-all
```

## Test Architecture

Each component test uses appropriate mocking strategies:

- **HTTP APIs:** Uses Go's `httptest` package to mock server responses
- **AI Provider:** Mock responses with known test patterns
- **File System:** In-memory mock structures

## Test Fixtures

When adding new tests, consider adding test fixtures in appropriate subdirectories:

- **Example Diffs:** Save examples of git diffs
- **Example AI Responses:** Both valid JSON and malformed responses
- **Example MR Contexts:** Complete MR context objects

## Debugging Failed Tests

When tests fail, look for:

1. **HTTP Errors:** Check for correct paths and methods in mock servers
2. **JSON Parsing:** Verify JSON structure in responses
3. **Model Types:** Ensure all struct fields match expected types

## Test Coverage

Aim for comprehensive test coverage for critical paths, especially:

- JSON parsing edge cases
- Error handling
- API connectivity issues
