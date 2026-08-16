#!/usr/bin/env python3
"""Row 22: "Are reviews getting faster?" Line + percentile bands. X: day.
Y: review duration (minutes). Median line, p10-p90 error band.
Usage: uv run generate_latency_bands.py [--org hexmos-internal] [--days 90]
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
    p.add_argument("--out", default=str(SCRIPT_DIR / "latency_bands.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT date_trunc('week', completed_at)::date AS week,
           percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM completed_at - created_at) / 60) AS p50,
           percentile_cont(0.1) WITHIN GROUP (ORDER BY extract(epoch FROM completed_at - created_at) / 60) AS p10,
           percentile_cont(0.9) WITHIN GROUP (ORDER BY extract(epoch FROM completed_at - created_at) / 60) AS p90
    FROM reviews
    WHERE org_id = {org_id} AND status = 'completed' AND completed_at IS NOT NULL
      AND completed_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1
    ORDER BY 1;
    """
    rows = run_query(db, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"week": r["week"], "p50": round(float(r["p50"]), 1), "p10": round(float(r["p10"]), 1), "p90": round(float(r["p90"]), 1)} for r in rows]
    first_p50, last_p50 = data[0]["p50"], data[-1]["p50"]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Review duration — {args.org}, last {args.days} days (p10-p90 band, p50 line)",
        "width": 700, "height": 340,
        "data": {"values": data},
        "layer": [
            {"mark": {"type": "errorband", "color": "#7c9cff", "opacity": 0.25},
             "encoding": {"x": {"field": "week", "type": "temporal", "title": "Week"},
                          "y": {"field": "p10", "type": "quantitative", "title": "Review duration (min)"}, "y2": {"field": "p90"}}},
            {"mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "point": True},
             "encoding": {"x": {"field": "week", "type": "temporal"}, "y": {"field": "p50", "type": "quantitative"},
                          "tooltip": [{"field": "week", "type": "temporal"}, {"field": "p50", "type": "quantitative", "title": "Median (min)"},
                                      {"field": "p10", "type": "quantitative", "title": "p10"}, {"field": "p90", "type": "quantitative", "title": "p90"}]}},
        ],
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Review Latency — {args.org}", spec=spec, view_max_width=740,
        stats_html=f"Median review duration went from <b>{first_p50:.1f} min</b> to <b>{last_p50:.1f} min</b> across the window.",
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
