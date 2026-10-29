# Integration Tests

This directory contains integration tests for the LiveReview pipeline that verify GitLab functionality with real API calls and payloads.

## Running Tests

**Run all integration tests:**
```bash
make -f config/Makefile.test test-all
```

**Run specific tests:**
```bash
make -f config/Makefile.test test-parser           # AI response parsing
make -f config/Makefile.test test-prompt           # Prompt construction
make -f config/Makefile.test test-api              # AI API calls
make -f config/Makefile.test test-gitlab-mr        # GitLab MR fetching
make -f config/Makefile.test test-gitlab-comments  # GitLab comment posting
```

**Run tests in this directory directly:**
```bash
go test -v ./tests
```

## Notes

- Integration tests require credentials and may be skipped in short mode
- For shell script tests (API/webhooks), see [../scripts/tests/](../scripts/tests/)
