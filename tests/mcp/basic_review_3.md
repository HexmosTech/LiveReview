### Test Case: Basic Review Flow

## Description
This test case verifies the complete end-to-end flow of performing a code review via the MCP server.

## Test Metadata
```yaml
test_id: basic_review_flow
timeout: 600
fail_fast: true
debug_on_failure: true
```

## Prerequisites
- Load `.env` from `.env`
- Variables available: `AI_PROVIDER`, `AI_TOKEN`, `GIT_PROVIDER`, `GIT_TOKEN`, `GIT_URL`, `TEST_REPO_URL`


## Execution Instructions (for MCP)
- Execute steps sequentially.
- Maintain state (share IDs across steps).
- On any failure: debug automatically (retry once, check inputs, show raw response), then continue to final report.
- **Final Output Only**: Use the exact "Test Execution Report" format at the bottom. No extra explanation.


## Steps

### 1. Add AI Connector
- **Action**: `create_ai_connector`
- **Input**:
  ```yaml
  provider: "{{AI_PROVIDER}}"
  token: "{{AI_TOKEN}}"
  ```
- **Expected**:
  - Status = success
  - Returns `id`

### 2. Add Git Connector
- **Action**: `create_git_connector`
- **Input**:
  ```yaml
  provider: "{{GIT_PROVIDER}}"
  token: "{{GIT_TOKEN}}"
  url: "{{GIT_URL}}"
  ```
- **Expected**:
  - Status = success
  - Returns `id`

### 3. Trigger Review
- **Action**: `trigger_review`
- **Input**:
  ```yaml
  repo_url: "{{TEST_REPO_URL}}"
  ```
- **Expected**:
  - Status = "accepted"
  - Returns `review_id`

### 4. Check Review Status (Poll)
- **Action**: `get_review_status`
- **Input**: `review_id: "{{review_id}}"`
- **Poll**: every 15s, max 300s
- **Expected**: Status eventually = "completed"

### 5. Get Comments
- **Action**: `get_review_comments`
- **Input**: `review_id: "{{review_id}}"`
- **Expected**: comments list is non-empty

---

## Expected Final Output Format (MCP must follow exactly)

```markdown
# Test Execution Report: Basic Review Flow

**Overall Result**: PASSED / FAILED

**Duration**: Xs

### Step Results
| Step | Action                  | Status   | Duration | Details |
|------|-------------------------|----------|----------|---------|
| 1    | Add AI Connector        | PASS/FAIL| 1.2s    | ai_connector_id = abc123 |
| 2    | Add Git Connector       | PASS/FAIL| ...     | ... |
| ...  | ...                     | ...      | ...     | ... |

### Debug Info (only if any failures)
- Step X failed with error: ...
- Raw response: ...
- Attempted fix: ...

### Captured Values

```yaml
ai_connector_id: "..."
git_connector_id: "..."
review_id: "..."
final_status: "completed"
comment_count: 12
```

**Conclusion**: All steps completed successfully with meaningful comments. (or failure reason)

### What changed & why it works

1. Added **Test Metadata** + **Execution Instructions** block — tells MCP exactly how to behave (stateful, debug on failure, output format).
2. Made steps more structured with YAML inputs and clear Expected outcomes.
3. Added **Expected Final Output Format** — this is the key. MCP is now forced to output **only** the test report at the end.
4. Included debugging instructions inside the test case so MCP tries to fix/debug automatically but still keeps final output clean.

Now when you feed this to your MCP, it should run the full coordinated flow, debug internally if needed, and respond with **just the Test Execution Report** — nothing else.

Would you like me to also give you the exact system prompt snippet you should add to your MCP to enforce this output discipline?


