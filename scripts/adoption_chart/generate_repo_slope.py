#!/usr/bin/env python3
"""Row 6: "Which repositories are gaining or losing engineering velocity?"
Slope graph. X: period (previous, current). Y: LOC reviewed. One line per
repo, colored by gain/loss.
Usage: uv run generate_repo_slope.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "repo_slope.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)
    half = args.days // 2

    sql = f"""
    SELECT r.repository,
           CASE WHEN l.accounted_at >= CURRENT_DATE - INTERVAL '{half} days' THEN 'Current' ELSE 'Previous' END AS period,
           sum(l.billable_loc) AS loc
    FROM loc_usage_ledger l
    JOIN reviews r ON r.id = l.review_id
    WHERE l.org_id = {org_id} AND l.status = 'accounted'
      AND l.accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1, 2
    ORDER BY 1, 2;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    by_repo: dict[str, dict[str, int]] = {}
    for r in rows:
        by_repo.setdefault(r["repository"], {})[r["period"]] = int(float(r["loc"]))

    data = []
    for repo, periods in by_repo.items():
        prev, cur = periods.get("Previous", 0), periods.get("Current", 0)
        trend = "gain" if cur > prev else ("loss" if cur < prev else "flat")
        data.append({"repository": repo, "period": "Previous", "loc": prev, "trend": trend})
        data.append({"repository": repo, "period": "Current", "loc": cur, "trend": trend})

    gains = sum(1 for d in by_repo.values() if d.get("Current", 0) > d.get("Previous", 0))
    losses = sum(1 for d in by_repo.values() if d.get("Current", 0) < d.get("Previous", 0))

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Repository velocity — {args.org}, last {args.days} days ({half}/{half} split)",
        "width": 500, "height": 380,
        "data": {"values": data},
        "mark": {"type": "line", "point": True, "strokeWidth": 2.5},
        "encoding": {
            "x": {"field": "period", "type": "nominal", "title": None, "sort": ["Previous", "Current"]},
            "y": {"field": "loc", "type": "quantitative", "title": "LOC reviewed"},
            "color": {"field": "trend", "type": "nominal", "title": "Trend",
                      "scale": {"domain": ["gain", "flat", "loss"], "range": ["#39d353", "#8b949e", "#ff5c7c"]}},
            "detail": {"field": "repository", "type": "nominal"},
            "tooltip": [{"field": "repository", "type": "nominal"}, {"field": "period", "type": "nominal"},
                        {"field": "loc", "type": "quantitative", "title": "LOC"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "legend": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5"},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Repo Velocity — {args.org}", spec=spec, view_max_width=560,
        stats_html=f"<b>{gains}</b> repos gained velocity, <b>{losses}</b> lost velocity, out of <b>{len(by_repo)}</b> tracked.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
