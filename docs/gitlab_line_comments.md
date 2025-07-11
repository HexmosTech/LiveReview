# GitLab Line Comments Feature

This document explains how to use the improved GitLab line comments feature in LiveReview.

## Overview

The line comments feature allows you to post comments on specific lines in files within GitLab merge requests. This implementation follows the GitLab API documentation and uses the best practices identified in the API documentation and the Python implementation example.

## Key Improvements

1. **Proper SHA references**: Now fetches merge request versions to get required SHA values for comment positioning.
2. **Correct position type**: Uses `text` as the position type as required by the GitLab API.
3. **Robust fallback mechanism**: Tries multiple methods to ensure comments are posted successfully.
4. **Clear error reporting**: Provides detailed error messages if comment posting fails.

## Usage

### Through the API

To post a comment on a specific line:

```go
comment := &models.ReviewComment{
    FilePath: "path/to/file.go",
    Line:     42,
    Content:  "This line needs improvement",
    Severity: models.SeverityWarning,
    Suggestions: []string{
        "Consider adding error handling",
        "Use a more descriptive variable name",
    },
}

// Post the comment
err := provider.PostComment(ctx, mrID, comment)
```

### Manual Testing

You can use the test script to manually verify line comment functionality:

1. Set the required environment variables:

```bash
export GITLAB_URL=https://git.example.com
export GITLAB_TOKEN=your_access_token
```

2. Run the test script:

```bash
./tests/test_gitlab_line_comment.sh "https://git.example.com/group/project/-/merge_requests/123" "src/main.go" 42
```

## Implementation Details

The implementation uses the GitLab Discussions API to create line comments. Key points:

1. First fetches merge request versions to get the required SHA values.
2. Creates a position object with the correct properties:
   - `position_type`: Always set to "text"
   - `base_sha`: SHA of the base commit in the source branch
   - `head_sha`: SHA of the HEAD commit in the source branch
   - `start_sha`: SHA of the commit in the target branch
   - `new_path`: Path of the file to comment on
   - `new_line`: Line number to comment on

3. Falls back to alternative methods if the preferred method fails:
   - First tries with position data in the discussions API
   - Then tries the discussions-based approach
   - Then tries the direct approach
   - Finally falls back to a regular comment with file/line info

## Troubleshooting

If line comments aren't appearing correctly:

1. Verify the SHA values are correct
2. Check that the file path exists in the merge request
3. Ensure the line number is valid
4. Check the GitLab API responses for any error messages

## References

- GitLab API Documentation: [Create new merge request thread](https://docs.gitlab.com/ee/api/discussions.html#create-new-merge-request-thread)
