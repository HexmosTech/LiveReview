#!/usr/bin/env python3
"""Row 25: "Which engineers are getting the most value from LR?" 2D
scatterplot. X: reviews/engineer. Y: useful findings/engineer. Size: LOC.
Color: feedback acceptance (up-vote share).

NOTE ON DATA: ai_comments (LR's own findings) has zero rows in this dev DB,
so there's no direct "findings" count to plot. This uses review_feedback's
up-votes on an engineer's reviews as a proxy for "useful findings" - it's
real data (users explicitly marking a finding useful), just a narrower
signal than a full findings count would be. Labeled in the page itself.

Usage: uv run generate_value_scatter.py [--org hexmos-internal] [--days 180]
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--days", type=int, default=180)
    p.add_argument("--out", default=str(SCRIPT_DIR / "value_scatter.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT r.author_username AS engineer,
           count(DISTINCT r.id) AS reviews,
           coalesce(sum(l.billable_loc), 0) AS loc,
           count(*) FILTER (WHERE f.vote_type = 'up') AS up_votes,
           count(*) FILTER (WHERE f.vote_type = 'down') AS down_votes
    FROM reviews r
    LEFT JOIN loc_usage_ledger l ON l.review_id = r.id AND l.status = 'accounted'
    LEFT JOIN review_feedback f ON f.review_id = r.id AND f.org_id = {org_id} AND f.retracted_at IS NULL
    WHERE r.org_id = {org_id} AND r.author_username IS NOT NULL
      AND COALESCE(r.completed_at, r.created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = []
    for r in rows:
        up, down = int(r["up_votes"]), int(r["down_votes"])
        total_votes = up + down
        acceptance = (up / total_votes * 100) if total_votes else None
        data.append({
            "engineer": r["engineer"], "reviews": int(r["reviews"]), "loc": int(float(r["loc"])),
            "useful_findings": up, "acceptance": acceptance if acceptance is not None else 0,
        })

    with_feedback = [d for d in data if d["useful_findings"] > 0]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Value from LiveReview per engineer — {args.org}, last {args.days} days",
        "width": 600, "height": 380,
        "data": {"values": data},
        "mark": {"type": "circle", "opacity": 0.85},
        "encoding": {
            "x": {"field": "reviews", "type": "quantitative", "title": "Reviews"},
            "y": {"field": "useful_findings", "type": "quantitative", "title": "Up-voted feedback (proxy for useful findings)"},
            "size": {"field": "loc", "type": "quantitative", "title": "LOC reviewed", "scale": {"range": [60, 900]}},
            "color": {"field": "acceptance", "type": "quantitative", "title": "Acceptance %", "scale": {"scheme": "greens"}},
            "tooltip": [{"field": "engineer", "type": "nominal"}, {"field": "reviews", "type": "quantitative"},
                        {"field": "useful_findings", "type": "quantitative", "title": "Up-votes"}, {"field": "loc", "type": "quantitative", "title": "LOC"}],
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
        title=f"Value Scatter — {args.org}", spec=spec, view_max_width=640,
        stats_html=(f"<b>{len(with_feedback)}</b> of <b>{len(data)}</b> engineers have received explicit feedback so far. "
                     f"<i>\"Useful findings\" here is a proxy (review_feedback up-votes) - ai_comments has no findings "
                     f"data yet in this dev DB, so a direct findings count isn't available.</i>"),
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
