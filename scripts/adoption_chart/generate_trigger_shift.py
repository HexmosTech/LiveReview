#!/usr/bin/env python3
"""Row 15: "Are we moving review earlier in the development lifecycle?"
100% stacked area. X: day/week. Y: % of reviews. Color: trigger source.
Same underlying data as generate_trigger_share.py (row 14) but an area mark
over a finer time bucket, since the strategic question here is the *shape*
of the transition between workflows over time, not a single period's split.
Usage: uv run generate_trigger_shift.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "trigger_shift.html"))
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
    weeks = sorted({d["week"] for d in data})
    def precommit_share(week):
        total = sum(d["n"] for d in data if d["week"] == week)
        pre = sum(d["n"] for d in data if d["week"] == week and d["trigger_type"] == "cli_diff")
        return (pre / total * 100) if total else 0
    first_share, last_share = precommit_share(weeks[0]), precommit_share(weeks[-1])

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Review-trigger mix over time — {args.org}, last {args.days} days",
        "width": 700, "height": 340,
        "data": {"values": data},
        "mark": {"type": "area", "interpolate": "monotone"},
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
        title=f"Trigger Shift — {args.org}", spec=spec, view_max_width=740,
        stats_html=f"Pre-commit (cli_diff) share went from <b>{first_share:.0f}%</b> to <b>{last_share:.0f}%</b> across the window.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
