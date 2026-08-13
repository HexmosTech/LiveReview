#!/usr/bin/env python3
"""Renders the "Is LiveReview adoption increasing?" chart the techlead spec'd:

    Layered line + area + target rule
    X: Day   Y: Reviews/day
    Area = reviews; line = 7-day rolling avg; rule = target (baseline avg)

Rerunnable daily: pulls the trailing --days window ending today from the dev
DB pointed to by ../../.env's DATABASE_URL (never .env.prod), for the given
org name. Writes a self-contained HTML file next to this script using
vega-embed from a CDN (the file is opened in a browser, which has its own
internet access - this is a local dev script, not a published artifact).

Usage:
    uv run generate.py [--org hexmos-internal] [--days 90] [--out FILE]
"""
import argparse
import csv
import io
import json
import os
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT_DIR = Path(__file__).resolve().parent


def load_database_url(env_path: Path) -> str:
    if not env_path.exists():
        sys.exit(f"env file not found: {env_path}")
    for line in env_path.read_text().splitlines():
        m = re.match(r"^\s*DATABASE_URL\s*=\s*(.+?)\s*$", line)
        if m:
            return m.group(1).strip('"').strip("'")
    sys.exit(f"DATABASE_URL not set in {env_path}")


def run_query(database_url: str, sql: str) -> list[dict]:
    result = subprocess.run(
        ["psql", database_url, "-v", "ON_ERROR_STOP=1", "--csv", "-c", sql],
        capture_output=True, text=True,
    )
    if result.returncode != 0:
        sys.exit(f"psql query failed:\n{result.stderr}")
    reader = csv.DictReader(io.StringIO(result.stdout))
    return list(reader)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--org", default="hexmos-internal")
    parser.add_argument("--days", type=int, default=90)
    parser.add_argument("--out", default=str(SCRIPT_DIR / "adoption_chart.html"))
    args = parser.parse_args()

    database_url = load_database_url(REPO_ROOT / ".env")

    org_rows = run_query(database_url, f"SELECT id, name FROM orgs WHERE name = '{args.org}'")
    if not org_rows:
        sys.exit(f"no org named {args.org!r} found")
    org_id = int(org_rows[0]["id"])

    # generate_series fills every day in the window, even ones with zero
    # reviews - required so the area/line don't silently skip gaps and the
    # 7-day rolling average isn't computed over a sparser series than it looks.
    sql = f"""
    WITH days AS (
      SELECT generate_series(
        (CURRENT_DATE - INTERVAL '{args.days} days')::date,
        CURRENT_DATE::date,
        '1 day'
      )::date AS day
    ),
    daily AS (
      SELECT date_trunc('day', COALESCE(completed_at, created_at))::date AS day, count(*) AS n
      FROM reviews
      WHERE org_id = {org_id}
        AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
      GROUP BY 1
    ),
    filled AS (
      SELECT d.day, COALESCE(daily.n, 0) AS reviews
      FROM days d
      LEFT JOIN daily ON daily.day = d.day
    )
    SELECT day, reviews,
           round(avg(reviews) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW), 2) AS rolling_avg_7d,
           round(avg(reviews) OVER (), 2) AS period_avg
    FROM filled
    ORDER BY day;
    """
    rows = run_query(database_url, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [
        {
            "day": r["day"],
            "reviews": int(r["reviews"]),
            "rolling_avg_7d": float(r["rolling_avg_7d"]),
        }
        for r in rows
    ]
    period_avg = float(rows[0]["period_avg"])
    total_reviews = sum(d["reviews"] for d in data)
    first_half = data[: len(data) // 2]
    second_half = data[len(data) // 2 :]
    first_half_avg = sum(d["reviews"] for d in first_half) / max(len(first_half), 1)
    second_half_avg = sum(d["reviews"] for d in second_half) / max(len(second_half), 1)
    pct_change = ((second_half_avg - first_half_avg) / first_half_avg * 100) if first_half_avg else 0.0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Is {args.org} adopting LiveReview? — reviews/day, last {args.days} days",
        "width": 900,
        "height": 420,
        "data": {"values": data},
        "layer": [
            {
                "mark": {"type": "area", "opacity": 0.25, "color": "#7c9cff", "interpolate": "monotone"},
                "encoding": {
                    "x": {"field": "day", "type": "temporal", "title": "Day"},
                    "y": {"field": "reviews", "type": "quantitative", "title": "Reviews / day"},
                    "tooltip": [
                        {"field": "day", "type": "temporal", "title": "Day"},
                        {"field": "reviews", "type": "quantitative", "title": "Reviews"},
                    ],
                },
            },
            {
                "mark": {"type": "line", "color": "#ffb454", "strokeWidth": 2.5, "interpolate": "monotone"},
                "encoding": {
                    "x": {"field": "day", "type": "temporal"},
                    "y": {"field": "rolling_avg_7d", "type": "quantitative"},
                    "tooltip": [
                        {"field": "day", "type": "temporal", "title": "Day"},
                        {"field": "rolling_avg_7d", "type": "quantitative", "title": "7-day avg"},
                    ],
                },
            },
            {
                "mark": {"type": "rule", "color": "#ff5c7c", "strokeDash": [6, 4], "strokeWidth": 1.5},
                "data": {"values": [{"period_avg": period_avg}]},
                "encoding": {"y": {"field": "period_avg", "type": "quantitative"}},
            },
        ],
        "resolve": {"scale": {"y": "shared"}},
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }

    html = f"""<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>LiveReview Adoption — {args.org}</title>
<script src="https://cdn.jsdelivr.net/npm/vega@5"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-lite@5"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-embed@6"></script>
<style>
  body {{ background:#0b0e17; color:#e6ebf5; font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin:0; padding:32px; }}
  .legend {{ display:flex; gap:24px; margin:16px 0 4px; font-size:13px; color:#aab4c8; }}
  .swatch {{ display:inline-block; width:12px; height:12px; border-radius:2px; margin-right:6px; vertical-align:middle; }}
  .stats {{ margin-top:20px; font-size:14px; line-height:1.6; color:#c9d1e0; max-width:900px; }}
  .stats b {{ color:#fff; }}
  #view {{ max-width: 940px; }}
</style>
</head>
<body>
  <div id="view"></div>
  <div class="legend">
    <span><span class="swatch" style="background:#7c9cff"></span>Reviews/day</span>
    <span><span class="swatch" style="background:#ffb454"></span>7-day rolling average</span>
    <span><span class="swatch" style="background:#ff5c7c"></span>Period average (baseline rule)</span>
  </div>
  <div class="stats">
    Total reviews in window: <b>{total_reviews}</b><br>
    First-half daily average: <b>{first_half_avg:.1f}</b> &rarr; second-half daily average: <b>{second_half_avg:.1f}</b>
    (<b>{pct_change:+.0f}%</b>)<br>
    Period average (dashed rule): <b>{period_avg:.1f}</b> reviews/day
  </div>
  <script>
    vegaEmbed('#view', {json.dumps(spec)}, {{actions: false}});
  </script>
</body>
</html>
"""
    out_path = Path(args.out)
    out_path.write_text(html)
    print(f"wrote {out_path}")
    print(f"total_reviews={total_reviews} first_half_avg={first_half_avg:.2f} second_half_avg={second_half_avg:.2f} pct_change={pct_change:+.1f}% period_avg={period_avg:.2f}")


if __name__ == "__main__":
    main()
