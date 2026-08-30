#!/usr/bin/env python3
"""Renders the "Are engineers actually incorporating reviews into their daily
workflow?" chart the techlead spec'd:

    Calendar heatmap
    X: Day (bucketed by week)   Y: Day-of-week   Color: reviews or reviewed LOC

Gives an immediate "usage rhythm" - weekday gaps, isolated bursts, and sudden
changes are visually obvious at a glance. Styled to match GitHub's own
contribution graph (green scale, small square cells, month labels shown once
per month, only Mon/Wed/Fri row labels, a "Less -> More" legend) since that's
the exact visual vocabulary this chart type is already familiar from -
reviews stand in for contributions. Default window is a full year (365 days,
like GitHub's own graph); pass --days 30 for the tighter trial-window view
the original spec called out.

Rerunnable daily: pulls the trailing --days window ending today from the dev
DB pointed to by ../../.env's DATABASE_URL (never .env.prod), for the given
org name. Writes a self-contained HTML file next to this script using
vega-embed from a CDN (the file is opened in a browser, which has its own
internet access - this is a local dev script, not a published artifact).

Usage:
    uv run generate_heatmap.py [--org hexmos-internal] [--days 365] [--out FILE]
    uv run generate_heatmap.py --metric loc   # color by reviewed LOC instead of review count
"""
import argparse
import csv
import datetime
import html
import io
import json
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
    parser.add_argument("--days", type=int, default=365)
    parser.add_argument("--metric", choices=["reviews", "loc"], default="reviews",
                         help="color cells by review count (default) or reviewed LOC")
    parser.add_argument("--out", default=str(SCRIPT_DIR / "adoption_heatmap.html"))
    args = parser.parse_args()

    database_url = load_database_url(REPO_ROOT / ".env")

    org_rows = run_query(database_url, f"SELECT id, name FROM orgs WHERE name = '{args.org}'")
    if not org_rows:
        sys.exit(f"no org named {args.org!r} found")
    org_id = int(org_rows[0]["id"])

    # generate_series fills every day in the window, even ones with zero
    # activity - required so a genuinely quiet weekday reads as an empty
    # cell rather than silently not appearing in the grid at all.
    if args.metric == "reviews":
        metric_sql = f"""
        daily AS (
          SELECT date_trunc('day', COALESCE(completed_at, created_at))::date AS day, count(*) AS value
          FROM reviews
          WHERE org_id = {org_id}
            AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
          GROUP BY 1
        )
        """
        value_label = "Reviews"
    else:
        metric_sql = f"""
        daily AS (
          SELECT accounted_at::date AS day, sum(billable_loc) AS value
          FROM loc_usage_ledger
          WHERE org_id = {org_id} AND status = 'accounted'
            AND accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
          GROUP BY 1
        )
        """
        value_label = "Reviewed LOC"

    sql = f"""
    WITH days AS (
      SELECT generate_series(
        (CURRENT_DATE - INTERVAL '{args.days} days')::date,
        CURRENT_DATE::date,
        '1 day'
      )::date AS day
    ),
    {metric_sql}
    SELECT d.day, COALESCE(daily.value, 0) AS value
    FROM days d
    LEFT JOIN daily ON daily.day = d.day
    ORDER BY d.day;
    """
    rows = run_query(database_url, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = [{"day": r["day"], "value": int(float(r["value"]))} for r in rows]

    active_days = sum(1 for d in data if d["value"] > 0)
    total_value = sum(d["value"] for d in data)
    by_dow = {}
    for r, d in zip(rows, data):
        # Postgres date -> Python: derive weekday name from the ISO date string
        # rather than re-querying, to keep this a single round trip.
        dow = datetime.date.fromisoformat(r["day"]).strftime("%A")
        by_dow.setdefault(dow, 0)
        by_dow[dow] += d["value"]
    quietest_weekday = min(
        (d for d in by_dow if d not in ("Saturday", "Sunday")),
        key=lambda d: by_dow[d],
        default=None,
    )

    # GitHub's own green scale, so "Less -> More" swatches below match the
    # chart exactly instead of approximating a generic vega-lite scheme.
    github_greens = ["#161b22", "#0e4429", "#006d32", "#26a641", "#39d353"]
    cell_step = 11 if args.days > 120 else 20

    period_label = "in the last year" if args.days >= 360 else f"in the last {args.days} days"

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"{total_value} {value_label.lower()} {period_label} — {args.org}",
        "width": {"step": cell_step},
        "height": {"step": cell_step},
        "data": {"values": data},
        "mark": {"type": "rect", "cornerRadius": 2},
        "encoding": {
            "x": {
                # type "ordinal", not "temporal": a temporal x here puts week
                # columns on a continuous time scale, whose band-width math
                # for a rect mark collapses/overlaps adjacent weeks into each
                # other instead of giving each week its own fixed-width
                # column - the official Vega-Lite calendar-heatmap examples
                # (rect_heatmap_weather.html et al.) all use ordinal for
                # exactly this reason. y (below) already did this correctly.
                "field": "day", "type": "ordinal", "timeUnit": "yearweek",
                "title": None,
                # paddingInner is what actually creates the gap GitHub has
                # between cells - an ordinal band scale defaults to 0
                # padding, which is why cells touched edge-to-edge with no
                # visible gap even after fixing the temporal/overlap bug.
                "scale": {"paddingInner": 0.15},
                "axis": {
                    "format": "%b",
                    # Only label a week column if it's the first week whose
                    # start falls in a new month - otherwise every one of the
                    # ~4-5 weeks in a month would repeat that month's label.
                    "labelExpr": "date(datum.value) <= 7 ? timeFormat(datum.value, '%b') : ''",
                    "labelAngle": 0, "ticks": False, "domain": False, "grid": False,
                },
            },
            "y": {
                "field": "day", "type": "ordinal", "timeUnit": "day",
                "title": None,
                "sort": ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"],
                "scale": {"paddingInner": 0.15},
                # GitHub only labels every other row (Mon/Wed/Fri) to keep the
                "axis": {"values": ["Mon", "Wed", "Fri"], "ticks": False, "domain": False, "grid": False},
            },
            "color": {
                "field": "value", "type": "quantitative", "title": value_label,
                "scale": {"type": "threshold", "domain": [1, 3, 6, 10], "range": github_greens},
                "legend": None,
            },
            "tooltip": [
                {"field": "day", "type": "temporal", "title": "Date", "format": "%A, %b %d, %Y"},
                {"field": "value", "type": "quantitative", "title": value_label},
            ],
        },
        "config": {
            "background": "#0d1117",
            "axis": {"labelColor": "#8b949e", "titleColor": "#e6ebf5", "labelFontSize": 10},
            "title": {"color": "#e6ebf5", "fontSize": 16, "anchor": "start"},
            "view": {"stroke": "transparent"},
        },
    }

    swatches = "".join(f'<span class="swatch" style="background:{c}"></span>' for c in github_greens)

    page_html = f"""<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>LiveReview Activity Rhythm — {args.org}</title>
<script src="https://cdn.jsdelivr.net/npm/vega@5.33.1" integrity="sha384-NMXhl2TbCXxcN7o4ROC56Funm78m4AylL8gMg/7Kn4YU+wrm23K9l7cY8lDRXQ9d" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-lite@5.23.0" integrity="sha384-D9LYH0esGjcxQJsBuxOuXtCDJGXRWW1+KhluzWPqi0rLJmiR/ygPChefaD+rFFDQ" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-embed@6.29.0" integrity="sha384-M+Ax7e/WFJpxSOF09HzI+Sj4wg9ottVd/uxmV2ItGGh02fLH28t2FAOJx3TJBap5" crossorigin="anonymous"></script>
<style>
  body {{ background:#0d1117; color:#e6ebf5; font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin:0; padding:32px; }}
  .card {{ border:1px solid #30363d; border-radius:8px; padding:20px; max-width:960px; }}
  .footer-row {{ display:flex; justify-content:space-between; align-items:center; margin-top:8px; font-size:12px; color:#7d8590; }}
  .legend {{ display:flex; align-items:center; gap:4px; }}
  .swatch {{ display:inline-block; width:10px; height:10px; border-radius:2px; }}
  .stats {{ margin-top:20px; font-size:14px; line-height:1.6; color:#c9d1e0; max-width:900px; }}
  .stats b {{ color:#fff; }}
  #view {{ max-width: 940px; overflow-x:auto; }}
  details {{ margin-top:20px; max-width:900px; }}
  summary {{ cursor:pointer; color:#7d8590; font-size:13px; }}
  pre.sql {{ margin-top:10px; padding:12px 14px; background:#0b0e17; border:1px solid #30363d; border-radius:8px;
             font-family: "SF Mono", Menlo, Consolas, monospace; font-size:12px; line-height:1.5; color:#a3b3cc;
             white-space:pre-wrap; overflow-x:auto; }}
</style>
</head>
<body>
  <div class="card">
    <div id="view"></div>
    <div class="footer-row">
      <span>Learn how LiveReview counts activity</span>
      <span class="legend">Less {swatches} More</span>
    </div>
  </div>
  <div class="stats">
    Active days: <b>{active_days}</b> / {args.days} ({active_days / args.days * 100:.0f}%)<br>
    {"Quietest weekday (excl. weekends): <b>" + quietest_weekday + "</b><br>" if quietest_weekday else ""}
  </div>
  <details>
    <summary>Query used</summary>
    <pre class="sql">{html.escape(sql.strip())}</pre>
  </details>
  <script>
    vegaEmbed('#view', {json.dumps(spec)}, {{actions: false}});
  </script>
</body>
</html>
"""
    out_path = Path(args.out)
    out_path.write_text(page_html)
    print(f"wrote {out_path}")
    print(f"active_days={active_days}/{args.days} total_{args.metric}={total_value} quietest_weekday={quietest_weekday}")


if __name__ == "__main__":
    main()
