#!/usr/bin/env python3
"""Row 20: "How much does LiveReview save versus alternatives?" Waterfall
chart. X: cost components. Y: USD. Positive = avoided cost, negative = LR cost.

The "avoided cost" side is a hypothetical (an assumed cost-per-hour and
review time for manual code review) - the LR cost side is real, pulled from
loc_usage_ledger.llm_cost_usd. Labeled clearly as an estimate in the page
itself; don't take the total as more precise than the assumption feeding it.

Usage:
    uv run generate_cost_waterfall.py [--org hexmos-internal] [--days 90]
    uv run generate_cost_waterfall.py --hourly-rate 80 --minutes-per-review 20
"""
import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _common import SCRIPT_DIR, load_database_url, resolve_org_id, run_query, wrap_page


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--org", default="hexmos-internal")
    p.add_argument("--days", type=int, default=90)
    p.add_argument("--hourly-rate", type=float, default=75.0, help="assumed engineer hourly cost, USD")
    p.add_argument("--minutes-per-review", type=float, default=15.0, help="assumed manual review time per review")
    p.add_argument("--subscription-cost", type=float, default=0.0, help="LiveReview subscription cost for the window, USD")
    p.add_argument("--out", default=str(SCRIPT_DIR / "cost_waterfall.html"))
    args = p.parse_args()

    db = load_database_url()
    org_id = resolve_org_id(db, args.org)

    sql = f"""
    SELECT count(*) AS reviews, coalesce(sum(llm_cost_usd), 0) AS llm_cost
    FROM loc_usage_ledger
    WHERE org_id = {org_id} AND status = 'accounted'
      AND accounted_at >= CURRENT_DATE - INTERVAL '{args.days} days';
    """
    rows = run_query(db, sql)
    if not rows or int(rows[0]["reviews"]) == 0:
        sys.exit("no accounted LOC usage found")

    reviews = int(rows[0]["reviews"])
    llm_cost = float(rows[0]["llm_cost"])
    hypothetical_manual_cost = reviews * (args.minutes_per_review / 60) * args.hourly_rate

    steps = [
        {"label": "Hypothetical manual review cost", "value": hypothetical_manual_cost, "kind": "start"},
        {"label": "LiveReview LLM cost", "value": -llm_cost, "kind": "cost"},
        {"label": "LiveReview subscription", "value": -args.subscription_cost, "kind": "cost"},
        {"label": "Net savings", "value": hypothetical_manual_cost - llm_cost - args.subscription_cost, "kind": "end"},
    ]
    running = 0.0
    data = []
    for s in steps:
        if s["kind"] == "start":
            base, top = 0, s["value"]
        elif s["kind"] == "end":
            base, top = 0, s["value"]
        else:
            base = running
            top = running + s["value"]
        data.append({"label": s["label"], "base": min(base, top), "top": max(base, top),
                      "color": "positive" if s["value"] >= 0 else "negative"})
        if s["kind"] != "end":
            running += s["value"]

    net_savings = steps[-1]["value"]

    spec = {
        "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
        "title": f"Estimated cost savings — {args.org}, last {args.days} days",
        "width": 600, "height": 380,
        "data": {"values": data},
        "mark": {"type": "bar", "size": 60},
        "encoding": {
            "x": {"field": "label", "type": "nominal", "title": None, "sort": None, "axis": {"labelAngle": -20}},
            "y": {"field": "base", "type": "quantitative", "title": "USD"},
            "y2": {"field": "top"},
            "color": {"field": "color", "type": "nominal", "legend": None,
                      "scale": {"domain": ["positive", "negative"], "range": ["#39d353", "#ff5c7c"]}},
            "tooltip": [{"field": "label", "type": "nominal"}, {"field": "top", "type": "quantitative", "title": "USD", "format": ".2f"}],
        },
        "config": {
            "background": "#0f1420",
            "axis": {"labelColor": "#c9d1e0", "titleColor": "#e6ebf5", "gridColor": "#232a3d", "domainColor": "#3a4358"},
            "title": {"color": "#e6ebf5", "fontSize": 16},
            "view": {"stroke": "transparent"},
        },
    }
    wrap_page(
        title=f"Cost Savings Estimate — {args.org}", spec=spec, view_max_width=640,
        stats_html=(
            f"<b>{reviews}</b> reviews cost <b>${llm_cost:.2f}</b> in LLM usage. "
            f"Assuming manual review would take <b>{args.minutes_per_review:g} min</b> at <b>${args.hourly_rate:g}/hr</b>, "
            f"estimated net savings: <b>${net_savings:.2f}</b>.<br>"
            f"<i>This is an estimate based on the assumed manual-review time/rate above, not a measured figure — adjust "
            f"--hourly-rate/--minutes-per-review to your own numbers.</i>"
        ),
        sql=sql, out_path=Path(args.out),
    )


if __name__ == "__main__":
    main()
