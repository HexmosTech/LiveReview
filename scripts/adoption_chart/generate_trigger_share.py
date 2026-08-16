#!/usr/bin/env python3
"""Row 14: "Where are reviews happening?" Normalized (100%) stacked bar.
X: time period (week). Y: % of reviews. Color: trigger_type.
Usage: uv run generate_trigger_share.py [--org hexmos-internal] [--days 90]
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--days", type=int, default=90)
    p.add_argument("--out", default=str(SCRIPT_DIR / "trigger_share.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT date_trunc('week', COALESCE(completed_at, created_at))::date AS week,
           trigger_type, count(*) AS n
    FROM reviews
    WHERE org_id = {org_id}
      AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1, 2
    ORDER BY 1;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"week": r["week"], "trigger_type": r["trigger_type"], "n": int(r["n"])} for r in rows]
    totals: dict[str, int] = {}
    for r in rows:
        totals[r["trigger_type"]] = totals.get(r["trigger_type"], 0) + int(r["n"])
    grand_total = sum(totals.values())
    precommit_pct = round(totals.get("cli_diff", 0) / grand_total * 100) if grand_total else 0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Where reviews are triggered — {args.org}, last {args.days} days",
        "width": 700, "height": 340,
        "data": {"values": data},
        "mark": {"type": "bar"},
        "encoding": {
            "x": {"field": "week", "type": "temporal", "title": "Week"},
            "y": {"field": "n", "type": "quantitative", "title": "% of reviews", "stack": "normalize", "axis": {"format": "%"}},
            "color": {"field": "trigger_type", "type": "nominal", "title": "Trigger",
                      "legend": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5"}},
            "tooltip": [{"field": "week", "type": "temporal", "title": "Week"}, {"field": "trigger_type", "type": "nominal"},
                        {"field": "n", "type": "quantitative", "title": "Reviews"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Trigger Share — {args.org}", spec=spec, view_max_width=740,
        stats_html=f"<b>cli_diff</b> (pre-commit) accounts for <b>{precommit_pct}%</b> of all {grand_total} reviews in this window.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
