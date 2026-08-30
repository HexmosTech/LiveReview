#!/usr/bin/env python3
"""Row 8: "Which repositories are unusually active or inactive?" Scatterplot.
X: LOC reviewed. Y: reviews. Size: active engineers. Color: repository.
Usage: uv run generate_repo_scatter.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "repo_scatter.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT r.repository,
           count(*) AS reviews,
           count(DISTINCT r.author_username) AS engineers,
           coalesce(sum(l.billable_loc), 0) AS loc
    FROM reviews r
    LEFT JOIN loc_usage_ledger l ON l.review_id = r.id AND l.status = 'accounted'
    WHERE r.org_id = {org_id}
      AND COALESCE(r.completed_at, r.created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1
    ORDER BY reviews DESC;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"repository": r["repository"], "reviews": int(r["reviews"]),
              "engineers": int(r["engineers"]), "loc": int(float(r["loc"]))} for r in rows]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Repository activity — {args.org}, last {args.days} days",
        "width": 600, "height": 380,
        "data": {"values": data},
        "mark": {"type": "circle", "opacity": 0.85},
        "encoding": {
            "x": {"field": "loc", "type": "quantitative", "title": "LOC reviewed"},
            "y": {"field": "reviews", "type": "quantitative", "title": "Reviews"},
            "size": {"field": "engineers", "type": "quantitative", "title": "Active engineers", "scale": {"range": [80, 1200]}},
            "color": {"field": "repository", "type": "nominal", "legend": None},
            "tooltip": [{"field": "repository", "type": "nominal"}, {"field": "loc", "type": "quantitative", "title": "LOC"},
                        {"field": "reviews", "type": "quantitative"}, {"field": "engineers", "type": "quantitative", "title": "Engineers"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    biggest = max(data, key=lambda d: d["loc"])
    wrap_page(
        title=f"Repo Activity — {args.org}", spec=spec, view_max_width=640,
        stats_html=f"<b>{len(data)}</b> repositories active. Largest by LOC: <b>{biggest['repository']}</b> ({biggest['loc']} LOC, {biggest['reviews']} reviews).",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
