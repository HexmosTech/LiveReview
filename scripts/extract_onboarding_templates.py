#!/usr/bin/env python3
"""
One-time extraction script: reads a Livi chat debug export JSON and produces
a parameterized onboarding report template catalog (templates.json).

Usage:
    python3 scripts/extract_onboarding_templates.py ~/Downloads/livereview-debug-export-27.json \
        > internal/onboardingreport/templates.json
"""

import json
import re
import sys
import hashlib

# Section mapping: question_seq -> section
# Based on the 32 questions in the export, grouped thematically.
SECTION_MAP = {
    1:  "adoption",     # Is LiveReview adoption increasing?
    3:  "adoption",     # Are engineers incorporating reviews into daily workflow?
    5:  "adoption",     # How broadly has the org adopted LiveReview?
    7:  "adoption",     # Who has adopted - and who hasn't?
    9:  "adoption",     # Who has adopted - and who hasn't? (retry)
    11: "adoption",     # Is adoption becoming broader over time?
    13: "repos",        # Which repos gaining/losing velocity?
    15: "repos",        # Where is org velocity concentrated?
    17: "repos",        # Engineering activity across repos and days
    19: "repos",        # Engineering activity across repos and days (retry)
    21: "repos",        # What happened to a repo's velocity?
    23: "engineers",    # Which engineers carrying git-lrc?
    25: "engineers",    # What does each engineer spend review activity on?
    27: "quality",      # Where are reviews happening?
    29: "quality",      # Moving review earlier in lifecycle?
    31: "quality",      # Serious issues caught before PR/MR?
    33: "quality",      # What kinds of problems is LR finding?
    35: "quality",      # Where are issues concentrated?
    37: "quality",      # Blast radius of issues?
    39: "cost",         # How much does LR save vs alternatives?
    41: "cost",         # How much code has LR reviewed?
    43: "cost",         # Are reviews getting faster?
    45: "cost",         # How much engineering work covered by LR?
    47: "engagement",   # Are reviews becoming more iterative?
    49: "engagement",   # Are reviews becoming more iterative? (retry)
    51: "engagement",   # Which engineers getting most value from LR?
    53: "engagement",   # Are people trusting the reviews?
    55: "engagement",   # Which repos have highest review coverage?
    57: "summary",      # What does a healthy workflow look like?
    59: "summary",      # What does a healthy workflow look like? (retry)
    61: "summary",      # What does a healthy workflow look like? (retry)
    63: "summary",      # What changed between week 1 and week 2?
}

SECTION_ORDER = ["adoption", "repos", "engineers", "quality", "cost", "engagement", "summary"]

SECTION_LABELS = {
    "adoption":    "Adoption & Growth",
    "repos":       "Repository Analysis",
    "engineers":   "Engineer Analysis",
    "quality":     "Review Quality & Findings",
    "cost":        "Cost & Efficiency",
    "engagement":  "Engagement & Trust",
    "summary":     "Summary & Comparison",
}


def parameterize_sql(sql: str) -> str:
    """Replace hardcoded org_id = 151 with {{.OrgID}} template placeholder."""
    # Match org_id = 151 in various contexts (WHERE, AND, JOIN ON, etc.)
    result = re.sub(r'\borg_id\s*=\s*151\b', 'org_id = {{.OrgID}}', sql)
    return result


def make_id(title: str) -> str:
    """Generate a stable ID from a chart title."""
    slug = re.sub(r'[^a-z0-9]+', '-', title.lower()).strip('-')
    return slug


def deduplicate_charts(charts: list) -> list:
    """Remove near-duplicate charts, keeping the most detailed version."""
    seen = {}
    for chart in charts:
        key = chart["title"]
        if key in seen:
            # Keep the one with longer SQL (more detailed)
            if len(chart["sql"]) > len(seen[key]["sql"]):
                seen[key] = chart
        else:
            seen[key] = chart
    return list(seen.values())


def main():
    if len(sys.argv) < 2:
        print("Usage: extract_onboarding_templates.py <export.json>", file=sys.stderr)
        sys.exit(1)

    with open(sys.argv[1]) as f:
        export = json.load(f)

    # Build question_seq -> section mapping from user turns
    question_map = {}
    for turn in export["turns"]:
        if turn["role"] == "user":
            question_map[turn["seq"]] = turn.get("text", "")

    # Extract all interpretations from assistant turns
    all_charts = []
    for turn in export["turns"]:
        if turn["role"] != "assistant":
            continue
        if not turn.get("debug_artifacts"):
            continue
        if not turn["debug_artifacts"].get("interpretations"):
            continue

        # The user question seq is turn["seq"] - 1
        question_seq = turn["seq"] - 1
        section = SECTION_MAP.get(question_seq, "summary")

        for interp in turn["debug_artifacts"]["interpretations"]:
            sql = parameterize_sql(interp["sql"])
            chart_id = make_id(interp["title"])

            chart = {
                "id": chart_id,
                "section": section,
                "section_label": SECTION_LABELS[section],
                "title": interp["title"],
                "description": interp.get("description", ""),
                "query_summary": interp.get("query_summary", ""),
                "sql": sql,
                "vega_spec": interp["vega_lite_spec"],
                "chart_type": interp.get("chart_type", "line"),
                "granularity": interp.get("granularity", "Daily"),
                "time_range": interp.get("time_range", "All time"),
            }
            all_charts.append(chart)

    # Deduplicate
    unique_charts = deduplicate_charts(all_charts)

    # Assign sort order within each section
    section_counters = {s: 0 for s in SECTION_ORDER}
    for chart in unique_charts:
        s = chart["section"]
        chart["sort_order"] = section_counters[s]
        section_counters[s] += 1

    # Sort by section order, then sort_order
    def sort_key(c):
        return (SECTION_ORDER.index(c["section"]), c["sort_order"])

    unique_charts.sort(key=sort_key)

    # Build final output
    output = {
        "version": "1.0.0",
        "generated_from": "livereview-debug-export-27.json",
        "total_charts": len(unique_charts),
        "sections": [
            {"id": s, "label": SECTION_LABELS[s]}
            for s in SECTION_ORDER
        ],
        "charts": unique_charts,
    }

    json.dump(output, sys.stdout, indent=2)
    print()  # trailing newline

    # Print summary to stderr
    print(f"\nExtracted {len(all_charts)} charts, deduplicated to {len(unique_charts)}", file=sys.stderr)
    for s in SECTION_ORDER:
        count = sum(1 for c in unique_charts if c["section"] == s)
        print(f"  {SECTION_LABELS[s]}: {count} charts", file=sys.stderr)


if __name__ == "__main__":
    main()
