#!/usr/bin/env python3
"""Blast radius risk: "How many reviews landed with critical/high blast
radius this month, and where?" Blast-radius reports are computed
client-side by git-lrc and stored as opportunistic per-review artifacts in
the configured blob store (internal/api/diff_review.go), not in Postgres -
so this script, unlike its siblings, does two passes: SQL for the review/
repository list, then S3 GetObject per review for the artifact itself.

Each artifact's hunks carry a 0-100 "Combined" score; a review's overall
risk is its single highest-scoring hunk, bucketed the same way the diff
viewer buckets badges (ui/src/lib/blastRadius.ts's blastRadiusTier):
  >=66 High | 33-65 Moderate | 1-32 Low | 0 Minimal
There is no separate "critical" bucket in the current scoring - "High" is
the top tier, so a "critical or high" ask maps to High here.

Only reviews actually run through the git-lrc CLI produce this artifact, so
the denominator is "reviews with a blast-radius report", not "all reviews".

Usage: uv run generate_blast_radius.py [--org hexmos-internal] [--days 30]
"""
import argparse
import json
import subprocess
import sys
import tempfile
from collections import Counter, defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page

TIERS = ["High", "Moderate", "Low", "Minimal"]
TIER_COLOR = {"High": "#ff5c7c", "Moderate": "#ffb454", "Low": "#7c9cff", "Minimal": "#3a4358"}


def tier(score: float) -> str:
    if score >= 66:
        return "High"
    if score >= 33:
        return "Moderate"
    if score > 0:
        return "Low"
    return "Minimal"


def load_blob_config(db: str) -> dict:
    rows = run_query(db, "SELECT data FROM system_settings WHERE name = 'blob_storage'")
    if not rows:
        sys.exit("no blob_storage config in system_settings - nothing to fetch artifacts from")
    return json.loads(rows[0]["data"])


def fetch_artifact(cfg: dict, org_id: int, review_id: str) -> dict | None:
    # aws s3api get-object writes the object body to the given outfile AND
    # prints a metadata JSON blob to stdout - passing "/dev/stdout" for
    # both merges them into one unparseable stream, so the body must go to
    # a real temp file and get read back separately.
    key = f"org/{org_id}/review/{review_id}/artifacts/blast-radius.json"
    env = {
        "AWS_ACCESS_KEY_ID": cfg["access_key_id"],
        "AWS_SECRET_ACCESS_KEY": cfg["secret_access_key"],
        "AWS_DEFAULT_REGION": cfg.get("region", "us-east-1"),
        "PATH": "/usr/bin:/usr/local/bin",
    }
    with tempfile.NamedTemporaryFile() as tmp:
        result = subprocess.run(
            ["aws", "s3api", "get-object", "--endpoint-url", cfg["endpoint"],
             "--bucket", cfg["bucket"], "--key", key, tmp.name],
            capture_output=True, env=env,
        )
        if result.returncode != 0:
            return None
        try:
            return json.load(tmp)
        except json.JSONDecodeError:
            return None


def max_combined(report: dict) -> float:
    best = 0.0
    for f in (report.get("Files") or []):
        for h in (f.get("Hunks") or []):
            c = h.get("Combined") or 0
            if c > best:
                best = c
    return best


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--days", type=int, default=30)
    p.add_argument("--out", default=str(SCRIPT_DIR / "blast_radius.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)
    blob_cfg = load_blob_config(db)

    sql = f"""
    SELECT id, repository
    FROM reviews
    WHERE org_id = {org_id}
      AND created_at >= CURRENT_DATE - INTERVAL '{args.days} days'
    ORDER BY id;
    """
    review_rows = run_query(db, sql)
    if not review_rows:
        sys.exit("query returned no rows")

    per_repo_tier = defaultdict(lambda: Counter())
    scored = 0
    with ThreadPoolExecutor(max_workers=16) as pool:
        futures = {
            pool.submit(fetch_artifact, blob_cfg, org_id, row["id"]): row["repository"]
            for row in review_rows
        }
        for fut in as_completed(futures):
            report = fut.result()
            if report is None:
                continue
            scored += 1
            t = tier(max_combined(report))
            per_repo_tier[futures[fut]][t] += 1

    if scored == 0:
        sys.exit("no blast-radius artifacts found for any review in this window")

    total_high = sum(c["High"] for c in per_repo_tier.values())
    total_moderate = sum(c["Moderate"] for c in per_repo_tier.values())

    # Sort repos by High count desc, then total desc, keep the top 20 so the
    # chart stays readable - the long tail is single-digit repos.
    repo_order = sorted(
        per_repo_tier.keys(),
        key=lambda r: (-per_repo_tier[r]["High"], -sum(per_repo_tier[r].values())),
    )[:20]

    data = []
    for repo in repo_order:
        for t in TIERS:
            n = per_repo_tier[repo][t]
            if n:
                data.append({"repository": repo, "tier": t, "count": n})

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Blast radius risk by repository — {args.org}, last {args.days} days",
        "width": 640,
        "height": {"step": 18},
        "data": {"values": data},
        "mark": {"type": "bar"},
        "encoding": {
            "y": {"field": "repository", "type": "nominal", "sort": repo_order, "title": None},
            "x": {"field": "count", "type": "quantitative", "title": "Reviews", "stack": "zero"},
            "color": {
                "field": "tier", "type": "nominal", "title": "Blast radius",
                "sort": TIERS,
                "scale": {"domain": TIERS, "range": [TIER_COLOR[t] for t in TIERS]},
            },
            "order": {"field": "tier", "sort": "ascending"},
            "tooltip": [
                {"field": "repository", "type": "nominal"},
                {"field": "tier", "type": "nominal", "title": "Blast radius"},
                {"field": "count", "type": "quantitative", "title": "Reviews"},
            ],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "legend": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }

    top_repo = repo_order[0] if repo_order else "n/a"
    stats_html = (
        f"<b>{total_high}</b> of <b>{scored}</b> scored reviews ({total_high / scored * 100:.0f}%) hit "
        f"<b>High</b> blast radius (score ≥ 66, the top tier - there is no separate \"critical\" bucket) "
        f"in the last {args.days} days; <b>{total_moderate}</b> more were Moderate.<br>"
        f"Riskiest repository: <b>{top_repo}</b> ({per_repo_tier[top_repo]['High']} High-risk reviews).<br>"
        f"Only reviews run through the git-lrc CLI produce a blast-radius report, so this covers "
        f"<b>{scored}</b> of the org's reviews in-window, not all of them."
    )

    wrap_page(
        title=f"Blast Radius Risk — {args.org}", spec=spec, view_max_width=680,
        stats_html=stats_html, sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
