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
2. Add to AI prompt and ask to fix the vulnerabilities.
3. AI will fix the vulnerabilities by updating the dependencies.
4. Run `make security-osv` again to verify that the vulnerabilities are fixed.
5. If vulnerabilities are still present, repeat the process by actually looking into each vulnerability and fix it manually.

## Verify the fix

If all vulnerabilities are fixed, the `make security-osv` command will not find any vulnerabilities and the report will be empty.

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

Now Verify by running ui, server and extension.
This is for local verification wheather the change in package.json or go.mod is correct or not.

