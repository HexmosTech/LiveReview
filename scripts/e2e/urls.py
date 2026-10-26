#!/usr/bin/env python3
"""Probe all manual-talent reverse-tunnel URLs plus the selected target."""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List

from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import e2e_lib as lib


def probe(url: str, health_base: str = "") -> Dict[str, Any]:
    parsed = urllib.parse.urlparse(url)
    host = parsed.hostname or url
    api_base = (health_base or url).rstrip("/")
    out: Dict[str, Any] = {"host": host, "base": url, "health": None, "version": None, "ui": None, "detail": None}
    try:
        req = urllib.request.Request(api_base + "/health", method="GET")
        # URL comes from E2E config / the fixed manual-talent host list.
        # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected
        with urllib.request.urlopen(req, timeout=8) as resp:
            out["health"] = resp.status
    except urllib.error.HTTPError as exc:
        out["health"] = exc.code
    except Exception as exc:
        out["detail"] = str(exc)[:160]

    try:
        req = urllib.request.Request(api_base + "/api/version", method="GET")
        # URL comes from E2E config / the fixed manual-talent host list.
        # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected
        with urllib.request.urlopen(req, timeout=8) as resp:
            out["version"] = resp.status
            try:
                payload = json.loads(resp.read().decode("utf-8"))
                if isinstance(payload, dict):
                    out["version_value"] = payload.get("version") or payload.get("git_commit")
            except Exception:
                pass
    except urllib.error.HTTPError as exc:
        out["version"] = exc.code
    except Exception:
        pass

    try:
        req = urllib.request.Request(url.rstrip("/") + "/", method="GET")
        # URL comes from E2E config / the fixed manual-talent host list.
        # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected
        with urllib.request.urlopen(req, timeout=8) as resp:
            out["ui"] = resp.status
    except urllib.error.HTTPError as exc:
        out["ui"] = exc.code
    except Exception:
        pass
    # A public reverse proxy may not route /health, but /api/version proves the
    # API is reachable and / proves the UI bundle is served.
    out["ok"] = out["version"] == 200 and out["ui"] == 200
    return out


def main() -> int:
    lib.load_env()
    parser = argparse.ArgumentParser(description="Probe manual-talent URLs")
    parser.add_argument("--strict", action="store_true", help="fail if any URL is unhealthy")
    args = parser.parse_args()

    configured = lib.cfg("E2E_URLS")
    if configured:
        hosts = [h.strip() for h in configured.replace(",", " ").split() if h.strip()]
    else:
        hosts = [
            "manual-talent",
            "manual-talent2",
            "manual-talent3",
            "manual-talent4",
            "manual-talent5",
        ]
    urls: List[str] = []
    for host in hosts:
        if host.startswith("http://") or host.startswith("https://"):
            urls.append(host.rstrip("/"))
        else:
            urls.append(f"https://{host}.apps.hexmos.com")

    selected = lib.base_url()
    if selected not in urls:
        urls.insert(0, selected)

    results = [
        probe(u, lib.api_url() if (u.rstrip("/") == selected.rstrip("/") and lib.is_local()) else "")
        for u in urls
    ]
    print("Public URL smoke")
    for row in results:
        marker = "selected" if row["base"].rstrip("/") == selected.rstrip("/") else ""
        flag = "ok" if row["ok"] else "!!"
        version = row.get("version_value") or ""
        print(
            f"  [{flag}] {row['host']}: health={row['health']} ui={row['ui']} version={row['version']} {version} {marker}".rstrip()
        )
        if row.get("detail"):
            print(f"        {row['detail']}")
    report = {"generated_at": lib.now_iso(), "selected": selected, "urls": results}
    lib.secure_dir(lib.STATE_DIR)
    lib.write_json(lib.STATE_DIR / "url-report.json", report)
    if args.strict and not all(r["ok"] for r in results):
        return 1
    if not any(r["base"].rstrip("/") == selected.rstrip("/") and r["ok"] for r in results):
        print("Selected target is not healthy.")
        return 1 if args.strict else 0
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
