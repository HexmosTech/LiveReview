#!/usr/bin/env python3
"""
interpretation.py — Use dbctx + Gemini Flash to interpret a natural-language
analytics query into structured proxy metrics + tables/fields.

Flow:
  1. Run user query through dbctx → get relevant tables/fields
  2. Feed dbctx context + user query into Gemini → structured JSON

Usage:
    python3 interpretation.py                          # uses default query
    python3 interpretation.py "Your custom query here" # custom query
"""

import json
import subprocess
import sys
import urllib.request
import urllib.error

from pathlib import Path

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

GEMINI_MODEL = "gemini-2.5-flash"
GEMINI_ENDPOINT = (
    "https://generativelanguage.googleapis.com/v1beta/models/"
    f"{GEMINI_MODEL}:generateContent"
)

# dbctx .dtx file lives in project root
DTX_PATH = Path(__file__).resolve().parent.parent.parent / "livereviewctx.dtx"

DEFAULT_QUERY = "How broadly has the organization adopted LiveReview?"

SYSTEM_PROMPT = """\
You are a database-aware analytics interpreter for LiveReview, an AI-powered code review SaaS.

## Task
1. You receive a user's natural-language question followed by dbctx schema context (tables, columns, foreign keys, field stats, sample values).
2. You produce a JSON object with proxy metrics that answer the question via SQL.

## Output format
- Respond with ONLY valid JSON, no markdown, no code fences.
- Schema:
  {
    "query": "<original user query>",
    "interpretation": "<1-2 sentence restatement>",
    "proxy_metrics": [
      {
        "name": "<short metric name>",
        "description": "<what it measures>",
        "dimension": "breadth|depth|time|duration",
        "tables": ["<table1>"],
        "fields": ["<table.field1>"],
        "filters": ["<optional WHERE fragments>"],
        "aggregation": "<COUNT / SUM / GROUP BY / etc.>"
      }
    ]
  }

## Metric validity rules
1. Use ONLY tables and fields present in the dbctx context — never invent columns.
2. Every field must be qualified with its table name (e.g. `reviews.status`).
3. Include filters where relevant (e.g. `status='completed'`, `is_active=true`).
4. Be specific about aggregation (COUNT DISTINCT, SUM, DATE_TRUNC, etc.).

## How many metrics to generate
5. Match the user's intent, not a fixed count.
6. If the query is specific and unambiguous (e.g. "how many reviews last month?") → generate 1-2 metrics that directly answer it. Do not pad.
7. If the query is broad or ambiguous (e.g. "how broadly adopted is LiveReview?") → generate 3-5 metrics covering different plausible interpretations. Each metric should be a distinct angle, not a variation of the same query.
8. Never exceed 5 metrics. If you want more, you are over-decomposing — combine related things into one metric instead.

## Which metrics to prefer
9. First: what the user explicitly asked about (match keywords).
10. Second: the closest behavioral proxy if the literal thing doesn't exist in the schema (e.g. "engagement" → review count, not login count).
11. Third: a temporal view if the question implies trend or change over time.
12. Fourth: a breadth view if the question implies "how many" or "how widely".
"""

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def load_api_key() -> str:
    """Load GEMINI_API_KEY from .env file next to this script."""
    env_path = Path(__file__).parent / ".env"
    if not env_path.exists():
        print(f"Error: {env_path} not found", file=sys.stderr)
        sys.exit(1)
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line.startswith("GEMINI_API_KEY="):
            return line.split("=", 1)[1].strip()
    print("Error: GEMINI_API_KEY not found in .env", file=sys.stderr)
    sys.exit(1)


def query_dbctx(query: str) -> str:
    """Run dbctx query and return the output as context string."""
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


def call_gemini(api_key: str, user_query: str, dbctx_context: str) -> dict:
    """Call Gemini Flash with user query + dbctx context, return parsed JSON."""
    user_message = (
        f"User query: {user_query}\n\n"
        f"--- dbctx schema context ---\n{dbctx_context}"
    )

    payload = {
        "system_instruction": {
            "parts": [{"text": SYSTEM_PROMPT}]
        },
        "contents": [
            {
                "role": "user",
                "parts": [{"text": user_message}]
            }
        ],
        "generationConfig": {
            "temperature": 0.2,
            "maxOutputTokens": 4096,
            "responseMimeType": "application/json"
        }
    }

    url = f"{GEMINI_ENDPOINT}?key={api_key}"
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
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
        print(f"Unexpected Gemini response structure: {e}", file=sys.stderr)
        print(json.dumps(body, indent=2), file=sys.stderr)
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


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    query = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_QUERY
    api_key = load_api_key()

    print(f"Query: {query}", file=sys.stderr)
    print(f"Model: {GEMINI_MODEL}", file=sys.stderr)

    # Step 1: Get schema context from dbctx
    print("Querying dbctx...", file=sys.stderr)
    dbctx_context = query_dbctx(query)
    print(f"dbctx returned {len(dbctx_context)} chars", file=sys.stderr)

    # Step 2: Send to Gemini
    print("Calling Gemini...", file=sys.stderr)
    result = call_gemini(api_key, query, dbctx_context)

    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
