#!/usr/bin/env python3
"""Renders "Is adoption becoming broader over time?" (row 5 of
cto_chart_ideas.html):

    Stacked area chart
    X: Week   Y: Active engineers   Color: 0 reviews / 1-4 / 5+ / heavy users

More interesting than a raw active-user count: shows whether the
organization is moving from "a few enthusiasts" toward broad, habitual
adoption, or whether growth is really just the same few people reviewing
more. Bands are computed per-week (an engineer's tier can change week to
week), using the same band definitions as generate_breadth.py /
generate_leaderboard.py.

Rerunnable daily: pulls the trailing --days window ending today from the dev
DB pointed to by ../../.env's DATABASE_URL (never .env.prod), for the given
org name. Writes a self-contained HTML file next to this script using
vega-embed from a CDN (the file is opened in a browser, which has its own
internet access - this is a local dev script, not a published artifact).

Usage:
    uv run generate_growth.py [--org hexmos-internal] [--days 180] [--out FILE]
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
    parser.add_argument("--days", type=int, default=180)
    parser.add_argument("--out", default=str(SCRIPT_DIR / "adoption_growth.html"))
    args = parser.parse_args()

    database_url = load_database_url(REPO_ROOT / ".env")

    org_rows = run_query(database_url, f"SELECT id, name FROM orgs WHERE name = '{args.org}'")
    if not org_rows:
        sys.exit(f"no org named {args.org!r} found")
    org_id = int(org_rows[0]["id"])

    # Per-engineer, per-week review counts - only rows with n >= 1 exist
    # here (an engineer with zero reviews in a week just has no row), which
    # is correct: this chart is about the shape of *active* usage, not
    # about penalizing engineers who simply didn't touch a review that week.
    sql = f"""
    SELECT date_trunc('week', COALESCE(completed_at, created_at))::date AS week,
           author_username, count(*) AS n
    FROM reviews
    WHERE org_id = {org_id}
      AND author_username IS NOT NULL
      AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1, 2
    ORDER BY 1;
    """
    rows = run_query(database_url, sql)
    if not rows:
        sys.exit("query returned no rows")

    # band_for's first bucket is "0 reviews", which never applies here (see
    # the SQL comment above) - only bands 2-4 (1-4 / 5-19 / 20+) are used.
    active_bands = [b for b in BANDS if b[0] >= 1]
    band_order = [b[2] for b in active_bands]
    color_range = [b[3] for b in active_bands]

    counts: dict[tuple[str, str], int] = {}
    for r in rows:
        n = int(r["n"])
        label, _ = band_for(n)
        if label not in band_order:
            continue
        key = (r["week"], label)
        counts[key] = counts.get(key, 0) + 1

    data = [{"week": week, "band": band, "engineers": n} for (week, band), n in counts.items()]
    weeks = sorted({d["week"] for d in data})
    total_by_week = {}
    for d in data:
        total_by_week[d["week"]] = total_by_week.get(d["week"], 0) + d["engineers"]
    first_total = total_by_week.get(weeks[0], 0) if weeks else 0
    last_total = total_by_week.get(weeks[-1], 0) if weeks else 0
    heavy_label = active_bands[-1][2]
    first_heavy = counts.get((weeks[0], heavy_label), 0) if weeks else 0
    last_heavy = counts.get((weeks[-1], heavy_label), 0) if weeks else 0

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Adoption growth — {args.org}, last {args.days} days",
        "width": 800,
        "height": 380,
        "data": {"values": data},
        "mark": {"type": "area", "interpolate": "monotone", "line": {"strokeWidth": 1.5}},
        "encoding": {
            "x": {"field": "week", "type": "temporal", "title": "Week"},
            "y": {"field": "engineers", "type": "quantitative", "title": "Active engineers", "stack": True},
            "color": {"field": "band", "type": "nominal", "title": "Tier", "sort": band_order,
                      "scale": {"domain": band_order, "range": color_range},
                      "legend": {"orient": "top", "titleColor": "#e6ebf5", "labelColor": "#c9d1e0"}},
            "order": {"field": "band", "sort": "ascending"},
            "tooltip": [
                {"field": "week", "type": "temporal", "title": "Week"},
                {"field": "band", "type": "nominal", "title": "Tier"},
                {"field": "engineers", "type": "quantitative", "title": "Engineers"},
            ],
        },
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
<title>LiveReview Adoption Growth — {args.org}</title>
<script src="https://cdn.jsdelivr.net/npm/vega@5.33.1" integrity="sha384-NMXhl2TbCXxcN7o4ROC56Funm78m4AylL8gMg/7Kn4YU+wrm23K9l7cY8lDRXQ9d" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-lite@5.23.0" integrity="sha384-D9LYH0esGjcxQJsBuxOuXtCDJGXRWW1+KhluzWPqi0rLJmiR/ygPChefaD+rFFDQ" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-embed@6.29.0" integrity="sha384-M+Ax7e/WFJpxSOF09HzI+Sj4wg9ottVd/uxmV2ItGGh02fLH28t2FAOJx3TJBap5" crossorigin="anonymous"></script>
<style>
  body {{ background:#0b0e17; color:#e6ebf5; font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin:0; padding:32px; }}
  .stats {{ margin-top:20px; font-size:14px; line-height:1.6; color:#c9d1e0; max-width:900px; }}
  .stats b {{ color:#fff; }}
  #view {{ max-width: 840px; }}
  details {{ margin-top:20px; max-width:900px; }}
  summary {{ cursor:pointer; color:#aab4c8; font-size:13px; }}
  pre.sql {{ margin-top:10px; padding:12px 14px; background:#0b0e17; border:1px solid #232a3d; border-radius:8px;
             font-family: "SF Mono", Menlo, Consolas, monospace; font-size:12px; line-height:1.5; color:#a3b3cc;
             white-space:pre-wrap; overflow-x:auto; }}
</style>
</head>
<body>
  <div id="view"></div>
  <div class="stats">
    Total active engineers/week went from <b>{first_total}</b> to <b>{last_total}</b>.<br>
    Heavy users (20+ reviews/week) went from <b>{first_heavy}</b> to <b>{last_heavy}</b>.
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
    print(f"weeks={len(weeks)} first_total={first_total} last_total={last_total}")


if __name__ == "__main__":
    main()
