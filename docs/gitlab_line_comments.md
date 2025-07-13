# GitLab Line Comments Feature

This document explains how to use the improved GitLab line comments feature in LiveReview.

## Overview

The line comments feature allows you to post comments on specific lines in files within GitLab merge requests. This implementation follows the GitLab API documentation and uses the best practices identified in the API documentation for creating line-specific comments in merge request diffs.

## Key Improvements

1. **Proper SHA references**: Now fetches merge request versions to get required SHA values for comment positioning.
2. **Correct position type**: Uses `text` as the position type as required by the GitLab API.
3. **Valid line_code generation**: Implements a proper line_code generator following GitLab's format.
4. **Robust fallback mechanism**: Tries multiple methods to ensure comments are posted successfully.
5. **Clear error reporting**: Provides detailed error messages if comment posting fails.

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

```bash
export GITLAB_URL=https://git.example.com
export GITLAB_TOKEN=your_access_token
./tests/test_gitlab_line_comment.sh "https://git.example.com/group/project/-/merge_requests/123" "src/main.go" 42
```

## Implementation Details

The implementation uses the GitLab Discussions API to create line comments. Key points:

1. First fetches merge request versions to get the required SHA values.
2. Generates a valid `line_code` in the format: `<start_sha>_<head_sha>_<normalized_path>_<side>_<line>`.
3. Creates a position object with the correct properties:
   - `position_type`: Always set to "text"
   - `base_sha`: SHA of the base commit in the source branch
   - `head_sha`: SHA of the HEAD commit in the source branch
   - `start_sha`: SHA of the commit in the target branch
   - `new_path`: Path of the file to comment on
   - `old_path`: Original path of the file (same as new_path for unchanged files)
   - `new_line`: Line number to comment on
   - `line_code`: The generated line code to properly position the comment

4. Falls back to alternative methods if the preferred method fails:
   - First tries with complete position data including line_code
   - Then tries an alternative discussions-based approach
   - Finally falls back to a regular comment with file/line info in the text

## Line Code Generation

The `GenerateLineCode` function creates a valid line code following GitLab's format:

```go
func GenerateLineCode(startSHA, headSHA, filePath string, lineNum int) string {
    // Take first 8 characters of each SHA
    shortStartSHA := startSHA[:8]
    shortHeadSHA := headSHA[:8]
    
    // Normalize the file path by replacing slashes with underscores
    normalizedPath := strings.ReplaceAll(filePath, "/", "_")
    
    // Create a unique file identifier using SHA-1 hash of the file path
    pathHash := fmt.Sprintf("%x", sha1.Sum([]byte(filePath)))[:8]
    
    // New style format used in newer GitLab versions (path hash and line number)
    newStyleCode := fmt.Sprintf("%s_%d", pathHash, lineNum)
    
    // Old style format with SHAs and normalized path
    oldStyleCode := fmt.Sprintf("%s_%s_%s_%s_%d",
        shortStartSHA,
        shortHeadSHA,
        normalizedPath,
        "right",
        lineNum)
        
    // Return combined format that our code will parse and try both styles
    return newStyleCode + ":" + oldStyleCode
}
```

The implementation tries both line code formats to ensure compatibility with different GitLab versions and configurations.

## Troubleshooting

If line comments aren't appearing correctly:

1. **Verify the SHA values**: Ensure the merge request version information is correctly fetched.
2. **Check file paths**: Ensure file paths don't have leading slashes and exist in the merge request.
3. **Line numbers**: Verify that the line number exists in the latest version of the file.
4. **Line code format**: Check that the line_code is being generated with the correct format.
5. **GitLab version**: Different GitLab versions may have slightly different API requirements.

## References

- GitLab API Documentation: [Create new merge request thread](https://docs.gitlab.com/ee/api/discussions.html#create-new-merge-request-thread)
- GitLab API Documentation: [Line code format](https://docs.gitlab.com/ee/api/discussions.html#line-code)
