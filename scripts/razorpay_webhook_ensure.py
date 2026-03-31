#!/usr/bin/env python3
"""Ensure LiveReview Razorpay webhook configuration.

Usage examples:
  python3 scripts/razorpay_webhook_ensure.py --base-url https://manual-talent2.apps.hexmos.com --mode test
  python3 scripts/razorpay_webhook_ensure.py --base-url manual-talent2.apps.hexmos.com --mode test --dry-run
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import sys
from dataclasses import dataclass
from typing import Any, Dict, List, Optional
from urllib import error, parse, request

RAZORPAY_API_BASE = "https://api.razorpay.com"
DEFAULT_EVENTS = [
    "subscription.activated",
    "subscription.charged",
    "subscription.cancelled",
    "subscription.completed",
    "subscription.paused",
    "subscription.resumed",
    "subscription.pending",
    "subscription.halted",
    "subscription.authenticated",
    "payment.authorized",
    "payment.captured",
    "payment.failed",
]


def clean_value(raw: str) -> str:
    value = raw.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
        return value[1:-1]
    return value


@dataclass
class RazorpayCredentials:
    key_id: str
    secret: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Ensure LiveReview Razorpay webhook exists and matches expected events")
    parser.add_argument("--base-url", required=True, help="Base URL of LiveReview deployment")
    parser.add_argument("--mode", choices=["test", "live"], default=os.getenv("RAZORPAY_MODE", "test"), help="Razorpay mode")
    parser.add_argument("--webhook-secret", default=os.getenv("RAZORPAY_WEBHOOK_SECRET", ""), help="Razorpay webhook signing secret")
    parser.add_argument("--alert-email", default=os.getenv("RAZORPAY_WEBHOOK_ALERT_EMAIL", ""), help="Optional alert email")
    parser.add_argument("--dry-run", action="store_true", help="Print actions without mutating Razorpay")
    parser.add_argument("--force-update", action="store_true", help="Force update when URL already exists")
    args = parser.parse_args()
    args.webhook_secret = clean_value(args.webhook_secret)
    args.alert_email = clean_value(args.alert_email)
    return args


def normalize_base_url(raw_base_url: str) -> str:
    base = raw_base_url.strip()
    if not base:
        raise ValueError("base URL cannot be empty")

    if "://" not in base:
        base = f"https://{base}"

    parsed = parse.urlparse(base)
    if parsed.scheme not in {"http", "https"}:
        raise ValueError("base URL must use http or https")
    if not parsed.netloc:
        raise ValueError("base URL is missing hostname")

    path = parsed.path.rstrip("/")
    normalized = parse.urlunparse((parsed.scheme, parsed.netloc, path, "", "", ""))
    return normalized


def build_webhook_url(base_url: str) -> str:
    return f"{base_url}/api/v1/webhooks/razorpay"


def resolve_credentials(mode: str) -> RazorpayCredentials:
    if mode == "test":
        key_id = clean_value(os.getenv("RAZORPAY_TEST_KEY", ""))
        secret = clean_value(os.getenv("RAZORPAY_TEST_SECRET", ""))
        if not key_id or not secret:
            raise ValueError("RAZORPAY_TEST_KEY and RAZORPAY_TEST_SECRET must be set for test mode")
        return RazorpayCredentials(key_id=key_id, secret=secret)

    key_id = clean_value(os.getenv("RAZORPAY_LIVE_KEY", ""))
    secret = clean_value(os.getenv("RAZORPAY_LIVE_SECRET", ""))
    if not key_id or not secret:
        raise ValueError("RAZORPAY_LIVE_KEY and RAZORPAY_LIVE_SECRET must be set for live mode")
    return RazorpayCredentials(key_id=key_id, secret=secret)


def razorpay_request(method: str, path: str, creds: RazorpayCredentials, payload: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    url = f"{RAZORPAY_API_BASE}{path}"
    body = None
    if payload is not None:
        body = json.dumps(payload).encode("utf-8")

    req = request.Request(url=url, data=body, method=method)
    token = base64.b64encode(f"{creds.key_id}:{creds.secret}".encode("utf-8")).decode("ascii")
    req.add_header("Authorization", f"Basic {token}")
    req.add_header("Content-Type", "application/json")

    try:
        with request.urlopen(req, timeout=30) as resp:
            data = resp.read().decode("utf-8")
    except error.HTTPError as exc:
        err_body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Razorpay API {method} {path} failed ({exc.code}): {err_body}") from exc
    except error.URLError as exc:
        raise RuntimeError(f"Razorpay API {method} {path} failed: {exc}") from exc

    if not data:
        return {}

    try:
        return json.loads(data)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"Failed to decode Razorpay response for {method} {path}: {data}") from exc


def list_webhooks(creds: RazorpayCredentials) -> List[Dict[str, Any]]:
    resp = razorpay_request("GET", "/v1/webhooks", creds)
    if isinstance(resp, dict) and "items" in resp and isinstance(resp["items"], list):
        return resp["items"]
    if isinstance(resp, list):
        return resp
    return []


def create_webhook(creds: RazorpayCredentials, payload: Dict[str, Any]) -> Dict[str, Any]:
    return razorpay_request("POST", "/v1/webhooks", creds, payload)


def update_webhook(creds: RazorpayCredentials, webhook_id: str, payload: Dict[str, Any]) -> Dict[str, Any]:
    return razorpay_request("PUT", f"/v1/webhooks/{webhook_id}", creds, payload)


def events_match(existing: Any, expected: List[str]) -> bool:
    if isinstance(existing, list):
        return sorted(existing) == sorted(expected)
    if isinstance(existing, dict):
        # Razorpay webhook APIs use {"event.name": true} shape.
        enabled = sorted([k for k, v in existing.items() if bool(v)])
        return enabled == sorted(expected)
    return False


def build_payload(webhook_url: str, webhook_secret: str, alert_email: str) -> Dict[str, Any]:
    events_map = {event_name: True for event_name in DEFAULT_EVENTS}
    payload: Dict[str, Any] = {
        "url": webhook_url,
        "secret": webhook_secret,
        "active": True,
        "events": events_map,
    }
    if clean_value(alert_email):
        payload["alert_email"] = clean_value(alert_email)
    return payload


def payload_for_logging(payload: Dict[str, Any]) -> Dict[str, Any]:
    redacted = dict(payload)
    if "secret" in redacted:
        redacted["secret"] = "***redacted***"
    return redacted


def ensure_webhook(args: argparse.Namespace) -> int:
    if not args.webhook_secret.strip():
        raise ValueError("webhook secret is required (set RAZORPAY_WEBHOOK_SECRET or pass --webhook-secret)")

    normalized_base = normalize_base_url(args.base_url)
    webhook_url = build_webhook_url(normalized_base)
    creds = resolve_credentials(args.mode)

    print(f"Mode: {args.mode}")
    print(f"Base URL: {normalized_base}")
    print(f"Webhook URL: {webhook_url}")
    print(f"Dry run: {args.dry_run}")

    payload = build_payload(webhook_url, args.webhook_secret, args.alert_email)

    webhooks = list_webhooks(creds)
    matches = [w for w in webhooks if str(w.get("url", "")).rstrip("/") == webhook_url.rstrip("/")]

    if len(matches) > 1:
        ids = ", ".join(str(w.get("id", "unknown")) for w in matches)
        raise RuntimeError(f"found multiple Razorpay webhooks for URL {webhook_url}: {ids}")

    if not matches:
        print("Action: create webhook")
        if args.dry_run:
            print(json.dumps(payload_for_logging(payload), indent=2))
            return 0
        created = create_webhook(creds, payload)
        print(f"Created webhook id={created.get('id', 'unknown')} active={created.get('active')}")
        return 0

    existing = matches[0]
    webhook_id = str(existing.get("id", ""))
    if not webhook_id:
        raise RuntimeError("matched webhook missing id")

    needs_update = args.force_update or not bool(existing.get("active", False)) or not events_match(existing.get("events"), DEFAULT_EVENTS)

    if not needs_update:
        print(f"Webhook already configured (id={webhook_id}). No changes needed.")
        return 0

    print(f"Action: update webhook id={webhook_id}")
    if args.dry_run:
        print(json.dumps(payload_for_logging(payload), indent=2))
        return 0

    updated = update_webhook(creds, webhook_id, payload)
    print(f"Updated webhook id={updated.get('id', webhook_id)} active={updated.get('active')}")
    return 0


def main() -> int:
    args = parse_args()
    try:
        return ensure_webhook(args)
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
