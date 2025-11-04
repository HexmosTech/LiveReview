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
    print(json.dumps(audit_data_last24h, indent=2))


if __name__ == "__main__":
    main()


