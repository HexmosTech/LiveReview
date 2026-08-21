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

import csv
import io
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


def load_dotenv(env_path: Path) -> None:
    """Populate os.environ from a simple KEY=VALUE .env file. Real
    environment variables (already set before this runs) always win -
    matches standard dotenv precedence. Blank lines and '#' comments are
    skipped; keys/values are whitespace-trimmed.
    """
    if not env_path.exists():
        return
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip()
        if key and key not in os.environ:
            os.environ[key] = value


load_dotenv(SCRIPT_DIR / ".env")

GEMINI_MODEL = "gemini-2.5-flash"
GEMINI_ENDPOINT = (
    "https://generativelanguage.googleapis.com/v1beta/models/"
    f"{GEMINI_MODEL}:generateContent"
)

# Atlas Cloud is OpenAI-compatible (see internal/aiconnectors/connector.go's
# createAtlasModel and models_sync.go's default model for the Go side of
# this same provider).
ATLAS_MODEL = "google/gemini-2.5-flash"
ATLAS_ENDPOINT = "https://api.atlascloud.ai/v1/chat/completions"

PROVIDER = os.environ.get("PROVIDER", "gemini").strip().lower() or "gemini"

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

def load_api_keys() -> list[str]:
    env_path = SCRIPT_DIR / ".env"
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line.startswith("GEMINI_API_KEY="):
            raw = line.split("=", 1)[1].strip()
            return [k.strip() for k in raw.split(",") if k.strip()]
    print("Error: GEMINI_API_KEY not found in .env", file=sys.stderr)
    sys.exit(1)


def load_atlas_api_key() -> str:
    api_key = os.environ.get("ATLAS_API_KEY", "").strip()
    if not api_key:
        print("Error: ATLAS_API_KEY not set (required when PROVIDER=atlas)", file=sys.stderr)
        sys.exit(1)
    return api_key


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

def call_gemini(api_keys: list[str], user_message: str) -> tuple[dict, dict]:
    """Call Gemini and return (parsed_json, raw_response_body).
    Tries each key in order; skips on 429 quota error; retries on 503.
    """
    import time

    payload = {
        "system_instruction": {"parts": [{"text": SYSTEM_PROMPT}]},
        "contents": [{"role": "user", "parts": [{"text": user_message}]}],
        "generationConfig": {
            "temperature": 0.2,
            "maxOutputTokens": 8192,
            "responseMimeType": "application/json"
        }
    }

    last_error = None
    for i, api_key in enumerate(api_keys):
        for attempt in range(3):  # up to 3 attempts per key
            url = f"{GEMINI_ENDPOINT}?key={api_key}"
            data = json.dumps(payload).encode("utf-8")
            req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method="POST")

            try:
                with urllib.request.urlopen(req, timeout=120) as resp:
                    body = json.loads(resp.read().decode("utf-8"))
            except urllib.error.HTTPError as e:
                err_body = e.read().decode("utf-8", errors="replace")
                if e.code == 429:
                    print(f"  Key {i+1}/{len(api_keys)} quota exhausted, trying next...", file=sys.stderr)
                    last_error = err_body
                    break  # try next key
                if e.code == 503 and attempt < 2:
                    print(f"  Key {i+1}/{len(api_keys)} got 503, retrying in 2s...", file=sys.stderr)
                    time.sleep(2)
                    continue
                print(f"Gemini API error {e.code}: {err_body}", file=sys.stderr)
                sys.exit(1)

            text = body["candidates"][0]["content"]["parts"][0]["text"]
            try:
                parsed = json.loads(text)
            except json.JSONDecodeError:
                cleaned = text.strip()
                if cleaned.startswith("```"):
                    cleaned = "\n".join(cleaned.split("\n")[1:])
                if cleaned.endswith("```"):
                    cleaned = cleaned.rsplit("```", 1)[0]
                parsed = json.loads(cleaned.strip())

            return parsed, body

    print(f"All {len(api_keys)} keys exhausted. Last error: {last_error}", file=sys.stderr)
    sys.exit(1)


def call_atlas(api_key: str, user_message: str) -> tuple[dict, dict]:
    """Call Atlas Cloud (OpenAI-compatible chat completions) and return
    (parsed_json, raw_response_body). Retries on 503, same as call_gemini.
    """
    import time

    payload = {
        "model": ATLAS_MODEL,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": user_message},
        ],
        "temperature": 0.2,
        "max_tokens": 8192,
        "response_format": {"type": "json_object"},
    }

    data = json.dumps(payload).encode("utf-8")
    last_error = None
    for attempt in range(3):
        req = urllib.request.Request(
            ATLAS_ENDPOINT,
            data=data,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {api_key}",
                "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) livereview-prelivi/1.0",
                "Accept": "application/json",
            },
            method="POST",
        )

        try:
            with urllib.request.urlopen(req, timeout=120) as resp:
                body = json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            err_body = e.read().decode("utf-8", errors="replace")
            if e.code == 503 and attempt < 2:
                print(f"  Atlas Cloud got 503, retrying in 2s...", file=sys.stderr)
                last_error = err_body
                time.sleep(2)
                continue
            print(f"Atlas Cloud API error {e.code}: {err_body}", file=sys.stderr)
            sys.exit(1)

        text = body["choices"][0]["message"]["content"]
        try:
            parsed = json.loads(text)
        except json.JSONDecodeError:
            cleaned = text.strip()
            if cleaned.startswith("```"):
                cleaned = "\n".join(cleaned.split("\n")[1:])
            if cleaned.endswith("```"):
                cleaned = cleaned.rsplit("```", 1)[0]
            parsed = json.loads(cleaned.strip())

        return parsed, body

    print(f"Atlas Cloud request failed after retries: {last_error}", file=sys.stderr)
    sys.exit(1)


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
# Helpers — serialization
# ---------------------------------------------------------------------------

def json_default(obj):
    if isinstance(obj, (datetime, date)):
        return obj.isoformat()
    if isinstance(obj, Decimal):
        return float(obj)
    if isinstance(obj, timedelta):
        return obj.total_seconds()
    raise TypeError(f"Object of type {type(obj)} is not JSON serializable")


def rows_to_csv(rows: list[dict]) -> str:
    """Convert list of dicts to CSV string."""
    if not rows:
        return ""
    buf = io.StringIO()
    writer = csv.DictWriter(buf, fieldnames=list(rows[0].keys()))
    writer.writeheader()
    for r in rows:
        clean = {}
        for k, v in r.items():
            if isinstance(v, Decimal):
                clean[k] = float(v)
            elif isinstance(v, (datetime, date)):
                clean[k] = v.isoformat()
            else:
                clean[k] = v
        writer.writerow(clean)
    return buf.getvalue()


# ---------------------------------------------------------------------------
# Helpers — post-processing
# ---------------------------------------------------------------------------

def smart_aggregate_time(rows: list[dict], time_field: str, value_fields: list[str], group_fields: list[str] = None) -> tuple[list[dict], str]:
    """Re-aggregate day-level time series data to the right granularity."""
    if not rows or time_field not in rows[0]:
        return rows, "yearmonthdate"

    if group_fields is None:
        group_fields = []

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

    if span_days <= 14 or unique_days <= 7:
        # Day level — strip timezone for Vega compatibility
        for r in rows:
            v = r.get(time_field)
            if isinstance(v, str):
                r[time_field] = v.replace("+00:00", "")
            elif isinstance(v, datetime):
                r[time_field] = v.strftime("%Y-%m-%dT%H:%M:%S")
        return rows, "yearmonthdate"
    elif span_days <= 90:
        agg_unit = "yearweek"
    else:
        agg_unit = "yearmonth"

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
    if not rows:
        return True
    if len(rows) == 1:
        return True
    return False


def ensure_diversity(interpretations: list[dict]) -> list[dict]:
    type_counts = Counter(i["chart_type"] for i in interpretations)
    if len(type_counts) == len(interpretations):
        return interpretations

    seen = set()
    diversified = []
    for interp in interpretations:
        ct = interp["chart_type"]
        if ct in seen and type_counts[ct] > 1:
            swap_map = {
                "line": "bar", "bar": "horizontal_bar", "horizontal_bar": "bar",
                "pie": "bar", "area": "line", "stacked_bar": "bar",
                "scatter": "bar", "heatmap": "bar",
            }
            new_ct = swap_map.get(ct, "bar")
            interp["chart_type"] = new_ct
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
    if isinstance(v, datetime):
        return v.strftime("%Y-%m-%d")
    if isinstance(v, date):
        return v.isoformat()
    if isinstance(v, str):
        v = v.replace("+00:00", "").replace("T00:00:00", "")
        if len(v) > 10:
            v = v[:10]
        return v
    return str(v)


def _to_num(v):
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
    if not rows:
        return ["No data returned."]

    lines = []
    keys = list(rows[0].keys())

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

    if not numeric_fields:
        for k in keys:
            if all(_to_num(r.get(k)) is not None for r in rows[:3]):
                numeric_fields.append(k)
                break

    nf = numeric_fields[0] if numeric_fields else None

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

    else:
        lines.append(f"{len(rows)} rows returned")
        for r in rows[:5]:
            parts = [f"{k}={v}" for k, v in r.items() if v is not None]
            lines.append("  " + ", ".join(parts[:4]))

    return lines


# ---------------------------------------------------------------------------
# HTML generation — full pipeline view
# ---------------------------------------------------------------------------

def _esc(s: str) -> str:
    """HTML-escape a string."""
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;")


def generate_html(query: str, dbctx_context: str, user_message: str,
                  system_prompt: str, gemini_response: dict,
                  pipeline_steps: list[dict], run_dir: Path):
    """Generate full pipeline HTML.

    pipeline_steps: list of dicts with keys:
      name, description, chart_type, sql, raw_csv_path, agg_csv_path,
      stats, image_path, status, skip_reason
    """
    n_charts = sum(1 for s in pipeline_steps if s["status"] == "rendered")
    n_skipped = sum(1 for s in pipeline_steps if s["status"] == "skipped")

    # Build pipeline sections
    steps_html = ""
    for i, step in enumerate(pipeline_steps):
        status_badge = (
            f'<span class="badge ok">rendered</span>' if step["status"] == "rendered"
            else f'<span class="badge skip">skipped: {_esc(step.get("skip_reason", ""))}</span>'
        )

        sql_block = f'<pre class="code">{_esc(step["sql"])}</pre>' if step.get("sql") else ""

        raw_link = f'<a class="file-link" onclick="showCSV(\'{step["raw_csv_path"]}\')">{step["raw_csv_path"]}</a>' if step.get("raw_csv_path") else ""
        agg_link = f'<a class="file-link" onclick="showCSV(\'{step["agg_csv_path"]}\')">{step["agg_csv_path"]}</a>' if step.get("agg_csv_path") else ""
        img_tag = f'<img src="{step["image_path"]}" class="chart-img">' if step.get("image_path") else ""

        stats_html = ""
        if step.get("stats"):
            stats_html = '<div class="stats">' + "".join(f'<div class="stat">{_esc(s)}</div>' for s in step["stats"]) + '</div>'

        steps_html += f"""
<div class="step">
  <div class="step-header">
    <span class="step-num">{i+1}</span>
    <h3>{_esc(step["name"])}</h3>
    {status_badge}
    <span class="chart-type">{_esc(step.get("chart_type", ""))}</span>
  </div>
  <p class="step-desc">{_esc(step.get("description", ""))}</p>
  {sql_block}
  <div class="file-links">{raw_link} {agg_link}</div>
  {img_tag}
  {stats_html}
</div>
"""

    html = f"""\
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{_esc(query)}</title>
<style>
  * {{ margin: 0; padding: 0; box-sizing: border-box; }}
  body {{ font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
         background: #0f172a; color: #e2e8f0; }}
  .container {{ max-width: 960px; margin: 0 auto; padding: 1.5rem; }}

  /* Pipeline header */
  .pipeline-header {{ text-align: center; padding: 2rem 0 1rem; }}
  .pipeline-header h1 {{ font-size: 1.6rem; color: #f8fafc; }}
  .pipeline-header .meta {{ color: #94a3b8; font-size: 0.85rem; margin-top: 0.5rem; }}

  /* Pipeline flow */
  .flow {{ display: flex; flex-wrap: wrap; gap: 0.5rem; justify-content: center;
           margin: 1.5rem 0; }}
  .flow a {{ background: #1e293b; border: 1px solid #334155; border-radius: 6px;
             padding: 0.4rem 0.8rem; color: #60a5fa; text-decoration: none;
             font-size: 0.8rem; }}
  .flow a:hover {{ background: #334155; }}
  .flow .arrow {{ color: #475569; padding: 0.4rem 0; }}

  /* Collapsible sections */
  details {{ margin: 1rem 0; }}
  details summary {{ cursor: pointer; color: #94a3b8; font-size: 0.85rem;
                     padding: 0.5rem; background: #1e293b; border-radius: 6px;
                     border: 1px solid #334155; }}
  details summary:hover {{ background: #334155; }}
  details[open] summary {{ border-radius: 6px 6px 0 0; }}
  details .content {{ background: #1e293b; border: 1px solid #334155; border-top: none;
                      border-radius: 0 0 6px 6px; padding: 1rem; }}
  pre.code {{ background: #0f172a; border: 1px solid #334155; border-radius: 4px;
              padding: 0.8rem; font-size: 0.78rem; overflow-x: auto; color: #cbd5e1;
              white-space: pre-wrap; word-break: break-all; }}
  pre.code.large {{ max-height: 300px; overflow-y: auto; }}

  /* Steps */
  .step {{ background: #1e293b; border: 1px solid #334155; border-radius: 8px;
           padding: 1.2rem; margin: 1rem 0; }}
  .step-header {{ display: flex; align-items: center; gap: 0.8rem; flex-wrap: wrap; }}
  .step-num {{ background: #334155; color: #60a5fa; border-radius: 50%;
               width: 28px; height: 28px; display: flex; align-items: center;
               justify-content: center; font-size: 0.8rem; font-weight: 600; }}
  .step-header h3 {{ font-size: 1.05rem; color: #f1f5f9; flex: 1; }}
  .badge {{ font-size: 0.7rem; padding: 0.2rem 0.5rem; border-radius: 4px; }}
  .badge.ok {{ background: #064e3b; color: #6ee7b7; }}
  .badge.skip {{ background: #451a03; color: #fbbf24; }}
  .chart-type {{ font-size: 0.7rem; color: #64748b; background: #0f172a;
                 padding: 0.2rem 0.5rem; border-radius: 4px; }}
  .step-desc {{ color: #94a3b8; font-size: 0.85rem; margin: 0.5rem 0; }}
  .file-links {{ display: flex; gap: 0.8rem; margin: 0.5rem 0; }}
  .file-link {{ color: #60a5fa; text-decoration: none; font-size: 0.8rem;
                border: 1px solid #334155; padding: 0.2rem 0.6rem; border-radius: 4px;
                cursor: pointer; }}
  .file-link:hover {{ background: #334155; }}
  .chart-img {{ max-width: 100%; border-radius: 6px; margin: 0.8rem 0;
                border: 1px solid #334155; }}
  .stats {{ display: flex; flex-wrap: wrap; gap: 0.5rem; margin-top: 0.8rem; }}
  .stat {{ background: #0f172a; border: 1px solid #334155; border-radius: 4px;
           padding: 0.4rem 0.8rem; font-size: 0.8rem; color: #cbd5e1; }}

  /* CSV modal */
  .modal-overlay {{ display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%;
                    background: rgba(0,0,0,0.7); z-index: 1000; justify-content: center;
                    align-items: center; }}
  .modal-overlay.open {{ display: flex; }}
  .modal {{ background: #1e293b; border: 1px solid #334155; border-radius: 10px;
            max-width: 90vw; max-height: 85vh; display: flex; flex-direction: column; }}
  .modal-header {{ display: flex; justify-content: space-between; align-items: center;
                   padding: 0.8rem 1rem; border-bottom: 1px solid #334155; }}
  .modal-header h3 {{ font-size: 0.95rem; color: #f1f5f9; }}
  .modal-close {{ background: #334155; border: none; color: #e2e8f0; border-radius: 4px;
                  padding: 0.3rem 0.6rem; cursor: pointer; font-size: 0.85rem; }}
  .modal-close:hover {{ background: #475569; }}
  .modal-body {{ overflow: auto; padding: 0.8rem; }}
  .csv-table {{ border-collapse: collapse; font-size: 0.78rem; width: 100%; }}
  .csv-table th {{ background: #334155; color: #e2e8f0; padding: 0.4rem 0.6rem;
                   text-align: left; position: sticky; top: 0; white-space: nowrap; }}
  .csv-table td {{ padding: 0.35rem 0.6rem; border-bottom: 1px solid #1e293b;
                   color: #cbd5e1; white-space: nowrap; }}
  .csv-table tr:nth-child(even) td {{ background: rgba(255,255,255,0.02); }}
  .csv-table tr:hover td {{ background: rgba(96,165,250,0.08); }}
  .csv-meta {{ color: #64748b; font-size: 0.75rem; padding: 0.4rem 0.8rem;
               border-top: 1px solid #334155; text-align: right; }}
</style>
</head>
<body>
<div class="container">

<div class="pipeline-header">
  <h1>{_esc(query)}</h1>
  <div class="meta">{_esc(ORG_NAME)} (org_id={ORG_ID}) &middot; {n_charts} charts rendered, {n_skipped} skipped</div>
</div>

<div class="flow">
  <a href="01_query.txt">01 query</a><span class="arrow">&rarr;</span>
  <a href="02_dbctx_context.txt">02 dbctx</a><span class="arrow">&rarr;</span>
  <a href="03_prompt.txt">03 prompt</a><span class="arrow">&rarr;</span>
  <a href="04_gemini_response.json">04 gemini</a><span class="arrow">&rarr;</span>
  <span style="color:#94a3b8;font-size:0.8rem;padding:0.4rem">SQL &rarr; data &rarr; chart</span>
</div>

<details>
  <summary>01 &mdash; User query</summary>
  <div class="content"><pre class="code">{_esc(query)}</pre></div>
</details>

<details>
  <summary>02 &mdash; dbctx schema context ({len(dbctx_context):,} chars)</summary>
  <div class="content"><pre class="code large">{_esc(dbctx_context[:5000])}{"..." if len(dbctx_context) > 5000 else ""}</pre>
  <a class="file-link" href="02_dbctx_context.txt">full file</a></div>
</details>

<details>
  <summary>03 &mdash; Full prompt sent to Gemini ({len(system_prompt) + len(user_message):,} chars)</summary>
  <div class="content">
    <h4 style="color:#94a3b8;font-size:0.8rem;margin-bottom:0.5rem">System prompt</h4>
    <pre class="code large">{_esc(system_prompt)}</pre>
    <h4 style="color:#94a3b8;font-size:0.8rem;margin:1rem 0 0.5rem">User message</h4>
    <pre class="code large">{_esc(user_message[:3000])}{"..." if len(user_message) > 3000 else ""}</pre>
  </div>
</details>

<details>
  <summary>04 &mdash; Gemini response JSON</summary>
  <div class="content"><pre class="code large">{_esc(json.dumps(gemini_response, indent=2, default=json_default))}</pre></div>
</details>

<h2 style="color:#f1f5f9;font-size:1.2rem;margin:2rem 0 1rem">Interpretations</h2>

{steps_html}

</div>

<!-- CSV Modal -->
<div class="modal-overlay" id="csvModal" onclick="if(event.target===this)closeModal()">
  <div class="modal">
    <div class="modal-header">
      <h3 id="modalTitle">CSV</h3>
      <button class="modal-close" onclick="closeModal()">&times;</button>
    </div>
    <div class="modal-body" id="modalBody"></div>
    <div class="csv-meta" id="modalMeta"></div>
  </div>
</div>

<script>
function showCSV(path) {{
  fetch(path)
    .then(r => r.text())
    .then(text => {{
      document.getElementById('modalTitle').textContent = path;
      const lines = text.trim().split('\\n');
      if (lines.length === 0) return;
      const headers = parseCSVLine(lines[0]);
      let html = '<table class="csv-table"><thead><tr>';
      headers.forEach(h => html += '<th>' + escHTML(h) + '</th>');
      html += '</tr></thead><tbody>';
      for (let i = 1; i < lines.length; i++) {{
        const cells = parseCSVLine(lines[i]);
        html += '<tr>';
        cells.forEach(c => html += '<td>' + escHTML(c) + '</td>');
        html += '</tr>';
      }}
      html += '</tbody></table>';
      document.getElementById('modalBody').innerHTML = html;
      document.getElementById('modalMeta').textContent = (lines.length - 1) + ' rows, ' + headers.length + ' columns';
      document.getElementById('csvModal').classList.add('open');
    }});
}}
function closeModal() {{ document.getElementById('csvModal').classList.remove('open'); }}
document.addEventListener('keydown', e => {{ if (e.key === 'Escape') closeModal(); }});
function escHTML(s) {{ return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }}
function parseCSVLine(line) {{
  const result = [];
  let current = '';
  let inQuotes = false;
  for (let i = 0; i < line.length; i++) {{
    const c = line[i];
    if (c === '"') {{ inQuotes = !inQuotes; }}
    else if (c === ',' && !inQuotes) {{ result.push(current); current = ''; }}
    else {{ current += c; }}
  }}
  result.push(current);
  return result;
}}

// Carousel
let current = 0;
const total = {len(pipeline_steps)};
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

    if PROVIDER == "atlas":
        atlas_api_key = load_atlas_api_key()
    else:
        api_keys = load_api_keys()
    chart_types = load_chart_types()

    print(f"Query:    {query}", file=sys.stderr)
    print(f"Org:      {ORG_NAME} (id={ORG_ID})", file=sys.stderr)
    print(f"Provider: {PROVIDER}", file=sys.stderr)
    if PROVIDER == "atlas":
        print(f"Model:    {ATLAS_MODEL}", file=sys.stderr)
    else:
        print(f"Model:    {GEMINI_MODEL}", file=sys.stderr)
        print(f"Keys:     {len(api_keys)} configured", file=sys.stderr)

    # Step 1: dbctx context
    print("Querying dbctx...", file=sys.stderr)
    dbctx_context = query_dbctx(query)
    print(f"dbctx returned {len(dbctx_context)} chars", file=sys.stderr)

    # Step 2: Build prompt
    user_message = (
        f"User query: {query}\n"
        f"Org context: org_id = {ORG_ID} ({ORG_NAME})\n\n"
        f"--- dbctx schema context ---\n{dbctx_context}\n\n"
        f"--- available chart types ---\n{chart_types}"
    )

    # Step 3: Call the LLM
    if PROVIDER == "atlas":
        print("Calling Atlas Cloud...", file=sys.stderr)
        result, raw_body = call_atlas(atlas_api_key, user_message)
    else:
        print("Calling Gemini...", file=sys.stderr)
        result, raw_body = call_gemini(api_keys, user_message)

    interpretations = result.get("interpretations", [])
    print(f"{PROVIDER} returned {len(interpretations)} interpretations", file=sys.stderr)

    if no_render:
        print(json.dumps(result, indent=2, default=json_default))
        return

    # Step 4: Create run directory and save artifacts
    run_dir = SCRIPT_DIR / "output" / datetime.now().strftime("run_%Y%m%d_%H%M%S")
    run_dir.mkdir(parents=True, exist_ok=True)

    (run_dir / "01_query.txt").write_text(query)
    (run_dir / "02_dbctx_context.txt").write_text(dbctx_context)
    (run_dir / "03_prompt.txt").write_text(f"=== SYSTEM PROMPT ===\n{SYSTEM_PROMPT}\n\n=== USER MESSAGE ===\n{user_message}")
    (run_dir / "04_gemini_response.json").write_text(json.dumps(result, indent=2, default=json_default))

    # Step 5: Execute SQL, post-process, render
    conn = get_db_connection()
    pipeline_steps = []

    for i, interp in enumerate(interpretations):
        name = interp.get("name", f"chart_{i}")
        description = interp.get("description", "")
        chart_type = interp.get("chart_type", "")
        sql = interp.get("sql", "")
        spec = interp.get("vega_lite_spec", {})
        safe_name = name.lower().replace(" ", "_").replace("/", "_")[:40]
        prefix = f"05_{i+1:03d}_{safe_name}"

        step = {
            "name": name,
            "description": description,
            "chart_type": chart_type,
            "sql": sql,
            "raw_csv_path": None,
            "agg_csv_path": None,
            "stats": None,
            "image_path": None,
            "status": "skipped",
            "skip_reason": "",
        }

        # Save SQL
        if sql:
            (run_dir / f"{prefix}.sql").write_text(sql)

        if not sql or not spec:
            step["skip_reason"] = "missing sql or spec"
            pipeline_steps.append(step)
            print(f"  [{i+1}] SKIP {name}: missing sql or spec", file=sys.stderr)
            continue

        print(f"  [{i+1}] {name}", file=sys.stderr)

        try:
            rows = execute_sql(conn, sql)
            print(f"      SQL → {len(rows)} rows", file=sys.stderr)

            # Save raw CSV
            raw_csv = rows_to_csv(rows)
            raw_csv_path = f"{prefix}_raw.csv"
            (run_dir / raw_csv_path).write_text(raw_csv)
            step["raw_csv_path"] = raw_csv_path

            # Check if weak result
            if is_weak_result(rows, interp):
                step["skip_reason"] = f"single-row result ({len(rows)} rows)"
                pipeline_steps.append(step)
                print(f"      SKIP: single-number / weak result", file=sys.stderr)
                continue

            # Smart time aggregation if this is a time series
            time_field = None
            value_fields = []
            group_fields = []
            encoding = spec.get("encoding", {})
            for field_name, enc in encoding.items():
                if not isinstance(enc, dict):
                    continue
                if enc.get("type") == "temporal":
                    time_field = enc.get("field")
                elif enc.get("type") == "quantitative":
                    value_fields.append(enc.get("field"))
                elif enc.get("type") == "nominal" and field_name == "color":
                    group_fields.append(enc.get("field"))

            if time_field and value_fields:
                rows, time_unit = smart_aggregate_time(rows, time_field, value_fields, group_fields)
                print(f"      Aggregated to {time_unit}, {len(rows)} points", file=sys.stderr)
                # Save aggregated CSV
                agg_csv = rows_to_csv(rows)
                agg_csv_path = f"{prefix}_agg.csv"
                (run_dir / agg_csv_path).write_text(agg_csv)
                step["agg_csv_path"] = agg_csv_path

            # Render chart
            png_path = f"{prefix}.png"
            render_chart(spec, rows, run_dir / png_path)
            step["image_path"] = png_path
            print(f"      → {png_path}", file=sys.stderr)

            # Compute stats
            step["stats"] = compute_stats(rows, interp)
            step["status"] = "rendered"

        except Exception as e:
            step["skip_reason"] = str(e)[:100]
            print(f"      Error: {e}", file=sys.stderr)

        pipeline_steps.append(step)

    conn.close()

    # Step 6: Generate HTML
    generate_html(query, dbctx_context, user_message, SYSTEM_PROMPT, result, pipeline_steps, run_dir)
    print(f"\nHTML: {run_dir / 'index.html'}", file=sys.stderr)

    n_charts = sum(1 for s in pipeline_steps if s["status"] == "rendered")
    print(f"Kept {n_charts} charts in {run_dir}/", file=sys.stderr)

    print(json.dumps(result, indent=2, default=json_default))


if __name__ == "__main__":
    main()
