#!/usr/bin/env python3
"""Row 7: "Where is organizational velocity concentrated?" Pareto /
cumulative distribution. X: repos ranked by LOC. Y: LOC reviewed, layered
with a cumulative % line.
Usage: uv run generate_repo_pareto.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "repo_pareto.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT r.repository, sum(l.billable_loc) AS loc
    FROM loc_usage_ledger l
    JOIN reviews r ON r.id = l.review_id
    WHERE l.org_id = {org_id} AND l.status = 'accounted'
      AND l.accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1
    ORDER BY 2 DESC;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    total = sum(int(float(r["loc"])) for r in rows)
    data, running = [], 0
    for r in rows:
        loc = int(float(r["loc"]))
        running += loc
        data.append({"repository": r["repository"], "loc": loc, "cum_pct": round(running / total * 100, 1)})

    top3_pct = round(sum(int(float(r["loc"])) for r in rows[:3]) / total * 100) if total else 0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Where velocity concentrates — {args.org}, last {args.days} days",
        "width": 600, "height": 360,
        "data": {"values": data},
        "layer": [
            {"mark": {"type": "bar", "color": "#7c9cff"},
             "encoding": {"x": {"field": "repository", "type": "nominal", "sort": "-y", "title": "Repository"},
                          "y": {"field": "loc", "type": "quantitative", "title": "LOC reviewed"},
                          "tooltip": [{"field": "repository", "type": "nominal"}, {"field": "loc", "type": "quantitative"}]}},
            {"mark": {"type": "line", "point": True, "color": "#ff5c7c", "strokeWidth": 2},
             "encoding": {"x": {"field": "repository", "type": "nominal", "sort": "-y"},
                          "y": {"field": "cum_pct", "type": "quantitative", "title": "Cumulative %", "axis": {"orient": "right"}},
                          "tooltip": [{"field": "repository", "type": "nominal"}, {"field": "cum_pct", "type": "quantitative", "title": "Cumulative %"}]}},
        ],
        "resolve": {"scale": {"y": "independent"}},
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358", "labelAngle": -30},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Velocity Concentration — {args.org}", spec=spec, view_max_width=640,
        stats_html=f"Top 3 repositories account for <b>{top3_pct}%</b> of all LOC reviewed (<b>{len(data)}</b> repos total).",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
