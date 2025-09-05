# How MR line numbers work

## GitLab

This section formalizes how to attach comments to lines in a Merge Request (MR) using the GitLab Discussions API. It focuses on text diffs (source code) and covers all positioning cases.

### Endpoints used
- Get MR diff versions (to retrieve SHAs for positioning)
  - GET /api/v4/projects/:id/merge_requests/:iid/versions
- Create a discussion (line/inline comment in MR diff)
  - POST /api/v4/projects/:id/merge_requests/:iid/discussions

### Required positioning fields (text diffs)
When sending a line (diff) comment, include a `position` object with:
- position_type: "text"
- base_sha, start_sha, head_sha: from the latest MR version (top entry from /versions)
- old_path, new_path: file path before and after the change (often identical unless rename)
- Exactly one or both of:
  - new_line: for added/context lines on the right side (after change)
  - old_line: for deleted/context lines on the left side (before change)

Notes
- For added lines, use new_line only (no old_line).
- For deleted lines, use old_line only (no new_line).
- For unchanged (context) lines, include both old_line and new_line (they may differ if prior edits shifted line numbers).
- Do not include `line_code` for single-line comments; it is only required inside `line_range` for multi-line notes.

### Multi-line (range) comments
For multi-line comments, provide `position[line_range]` with both `start` and `end` entries:
- line_range[start|end][line_code]: string of the form <SHA>_<old>_<new>
  - SHA is the SHA1 of the filename
  - old/new are the before/after line numbers for that anchor
- line_range[start|end][type]: "new" or "old" (whether the anchor is on the right/left side)
- Optionally include line_range[start|end][old_line] and/or ...[new_line]
- You still need base_sha/start_sha/head_sha, old_path/new_path, and either (new_line/old_line) at the top-level position per GitLab’s docs.

### Mapping algorithm (how to pick old_line/new_line)
1. Fetch latest MR version: GET /versions, use the first element’s SHAs.
2. Parse the MR diff hunks for the target file (unified diff headers `@@ -a,b +c,d @@`):
   - Track current old and new line counters for the hunk.
   - For each hunk line:
     - ' ' (space): context → advance both counters (old++, new++).
     - '+': added → advance new only (new++).
     - '-': removed → advance old only (old++).
3. For a candidate (file, display line N in the MR UI): determine whether that display line corresponds to a '+', '-', or ' ' line in the parsed hunk:
   - '+' → added line: set new_line to that new counter value; omit old_line.
   - '-' → deleted line: set old_line to that old counter value; omit new_line.
   - ' ' → context line: set both old_line and new_line to their respective counters for that line.

### File state cases
- New file (added):
  - old_path and new_path are the same path;
  - Only new_line is valid.
- Deleted file:
  - old_path and new_path are the same path (unless repository uses special paths for deletions);
  - Only old_line is valid.
- Renamed file:
  - old_path != new_path;
  - Use new_line for comments on added/context lines; old_line for deleted/context lines.

### Payload templates (JSON)
- Added line (green):
```
{
  "body": "<comment>",
  "position": {
    "position_type": "text",
    "base_sha": "<base>",
    "start_sha": "<start>",
    "head_sha": "<head>",
    "old_path": "path/file.go",
    "new_path": "path/file.go",
    "new_line": 42
  }
}
```

- Deleted line (red):
```
{
  "body": "<comment>",
  "position": {
    "position_type": "text",
    "base_sha": "<base>",
    "start_sha": "<start>",
    "head_sha": "<head>",
    "old_path": "path/file.go",
    "new_path": "path/file.go",
    "old_line": 58
  }
}
```

- Unchanged/context line:
```
{
  "body": "<comment>",
  "position": {
    "position_type": "text",
    "base_sha": "<base>",
    "start_sha": "<start>",
    "head_sha": "<head>",
    "old_path": "path/file.go",
    "new_path": "path/file.go",
    "old_line": 120,
    "new_line": 133
  }
}
```

- Multi-line (range):
```
{
  "body": "<comment>",
  "position": {
    "position_type": "text",
    "base_sha": "<base>",
    "start_sha": "<start>",
    "head_sha": "<head>",
    "old_path": "path/file.go",
    "new_path": "path/file.go",
    "new_line": 200, // anchor within the range
    "line_range": {
      "start": {
        "line_code": "<sha1_of_filename>_198_198",
        "type": "new",
        "new_line": 198
      },
      "end": {
        "line_code": "<sha1_of_filename>_202_202",
        "type": "new",
        "new_line": 202
      }
    }
  }
}
```

### Validation and pitfalls
- Always use the latest MR version SHAs, otherwise GitLab may reject with 400 (see upstream issues).
- Include both old_path and new_path even if identical.
- For single-line comments, omit `line_code`. For multi-line, provide `line_code` for start and end inside `line_range`.
- For deleted lines, ensure you set only old_line (not new_line). For added lines, only new_line.
- On context lines, old_line and new_line can differ due to previous edits.

### Image and file position types (for completeness)
- position_type = "image": use width, height, x, y.
- position_type = "file" (>= 16.4): used for file-level positions; refer to GitLab docs for exact semantics.

### Recommended implementation notes
- Maintain a robust hunk parser to classify lines (+, -, context) and track old/new counters.
- Derive old_line/new_line strictly from hunk state; do not guess from display numbers.
- Handle renames by carrying both old_path and new_path from the MR changes payload.
- Prefer JSON bodies; form-encoded works too but JSON is clearer and consistent.
