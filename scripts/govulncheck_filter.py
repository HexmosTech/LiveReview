#!/usr/bin/env python3
"""Waive specific, documented govulncheck findings without silencing the check.

govulncheck has no native ignore-list feature (unlike config/osv-scanner.toml,
which does). This script runs it, and if govulncheck's own exit status says
"vulnerabilities found" (status 3), it parses the human-readable output for
`Vulnerability #N: GO-XXXX-YYYY` lines and compares the set of reported IDs
against config/govulncheck-allowlist.txt. Any reported ID *not* in that file
still fails the build - this never turns the check off, it only lets specific,
reasoned, dated waivers through. Any other govulncheck exit status (a real
tool/compile error, or 0 for "nothing found") passes through unchanged.
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

    if args.status != 3:
        # 0 = nothing found, anything else = a real tool error (bad packages,
        # compile failure, network issue). Neither is ours to filter.
        return args.status

    found = set(VULN_RE.findall(log))
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
