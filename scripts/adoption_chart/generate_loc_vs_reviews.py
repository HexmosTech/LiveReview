#!/usr/bin/env python3
"""Row 23: "How much engineering work is being covered by LR?" Dual layered
line. X: day. Y (two lines, own scales): LOC reviewed and review count.
Distinguishes "more reviews" from "genuinely more code inspected."
Usage: uv run generate_loc_vs_reviews.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "loc_vs_reviews.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    WITH days AS (
      SELECT generate_series((CURRENT_DATE - INTERVAL '{args.days} days')::date, CURRENT_DATE::date, '1 day')::date AS day
    ),
    reviews_d AS (
      SELECT date_trunc('day', COALESCE(completed_at, created_at))::date AS day, count(*) AS n
      FROM reviews WHERE org_id = {org_id}
        AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
      GROUP BY 1
    ),
    loc_d AS (
      SELECT accounted_at::date AS day, sum(billable_loc) AS loc
      FROM loc_usage_ledger WHERE org_id = {org_id} AND status = 'accounted'
        AND accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
      GROUP BY 1
    )
    SELECT d.day, COALESCE(reviews_d.n, 0) AS reviews, COALESCE(loc_d.loc, 0) AS loc
    FROM days d
    LEFT JOIN reviews_d ON reviews_d.day = d.day
    LEFT JOIN loc_d ON loc_d.day = d.day
    ORDER BY d.day;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"day": r["day"], "reviews": int(r["reviews"]), "loc": int(float(r["loc"]))} for r in rows]
    total_reviews = sum(d["reviews"] for d in data)
    total_loc = sum(d["loc"] for d in data)
    avg_loc_per_review = total_loc / total_reviews if total_reviews else 0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"LOC vs review count — {args.org}, last {args.days} days",
        "width": 800, "height": 340,
        "data": {"values": data},
        "layer": [
            {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2},
             "encoding": {"x": {"field": "day", "type": "temporal", "title": "Day"},
                          "y": {"field": "loc", "type": "quantitative", "title": "LOC reviewed", "axis": {"titleColor": "#ffb454"}},
                          "tooltip": [{"field": "day", "type": "temporal"}, {"field": "loc", "type": "quantitative", "title": "LOC"}]}},
            {"mark": {"type": "line", "color": "#7c9cff", "strokeWidth": 2},
             "encoding": {"x": {"field": "day", "type": "temporal"},
                          "y": {"field": "reviews", "type": "quantitative", "title": "Reviews", "axis": {"titleColor": "#7c9cff", "orient": "right"}},
                          "tooltip": [{"field": "day", "type": "temporal"}, {"field": "reviews", "type": "quantitative"}]}},
        ],
        "resolve": {"scale": {"y": "independent"}},
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"LOC vs Reviews — {args.org}", spec=spec, view_max_width=840,
        stats_html=f"Average <b>{avg_loc_per_review:.1f} LOC</b> per review across <b>{total_reviews}</b> reviews / <b>{total_loc}</b> LOC total.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
