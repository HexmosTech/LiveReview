#!/usr/bin/env python3
"""Row 26: "Are people trusting the reviews?" Diverging stacked bar.
X: engineer (whose review the feedback is on). Y: feedback count. Up vs down.
review_feedback records up/down votes explicitly, so this is grounded
directly in the schema rather than a proxy.
Usage: uv run generate_feedback_trust.py [--org hexmos-internal] [--days 180]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "feedback_trust.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT r.author_username AS engineer, f.vote_type, count(*) AS n
    FROM review_feedback f
    JOIN reviews r ON r.id = f.review_id
    WHERE f.org_id = {org_id} AND r.author_username IS NOT NULL
      AND f.retracted_at IS NULL
      AND f.created_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1, 2;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("no review_feedback rows found in this window - try a larger --days")

    data = []
    for r in rows:
        n = int(r["n"])
        data.append({"engineer": r["engineer"], "vote_type": r["vote_type"], "n": n if r["vote_type"] == "up" else -n})

    total_up = sum(int(r["n"]) for r in rows if r["vote_type"] == "up")
    total_down = sum(int(r["n"]) for r in rows if r["vote_type"] == "down")
    trust_pct = round(total_up / (total_up + total_down) * 100) if (total_up + total_down) else 0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Review feedback — {args.org}, last {args.days} days",
        "width": 600, "height": 320,
        "data": {"values": data},
        "mark": {"type": "bar"},
        "encoding": {
            "y": {"field": "engineer", "type": "nominal", "title": None},
            "x": {"field": "n", "type": "quantitative", "title": "Feedback count (down <- 0 -> up)"},
            "color": {"field": "vote_type", "type": "nominal", "title": "Vote",
                      "scale": {"domain": ["up", "down"], "range": ["#39d353", "#ff5c7c"]},
                      "legend": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5"}},
            "tooltip": [{"field": "engineer", "type": "nominal"}, {"field": "vote_type", "type": "nominal"}, {"field": "n", "type": "quantitative"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Feedback Trust — {args.org}", spec=spec, view_max_width=640,
        stats_html=f"<b>{total_up}</b> up-votes vs <b>{total_down}</b> down-votes (<b>{trust_pct}%</b> positive).",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
