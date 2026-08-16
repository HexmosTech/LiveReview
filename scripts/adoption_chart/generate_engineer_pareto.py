#!/usr/bin/env python3
"""Row 29: "How much of the organization's activity is covered by the top
users?" Cumulative area / Pareto. X: engineers ranked by activity. Y:
cumulative % of reviews. Same shape as generate_repo_pareto.py (row 7) but
for engineers instead of repositories - makes "adoption is broad" vs "three
people are doing everything" immediately apparent.
Usage: uv run generate_engineer_pareto.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "engineer_pareto.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT author_username, count(*) AS reviews
    FROM reviews
    WHERE org_id = {org_id} AND author_username IS NOT NULL
      AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1
    ORDER BY 2 DESC;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    total = sum(int(r["reviews"]) for r in rows)
    data, running = [], 0
    for r in rows:
        n = int(r["reviews"])
        running += n
        data.append({"engineer": r["author_username"], "reviews": n, "cum_pct": round(running / total * 100, 1)})

    n_for_80 = next((i + 1 for i, d in enumerate(data) if d["cum_pct"] >= 80), len(data))

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Adoption concentration — {args.org}, last {args.days} days",
        "width": 600, "height": 360,
        "data": {"values": data},
        "layer": [
            {"mark": {"type": "bar", "color": "#7c9cff"},
             "encoding": {"x": {"field": "engineer", "type": "nominal", "sort": "-y", "title": "Engineer"},
                          "y": {"field": "reviews", "type": "quantitative", "title": "Reviews"},
                          "tooltip": [{"field": "engineer", "type": "nominal"}, {"field": "reviews", "type": "quantitative"}]}},
            {"mark": {"type": "line", "point": True, "color": "#ff5c7c", "strokeWidth": 2},
             "encoding": {"x": {"field": "engineer", "type": "nominal", "sort": "-y"},
                          "y": {"field": "cum_pct", "type": "quantitative", "title": "Cumulative %", "axis": {"orient": "right"}},
                          "tooltip": [{"field": "engineer", "type": "nominal"}, {"field": "cum_pct", "type": "quantitative", "title": "Cumulative %"}]}},
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
        title=f"Engineer Pareto — {args.org}", spec=spec, view_max_width=640,
        stats_html=f"<b>{n_for_80}</b> of <b>{len(data)}</b> engineers account for 80% of all review activity.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
