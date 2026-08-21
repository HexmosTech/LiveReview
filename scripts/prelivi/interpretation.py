#!/usr/bin/env python3
"""
interpretation.py — dbctx + Gemini → SQL + Vega-Lite charts.

Pipeline:
  1. User query → dbctx query → schema context
  2. Schema context + chart types + query → Gemini → JSON with SQL + chart specs
  3. Execute SQL against production DB → data rows
  4. Plug data into Vega-Lite templates → render PNG via vl-convert

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
from datetime import date, datetime
from decimal import Decimal

from pathlib import Path

import psycopg2
import vl_convert as vlc

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------

SCRIPT_DIR = Path(__file__).parent
PROJECT_ROOT = SCRIPT_DIR.parent.parent
DTX_PATH = PROJECT_ROOT / "livereviewctx.dtx"
CHART_TYPES_PATH = SCRIPT_DIR / "chart_types.json"
OUTPUT_DIR = SCRIPT_DIR / "output"

# ---------------------------------------------------------------------------
# Gemini config
# ---------------------------------------------------------------------------

GEMINI_MODEL = "gemini-2.5-flash"
GEMINI_ENDPOINT = (
    "https://generativelanguage.googleapis.com/v1beta/models/"
    f"{GEMINI_MODEL}:generateContent"
)

DEFAULT_QUERY = "How broadly has the organization adopted LiveReview?"

# All queries run within this org context
ORG_ID = 677
ORG_NAME = "Ostrelle Systems"

SYSTEM_PROMPT = f"""\
You are a database-aware analytics interpreter for LiveReview, an AI-powered code review SaaS.

## Task
1. You receive a user's natural-language question followed by dbctx schema context (tables, columns, foreign keys, field stats, sample values).
2. You produce a JSON object with interpretations, each containing a SQL query and a Vega-Lite chart spec.

## Org context
- All queries run within a single organization: org_id = {ORG_ID} ("{ORG_NAME}").
- Every SQL query MUST include a `WHERE org_id = {ORG_ID}` filter (or join through a table that has org_id).
- Tables with direct org_id: reviews, repositories, user_roles, api_keys, loc_usage_ledger, scheduled_review_configs, review_events, review_feedback, recent_activity, webhook_registry, org_slack_configs, org_discord_configs, chat_conversations, ai_connectors, org_review_ai_settings, org_billing_state.
- Tables without org_id (join through related table): users (join via user_roles), subscriptions (has org_id), pull_requests (has org_id).
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
        "chart_type": "<one of the available chart type IDs>",
        "sql": "<PostgreSQL query that returns columns matching the chart template>",
        "vega_lite_spec": {{ <complete Vega-Lite spec with DATA_PLACEHOLDER in data.values> }}
      }}
    ]
  }}

## Metric validity rules
1. Use ONLY tables and fields present in the dbctx context — never invent columns.
2. Every field must be qualified with its table name (e.g. `reviews.status`).
3. Include filters where relevant (e.g. `status='completed'`, `is_active=true`).
4. Be specific about aggregation (COUNT DISTINCT, SUM, DATE_TRUNC, etc.).

## SQL rules
5. The SQL must be valid PostgreSQL.
6. Column aliases in the SQL must match the field names used in the Vega-Lite spec's encoding.
7. Use meaningful aliases (e.g. `COUNT(*) AS review_count`, not `count`).
8. For time series, use `DATE_TRUNC('month', created_at) AS month` and sort by it.
9. Limit results to a reasonable number (e.g. TOP 20 for rankings).

## Chart selection rules
10. Pick the chart type whose `use_when` best matches the data shape.
11. Available chart types and their templates are provided below.
12. The `vega_lite_spec` must be a complete valid Vega-Lite spec.
13. Use `DATA_PLACEHOLDER` as the value of `data.values` — the pipeline will replace it with actual rows.
14. Field names in the spec's encoding must match the SQL column aliases exactly.

## How many interpretations to generate
15. Match the user's intent, not a fixed count.
16. If the query is specific and unambiguous → 1-2 interpretations.
17. If the query is broad or ambiguous → 3-5 interpretations covering different angles.
18. Never exceed 5 interpretations.
"""


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def load_api_key() -> str:
    env_path = SCRIPT_DIR / ".env"
    if not env_path.exists():
        print(f"Error: {env_path} not found", file=sys.stderr)
        sys.exit(1)
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line.startswith("GEMINI_API_KEY="):
            return line.split("=", 1)[1].strip()
    print("Error: GEMINI_API_KEY not found in .env", file=sys.stderr)
    sys.exit(1)


def load_chart_types() -> str:
    if not CHART_TYPES_PATH.exists():
        print(f"Error: {CHART_TYPES_PATH} not found", file=sys.stderr)
        sys.exit(1)
    return CHART_TYPES_PATH.read_text()


def query_dbctx(query: str) -> str:
    if not DTX_PATH.exists():
        print(f"Error: dbctx index not found at {DTX_PATH}", file=sys.stderr)
        sys.exit(1)
    result = subprocess.run(
        ["dbctx", "query", str(DTX_PATH), query],
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode != 0:
        print(f"dbctx error: {result.stderr}", file=sys.stderr)
        sys.exit(1)
    return result.stdout


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
    except Exception as e:
        print(f"Request failed: {e}", file=sys.stderr)
        sys.exit(1)

    try:
        text = body["candidates"][0]["content"]["parts"][0]["text"]
    except (KeyError, IndexError) as e:
        print(f"Unexpected Gemini response: {e}", file=sys.stderr)
        sys.exit(1)

    try:
        return json.loads(text)
    except json.JSONDecodeError:
        cleaned = text.strip()
        if cleaned.startswith("```"):
            cleaned = "\n".join(cleaned.split("\n")[1:])
        if cleaned.endswith("```"):
            cleaned = cleaned.rsplit("```", 1)[0]
        return json.loads(cleaned.strip())


def get_db_connection():
    dsn = os.environ.get("DATABASE_URL")
    if not dsn:
        # Try loading from .env.prod
        env_path = PROJECT_ROOT / ".env.prod"
        if env_path.exists():
            for line in env_path.read_text().splitlines():
                if line.startswith("DATABASE_URL="):
                    dsn = line.split("=", 1)[1].strip()
                    break
    if not dsn:
        print("Error: DATABASE_URL not found", file=sys.stderr)
        sys.exit(1)
    return psycopg2.connect(dsn)


def execute_sql(conn, sql: str) -> list[dict]:
    with conn.cursor() as cur:
        cur.execute(sql)
        columns = [desc[0] for desc in cur.description]
        rows = cur.fetchall()
        return [dict(zip(columns, row)) for row in rows]


def render_chart(spec: dict, data: list[dict], output_path: Path):
    # Replace DATA_PLACEHOLDER with actual data
    spec_str = json.dumps(spec)
    spec_str = spec_str.replace('"DATA_PLACEHOLDER"', json.dumps(data, default=_json_default))
    spec = json.loads(spec_str)

    # Render to PNG via vl-convert
    png_bytes = vlc.vegalite_to_png(json.dumps(spec))
    output_path.write_bytes(png_bytes)


def _json_default(obj):
    if isinstance(obj, (datetime, date)):
        return obj.isoformat()
    if isinstance(obj, Decimal):
        return float(obj)
    raise TypeError(f"Object of type {type(obj)} is not JSON serializable")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    query = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_QUERY
    no_render = "--no-render" in sys.argv

    api_key = load_api_key()
    chart_types = load_chart_types()

    print(f"Query: {query}", file=sys.stderr)
    print(f"Model: {GEMINI_MODEL}", file=sys.stderr)

    # Step 1: dbctx context
    print("Querying dbctx...", file=sys.stderr)
    dbctx_context = query_dbctx(query)
    print(f"dbctx returned {len(dbctx_context)} chars", file=sys.stderr)

    # Step 2: Gemini
    print("Calling Gemini...", file=sys.stderr)
    result = call_gemini(api_key, query, dbctx_context, chart_types)

    # Print the raw JSON to stdout
    print(json.dumps(result, indent=2))

    if no_render:
        return

    # Step 3: Execute SQL and render charts
    OUTPUT_DIR.mkdir(exist_ok=True)
    conn = get_db_connection()

    for i, interp in enumerate(result.get("interpretations", [])):
        name = interp.get("name", f"chart_{i}")
        sql = interp.get("sql")
        spec = interp.get("vega_lite_spec")

        if not sql or not spec:
            print(f"  Skipping '{name}': missing sql or vega_lite_spec", file=sys.stderr)
            continue

        safe_name = name.lower().replace(" ", "_").replace("/", "_")[:40]
        output_path = OUTPUT_DIR / f"{safe_name}.png"

        print(f"  [{i+1}] {name}", file=sys.stderr)
        print(f"      SQL: {sql[:100]}...", file=sys.stderr)

        try:
            rows = execute_sql(conn, sql)
            print(f"      → {len(rows)} rows", file=sys.stderr)

            if not rows:
                print(f"      Skipping: no data", file=sys.stderr)
                continue

            render_chart(spec, rows, output_path)
            print(f"      → {output_path}", file=sys.stderr)
        except Exception as e:
            print(f"      Error: {e}", file=sys.stderr)

    conn.close()
    print(f"\nDone. Charts in {OUTPUT_DIR}/", file=sys.stderr)


if __name__ == "__main__":
    main()
