#!/usr/bin/env python3
"""Normalize OSV scanner JSON into CSV and Markdown triage artifacts.

Usage:
  python scripts/extract_osv_report.py \
    --input security_issues/osv-scanner-latest.json \
    --csv security_issues/osv-triage-latest.csv \
    --md security_issues/osv-triage-latest.md
"""

from __future__ import annotations

import argparse
import csv
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List


@dataclass
class VulnRow:
    go_id: str
    cve: str
    aliases: str
    summary: str
    package: str
    symbols: str
    introduced: str
    fixed: str
    called: str
    source_path: str
    disposition: str
    fix_track: str
    notes: str


def first_cve(aliases: List[str]) -> str:
    for alias in aliases:
        if alias.startswith("CVE-"):
            return alias
    return ""


def extract_ranges(vuln: dict) -> tuple[str, str]:
    introduced_values: List[str] = []
    fixed_values: List[str] = []
    for affected in vuln.get("affected", []):
        for rng in affected.get("ranges", []):
            for event in rng.get("events", []):
                if "introduced" in event:
                    introduced_values.append(event["introduced"])
                if "fixed" in event:
                    fixed_values.append(event["fixed"])
    return ", ".join(introduced_values), ", ".join(fixed_values)


def extract_symbols(vuln: dict) -> str:
    symbols: List[str] = []
    for affected in vuln.get("affected", []):
        eco = affected.get("ecosystem_specific", {})
        for imp in eco.get("imports", []):
            path = imp.get("path", "")
            syms = imp.get("symbols", [])
            if syms:
                symbols.append(f"{path}: {';'.join(syms)}")
            elif path:
                symbols.append(path)
    return " | ".join(symbols)


def build_called_map(groups: List[dict]) -> Dict[str, str]:
    called_map: Dict[str, str] = {}
    for group in groups:
        analyses = group.get("experimental_analysis", {})
        for go_id, details in analyses.items():
            called_map[go_id] = str(details.get("called", False)).lower()
    return called_map


def initial_disposition(called: str) -> tuple[str, str, str]:
    if called == "true":
        return (
            "actionable-now",
            "runtime-upgrade+targeted-hardening",
            "Reachable stdlib path reported by scanner; prioritize Go patch-level/major upgrade.",
        )
    return (
        "verify-reachability",
        "runtime-upgrade",
        "Scanner reports not-called; confirm with code-path review before closure.",
    )


def extract_rows(report: dict) -> List[VulnRow]:
    rows: List[VulnRow] = []
    for result in report.get("results", []):
        source_path = result.get("source", {}).get("path", "")
        for pkg in result.get("packages", []):
            groups = pkg.get("groups", [])
            called_map = build_called_map(groups)
            for vuln in pkg.get("vulnerabilities", []):
                go_id = vuln.get("id", "")
                aliases = vuln.get("aliases", [])
                cve = first_cve(aliases)
                summary = vuln.get("summary", "")
                introduced, fixed = extract_ranges(vuln)
                symbols = extract_symbols(vuln)

                package_name = ""
                for affected in vuln.get("affected", []):
                    package_name = affected.get("package", {}).get("name", "")
                    if package_name:
                        break

                called = called_map.get(go_id, "unknown")
                disposition, fix_track, notes = initial_disposition(called)

                rows.append(
                    VulnRow(
                        go_id=go_id,
                        cve=cve,
                        aliases=";".join(aliases),
                        summary=summary,
                        package=package_name,
                        symbols=symbols,
                        introduced=introduced,
                        fixed=fixed,
                        called=called,
                        source_path=source_path,
                        disposition=disposition,
                        fix_track=fix_track,
                        notes=notes,
                    )
                )
    rows.sort(key=lambda x: x.go_id)
    return rows


def write_csv(rows: List[VulnRow], csv_path: Path) -> None:
    csv_path.parent.mkdir(parents=True, exist_ok=True)
    with csv_path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow(
            [
                "go_id",
                "cve",
                "aliases",
                "summary",
                "package",
                "symbols",
                "introduced",
                "fixed",
                "called",
                "source_path",
                "disposition",
                "fix_track",
                "notes",
            ]
        )
        for row in rows:
            writer.writerow(
                [
                    row.go_id,
                    row.cve,
                    row.aliases,
                    row.summary,
                    row.package,
                    row.symbols,
                    row.introduced,
                    row.fixed,
                    row.called,
                    row.source_path,
                    row.disposition,
                    row.fix_track,
                    row.notes,
                ]
            )


def write_markdown(rows: List[VulnRow], md_path: Path, input_file: str) -> None:
    md_path.parent.mkdir(parents=True, exist_ok=True)
    called_true = sum(1 for r in rows if r.called == "true")
    called_false = sum(1 for r in rows if r.called == "false")
    called_unknown = sum(1 for r in rows if r.called not in ("true", "false"))

    lines = [
        "# OSV Triage Matrix",
        "",
        f"Source report: `{input_file}`",
        "",
        f"- Total vulnerabilities: {len(rows)}",
        f"- Called true: {called_true}",
        f"- Called false: {called_false}",
        f"- Called unknown: {called_unknown}",
        "",
        "## Initial Disposition Rules",
        "",
        "- `actionable-now`: scanner marked called=true; treat as reachable until disproven.",
        "- `verify-reachability`: scanner marked called=false; confirm by code-path review.",
        "",
        "## Vulnerability Table",
        "",
        "| GO ID | CVE | Called | Summary | Fixed Versions | Disposition | Fix Track |",
        "|---|---|---|---|---|---|---|",
    ]

    for row in rows:
        summary = row.summary.replace("|", "\\|")
        fixed = row.fixed.replace("|", "\\|")
        lines.append(
            f"| {row.go_id} | {row.cve or '-'} | {row.called} | {summary} | {fixed} | {row.disposition} | {row.fix_track} |"
        )

    md_path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract OSV report into CSV/Markdown triage")
    parser.add_argument("--input", required=True, help="Path to OSV scanner JSON report")
    parser.add_argument("--csv", required=True, help="Output CSV path")
    parser.add_argument("--md", required=True, help="Output Markdown path")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    input_path = Path(args.input)
    report = json.loads(input_path.read_text(encoding="utf-8"))
    rows = extract_rows(report)
    write_csv(rows, Path(args.csv))
    write_markdown(rows, Path(args.md), args.input)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
