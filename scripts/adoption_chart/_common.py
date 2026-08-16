"""Shared boilerplate for every generate_*.py script in this folder: env/DB
loading, org resolution, and the dark-themed HTML page wrapper (CDN scripts
pinned with the same SRI hashes throughout, so semgrep's missing-integrity
rule never fires on anything this module produces).

Not a chart script itself - has no __main__/argparse of its own.
"""
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

_VEGA_CDN = """<script src="https://cdn.jsdelivr.net/npm/vega@5.33.1" integrity="sha384-NMXhl2TbCXxcN7o4ROC56Funm78m4AylL8gMg/7Kn4YU+wrm23K9l7cY8lDRXQ9d" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-lite@5.23.0" integrity="sha384-D9LYH0esGjcxQJsBuxOuXtCDJGXRWW1+KhluzWPqi0rLJmiR/ygPChefaD+rFFDQ" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-embed@6.29.0" integrity="sha384-M+Ax7e/WFJpxSOF09HzI+Sj4wg9ottVd/uxmV2ItGGh02fLH28t2FAOJx3TJBap5" crossorigin="anonymous"></script>"""


def load_database_url(env_path: Path = None) -> str:
    env_path = env_path or (REPO_ROOT / ".env")
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
        sys.exit(f"psql query failed:\n{result.stderr}\n\nSQL:\n{sql}")
    reader = csv.DictReader(io.StringIO(result.stdout))
    return list(reader)


def resolve_org_id(database_url: str, org_name: str) -> int:
    rows = run_query(database_url, f"SELECT id FROM orgs WHERE name = '{org_name}'")
    if not rows:
        sys.exit(f"no org named {org_name!r} found")
    return int(rows[0]["id"])


def wrap_page(*, title: str, spec: dict, stats_html: str, sql: str, out_path: Path,
              view_max_width: int = 900) -> None:
    """Writes a self-contained dark-themed HTML page embedding one Vega-Lite
    spec via vega-embed, a stats block, and a collapsible "Query used"
    section with the real SQL that produced it - the same shape every
    generate_*.py script in this folder already uses by hand; this just
    keeps that shape in one place instead of copy-pasted 20+ times.
    """
    page_html = f"""<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>{html.escape(title)}</title>
{_VEGA_CDN}
<style>
  body {{ background:#0b0e17; color:#e6ebf5; font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin:0; padding:32px; }}
  .stats {{ margin-top:20px; font-size:14px; line-height:1.6; color:#c9d1e0; max-width:900px; }}
  .stats b {{ color:#fff; }}
  #view {{ max-width: {view_max_width}px; overflow-x:auto; }}
  details {{ margin-top:20px; max-width:900px; }}
  summary {{ cursor:pointer; color:#aab4c8; font-size:13px; }}
  pre.sql {{ margin-top:10px; padding:12px 14px; background:#0b0e17; border:1px solid #232a3d; border-radius:8px;
             font-family: "SF Mono", Menlo, Consolas, monospace; font-size:12px; line-height:1.5; color:#a3b3cc;
             white-space:pre-wrap; overflow-x:auto; }}
</style>
</head>
<body>
  <div id="view"></div>
  <div class="stats">{stats_html}</div>
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
    out_path.write_text(page_html)
    print(f"wrote {out_path}")
