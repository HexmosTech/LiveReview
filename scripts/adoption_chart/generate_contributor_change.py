#!/usr/bin/env python3
"""Row 11: "Why did this repository's velocity change?" Small-multiple
contribution chart. X: contributor. Y: change in LOC. Color: positive/negative.
Compares the most recent half of --days against the prior half, per engineer,
for one repository.
Usage: uv run generate_contributor_change.py [--org hexmos-internal] [--repo NAME] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "contributor_change.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)
    half = args.days // 2

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
    SELECT r.author_username,
           CASE WHEN l.accounted_at >= CURRENT_DATE - INTERVAL '{half} days' THEN 'current' ELSE 'previous' END AS period,
           sum(l.billable_loc) AS loc
    FROM loc_usage_ledger l JOIN reviews r ON r.id = l.review_id
    WHERE l.org_id = {org_id} AND l.status = 'accounted' AND r.repository = '{repo}'
      AND r.author_username IS NOT NULL
      AND l.accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1, 2;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    by_eng: dict[str, dict[str, int]] = {}
    for r in rows:
        by_eng.setdefault(r["author_username"], {})[r["period"]] = int(float(r["loc"]))

    data = []
    for eng, periods in by_eng.items():
        prev, cur = periods.get("previous", 0), periods.get("current", 0)
        delta = cur - prev
        pct = (delta / prev * 100) if prev else (100.0 if cur else 0.0)
        data.append({"engineer": eng, "delta": delta, "pct": round(pct, 0), "direction": "up" if delta >= 0 else "down"})
    data.sort(key=lambda d: d["delta"])

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"{repo}: LOC change per contributor — last {args.days} days ({half}/{half} split)",
        "width": 600, "height": max(200, 30 * len(data)),
        "data": {"values": data},
        "mark": {"type": "bar"},
        "encoding": {
            "y": {"field": "engineer", "type": "nominal", "title": None, "sort": "x"},
            "x": {"field": "delta", "type": "quantitative", "title": "Change in LOC reviewed"},
            "color": {"field": "direction", "type": "nominal", "legend": None,
                      "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]}},
            "tooltip": [{"field": "engineer", "type": "nominal"}, {"field": "delta", "type": "quantitative", "title": "Delta LOC"},
                        {"field": "pct", "type": "quantitative", "title": "% change"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    summary = "; ".join(f"{d['engineer']} {d['pct']:+.0f}%" for d in data)
    wrap_page(
        title=f"{repo} Contributor Change", spec=spec, view_max_width=640,
        stats_html=f"{summary}",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
