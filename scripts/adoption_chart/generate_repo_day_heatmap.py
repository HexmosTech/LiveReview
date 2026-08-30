#!/usr/bin/env python3
"""Row 9: "What does engineering activity look like across repositories and
days?" Repository x day heatmap. X: day. Y: repository. Color: LOC reviewed.
Usage: uv run generate_repo_day_heatmap.py [--org hexmos-internal] [--days 60]
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--days", type=int, default=60)
    p.add_argument("--out", default=str(SCRIPT_DIR / "repo_day_heatmap.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT r.repository, l.accounted_at::date AS day, sum(l.billable_loc) AS loc
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

    data = [{"repository": r["repository"], "day": r["day"], "loc": int(float(r["loc"]))} for r in rows]
    repos = sorted({d["repository"] for d in data})
    busiest_day = max(data, key=lambda d: d["loc"])

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Activity by repository and day — {args.org}, last {args.days} days",
        "width": {"step": 14}, "height": {"step": 26},
        "data": {"values": data},
        "mark": {"type": "rect"},
        "encoding": {
            "x": {"field": "day", "type": "temporal", "title": "Day", "axis": {"format": "%b %d", "labelAngle": -40}},
            "y": {"field": "repository", "type": "nominal", "title": None, "sort": repos},
            "color": {"field": "loc", "type": "quantitative", "title": "LOC",
                      "scale": {"scheme": "blues"}, "legend": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5"}},
            "tooltip": [{"field": "repository", "type": "nominal"}, {"field": "day", "type": "temporal", "title": "Day"},
                        {"field": "loc", "type": "quantitative", "title": "LOC"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Repo x Day Activity — {args.org}", spec=spec, view_max_width=900,
        stats_html=f"Busiest cell: <b>{busiest_day['repository']}</b> on <b>{busiest_day['day']}</b> ({busiest_day['loc']} LOC). {len(repos)} repositories shown.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
