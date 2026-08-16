#!/usr/bin/env python3
"""Row 30: "What changed between week 1 and week 2?" Two-dimensional change
matrix. Rows: reviews, LOC, active engineers, repos active, pre-commit %.
Columns: W1 / W2 / delta. Color = delta. A compact "14-day verdict."
Usage: uv run generate_change_matrix.py [--org hexmos-internal] [--days 7]
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page


def one_period_metrics(db, org_id, start_days_ago, end_days_ago):
    rows = run_query(db, f"""
        WITH window_reviews AS (
          SELECT * FROM reviews WHERE org_id = {org_id}
            AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{start_days_ago} days'
            AND COALESCE(completed_at, created_at) < CURRENT_DATE - INTERVAL '{end_days_ago} days'
        )
        SELECT
          (SELECT count(*) FROM window_reviews) AS reviews,
          (SELECT count(DISTINCT author_username) FROM window_reviews WHERE author_username IS NOT NULL) AS engineers,
          (SELECT count(DISTINCT repository) FROM window_reviews) AS repos,
          (SELECT round(100.0 * count(*) FILTER (WHERE trigger_type = 'cli_diff') / NULLIF(count(*), 0), 0) FROM window_reviews) AS precommit_pct,
          (SELECT coalesce(sum(l.billable_loc), 0) FROM loc_usage_ledger l
             WHERE l.org_id = {org_id} AND l.status = 'accounted'
               AND l.accounted_at >= CURRENT_DATE - INTERVAL '{start_days_ago} days'
               AND l.accounted_at < CURRENT_DATE - INTERVAL '{end_days_ago} days') AS loc;
    """)
    return rows[0]


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--days", type=int, default=7, help="length of each of the two compared periods")
    p.add_argument("--out", default=str(SCRIPT_DIR / "change_matrix.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    w2 = one_period_metrics(db, org_id, args.days, 0)
    w1 = one_period_metrics(db, org_id, args.days * 2, args.days)

    metrics = [
        ("Reviews", "reviews"), ("LOC reviewed", "loc"), ("Active engineers", "engineers"),
        ("Repos active", "repos"), ("Pre-commit %", "precommit_pct"),
    ]
    data = []
    for label, key in metrics:
        v1 = float(w1[key] or 0)
        v2 = float(w2[key] or 0)
        delta_pct = ((v2 - v1) / v1 * 100) if v1 else (100.0 if v2 else 0.0)
        for col, val in (("W1", v1), ("W2", v2), ("Delta", delta_pct)):
            data.append({"metric": label, "period": col, "value": val, "is_delta": col == "Delta"})

    verdict_up = sum(1 for label, key in metrics if float(w2[key] or 0) >= float(w1[key] or 0))

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"{args.days}-day verdict — {args.org}",
        "width": 300, "height": 220,
        "data": {"values": data},
        "layer": [
            {"mark": {"type": "rect"},
             "encoding": {
                 "x": {"field": "period", "type": "nominal", "title": None, "sort": ["W1", "W2", "Delta"]},
                 "y": {"field": "metric", "type": "nominal", "title": None, "sort": [m[0] for m in metrics]},
                 "color": {
                     "condition": {"test": "datum.is_delta", "field": "value", "type": "quantitative",
                                   "scale": {"domainMid": 0, "range": ["#ff5c7c", "#232a3d", "#39d353"]}},
                     "value": "#161b22",
                 },
             }},
            {"mark": {"type": "text", "color": "#e6ebf5", "fontSize": 12},
             "encoding": {
                 "x": {"field": "period", "type": "nominal", "sort": ["W1", "W2", "Delta"]},
                 "y": {"field": "metric", "type": "nominal", "sort": [m[0] for m in metrics]},
                 "text": {"field": "value", "type": "quantitative", "format": ".1f"},
             }},
        ],
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"{args.days}-Day Change Matrix — {args.org}", spec=spec, view_max_width=340,
        stats_html=f"<b>{verdict_up}</b> of <b>{len(metrics)}</b> metrics improved or held steady period-over-period.",
        sql=f"-- two periods, each computed via one_period_metrics() in generate_change_matrix.py:\n"
            f"-- W1: {args.days*2} to {args.days} days ago\n-- W2: {args.days} days ago to now\n"
            f"-- (see the script for the underlying per-metric SQL)",
        out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
