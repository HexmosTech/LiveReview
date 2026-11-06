#!/usr/bin/env python3
"""Waive specific, documented govulncheck findings without silencing the check.

govulncheck has no native ignore-list feature (unlike config/osv-scanner.toml,
which does). This script runs it and parses the human-readable output for
`Vulnerability #N: GO-XXXX-YYYY` lines, comparing the set of reported IDs
against config/govulncheck-allowlist.txt. Any reported ID *not* in that file
still fails the build - this never turns the check off, it only lets specific,
reasoned, dated waivers through.

Note: the Makefile invokes govulncheck via `go run ...@version`, and `go run`
always remaps the wrapped program's real exit code to 1 (it only *prints* the
real code, e.g. "exit status 3", as informational text) - so this script
can't use the process exit status to detect "vulnerabilities found"; it
looks for `Vulnerability #` lines in the output instead. A nonzero status
with no such lines (a compile/network/tool error) passes through unchanged.
"""
import argparse
import re
import sys

VULN_RE = re.compile(r"^Vulnerability #\d+:\s*(\S+)", re.MULTILINE)


def load_allowlist(path: str) -> set[str]:
    ids = set()
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            ids.add(line)
    return ids


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--log", required=True, help="path to captured govulncheck output")
    parser.add_argument("--status", required=True, type=int, help="govulncheck's own exit status")
    parser.add_argument("--allowlist", required=True, help="path to config/govulncheck-allowlist.txt")
    args = parser.parse_args()

    with open(args.log, encoding="utf-8") as f:
        log = f.read()
    print(log, end="")

    found = set(VULN_RE.findall(log))

    if not found:
        # status 0 ("nothing found") or a real tool error (bad packages,
        # compile failure, network issue) - neither is ours to filter.
        return args.status

    allowlist = load_allowlist(args.allowlist)
    unwaived = found - allowlist
    waived = found & allowlist

    if waived:
        print(f"\n[govulncheck-filter] Waived (documented, see {args.allowlist}): {', '.join(sorted(waived))}")
    if unwaived:
        print(f"[govulncheck-filter] BLOCKING - not in {args.allowlist}: {', '.join(sorted(unwaived))}")
        return args.status

    print(f"[govulncheck-filter] All {len(found)} finding(s) are documented waivers - passing.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
