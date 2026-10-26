#!/usr/bin/env python3
"""Capability preflight: never assume a local AI or git connector is healthy."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, Dict, List, Optional

sys.path.insert(0, str(Path(__file__).resolve().parent))

import e2e_lib as lib


def _select_ai_connector(payload: Any) -> Optional[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    if isinstance(payload, list):
        rows = [r for r in payload if isinstance(r, dict)]
    elif isinstance(payload, dict):
        for key in ("connectors", "rows", "data"):
            value = payload.get(key)
            if isinstance(value, list):
                rows = [r for r in value if isinstance(r, dict)]
                break
    leaders = [r for r in rows if str(r.get("role") or "leader") == "leader"]
    candidates = leaders or rows
    if not candidates:
        return None
    # Prefer a connector with an explicit model; it is the most likely to have
    # been exercised recently.
    with_model = [r for r in candidates if r.get("selected_model")]
    return (with_model or candidates)[0]


def _select_git_connector(payload: Any) -> Optional[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    if isinstance(payload, list):
        rows = [r for r in payload if isinstance(r, dict)]
    elif isinstance(payload, dict):
        for key in ("connectors", "rows", "data"):
            value = payload.get(key)
            if isinstance(value, list):
                rows = [r for r in value if isinstance(r, dict)]
                break
    for row in rows:
        if str(row.get("provider") or "").lower() == "github":
            return row
    return rows[0] if rows else None


def _worker_up() -> bool:
    try:
        proc = subprocess.run(
            ["pgrep", "-af", "lrworker|lr-worker"],
            text=True,
            capture_output=True,
            timeout=5,
        )
        return proc.returncode == 0 and bool(proc.stdout.strip())
    except Exception:
        return False


def _ai_preflight(auth: Dict[str, Any], org_id: int) -> Dict[str, Any]:
    token = (auth.get("tokens") or {}).get("access_token")
    result: Dict[str, Any] = {
        "status": "skipped",
        "connector_id": None,
        "provider": None,
        "model": None,
        "latency_ms": None,
        "detail": None,
    }
    if lib.cfg("E2E_SKIP_AI", "0") == "1":
        result["detail"] = "E2E_SKIP_AI=1"
        return result
    try:
        status, payload, _ = lib.api(
            "GET",
            "/api/v1/aiconnectors",
            token=token,
            org_id=org_id,
            timeout=20,
        )
        connector = _select_ai_connector(payload) if status == 200 else None
        if not connector:
            result["status"] = "not_configured"
            result["detail"] = "No AI connector is configured for the owner organization."
            return result
        result["connector_id"] = connector.get("id")
        result["provider"] = connector.get("provider_name") or connector.get("provider")
        result["model"] = connector.get("selected_model")
        started = time.time()
        chat_status, chat_payload, _ = lib.api(
            "POST",
            "/api/v1/chat/send",
            token=token,
            org_id=org_id,
            body={"message": "Reply with exactly OK. Do not use any tools."},
            timeout=90,
            raise_on_error=False,
        )
        result["latency_ms"] = int((time.time() - started) * 1000)
        if chat_status == 200:
            text = ""
            if isinstance(chat_payload, dict):
                text = str(chat_payload.get("response") or chat_payload.get("message") or "")
            result["status"] = "ready"
            result["detail"] = f"hello-world chat returned {len(text)} chars"
        else:
            result["status"] = "broken"
            body = chat_payload if isinstance(chat_payload, str) else ""
            result["detail"] = f"hello-world chat HTTP {chat_status}: {body[:160]}"
    except Exception as exc:
        result["status"] = "broken"
        result["detail"] = f"{type(exc).__name__}: {exc}"
    return result


def _git_preflight(auth: Dict[str, Any], org_id: int) -> Dict[str, Any]:
    token = (auth.get("tokens") or {}).get("access_token")
    result: Dict[str, Any] = {
        "status": "skipped",
        "connector_id": None,
        "provider": None,
        "detail": None,
    }
    if lib.cfg("E2E_SKIP_GIT", "0") == "1":
        result["detail"] = "E2E_SKIP_GIT=1"
        return result
    try:
        status, payload, _ = lib.api(
            "GET",
            "/api/v1/connectors",
            token=token,
            org_id=org_id,
            timeout=20,
        )
        connector = _select_git_connector(payload) if status == 200 else None
        if not connector:
            result["status"] = "not_configured"
            result["detail"] = "No git connector is configured for the owner organization."
            return result
        result["connector_id"] = connector.get("id")
        result["provider"] = connector.get("provider")
        test_status, test_payload, _ = lib.api(
            "POST",
            f"/api/v1/connectors/{connector.get('id')}/test",
            token=token,
            org_id=org_id,
            timeout=45,
            raise_on_error=False,
        )
        valid = bool(isinstance(test_payload, dict) and test_payload.get("valid"))
        if test_status == 200 and valid:
            result["status"] = "ready"
            result["detail"] = str(test_payload.get("message") or "connection is working")
        else:
            result["status"] = "broken"
            msg = ""
            if isinstance(test_payload, dict):
                msg = str(test_payload.get("message") or test_payload.get("error") or "")
            result["detail"] = f"HTTP {test_status}: {msg[:200]}"
    except Exception as exc:
        result["status"] = "broken"
        result["detail"] = f"{type(exc).__name__}: {exc}"
    return result


def cmd_preflight(args: argparse.Namespace) -> int:
    lib.load_env()
    lib.secure_dir(lib.STATE_DIR)
    state = lib.load_state()
    auth = lib.get_auth()
    checks: Dict[str, Any] = {
        "generated_at": lib.now_iso(),
        "base_url": lib.base_url(),
        "api_url": lib.api_url(),
        "run_id": state.get("run_id"),
        "owner_org_id": state.get("owner_org_id"),
        "super_org_id": state.get("super_org_id"),
        "checks": {},
    }
    critical_ok = True

    def put(name: str, value: Dict[str, Any], critical: bool = False) -> None:
        nonlocal critical_ok
        checks["checks"][name] = value
        if critical and not value.get("ok"):
            critical_ok = False

    version_ok = False
    version_text = ""
    try:
        status, payload, _ = lib.api("GET", "/api/version", timeout=10, raise_on_error=False)
        version_ok = status == 200
        if isinstance(payload, dict):
            version_text = str(payload.get("version") or payload.get("git_commit") or "")
    except Exception:
        pass

    # Some public nginx front doors do not route /health, but /api/version is
    # the same API origin and is always proxied.
    api_health = lib.wait_http(lib.api_url() + "/health", timeout=5) or version_ok
    put("api", {"ok": api_health, "detail": lib.api_url() + "/health or /api/version"}, critical=True)

    ui_ok = lib.wait_http(lib.base_url() + "/", timeout=5)
    put("ui", {"ok": ui_ok, "detail": lib.base_url()}, critical=True)

    put("version", {"ok": version_ok, "detail": version_text})

    worker_ok = _worker_up()
    put("worker", {"ok": worker_ok, "detail": "River worker process found" if worker_ok else "worker process not found"})

    auth_ok = bool(auth and lib.validate_access(auth))
    put("auth", {"ok": auth_ok, "detail": "cached session valid" if auth_ok else "run `make e2e-auth`"}, critical=True)

    if auth_ok and state:
        owner_org = int(state.get("owner_org_id") or 0)
        ai = _ai_preflight(auth, owner_org)
        put("ai", {"ok": ai.get("status") == "ready", **ai})
        git = _git_preflight(auth, owner_org)
        put("git", {"ok": git.get("status") == "ready", **git})
    else:
        put("ai", {"ok": False, "status": "skipped", "detail": "auth/state unavailable"})
        put("git", {"ok": False, "status": "skipped", "detail": "auth/state unavailable"})

    lib.write_json(lib.PREFLIGHT_PATH, checks)
    print("E2E preflight")
    for name, value in checks["checks"].items():
        flag = "ok" if value.get("ok") else ("--" if value.get("status") == "skipped" else "!!")
        detail = value.get("detail") or value.get("status") or ""
        print(f"  [{flag}] {name}: {detail}")
    print(f"  wrote {lib.PREFLIGHT_PATH}")

    if args.strict:
        if not critical_ok:
            return 1
        # Strict means "prove the full capability set", including AI and git.
        for name, value in checks["checks"].items():
            if value.get("status") == "skipped":
                continue
            if not value.get("ok"):
                print(f"Strict preflight failed: {name}", file=sys.stderr)
                return 1
    return 0


def main() -> int:
    lib.load_env()
    parser = argparse.ArgumentParser(description="LiveReview E2E capability preflight")
    parser.add_argument("--strict", action="store_true", help="fail when API/UI/auth are not ready")
    args = parser.parse_args()
    return cmd_preflight(args)


if __name__ == "__main__":
    raise SystemExit(main())
