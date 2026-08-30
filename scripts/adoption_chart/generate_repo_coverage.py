#!/usr/bin/env python3
"""Row 27: "Which repositories have the highest review coverage?" Bubble
chart. X: reviews / PRs (coverage ratio). Y: LOC reviewed. Size: engineers.
Color: coverage tier.

NOTE ON DATA: the original idea's Y axis is "LOC reviewed / LOC changed" -
this schema doesn't track total changed LOC per PR (that's git-host diff
stats, not something reviews/pull_requests stores), so Y here is LOC
reviewed alone rather than a true reviewed/changed ratio. X (reviews/PRs) is
fully real.

Usage: uv run generate_repo_coverage.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "repo_coverage.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT rp.name AS repository,
           count(DISTINCT pr.id) AS prs,
           count(DISTINCT r.id) AS reviews,
           count(DISTINCT r.author_username) AS engineers,
           coalesce(sum(l.billable_loc), 0) AS loc
    FROM repositories rp
    LEFT JOIN pull_requests pr ON pr.repository_id = rp.id
      AND pr.provider_created_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    LEFT JOIN reviews r ON r.repository = rp.name AND r.org_id = {org_id}
      AND COALESCE(r.completed_at, r.created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    LEFT JOIN loc_usage_ledger l ON l.review_id = r.id AND l.status = 'accounted'
    WHERE rp.org_id = {org_id}
    GROUP BY 1
    HAVING count(DISTINCT r.id) > 0
    ORDER BY 3 DESC;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = []
    for r in rows:
        prs, reviews = int(r["prs"]), int(r["reviews"])
        coverage = (reviews / prs) if prs else None
        data.append({
            "repository": r["repository"], "prs": prs, "reviews": reviews,
            "coverage": round(coverage, 2) if coverage is not None else 0,
            "engineers": int(r["engineers"]), "loc": int(float(r["loc"])),
        })

    with_prs = [d for d in data if d["prs"] > 0]
    avg_coverage = sum(d["coverage"] for d in with_prs) / len(with_prs) if with_prs else 0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Repository review coverage — {args.org}, last {args.days} days",
        "width": 600, "height": 380,
        "data": {"values": data},
        "mark": {"type": "circle", "opacity": 0.85},
        "encoding": {
            "x": {"field": "coverage", "type": "quantitative", "title": "Reviews / PRs"},
            "y": {"field": "loc", "type": "quantitative", "title": "LOC reviewed"},
            "size": {"field": "engineers", "type": "quantitative", "title": "Engineers", "scale": {"range": [60, 900]}},
            "color": {"field": "coverage", "type": "quantitative", "title": "Coverage", "scale": {"scheme": "blues"}},
            "tooltip": [{"field": "repository", "type": "nominal"}, {"field": "prs", "type": "quantitative", "title": "PRs"},
                        {"field": "reviews", "type": "quantitative"}, {"field": "coverage", "type": "quantitative", "title": "Reviews/PR"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "legend": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5"},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Repo Coverage — {args.org}", spec=spec, view_max_width=640,
        stats_html=(f"Average coverage: <b>{avg_coverage:.2f}</b> reviews per PR across <b>{len(with_prs)}</b> repos with PR data.<br>"
                     f"<i>Y axis is LOC reviewed, not a reviewed/changed ratio - total changed LOC per PR (git-host diff stats) "
                     f"isn't tracked in this schema.</i>"),
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
