#!/usr/bin/env python3
"""Row 13: "What does each engineer actually spend their review activity
on?" Trellis/faceted stacked bars. X: engineer. Y: reviews. Color: repository.
Usage: uv run generate_engineer_trellis.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "engineer_trellis.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT author_username, repository, count(*) AS reviews
    FROM reviews
    WHERE org_id = {org_id} AND author_username IS NOT NULL
      AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1, 2
    ORDER BY 1, 3 DESC;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"engineer": r["author_username"], "repository": r["repository"], "reviews": int(r["reviews"])} for r in rows]

    by_eng: dict[str, set] = {}
    for d in data:
        by_eng.setdefault(d["engineer"], set()).add(d["repository"])
    most_spread = max(by_eng.items(), key=lambda kv: len(kv[1]))
    most_focused = min(by_eng.items(), key=lambda kv: len(kv[1]))

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Review activity by engineer and repository — {args.org}, last {args.days} days",
        "width": 700, "height": 340,
        "data": {"values": data},
        "mark": {"type": "bar"},
        "encoding": {
            "x": {"field": "engineer", "type": "nominal", "title": None, "sort": "-y"},
            "y": {"field": "reviews", "type": "quantitative", "title": "Reviews", "stack": True},
            "color": {"field": "repository", "type": "nominal", "title": "Repository",
                      "legend": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5"}},
            "tooltip": [{"field": "engineer", "type": "nominal"}, {"field": "repository", "type": "nominal"},
                        {"field": "reviews", "type": "quantitative"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Engineer x Repo — {args.org}", spec=spec, view_max_width=740,
        stats_html=f"Most spread across repos: <b>{most_spread[0]}</b> ({len(most_spread[1])} repos). "
                    f"Most focused: <b>{most_focused[0]}</b> ({len(most_focused[1])} repo(s)).",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
