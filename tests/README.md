# Integration Tests

This directory contains integration tests for the LiveReview pipeline that verify GitLab functionality with real API calls and payloads.

## Running Tests

**Run all integration tests:**
```bash
make -f Makefile.test test-all
```

**Run specific tests:**
```bash
make -f Makefile.test test-parser           # AI response parsing
make -f Makefile.test test-prompt           # Prompt construction
make -f Makefile.test test-api              # AI API calls
make -f Makefile.test test-gitlab-mr        # GitLab MR fetching
make -f Makefile.test test-gitlab-comments  # GitLab comment posting
make -f Makefile.test test-pipeline         # Complete pipeline
```

**Run tests in this directory directly:**
```bash
go test -v ./tests -run TestCompletePipeline
go test -v ./tests -run TestFetchMR335Details
```

## Notes

- Integration tests require credentials and may be skipped in short mode
- For debugging tools (Ollama/LLM connectivity), see [../debug/](../debug/)
- For shell script tests (API/webhooks), see [../scripts/tests/](../scripts/tests/)
