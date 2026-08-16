#!/usr/bin/env python3
"""Row 24: "Are reviews becoming more iterative?" Histogram. X: reviews per
commit (bucketed). Y: count of commits. A long right tail means some commits
are being reviewed repeatedly.
Usage: uv run generate_reviews_per_commit.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "reviews_per_commit.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT reviews_per_commit, count(*) AS commits
    FROM (
      SELECT commit_hash, count(*) AS reviews_per_commit
      FROM reviews
      WHERE org_id = {org_id} AND commit_hash IS NOT NULL
        AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
      GROUP BY 1
    ) t
    GROUP BY 1
    ORDER BY 1;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"reviews_per_commit": int(r["reviews_per_commit"]), "commits": int(r["commits"])} for r in rows]
    total_commits = sum(d["commits"] for d in data)
    repeated = sum(d["commits"] for d in data if d["reviews_per_commit"] > 1)
    repeated_pct = round(repeated / total_commits * 100) if total_commits else 0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Reviews per commit — {args.org}, last {args.days} days",
        "width": 600, "height": 340,
        "data": {"values": data},
        "mark": {"type": "bar", "color": "#7c9cff"},
        "encoding": {
            "x": {"field": "reviews_per_commit", "type": "ordinal", "title": "Reviews per commit"},
            "y": {"field": "commits", "type": "quantitative", "title": "Count of commits"},
            "tooltip": [{"field": "reviews_per_commit", "type": "ordinal", "title": "Reviews"}, {"field": "commits", "type": "quantitative"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Reviews per Commit — {args.org}", spec=spec, view_max_width=640,
        stats_html=f"<b>{repeated_pct}%</b> of <b>{total_commits}</b> commits were reviewed more than once.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
