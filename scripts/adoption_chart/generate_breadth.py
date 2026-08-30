#!/usr/bin/env python3
"""Renders "How broadly has the organization adopted LiveReview?" (row 3 of
cto_chart_ideas.html):

    Histogram / distribution + KPI overlay
    X: Reviews per engineer (bucketed into adoption bands)
    Y: Engineer count
    Color: adoption band

Shows whether usage is broad or concentrated - "500 reviews" means something
very different if one engineer generated 450 of them. The "KPI overlay" is
the stats block under the chart (median reviews/engineer, top contributor's
share of total) rather than an in-chart overlay mark, matching the pattern
adoption_chart.py/generate_heatmap.py already use in this folder.

Rerunnable daily: pulls the trailing --days window ending today from the dev
DB pointed to by ../../.env's DATABASE_URL (never .env.prod), for the given
org name. Writes a self-contained HTML file next to this script using
vega-embed from a CDN (the file is opened in a browser, which has its own
internet access - this is a local dev script, not a published artifact).

Usage:
    uv run generate_breadth.py [--org hexmos-internal] [--days 90] [--out FILE]
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

REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT_DIR = Path(__file__).resolve().parent

# Shared adoption-band thresholds/colors, used by generate_breadth.py,
# generate_leaderboard.py, and generate_growth.py so an engineer's tier
# means the same thing (and renders in the same color) across all three
# charts in this folder.
BANDS = [
    (0, 0, "0 reviews", "#3a4358"),
    (1, 4, "1-4 (light)", "#7c9cff"),
    (5, 19, "5-19 (regular)", "#ffb454"),
    (20, float("inf"), "20+ (heavy)", "#39d353"),
]


def band_for(n: int) -> tuple[str, str]:
    for lo, hi, label, color in BANDS:
        if lo <= n <= hi:
            return label, color
    return BANDS[-1][2], BANDS[-1][3]


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
    parser.add_argument("--out", default=str(SCRIPT_DIR / "adoption_breadth.html"))
    args = parser.parse_args()

    database_url = load_database_url(REPO_ROOT / ".env")

    org_rows = run_query(database_url, f"SELECT id, name FROM orgs WHERE name = '{args.org}'")
    if not org_rows:
        sys.exit(f"no org named {args.org!r} found")
    org_id = int(org_rows[0]["id"])

    sql = f"""
    SELECT author_username, count(*) AS reviews
    FROM reviews
    WHERE org_id = {org_id}
      AND author_username IS NOT NULL
      AND COALESCE(completed_at, created_at) >= CURRENT_DATE - INTERVAL '{args.days} days'
    GROUP BY 1
    ORDER BY 2 DESC;
    """
    rows = run_query(database_url, sql)
    if not rows:
        sys.exit("query returned no rows")

    per_engineer = [(r["author_username"], int(r["reviews"])) for r in rows]
    total_reviews = sum(n for _, n in per_engineer)
    engineer_count = len(per_engineer)
    top_name, top_n = per_engineer[0]
    top_share = (top_n / total_reviews * 100) if total_reviews else 0.0
    counts = sorted(n for _, n in per_engineer)
    median = counts[len(counts) // 2] if counts else 0

    band_counts: dict[str, int] = {}
    band_colors: dict[str, str] = {}
    for _, n in per_engineer:
        label, color = band_for(n)
        band_counts[label] = band_counts.get(label, 0) + 1
        band_colors[label] = color
    band_order = [b[2] for b in BANDS if b[2] in band_counts]
    data = [{"band": label, "engineers": band_counts[label]} for label in band_order]
    color_range = [band_colors[label] for label in band_order]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Adoption breadth — {args.org}, last {args.days} days",
        "width": 700,
        "height": 340,
        "data": {"values": data},
        "mark": {"type": "bar", "cornerRadiusTopLeft": 4, "cornerRadiusTopRight": 4},
        "encoding": {
            "x": {"field": "band", "type": "nominal", "title": "Reviews per engineer", "sort": band_order,
                  "axis": {"labelAngle": 0}},
            "y": {"field": "engineers", "type": "quantitative", "title": "Engineer count"},
            "color": {"field": "band", "type": "nominal", "sort": band_order,
                      "scale": {"domain": band_order, "range": color_range}, "legend": None},
            "tooltip": [
                {"field": "band", "type": "nominal", "title": "Band"},
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
<title>LiveReview Adoption Breadth — {args.org}</title>
<script src="https://cdn.jsdelivr.net/npm/vega@5.33.1" integrity="sha384-NMXhl2TbCXxcN7o4ROC56Funm78m4AylL8gMg/7Kn4YU+wrm23K9l7cY8lDRXQ9d" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-lite@5.23.0" integrity="sha384-D9LYH0esGjcxQJsBuxOuXtCDJGXRWW1+KhluzWPqi0rLJmiR/ygPChefaD+rFFDQ" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-embed@6.29.0" integrity="sha384-M+Ax7e/WFJpxSOF09HzI+Sj4wg9ottVd/uxmV2ItGGh02fLH28t2FAOJx3TJBap5" crossorigin="anonymous"></script>
<style>
  body {{ background:#0b0e17; color:#e6ebf5; font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin:0; padding:32px; }}
  .kpis {{ display:flex; gap:16px; margin:20px 0; flex-wrap:wrap; }}
  .kpi {{ background:#161b22; border:1px solid #30363d; border-radius:10px; padding:14px 18px; min-width:160px; }}
  .kpi .label {{ font-size:12px; color:#8b949e; }}
  .kpi .value {{ font-size:22px; font-weight:600; margin-top:4px; }}
  .stats {{ margin-top:12px; font-size:14px; line-height:1.6; color:#c9d1e0; max-width:900px; }}
  .stats b {{ color:#fff; }}
  #view {{ max-width: 740px; }}
  details {{ margin-top:20px; max-width:900px; }}
  summary {{ cursor:pointer; color:#aab4c8; font-size:13px; }}
  pre.sql {{ margin-top:10px; padding:12px 14px; background:#0b0e17; border:1px solid #232a3d; border-radius:8px;
             font-family: "SF Mono", Menlo, Consolas, monospace; font-size:12px; line-height:1.5; color:#a3b3cc;
             white-space:pre-wrap; overflow-x:auto; }}
</style>
</head>
<body>
  <div id="view"></div>
  <div class="kpis">
    <div class="kpi"><div class="label">Engineers active</div><div class="value">{engineer_count}</div></div>
    <div class="kpi"><div class="label">Median reviews/engineer</div><div class="value">{median}</div></div>
    <div class="kpi"><div class="label">Top contributor's share</div><div class="value">{top_share:.0f}%</div></div>
  </div>
  <div class="stats">
    <b>{html.escape(top_name)}</b> completed the most reviews (<b>{top_n}</b> of <b>{total_reviews}</b> total,
    <b>{top_share:.0f}%</b> of all activity).<br>
    {"Usage is broad-based: the median engineer completed " + str(median) + " reviews." if median >= 5 else "Usage is concentrated: over half of engineers completed fewer than 5 reviews."}
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
    print(f"engineers={engineer_count} median={median} top={top_name}({top_n}, {top_share:.1f}%)")


if __name__ == "__main__":
    main()
