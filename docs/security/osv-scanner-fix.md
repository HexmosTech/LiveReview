# How to fix OSV scanner vulnerabilities

This document describes how to run the same security scan locally and how to fix the vulnerabilities.


## How to run OSV scanner Localy

Generate a security report by running the following command:

```bash
make security-osv
```

The `make security-osv` Makefile target runs `osv-scanner` recursively across the repository and writes a dated JSON report to `security_issues/`. 

The output of the scan is:

```
Scanning dir .
Warning: plugin transitivedependency/pomxml can be risky when run on untrusted artifacts. Please ensure you trust the source code and artifacts before proceeding.
Starting filesystem walk for root: /
Scanned /home/gk/hex/LiveReview/go.mod file and found 148 packages
Scanned /home/gk/hex/LiveReview/internal/prompts/vendor/cmd file and found 0 packages
Scanned /home/gk/hex/LiveReview/extension/livereview/package-lock.json file and found 378 packages
Scanned /home/gk/hex/LiveReview/ui/package-lock.json file and found 1287 packages
End status: 407 dirs visited, 1971 inodes visited, 4 Extract calls, 602.472549ms elapsed, 602.472606ms wall time
Wrote security_issues/osv-scanner-05-04-2026.json
Updated security_issues/osv-scanner-latest.json
```

So, osv-scan report will be generaed.

This report will have all the vulnerabilities found in the repository.

Ideally, the report should be empty.

```json
{
  "results": [],
  "experimental_config": {
    "licenses": {
      "summary": false,
      "allowlist": null
    }
  }
}
```


## How to fix vulnerabilities

1. Select the osv-scanner report.
2. Identify the specific vulnerable libraries (especially transitive dependencies).

### For Node Modules (`package.json`)
3. **Be Surgical:** Never run a blind `npm audit fix --force`, and **never** delete `package-lock.json` before running `npm install`. Doing so will rewrite thousands of lines in the lockfile and introduce massive risk.
4. Instead, use the `overrides` section in `package.json` (or update direct `dependencies`) to pin the exact libraries and versions that need bumping.
5. Run `npm install` *without* deleting the lockfile. This ensures only your targeted packages are bumped.

### For Go Modules (`go.mod`)
3. Use `go get <package_name>@<fixed_version>` (or `@latest`) to bump the vulnerable dependency.
4. If it's a deeply nested transitive dependency that refuses to update, use the `replace` directive in `go.mod` to force the patched version: `replace example.com/vuln/pkg => example.com/vuln/pkg v1.2.3`
5. Always run `go mod tidy` afterward to clean up the module graph.

### Final Steps
6. Run `make security-osv` again to verify that the vulnerabilities are fixed.
7. If vulnerabilities are still present, repeat the process by looking into each vulnerability and fixing it manually.

## Verify the fix

If all vulnerabilities are perfectly resolved by upgrading packages, the `make security-osv` command will pass (exit code 0) and the report will be empty:

```json
{
  "results": [],
  "experimental_config": {
    "licenses": {
      "summary": false,
      "allowlist": null
    }
  }
}
```

**Note on Ignored Vulnerabilities:**
If a vulnerability cannot be fixed because of an upstream database error or false positive, it can be suppressed in `config/osv-scanner.toml`. In this case, `make security-osv` will successfully pass, but the `security_issues/osv-scanner-latest.json` report will **still contain the vulnerability data** for audit purposes. This is expected behavior and ensures suppressed vulnerabilities remain visible in the raw logs.

> [!WARNING]
> **Strict Policy on Ignoring Vulnerabilities**
> 1. **Never arbitrarily update the ignore list (`config/osv-scanner.toml`).**
> 2. **Investigate the true impact.** Before doing anything, research the vulnerability. Search the codebase to see exactly how and where the vulnerable library is implemented. Determine if the vulnerable execution path is actually reachable in our context to gain a full understanding of the risk.
> 3. **Always ask for permission.** Only add a vulnerability to the ignore list if the user has manually viewed the report, thoroughly inspected the exact impact on the codebase, and explicitly asked you to proceed.
> 4. **Explore all alternatives first.** Before resorting to an ignore rule, see if there are other options to fix the issue:
>    - Try downgrading the affected package to a known stable, secure version if upgrading isn't working.
>    - Verify that the codebase builds correctly and all tests pass with the alternative version.
> 5. **Always document the reason.** If authorized to ignore, the rule must include a detailed `reason` explaining exactly why it is safe, and note when the rule should be removed.

Now verify the fix by building the affected workspaces:
- **UI**: `cd ui && npm run build`
- **Extension**: `cd extension/livereview && npm run compile`
- **Server**: `make build`

This step is mandatory. Never commit a dependency bump without verifying that the build still succeeds locally.

