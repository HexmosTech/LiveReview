#!/usr/bin/env python3
"""Row 12: "Which engineers are carrying the repository?" Beeswarm / dot
plot. X: LOC reviewed. Y: contributor (jittered). Point size: reviews.
Usage: uv run generate_contributor_beeswarm.py [--org hexmos-internal] [--repo NAME] [--days 90]
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--repo", default=None)
    p.add_argument("--days", type=int, default=90)
    p.add_argument("--out", default=str(SCRIPT_DIR / "contributor_beeswarm.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    repo = args.repo
    if not repo:
        top = run_query(db, f"""
            SELECT r.repository, sum(l.billable_loc) AS loc
            FROM loc_usage_ledger l JOIN reviews r ON r.id = l.review_id
            WHERE l.org_id = {org_id} AND l.status = 'accounted'
              AND l.accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
            GROUP BY 1 ORDER BY 2 DESC LIMIT 1;
        """)
        if not top:
            sys.exit("no repository activity found")
        repo = top[0]["repository"]

    sql = f"""
    SELECT r.author_username, count(*) AS reviews, sum(l.billable_loc) AS loc
    FROM loc_usage_ledger l JOIN reviews r ON r.id = l.review_id
    WHERE l.org_id = {org_id} AND l.status = 'accounted' AND r.repository = '{repo}'
      AND r.author_username IS NOT NULL
      AND l.accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1
    ORDER BY 3 DESC;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"engineer": r["author_username"], "reviews": int(r["reviews"]), "loc": int(float(r["loc"]))} for r in rows]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"{repo}: who's carrying it — last {args.days} days",
        "width": 600, "height": max(200, 32 * len(data)),
        "data": {"values": data},
        "transform": [{"calculate": "random()", "as": "jitter"}],
        "mark": {"type": "circle", "opacity": 0.85},
        "encoding": {
            "x": {"field": "loc", "type": "quantitative", "title": "LOC reviewed"},
            "y": {"field": "engineer", "type": "nominal", "title": None, "sort": "-x"},
            "yOffset": {"field": "jitter", "type": "quantitative"},
            "size": {"field": "reviews", "type": "quantitative", "title": "Reviews", "scale": {"range": [60, 900]}},
            "color": {"field": "loc", "type": "quantitative", "scale": {"scheme": "blues"}, "legend": None},
            "tooltip": [{"field": "engineer", "type": "nominal"}, {"field": "loc", "type": "quantitative", "title": "LOC"},
                        {"field": "reviews", "type": "quantitative"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    top = data[0]
    total = sum(d["loc"] for d in data)
    wrap_page(
        title=f"{repo} Beeswarm", spec=spec, view_max_width=640,
        stats_html=f"<b>{top['engineer']}</b> carries the most at <b>{top['loc']}</b> LOC ({top['loc']/total*100:.0f}% of the repo's total).",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
