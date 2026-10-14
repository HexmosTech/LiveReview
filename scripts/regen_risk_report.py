#!/usr/bin/env python3
"""Regenerate docs/risk-policy-verification-report.html from real test output.

The report's prose is hand-written and stable. Every *number* in it --
scenario scores, percentile ranks, threshold tiers, approval rates, learning-
cycle scores, weight deltas -- comes from one of three Go tests and is
embedded in the page as a JS data variable. This script is the only thing
that should ever update those variables: it never invents a number, it only
runs the tests and copies out what they actually printed.

Run this after changing:
  - internal/riskscore/model.go (weights, DefaultCategoryMultiplier)
  - internal/riskscore/features.go (FeatureSpecs)
  - internal/riskscore/referencemix (the reference distribution)
  - internal/policyengine/testdata/scenarios.json (the scenario corpus)
  - internal/riskscore/threshold_experiment_test.go
  - internal/riskscore/learning_cycle_test.go

Usage:
  DATABASE_URL=postgres://... python3 scripts/regen_risk_report.py
  (DATABASE_URL defaults to the same local-dev DSN internal/*_test.go's
  testDB(t) helpers fall back to.)
"""
import json
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
REPORT_PATH = REPO_ROOT / "docs" / "risk-policy-verification-report.html"


def run_go_test(pattern: str, pkg: str) -> str:
    cmd = ["go", "test", pkg, "-run", pattern, "-v", "-count=1"]
    print(f"$ {' '.join(cmd)}", file=sys.stderr)
    result = subprocess.run(cmd, cwd=REPO_ROOT, capture_output=True, text=True)
    print(result.stdout, file=sys.stderr)
    if result.returncode != 0:
        print(result.stderr, file=sys.stderr)
        raise SystemExit(
            f"go test {pattern} failed (exit {result.returncode}) -- "
            "fix the failing test before regenerating the report; "
            "the report must never embed a lie."
        )
    return result.stdout


def extract_prefixed_json(output: str, prefix: str) -> list:
    """Every line like 'prefix{...json...}' in `go test -v` output, parsed."""
    out = []
    for line in output.splitlines():
        idx = line.find(prefix)
        if idx == -1:
            continue
        raw = line[idx + len(prefix):]
        out.append(json.loads(raw))
    return out


def extract_one_prefixed_json(output: str, prefix: str) -> dict:
    rows = extract_prefixed_json(output, prefix)
    if len(rows) != 1:
        raise SystemExit(f"expected exactly one {prefix!r} line, found {len(rows)}")
    return rows[0]


def replace_var(html: str, var_name: str, value) -> str:
    """Replace `var NAME = <anything>;` with `var NAME = <value>;`.

    Matches non-greedily up to the first `;` that is immediately followed by
    whitespace-then-`var ` or end of script -- good enough because these
    variables are always simple JSON literals with no embedded `;`.
    """
    pattern = re.compile(r"var " + re.escape(var_name) + r" = .*?;\n", re.S)
    new_assignment = f"var {var_name} = {json.dumps(value, separators=(',', ':'))};\n"
    new_html, n = pattern.subn(new_assignment, html, count=1)
    if n != 1:
        raise SystemExit(f"could not find `var {var_name} = ...;` in the report to replace")
    return new_html


def main() -> None:
    scenario_output = run_go_test("TestScenarios", "./internal/policyengine/...")
    scenarios = extract_prefixed_json(scenario_output, "SCDATA:")
    if len(scenarios) < 1:
        raise SystemExit("no SCDATA: lines found -- did TestScenarios actually run?")

    threshold_output = run_go_test(
        "TestThresholdExperimentApprovalRatesByTier|TestThresholdExperimentSamePercentileDifferentCutoff",
        "./internal/riskscore/...",
    )
    threshold_data = extract_one_prefixed_json(threshold_output, "TDDATA:")
    same_percentile_data = extract_one_prefixed_json(threshold_output, "TDSAME:")

    learning_output = run_go_test("TestLearningCycleProvesFeedbackLoopWorks", "./internal/riskscore/...")
    learning_data = extract_one_prefixed_json(learning_output, "LCDATA:")

    html = REPORT_PATH.read_text()
    html = replace_var(html, "SCENARIOS", scenarios)
    html = replace_var(html, "THRESHOLD_DATA", threshold_data)
    html = replace_var(html, "SAME_PERCENTILE_DATA", same_percentile_data)
    html = replace_var(html, "LEARNING_DATA", learning_data)
    REPORT_PATH.write_text(html)

    print(f"updated {REPORT_PATH.relative_to(REPO_ROOT)} from {len(scenarios)} scenarios, "
          f"{len(threshold_data['tiers'])} threshold tiers, and the learning-cycle test.",
          file=sys.stderr)


if __name__ == "__main__":
    main()
