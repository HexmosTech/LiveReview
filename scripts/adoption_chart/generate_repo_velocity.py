#!/usr/bin/env python3
"""Row 10: "What happened to a repository's velocity?" Layered line + rolling
average + highlighted interval. X: day. Y: LOC reviewed. Line = daily LOC;
second line = 7-day rolling avg; rectangle = most recent 14 days.
Usage: uv run generate_repo_velocity.py [--org hexmos-internal] [--repo NAME] [--days 90]
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--repo", default=None, help="repository name (default: the busiest one)")
    p.add_argument("--days", type=int, default=90)
    p.add_argument("--out", default=str(SCRIPT_DIR / "repo_velocity.html"))
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
    WITH days AS (
      SELECT generate_series((CURRENT_DATE - INTERVAL '{args.days} days')::date, CURRENT_DATE::date, '1 day')::date AS day
    ),
    daily AS (
      SELECT l.accounted_at::date AS day, sum(l.billable_loc) AS loc
      FROM loc_usage_ledger l JOIN reviews r ON r.id = l.review_id
      WHERE l.org_id = {org_id} AND l.status = 'accounted' AND r.repository = '{repo}'
        AND l.accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
      GROUP BY 1
    ),
    filled AS (
      SELECT d.day, COALESCE(daily.loc, 0) AS loc FROM days d LEFT JOIN daily ON daily.day = d.day
    )
    SELECT day, loc, round(avg(loc) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW), 1) AS rolling_avg
    FROM filled ORDER BY day;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"day": r["day"], "loc": int(float(r["loc"])), "rolling_avg": float(r["rolling_avg"])} for r in rows]
    highlight_start = data[-14]["day"] if len(data) >= 14 else data[0]["day"]
    highlight_end = data[-1]["day"]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"{repo} velocity — last {args.days} days",
        "width": 800, "height": 340,
        "layer": [
            {"data": {"values": [{"start": highlight_start, "end": highlight_end}]},
             "mark": {"type": "rect", "color": "#7c9cff", "opacity": 0.12},
             "encoding": {"x": {"field": "start", "type": "temporal"}, "x2": {"field": "end"}}},
            {"data": {"values": data}, "mark": {"type": "line", "color": "#3a4358", "strokeWidth": 1},
             "encoding": {"x": {"field": "day", "type": "temporal", "title": "Day"},
                          "y": {"field": "loc", "type": "quantitative", "title": "LOC reviewed"},
                          "tooltip": [{"field": "day", "type": "temporal"}, {"field": "loc", "type": "quantitative", "title": "LOC"}]}},
            {"data": {"values": data}, "mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5},
             "encoding": {"x": {"field": "day", "type": "temporal"},
                          "y": {"field": "rolling_avg", "type": "quantitative"},
                          "tooltip": [{"field": "day", "type": "temporal"}, {"field": "rolling_avg", "type": "quantitative", "title": "7-day avg", "format": ".1f"}]}},
        ],
        "resolve": {"scale": {"y": "shared"}},
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    last14_avg = sum(d["loc"] for d in data[-14:]) / min(14, len(data))
    prior_avg = sum(d["loc"] for d in data[-28:-14]) / max(min(14, len(data) - 14), 1)
    wrap_page(
        title=f"{repo} Velocity", spec=spec, view_max_width=840,
        stats_html=f"Highlighted: last 14 days, avg <b>{last14_avg:.1f}</b> LOC/day vs prior 14 days' <b>{prior_avg:.1f}</b> LOC/day.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
