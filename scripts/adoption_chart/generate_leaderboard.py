#!/usr/bin/env python3
"""Renders "Who has adopted LiveReview—and who hasn't?" (row 4 of
cto_chart_ideas.html):

    Sorted horizontal bar / dot plot
    X: Reviews or LOC   Y: Engineer   Color: adoption tier
    + a vertical target rule (default 5 reviews/day-equivalent over the window)

Ranks engineers from highest to lowest usage - directly actionable for
engineering leaders deciding who needs a nudge. Uses the same adoption-band
thresholds/colors as generate_breadth.py so a tier means the same thing
across both charts.

Rerunnable daily: pulls the trailing --days window ending today from the dev
DB pointed to by ../../.env's DATABASE_URL (never .env.prod), for the given
org name. Writes a self-contained HTML file next to this script using
vega-embed from a CDN (the file is opened in a browser, which has its own
internet access - this is a local dev script, not a published artifact).

Usage:
    uv run generate_leaderboard.py [--org hexmos-internal] [--days 90] [--out FILE]
    uv run generate_leaderboard.py --metric loc --target 100   # rank by LOC instead of review count
"""
import argparse
import csv
import html
import io
import json
import re
import subprocess
import sys
from pathlib import Path

from generate_breadth import BANDS, band_for

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
    parser.add_argument("--metric", choices=["reviews", "loc"], default="reviews",
                         help="rank by review count (default) or reviewed LOC")
    parser.add_argument("--target", type=float, default=None,
                         help="target-rule value (default: 5 for reviews, 200 for loc)")
    parser.add_argument("--out", default=str(SCRIPT_DIR / "adoption_leaderboard.html"))
    args = parser.parse_args()

    target = args.target if args.target is not None else (5 if args.metric == "reviews" else 200)

    database_url = load_database_url(REPO_ROOT / ".env")

    org_rows = run_query(database_url, f"SELECT id, name FROM orgs WHERE name = '{args.org}'")
    if not org_rows:
        sys.exit(f"no org named {args.org!r} found")
    org_id = int(org_rows[0]["id"])

    if args.metric == "reviews":
        sql = f"""
        SELECT author_username, count(*) AS value
        FROM reviews
        WHERE org_id = {org_id}
          AND author_username IS NOT NULL
          AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
        GROUP BY 1
        ORDER BY 2 DESC;
        """
        value_label = "Reviews"
    else:
        sql = f"""
        SELECT r.author_username, sum(l.billable_loc) AS value
        FROM loc_usage_ledger l
        JOIN reviews r ON r.id = l.review_id
        WHERE l.org_id = {org_id} AND l.status = 'accounted'
          AND r.author_username IS NOT NULL
          AND l.accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days'
        GROUP BY 1
        ORDER BY 2 DESC;
        """
        value_label = "LOC reviewed"

    rows = run_query(database_url, sql)
    if not rows:
        sys.exit("query returned no rows")

    data = []
    for r in rows:
        n = int(float(r["value"]))
        label, color = band_for(n) if args.metric == "reviews" else band_for(n // 20)  # coarser bands for LOC
        data.append({"engineer": r["author_username"], "value": n, "band": label, "color": color})

    below_target = sum(1 for d in data if d["value"] < target)
    band_order = [b[2] for b in BANDS]
    color_range = [b[3] for b in BANDS]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Who has adopted LiveReview — {args.org}, last {args.days} days",
        "width": 700,
        "height": max(200, 28 * len(data)),
        "data": {"values": data},
        "layer": [
            {
                "mark": {"type": "bar", "cornerRadiusTopRight": 3, "cornerRadiusBottomRight": 3},
                "encoding": {
                    "y": {"field": "engineer", "type": "nominal", "title": "Engineer",
                          "sort": "-x"},
                    "x": {"field": "value", "type": "quantitative", "title": value_label},
                    "color": {"field": "band", "type": "nominal",
                              "scale": {"domain": band_order, "range": color_range}, "legend": None},
                    "tooltip": [
                        {"field": "engineer", "type": "nominal", "title": "Engineer"},
                        {"field": "value", "type": "quantitative", "title": value_label},
                        {"field": "band", "type": "nominal", "title": "Tier"},
                    ],
                },
            },
            {
                "data": {"values": [{"target": target}]},
                "mark": {"type": "rule", "color": "#ff5c7c", "strokeDash": [6, 4], "strokeWidth": 1.5},
                "encoding": {"x": {"field": "target", "type": "quantitative"}},
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

    page_html = f"""<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>LiveReview Adoption Leaderboard — {args.org}</title>
<script src="https://cdn.jsdelivr.net/npm/vega@5.33.1" integrity="sha384-NMXhl2TbCXxcN7o4ROC56Funm78m4AylL8gMg/7Kn4YU+wrm23K9l7cY8lDRXQ9d" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-lite@5.23.0" integrity="sha384-D9LYH0esGjcxQJsBuxOuXtCDJGXRWW1+KhluzWPqi0rLJmiR/ygPChefaD+rFFDQ" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-embed@6.29.0" integrity="sha384-M+Ax7e/WFJpxSOF09HzI+Sj4wg9ottVd/uxmV2ItGGh02fLH28t2FAOJx3TJBap5" crossorigin="anonymous"></script>
<style>
  body {{ background:#0b0e17; color:#e6ebf5; font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin:0; padding:32px; }}
  .stats {{ margin-top:20px; font-size:14px; line-height:1.6; color:#c9d1e0; max-width:900px; }}
  .stats b {{ color:#fff; }}
  #view {{ max-width: 740px; }}
  .legend {{ display:flex; gap:20px; margin:16px 0 4px; font-size:13px; color:#aab4c8; }}
  .swatch {{ display:inline-block; width:12px; height:12px; border-radius:2px; margin-right:6px; vertical-align:middle; }}
  details {{ margin-top:20px; max-width:900px; }}
  summary {{ cursor:pointer; color:#aab4c8; font-size:13px; }}
  pre.sql {{ margin-top:10px; padding:12px 14px; background:#0b0e17; border:1px solid #232a3d; border-radius:8px;
             font-family: "SF Mono", Menlo, Consolas, monospace; font-size:12px; line-height:1.5; color:#a3b3cc;
             white-space:pre-wrap; overflow-x:auto; }}
</style>
</head>
<body>
  <div id="view"></div>
  <div class="legend">
    {"".join(f'<span><span class="swatch" style="background:{c}"></span>{html.escape(l)}</span>' for _, _, l, c in BANDS)}
    <span><span class="swatch" style="background:#ff5c7c"></span>Target ({target:g})</span>
  </div>
  <div class="stats">
    <b>{below_target}</b> of <b>{len(data)}</b> engineers are below the target of <b>{target:g} {value_label.lower()}</b>.
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
    print(f"engineers={len(data)} below_target={below_target} target={target}")


if __name__ == "__main__":
    main()
