#!/usr/bin/env python3
"""Row 21: "How much code has LR reviewed?" Horizon graph. X: day. Y: LOC.
Layered intensity bands - a compact long-term view (3 stacked area layers of
the same field, each clamped to a band, with rising opacity) instead of one
tall sparse line.
Usage: uv run generate_horizon.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "loc_horizon.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    WITH days AS (
      SELECT generate_series((CURRENT_DATE - INTERVAL '{args.days} days')::date, CURRENT_DATE::date, '1 day')::date AS day
    ),
    daily AS (
      SELECT accounted_at::date AS day, sum(billable_loc) AS loc
      FROM loc_usage_ledger
      WHERE org_id = {org_id} AND status = 'accounted'
        AND accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
      GROUP BY 1
    )
    SELECT d.day, COALESCE(daily.loc, 0) AS loc
    FROM days d LEFT JOIN daily ON daily.day = d.day
    ORDER BY d.day;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    values = [int(float(r["loc"])) for r in rows]
    band = max(values) / 3 if max(values) > 0 else 1
    data = []
    for r in rows:
        loc = int(float(r["loc"]))
        data.append({
            "day": r["day"], "loc": loc,
            "b1": min(loc, band),
            "b2": max(0, min(loc, 2 * band) - band),
            "b3": max(0, loc - 2 * band),
        })

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"LOC reviewed (horizon) — {args.org}, last {args.days} days",
        "width": 800, "height": 90,
        "data": {"values": data},
        "layer": [
            {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.35, "interpolate": "monotone"},
             "encoding": {"x": {"field": "day", "type": "temporal", "title": "Day"}, "y": {"field": "b1", "type": "quantitative", "title": "LOC", "scale": {"domain": [0, band]}}}},
            {"mark": {"type": "area", "color": "#7c9cff", "opacity": 0.6, "interpolate": "monotone"},
             "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b2", "type": "quantitative", "scale": {"domain": [0, band]}}}},
            {"mark": {"type": "area", "color": "#7c9cff", "opacity": 1.0, "interpolate": "monotone"},
             "encoding": {"x": {"field": "day", "type": "temporal"}, "y": {"field": "b3", "type": "quantitative", "scale": {"domain": [0, band]}}}},
        ],
        "resolve": {"scale": {"y": "shared"}},
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"LOC Horizon — {args.org}", spec=spec, view_max_width=840,
        stats_html=f"Total LOC reviewed: <b>{sum(values)}</b>. Peak day: <b>{max(values)}</b> LOC.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
