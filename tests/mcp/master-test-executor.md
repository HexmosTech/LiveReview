
# Master MCP Test Executor (Tests 3–10)

**Purpose**: Execute all test cases from 3 to 10 automatically and produce a clean, scrollable report.

## Instructions for Claude

You are the Test Executor.

1. Use the **filesystem MCP tool** to discover and read **all** markdown files in `tests/mcp/` that are numbered from `3.` to `10.` (e.g., `3.check_quota.md`, `4.xxx.md`, ..., `10.xxx.md`).

2. For **each** test file, do the following in order:
   - Read the full content of the file.
   - Follow **every** instruction in the file, especially:
     - Execution Instructions
     - Steps
     - Expected Final Output Format
   - If the file contains a section called `### Validation Requirements`, **strictly apply** all rules listed in that section on top of the test’s normal steps.
   - On any failure: retry once as per the file’s instructions and capture raw responses.

3. Execute tests **sequentially** in numerical order (3 → 10).

## Required Output Structure

**Output exactly in this order. Nothing else before or after.**

### 1. Summary Overview (First)

```markdown
# Test Suite Execution Summary (Tests 3–10)

**Total Tests**: X  
**Passed**: Y  
**Failed**: Z  
**Overall Result**: PASSED / PARTIALLY PASSED / FAILED  
**Total Duration**: Xs

| Test ID | File Name                | Overall Result | Duration | Notes                  |
|---------|--------------------------|----------------|----------|------------------------|
| 3       | 3.check_quota.md         | PASS/FAIL      | Xs       | Brief note if needed   |
| ...     | ...                      | ...            | ...      | ...                    |
```

### 2. Detailed Reports (After the table)

For each test, output:

```markdown
---

## Detailed Report: [Test Name from file]

[ Paste the **exact** "Test Execution Report" required by that test file ]
```

---

**Strict Rules**:
- Output **nothing** except the Summary table + Detailed Reports.
- No extra explanations, greetings, or conclusions.
- Separate each detailed report with `---`.
- Always respect and apply any `### Validation Requirements` section present in each individual test file.
- Preserve the exact markdown format demanded by each test file.

Begin execution now.
```
