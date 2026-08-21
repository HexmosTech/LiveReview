#!/usr/bin/env python3
"""
interpretation.py — dbctx + Gemini → SQL + Vega-Lite charts.

Pipeline:
  1. User query → dbctx query → schema context
  2. Schema context + chart types + query → Gemini → JSON with SQL + chart specs
  3. Post-process: skip weak charts, smart-aggregate time series, enforce diversity
  4. Execute SQL → plug data → render PNG via vl-convert

Usage:
    python3 interpretation.py                          # default query
    python3 interpretation.py "Your custom query here" # custom query
    python3 interpretation.py --no-render              # skip chart rendering
"""

import json
import os
import subprocess
import sys
import urllib.request
import urllib.error
from collections import Counter
from datetime import date, datetime, timedelta
from decimal import Decimal
from pathlib import Path

import psycopg2
import vl_convert as vlc

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

SCRIPT_DIR = Path(__file__).parent
PROJECT_ROOT = SCRIPT_DIR.parent.parent
DTX_PATH = PROJECT_ROOT / "livereviewctx.dtx"
CHART_TYPES_PATH = SCRIPT_DIR / "chart_types.json"

GEMINI_MODEL = "gemini-2.5-flash"
GEMINI_ENDPOINT = (
    "https://generativelanguage.googleapis.com/v1beta/models/"
    f"{GEMINI_MODEL}:generateContent"
)

DEFAULT_QUERY = "How broadly has the organization adopted LiveReview?"

# All queries run within this org context
ORG_ID = 677
ORG_NAME = "Ostrelle Systems"

# ---------------------------------------------------------------------------
# System prompt — no hardcoded table names, dbctx provides schema
# ---------------------------------------------------------------------------

SYSTEM_PROMPT = f"""\
You are a database-aware analytics interpreter for LiveReview, an AI-powered code review SaaS.

## Task
1. You receive a user query + dbctx schema context (tables, columns, foreign keys, field stats, sample values).
2. You produce a JSON object with interpretations, each containing a SQL query and a Vega-Lite chart spec.

## Org context
- All queries run within org_id = {ORG_ID} ("{ORG_NAME}").
- Every SQL MUST include `WHERE org_id = {ORG_ID}` or join through a table that has org_id.
- Never run a global query without org filtering.

## Output format
- Respond with ONLY valid JSON, no markdown, no code fences.
- Schema:
  {{
    "query": "<original user query>",
    "interpretation": "<1-2 sentence restatement>",
    "interpretations": [
      {{
        "name": "<short name>",
        "description": "<what this shows>",
        "chart_type": "<chart type ID>",
        "sql": "<PostgreSQL query>",
        "vega_lite_spec": {{ <Vega-Lite spec with DATA_PLACEHOLDER in data.values> }}
      }}
    ]
  }}

## Rules — schema
1. Use ONLY tables and fields from the dbctx context. Never invent columns.
2. Every field must be table-qualified (e.g. `reviews.status`).
3. Include filters where relevant (e.g. `status = 'completed'`).

## Rules — SQL
4. Valid PostgreSQL only.
5. Column aliases must match the Vega-Lite encoding field names exactly.
6. Use meaningful aliases (e.g. `COUNT(*) AS review_count`).
7. Limit results reasonably (TOP 20 for rankings).

## Rules — data quality (CRITICAL)
8. NEVER return single-number results. Every query must return multiple rows with a dimension (time, category, name) for comparison. A query returning 1 row is a failure.
9. For time series: always fetch at DAY granularity — `DATE_TRUNC('day', created_at) AS day`. The pipeline will re-aggregate to the right level based on data density. Do NOT aggregate by month or week yourself.
10. For small result sets (under 10 items), return the actual items (names, labels, details) not just counts. Example: instead of `COUNT(repositories) = 2`, return each repository's `name, provider, created_at`.
11. Prefer queries that reveal patterns: rankings, trends, distributions, comparisons. Avoid flat counts.

## Rules — chart selection
12. Pick the chart type whose `use_when` best matches the data shape.
13. Vary chart types across interpretations — never use the same chart type twice in one response.
14. The `vega_lite_spec` must be a complete valid Vega-Lite spec.
15. Use `DATA_PLACEHOLDER` as the value of `data.values`.
16. Field names in encoding must match SQL column aliases exactly.

## Rules — how many interpretations
17. Specific query → 1-2 interpretations.
18. Broad query → 3-5 interpretations covering different angles.
19. Never exceed 5.
"""

# ---------------------------------------------------------------------------
# Helpers — config loading
# ---------------------------------------------------------------------------

def load_api_key() -> str:
    env_path = SCRIPT_DIR / ".env"
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line.startswith("GEMINI_API_KEY="):
            return line.split("=", 1)[1].strip()
    print("Error: GEMINI_API_KEY not found in .env", file=sys.stderr)
    sys.exit(1)


def load_chart_types() -> str:
    return CHART_TYPES_PATH.read_text()


def query_dbctx(query: str) -> str:
    result = subprocess.run(
        ["dbctx", "query", str(DTX_PATH), query],
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode != 0:
        print(f"dbctx error: {result.stderr}", file=sys.stderr)
        sys.exit(1)
    return result.stdout


# ---------------------------------------------------------------------------
# Helpers — Gemini
# ---------------------------------------------------------------------------

def call_gemini(api_key: str, user_query: str, dbctx_context: str, chart_types: str) -> dict:
    user_message = (
        f"User query: {user_query}\n"
        f"Org context: org_id = {ORG_ID} ({ORG_NAME})\n\n"
        f"--- dbctx schema context ---\n{dbctx_context}\n\n"
        f"--- available chart types ---\n{chart_types}"
    )

    payload = {
        "system_instruction": {"parts": [{"text": SYSTEM_PROMPT}]},
        "contents": [{"role": "user", "parts": [{"text": user_message}]}],
        "generationConfig": {
            "temperature": 0.2,
            "maxOutputTokens": 8192,
            "responseMimeType": "application/json"
        }
    }

    url = f"{GEMINI_ENDPOINT}?key={api_key}"
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method="POST")

    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            body = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace")
        print(f"Gemini API error {e.code}: {err_body}", file=sys.stderr)
        sys.exit(1)

    text = body["candidates"][0]["content"]["parts"][0]["text"]
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        cleaned = text.strip()
        if cleaned.startswith("```"):
            cleaned = "\n".join(cleaned.split("\n")[1:])
        if cleaned.endswith("```"):
            cleaned = cleaned.rsplit("```", 1)[0]
        return json.loads(cleaned.strip())


# ---------------------------------------------------------------------------
# Helpers — DB
# ---------------------------------------------------------------------------

def get_db_connection():
    dsn = os.environ.get("DATABASE_URL")
    if not dsn:
        env_path = PROJECT_ROOT / ".env.prod"
        if env_path.exists():
            for line in env_path.read_text().splitlines():
                if line.startswith("DATABASE_URL="):
                    dsn = line.split("=", 1)[1].strip()
                    break
    return psycopg2.connect(dsn)


def execute_sql(conn, sql: str) -> list[dict]:
    with conn.cursor() as cur:
        cur.execute(sql)
        columns = [desc[0] for desc in cur.description]
        rows = cur.fetchall()
        return [dict(zip(columns, row)) for row in rows]


# ---------------------------------------------------------------------------
# Helpers — post-processing
# ---------------------------------------------------------------------------

def json_default(obj):
    if isinstance(obj, (datetime, date)):
        return obj.isoformat()
    if isinstance(obj, Decimal):
        return float(obj)
    if isinstance(obj, timedelta):
        return obj.total_seconds()
    raise TypeError(f"Object of type {type(obj)} is not JSON serializable")


def smart_aggregate_time(rows: list[dict], time_field: str, value_fields: list[str], group_fields: list[str] = None) -> tuple[list[dict], str]:
    """Re-aggregate day-level time series data to the right granularity.

    group_fields: additional fields to group by (e.g. trigger_type for multi-series).
    Returns (aggregated_rows, time_unit).
    """
    if not rows or time_field not in rows[0]:
        return rows, "yearmonthdate"

    if group_fields is None:
        group_fields = []

    # Parse dates and find span
    dates = []
    for r in rows:
        d = r[time_field]
        if isinstance(d, str):
            d = datetime.fromisoformat(d).date()
        elif isinstance(d, datetime):
            d = d.date()
        dates.append(d)

    if not dates:
        return rows, "yearmonthdate"

    span_days = (max(dates) - min(dates)).days + 1
    unique_days = len(set(dates))

    # Pick granularity based on data density
    if span_days <= 14 or unique_days <= 7:
        return rows, "yearmonthdate"       # day level — fine as-is
    elif span_days <= 90:
        agg_unit = "yearweek"
    else:
        agg_unit = "yearmonth"

    # Re-aggregate — group by (time_bucket, group_fields)
    grouped = {}
    for r in rows:
        d = r[time_field]
        if isinstance(d, str):
            d = datetime.fromisoformat(d).date()
        elif isinstance(d, datetime):
            d = d.date()

        if agg_unit == "yearweek":
            time_key = d - timedelta(days=d.weekday())
        else:
            time_key = d.replace(day=1)

        # Build composite key: (time, group1, group2, ...)
        group_vals = tuple(r.get(gf) for gf in group_fields)
        key = (time_key,) + group_vals

        if key not in grouped:
            base = {time_field: time_key.isoformat()}
            for gf in group_fields:
                base[gf] = r.get(gf)
            for vf in value_fields:
                base[vf] = 0
            grouped[key] = base

        for vf in value_fields:
            val = r.get(vf, 0)
            if isinstance(val, Decimal):
                val = float(val)
            grouped[key][vf] += val if val else 0

    result = sorted(grouped.values(), key=lambda r: r[time_field])
    return result, agg_unit


def is_weak_result(rows: list[dict], interp: dict) -> bool:
    """Check if a result is too weak to chart."""
    if not rows:
        return True
    # Single row = single number, useless as chart
    if len(rows) == 1:
        return True
    return False


def ensure_diversity(interpretations: list[dict]) -> list[dict]:
    """If chart types repeat, try to swap one to a different type."""
    type_counts = Counter(i["chart_type"] for i in interpretations)
    if len(type_counts) == len(interpretations):
        return interpretations  # all unique, fine

    # Find duplicates and try to diversify
    seen = set()
    diversified = []
    for interp in interpretations:
        ct = interp["chart_type"]
        if ct in seen and type_counts[ct] > 1:
            # Try to swap to a complementary type
            swap_map = {
                "line": "bar",
                "bar": "horizontal_bar",
                "horizontal_bar": "bar",
                "pie": "bar",
                "area": "line",
                "stacked_bar": "bar",
                "scatter": "bar",
                "heatmap": "bar",
            }
            new_ct = swap_map.get(ct, "bar")
            interp["chart_type"] = new_ct
            # Update spec mark
            if "mark" in interp.get("vega_lite_spec", {}):
                mark = interp["vega_lite_spec"]["mark"]
                if isinstance(mark, str):
                    interp["vega_lite_spec"]["mark"] = new_ct.replace("_", " ").replace("horizontal bar", "bar")
                elif isinstance(mark, dict):
                    if new_ct == "horizontal_bar":
                        mark["type"] = "bar"
                    elif new_ct in ("bar", "line", "area"):
                        mark["type"] = new_ct
        seen.add(interp["chart_type"])
        diversified.append(interp)

    return diversified


def patch_spec_timeunit(spec: dict, time_field: str, time_unit: str) -> dict:
    """Update the Vega-Lite spec's timeUnit for the time field encoding."""
    spec_str = json.dumps(spec)
    # Replace any existing timeUnit for the time field
    # This is a simple string replacement approach
    if '"yearmonth"' in spec_str and time_unit != "yearmonth":
        spec_str = spec_str.replace('"yearmonth"', f'"{time_unit}"')
    elif '"yearweek"' in spec_str and time_unit != "yearweek":
        spec_str = spec_str.replace('"yearweek"', f'"{time_unit}"')
    return json.loads(spec_str)


# ---------------------------------------------------------------------------
# Helpers — rendering
# ---------------------------------------------------------------------------

def render_chart(spec: dict, data: list[dict], output_path: Path):
    spec_str = json.dumps(spec)
    spec_str = spec_str.replace('"DATA_PLACEHOLDER"', json.dumps(data, default=json_default))
    spec = json.loads(spec_str)
    png_bytes = vlc.vegalite_to_png(json.dumps(spec))
    output_path.write_bytes(png_bytes)


# ---------------------------------------------------------------------------
# Helpers — data stats
# ---------------------------------------------------------------------------

def _fmt_date(v):
    """Format a date/datetime value for display."""
    if isinstance(v, datetime):
        return v.strftime("%Y-%m-%d")
    if isinstance(v, date):
        return v.isoformat()
    if isinstance(v, str):
        # Strip timezone and time portion if midnight
        v = v.replace("+00:00", "").replace("T00:00:00", "")
        if len(v) > 10:
            v = v[:10]
        return v
    return str(v)


def _to_num(v):
    """Coerce a value to float for stats computation."""
    if v is None:
        return None
    if isinstance(v, Decimal):
        return float(v)
    if isinstance(v, (int, float)):
        return float(v)
    try:
        return float(v)
    except (ValueError, TypeError):
        return None


def compute_stats(rows: list[dict], interp: dict) -> list[str]:
    """Generate human-readable stat lines from the data rows."""
    if not rows:
        return ["No data returned."]

    lines = []
    keys = list(rows[0].keys())

    # Identify column roles
    time_field = None
    category_field = None
    numeric_fields = []

    encoding = interp.get("vega_lite_spec", {}).get("encoding", {})
    for _, enc in encoding.items():
        if not isinstance(enc, dict):
            continue
        f = enc.get("field")
        t = enc.get("type")
        if t == "temporal" and f in keys:
            time_field = f
        elif t == "nominal" and f in keys:
            category_field = f
        elif t == "quantitative" and f in keys:
            numeric_fields.append(f)

    # Fallback: guess from data
    if not numeric_fields:
        for k in keys:
            if all(_to_num(r.get(k)) is not None for r in rows[:3]):
                numeric_fields.append(k)
                break

    nf = numeric_fields[0] if numeric_fields else None

    # --- Time series stats ---
    if time_field and nf:
        vals = [(r.get(time_field), _to_num(r.get(nf))) for r in rows]
        vals = [(t, v) for t, v in vals if v is not None]
        if vals:
            total = sum(v for _, v in vals)
            avg = total / len(vals)
            max_row = max(vals, key=lambda x: x[1])
            min_row = min(vals, key=lambda x: x[1])

            lines.append(f"Total: {total:,.0f}")
            lines.append(f"Avg per period: {avg:,.1f}")
            lines.append(f"Peak: {max_row[1]:,.0f} ({_fmt_date(max_row[0])})")
            lines.append(f"Low: {min_row[1]:,.0f} ({_fmt_date(min_row[0])})")

            if len(vals) >= 2:
                first_v = vals[0][1]
                last_v = vals[-1][1]
                if first_v > 0:
                    change_pct = ((last_v - first_v) / first_v) * 100
                    direction = "up" if change_pct > 0 else "down" if change_pct < 0 else "flat"
                    lines.append(f"Trend: {direction} {abs(change_pct):.0f}% ({_fmt_date(vals[0][0])} → {_fmt_date(vals[-1][0])})")
            lines.append(f"Data points: {len(vals)}")

    # --- Category / ranking stats ---
    elif category_field and nf:
        vals = [(r.get(category_field), _to_num(r.get(nf))) for r in rows]
        vals = [(c, v) for c, v in vals if v is not None]
        if vals:
            total = sum(v for _, v in vals)
            avg = total / len(vals)
            top = max(vals, key=lambda x: x[1])
            bottom = min(vals, key=lambda x: x[1])

            lines.append(f"Total: {total:,.0f}")
            lines.append(f"Across {len(vals)} categories")
            lines.append(f"Avg per category: {avg:,.1f}")
            lines.append(f"Highest: {top[0]} ({top[1]:,.0f})")
            lines.append(f"Lowest: {bottom[0]} ({bottom[1]:,.0f})")

            if len(vals) >= 3:
                sorted_vals = sorted(vals, key=lambda x: x[1], reverse=True)
                top3 = ", ".join(f"{c} ({v:,.0f})" for c, v in sorted_vals[:3])
                lines.append(f"Top 3: {top3}")

    # --- Generic fallback ---
    else:
        lines.append(f"{len(rows)} rows returned")
        # Show first few rows as a preview
        for r in rows[:5]:
            parts = [f"{k}={v}" for k, v in r.items() if v is not None]
            lines.append("  " + ", ".join(parts[:4]))

    return lines


# ---------------------------------------------------------------------------
# Helpers — HTML generation
# ---------------------------------------------------------------------------

def generate_html(slides: list[dict], query: str, run_dir: Path):
    """Generate a single-page HTML with a slider and stats under each chart.

    Each slide dict has: name, description, image_filename, stats (list of str).
    """
    html = f"""\
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{query}</title>
<style>
  * {{ margin: 0; padding: 0; box-sizing: border-box; }}
  body {{ font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
         background: #0f172a; color: #e2e8f0; min-height: 100vh; }}
  .header {{ text-align: center; padding: 2rem 1rem 1rem; }}
  .header h1 {{ font-size: 1.5rem; font-weight: 600; color: #f8fafc; }}
  .header .sub {{ color: #94a3b8; font-size: 0.9rem; margin-top: 0.5rem; }}
  .carousel {{ position: relative; max-width: 900px; margin: 1.5rem auto; }}
  .slide {{ display: none; text-align: center; }}
  .slide.active {{ display: block; }}
  .slide img {{ max-width: 100%; max-height: 500px; border-radius: 8px;
                border: 1px solid #334155; background: #1e293b; }}
  .slide h2 {{ margin: 1rem 0 0.3rem; font-size: 1.2rem; color: #f1f5f9; }}
  .slide .desc {{ color: #94a3b8; font-size: 0.85rem; margin-bottom: 1rem; }}
  .stats {{ display: flex; flex-wrap: wrap; justify-content: center; gap: 0.6rem;
            max-width: 800px; margin: 0 auto 1.5rem; padding: 0 1rem; }}
  .stat {{ background: #1e293b; border: 1px solid #334155; border-radius: 6px;
           padding: 0.5rem 1rem; font-size: 0.85rem; color: #cbd5e1; }}
  .nav {{ display: flex; justify-content: center; align-items: center; gap: 1rem;
          padding: 1rem; }}
  .nav button {{ background: #334155; color: #e2e8f0; border: none; border-radius: 6px;
                 padding: 0.6rem 1.2rem; cursor: pointer; font-size: 1rem; }}
  .nav button:hover {{ background: #475569; }}
  .nav .counter {{ color: #94a3b8; font-size: 0.9rem; }}
  .dots {{ display: flex; justify-content: center; gap: 0.5rem; padding: 0.5rem; }}
  .dot {{ width: 10px; height: 10px; border-radius: 50%; background: #334155;
          cursor: pointer; transition: background 0.2s; }}
  .dot.active {{ background: #60a5fa; }}
</style>
</head>
<body>
<div class="header">
  <h1>{query}</h1>
  <div class="sub">{ORG_NAME} &middot; {len(slides)} charts</div>
</div>
<div class="carousel">
"""
    for i, s in enumerate(slides):
        active = "active" if i == 0 else ""
        stats_html = "".join(f'<div class="stat">{st}</div>' for st in s["stats"])
        html += f"""\
  <div class="slide {active}">
    <img src="{s['image']}" alt="{s['name']}">
    <h2>{s['name']}</h2>
    <div class="desc">{s['description']}</div>
    <div class="stats">{stats_html}</div>
  </div>
"""

    html += """\
</div>
<div class="dots">
"""
    for i in range(len(slides)):
        active = "active" if i == 0 else ""
        html += f'  <div class="dot {active}" onclick="go({i})"></div>\n'

    html += f"""\
</div>
<div class="nav">
  <button onclick="go(current-1)">&larr; Prev</button>
  <span class="counter"><span id="cur">1</span> / {len(slides)}</span>
  <button onclick="go(current+1)">Next &rarr;</button>
</div>
<script>
let current = 0;
const total = {len(slides)};
function go(n) {{
  document.querySelectorAll('.slide')[current].classList.remove('active');
  document.querySelectorAll('.dot')[current].classList.remove('active');
  current = ((n % total) + total) % total;
  document.querySelectorAll('.slide')[current].classList.add('active');
  document.querySelectorAll('.dot')[current].classList.add('active');
  document.getElementById('cur').textContent = current + 1;
}}
document.addEventListener('keydown', e => {{
  if (e.key === 'ArrowLeft') go(current - 1);
  if (e.key === 'ArrowRight') go(current + 1);
}});
</script>
</body>
</html>
"""
    (run_dir / "index.html").write_text(html)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    query = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_QUERY
    no_render = "--no-render" in sys.argv

    api_key = load_api_key()
    chart_types = load_chart_types()

    print(f"Query: {query}", file=sys.stderr)
    print(f"Org:   {ORG_NAME} (id={ORG_ID})", file=sys.stderr)
    print(f"Model: {GEMINI_MODEL}", file=sys.stderr)

    # Step 1: dbctx context
    print("Querying dbctx...", file=sys.stderr)
    dbctx_context = query_dbctx(query)
    print(f"dbctx returned {len(dbctx_context)} chars", file=sys.stderr)

    # Step 2: Gemini
    print("Calling Gemini...", file=sys.stderr)
    result = call_gemini(api_key, query, dbctx_context, chart_types)

    interpretations = result.get("interpretations", [])
    print(f"Gemini returned {len(interpretations)} interpretations", file=sys.stderr)

    if no_render:
        print(json.dumps(result, indent=2, default=json_default))
        return

    # Step 3: Execute SQL, post-process, render
    run_dir = SCRIPT_DIR / "output" / datetime.now().strftime("run_%Y%m%d_%H%M%S")
    run_dir.mkdir(parents=True, exist_ok=True)

    # Save the raw Gemini output
    (run_dir / "gemini_output.json").write_text(json.dumps(result, indent=2, default=json_default))

    conn = get_db_connection()
    slides = []

    for i, interp in enumerate(interpretations):
        name = interp.get("name", f"chart_{i}")
        description = interp.get("description", "")
        sql = interp.get("sql")
        spec = interp.get("vega_lite_spec")

        if not sql or not spec:
            print(f"  [{i+1}] SKIP {name}: missing sql or spec", file=sys.stderr)
            continue

        safe_name = name.lower().replace(" ", "_").replace("/", "_")[:40]
        print(f"  [{i+1}] {name}", file=sys.stderr)

        try:
            rows = execute_sql(conn, sql)
            print(f"      SQL → {len(rows)} rows", file=sys.stderr)

            # Check if weak result
            if is_weak_result(rows, interp):
                print(f"      SKIP: single-number / weak result", file=sys.stderr)
                continue

            # Smart time aggregation if this is a time series
            time_field = None
            value_fields = []
            group_fields = []
            encoding = spec.get("encoding", {})
            for field_name, enc in encoding.items():
                if not isinstance(enc, dict):
                    continue  # skip lists (e.g. tooltip)
                if enc.get("type") == "temporal":
                    time_field = enc.get("field")
                elif enc.get("type") == "quantitative":
                    value_fields.append(enc.get("field"))
                elif enc.get("type") == "nominal" and field_name == "color":
                    group_fields.append(enc.get("field"))

            if time_field and value_fields:
                rows, time_unit = smart_aggregate_time(rows, time_field, value_fields, group_fields)
                print(f"      Aggregated to {time_unit}, {len(rows)} points", file=sys.stderr)

            # Render chart
            output_path = run_dir / f"{safe_name}.png"
            render_chart(spec, rows, output_path)
            print(f"      → {output_path.name}", file=sys.stderr)

            # Compute stats
            stats = compute_stats(rows, interp)

            slides.append({
                "name": name,
                "description": description,
                "image": f"{safe_name}.png",
                "stats": stats,
            })

        except Exception as e:
            print(f"      Error: {e}", file=sys.stderr)

    conn.close()

    # Generate HTML slider
    if slides:
        generate_html(slides, query, run_dir)
        print(f"\nHTML: {run_dir / 'index.html'}", file=sys.stderr)

    # Print summary
    print(f"Kept {len(slides)} charts in {run_dir}/", file=sys.stderr)
    for s in slides:
        print(f"  - {s['name']}", file=sys.stderr)

    # Print JSON to stdout too
    print(json.dumps(result, indent=2, default=json_default))


if __name__ == "__main__":
    main()
