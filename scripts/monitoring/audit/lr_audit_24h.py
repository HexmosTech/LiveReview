import argparse
import json
from datetime import datetime, timedelta, timezone
from pathlib import Path
import requests

try:
    from dateutil import parser as dt_parser
except ImportError as exc:
    raise SystemExit(
        "python-dateutil is required; install it with 'pip install python-dateutil'."
    ) from exc


def get_audit_data(cache=True, cache_path=None):
    cache_file = cache_path or Path(__file__).with_name("yesterday_audit.json")
    if cache:
        try:
            return cache_file.read_text(encoding="utf-8")
        except FileNotFoundError as exc:
            raise FileNotFoundError(
                f"Cache file '{cache_file}' not found; rerun with --refresh to populate it."
            ) from exc

    url = "https://parse.apps.hexmos.com/parse/classes/JL_Audit"

    headers = {
        "X-Parse-Application-Id": "impressionserver",
        "X-Parse-Master-Key": "impressionserver",
        "X-Parse-Client-Version": "js3.5.1",
        "X-Parse-Installation-Id": "e61bb099-43c7-478d-86bb-dc8ca6a09582",
        "Content-Type": "application/json",
        "Accept": "*/*",
        "Referer": "https://parseui.apps.hexmos.com/",
    }

    payload = {
        "where": {},
        "limit": 100,
        "order": "-createdAt",
        "_method": "GET",
    }

    response = requests.post(url, headers=headers, data=json.dumps(payload))
    response.raise_for_status()

    cache_file.write_text(response.text, encoding="utf-8")
    return response.text


def _extract_iso_datetime(entry):
    iso_value = entry.get("timestamp", {}).get("iso") or entry.get("createdAt")
    if not iso_value:
        return None
    try:
        return dt_parser.isoparse(iso_value)
    except (ValueError, TypeError):
        return None


def lasth(data, hours):
    cutoff = datetime.now(timezone.utc) - timedelta(hours=hours)
    results = data.get("results", [])

    def within_cutoff(entry):
        dt_value = _extract_iso_datetime(entry)
        return dt_value is not None and dt_value >= cutoff

    filtered = list(filter(within_cutoff, results))
    return {**data, "results": filtered}


def last24h(data):
    return lasth(data, 24)


def group_by_email(data):
    results = data.get("results", [])
    grouped = {}
    for entry in results:
        email = entry.get("email")
        if email:
            if email not in grouped:
                grouped[email] = []
            grouped[email].append(entry)
    return grouped


def build_summary_table(grouped_by_email):
    """Build pre-table structure with email, event count, and event names."""
    table_data = []
    for email, entries in grouped_by_email.items():
        event_count = len(entries)
        event_names = ", ".join(e.get("action", "") for e in entries)
        table_data.append({
            "email": email,
            "event_count": event_count,
            "event_names": event_names
        })
    # Sort by event count descending
    table_data.sort(key=lambda x: x["event_count"], reverse=True)
    return table_data


def pretty_print_markdown_table(headers, rows):
    """Pretty print markdown table with aligned columns."""
    if not rows:
        return "No data available"
    
    # Calculate column widths
    col_widths = [len(h) for h in headers]
    for row in rows:
        for i, cell in enumerate(row):
            col_widths[i] = max(col_widths[i], len(str(cell)))
    
    # Build header row
    header_cells = [f" {headers[i]:<{col_widths[i]}} " for i in range(len(headers))]
    header_line = "|" + "|".join(header_cells) + "|"
    
    # Build separator row
    sep_cells = ["-" * (col_widths[i] + 2) for i in range(len(headers))]
    sep_line = "|" + "|".join(sep_cells) + "|"
    
    # Build data rows
    lines = [header_line, sep_line]
    for row in rows:
        row_cells = [f" {str(row[i]):<{col_widths[i]}} " for i in range(len(row))]
        lines.append("|" + "|".join(row_cells) + "|")
    
    return "\n".join(lines)


def format_markdown_table(table_data):
    """Format pre-table structure as markdown table."""
    headers = ["Email", "Event Count", "Event Names"]
    rows = [
        [row["email"], row["event_count"], row["event_names"]]
        for row in table_data
    ]
    return pretty_print_markdown_table(headers, rows)


def post_to_discord(markdown_table, webhook_url):
    """Post markdown table to Discord webhook."""
    discord_payload = {
        "content": f"```\n{markdown_table}\n```"
    }
    
    try:
        response = requests.post(webhook_url, json=discord_payload)
        response.raise_for_status()
        print("\n✓ Posted to Discord successfully")
    except Exception as e:
        print(f"\n✗ Failed to post to Discord: {e}")


def parse_args():
    parser = argparse.ArgumentParser(description="Filter LiveReview audit data")
    parser.add_argument(
        "--refresh",
        action="store_true",
        help="Fetch fresh audit data and update the cache",
    )
    return parser.parse_args()


def main():
    args = parse_args()
    use_cache = not args.refresh
    audit_data = json.loads(get_audit_data(cache=use_cache))
    audit_data_last24h = last24h(audit_data)
    grouped_by_email = group_by_email(audit_data_last24h)
    
    table_data = build_summary_table(grouped_by_email)
    markdown_table = format_markdown_table(table_data)
    
    print(markdown_table)
    
    discord_webhook_url = "https://discord.com/api/webhooks/1394676585151332402/Gwp-Qvt-_0UHK8yVZ_6rPxRHm3Y0x_cdQICstDD7MQ2eBNyqJaatL-uyixTnFMy8KV_H"
    post_to_discord(markdown_table, discord_webhook_url)


if __name__ == "__main__":
    main()


