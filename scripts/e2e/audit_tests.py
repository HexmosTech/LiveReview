#!/usr/bin/env python3
"""Real-account audit assertions for the local E2E harness.

These tests intentionally use the real API and the roles provisioned by
`control.py up` (super admin + owner + ephemeral member).  They never delete
audit history; the append-only assertion is run inside a rolled-back psql
transaction.
"""

from __future__ import annotations

import argparse
import json
import sys
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

sys.path.insert(0, str(Path(__file__).resolve().parent))

import e2e_lib as lib


class Results:
    def __init__(self) -> None:
        self.rows: List[Dict[str, Any]] = []
        self.failures = 0

    def pass_(self, name: str, detail: str = "") -> None:
        self.rows.append({"name": name, "status": "pass", "detail": detail})
        print(f"  [ok] {name}" + (f": {detail}" if detail else ""))

    def skip(self, name: str, detail: str) -> None:
        self.rows.append({"name": name, "status": "skip", "detail": detail})
        print(f"  [--] {name}: {detail}")

    def fail(self, name: str, detail: str) -> None:
        self.failures += 1
        self.rows.append({"name": name, "status": "fail", "detail": detail})
        print(f"  [!!] {name}: {detail}")

    def check(self, name: str, ok: bool, detail: str = "") -> None:
        (self.pass_ if ok else self.fail)(name, detail)


def _window(days: int = 30) -> Dict[str, str]:
    now = datetime.now(timezone.utc)
    return {
        "from": (now - timedelta(days=days)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "to": (now + timedelta(minutes=5)).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }


def _token(auth: Dict[str, Any]) -> str:
    token = (auth.get("tokens") or {}).get("access_token")
    if not token:
        raise lib.AuthRequired("No access token")
    return token


def _events(
    auth: Dict[str, Any],
    org_id: int,
    *,
    admin: bool = False,
    extra: Optional[Dict[str, str]] = None,
    limit: int = 200,
) -> Dict[str, Any]:
    window = _window()
    params = dict(window)
    params["limit"] = str(limit)
    if extra:
        params.update({k: v for k, v in extra.items() if v is not None})
    query = "&".join(f"{k}={lib.urllib.parse.quote(str(v))}" for k, v in params.items())
    path = (
        f"/api/v1/admin/audit-events?org_id={org_id}&{query}"
        if admin
        else f"/api/v1/orgs/{org_id}/audit-events?{query}"
    )
    status, payload, _ = lib.api("GET", path, token=_token(auth), org_id=org_id)
    if status != 200 or not isinstance(payload, dict):
        raise lib.ApiError("GET", path, status, payload)
    return payload


def _admin_all_events(
    auth: Dict[str, Any], *, extra: Optional[Dict[str, str]] = None, limit: int = 500
) -> Dict[str, Any]:
    window = _window()
    params = dict(window)
    params["limit"] = str(limit)
    if extra:
        params.update({k: v for k, v in extra.items() if v is not None})
    query = "&".join(f"{k}={lib.urllib.parse.quote(str(v))}" for k, v in params.items())
    path = f"/api/v1/admin/audit-events?{query}"
    status, payload, _ = lib.api("GET", path, token=_token(auth))
    if status != 200 or not isinstance(payload, dict):
        raise lib.ApiError("GET", path, status, payload)
    return payload


def _actions(payload: Dict[str, Any]) -> List[str]:
    return [str(e.get("action") or "") for e in payload.get("events") or []]


def _find(payload: Dict[str, Any], predicate) -> Optional[Dict[str, Any]]:
    for event in payload.get("events") or []:
        if predicate(event):
            return event
    return None


def _export(
    auth: Dict[str, Any],
    org_id: int,
    fmt: str,
    *,
    admin: bool = False,
) -> Dict[str, Any]:
    window = _window(30)
    base = f"/api/v1/admin/audit-events/export?org_id={org_id}" if admin else f"/api/v1/orgs/{org_id}/audit-events/export?"
    sep = "&" if "?" in base else "?"
    url = f"{base}{sep}from={lib.urllib.parse.quote(window['from'])}&to={lib.urllib.parse.quote(window['to'])}&format={fmt}"
    status, payload, headers = lib.api(
        "GET", url, token=_token(auth), org_id=org_id, raw=True, timeout=90
    )
    return {"status": status, "body": payload or b"", "headers": headers}


def setup_context() -> Dict[str, Any]:
    state = lib.load_state()
    auth = lib.get_auth()
    if not state:
        raise RuntimeError("No E2E state. Run `make dev-up NICEURL=niceurl3` or `make e2e-up` first.")
    if not auth or not lib.validate_access(auth):
        auth = lib.ensure_auth(interactive=False, allow_browser=False)
    owner = int(state["owner_org_id"])
    super_org = int(state.get("super_org_id") or owner)
    member_auth = lib.ensure_fixture_auth(state.get("member_auth"), "member")
    owner_auth = lib.ensure_fixture_auth(state.get("owner_auth"), "owner")
    state["member_auth"] = member_auth
    state["owner_auth"] = owner_auth
    lib.save_state(state)
    return {
        "state": state,
        "auth": auth,
        "owner_auth": owner_auth,
        "member_auth": member_auth,
        "owner_org": owner,
        "super_org": super_org,
        "member_id": int(state["member_id"]),
    }


def run_tests(ctx: Dict[str, Any]) -> Results:
    r = Results()
    auth = ctx["auth"]
    owner_auth = ctx["owner_auth"]
    member_auth = ctx["member_auth"]
    owner = ctx["owner_org"]
    super_org = ctx["super_org"]
    member_id = ctx["member_id"]
    state = ctx["state"]
    member_email = state["member_email"]

    # Make sure there is a failed-login event against the member's email. The
    # member must not see it; owner/super admin must.
    lib.api(
        "POST",
        "/api/v1/auth/login",
        body={"email": member_email, "password": "definitely-not-the-password"},
        raise_on_error=False,
    )
    # The invitation API is unavailable on a fully local stack, so the member
    # fixture may be created directly in the DB. Generate a semantic
    # owner-performed user update so the owner-visibility assertions always
    # have a real API-emitted event to find.
    lib.api(
        "PUT",
        f"/api/v1/orgs/{owner}/users/{member_id}",
        token=_token(owner_auth),
        org_id=owner,
        body={"first_name": "E2E Member"},
        raise_on_error=False,
    )

    # ---------------------------------------------------------------- read APIs
    try:
        events = _events(owner_auth, owner, limit=50)
        r.check(
            "owner audit list",
            len(events.get("events") or []) > 0,
            f"{len(events.get('events') or [])} rows in the last 30 days",
        )
        status, meta, _ = lib.api(
            "GET", f"/api/v1/orgs/{owner}/audit-events/meta", token=_token(owner_auth), org_id=owner
        )
        r.check("audit meta", status == 200 and isinstance(meta, dict), f"HTTP {status}")
        status, facets, _ = lib.api(
            "GET", f"/api/v1/orgs/{owner}/audit-events/facets", token=_token(owner_auth), org_id=owner
        )
        r.check(
            "audit facets",
            status == 200 and isinstance(facets, dict) and bool(facets.get("categories") or facets.get("actions")),
            f"HTTP {status}",
        )
    except Exception as exc:
        r.fail("owner audit read APIs", f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------ owner visibility
    try:
        owner_events = _events(owner_auth, owner, limit=500)
        owner_actions = _actions(owner_events)
        r.check(
            "owner sees member provisioning/update",
            "user.create" in owner_actions or "user.update" in owner_actions,
            f"actions={','.join(sorted(set(owner_actions))[:12])}",
        )
        r.check(
            "owner sees member sign-in",
            "auth.login.success" in owner_actions,
            "member login event present",
        )
        admin_rows = [
            e for e in owner_events.get("events") or []
            if str(e.get("actor_capacity") or "") == "admin"
            or str(e.get("actor_kind") or "") == "super_admin"
        ]
        r.check("owner excludes admin-capacity rows", not admin_rows, f"{len(admin_rows)} admin rows leaked")
        r.check(
            "owner does not see super-admin-only actions",
            "admin.user.create" not in owner_actions,
            "admin.user.create absent",
        )
    except Exception as exc:
        r.fail("owner visibility", f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------ member visibility
    try:
        member_events = _events(member_auth, owner, limit=500)
        foreign = [
            e for e in member_events.get("events") or []
            if e.get("actor_user_id") is not None and int(e.get("actor_user_id")) != member_id
        ]
        r.check("member sees only self", not foreign, f"{len(foreign)} foreign rows")
        r.check(
            "member sees own sign-in",
            "auth.login.success" in _actions(member_events),
            "own login present",
        )
        failed = _events(
            member_auth, owner, extra={"action": "auth.login.failure"}, limit=50
        )
        r.check(
            "member cannot see failed login against own email",
            not failed.get("events"),
            f"{len(failed.get('events') or [])} failure rows visible",
        )
        owner_failed = _events(
            owner_auth, owner, extra={"action": "auth.login.failure", "actor_email": member_email}, limit=50
        )
        r.check(
            "owner sees failed login against member email",
            bool(owner_failed.get("events")),
            f"{len(owner_failed.get('events') or [])} failure rows visible",
        )
    except Exception as exc:
        r.fail("member visibility", f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------- super-admin visibility
    try:
        if super_org == owner:
            r.skip("super-admin cross-org view", "owner and super-admin org are the same")
        else:
            admin_events = _events(auth, owner, admin=True, limit=500)
            admin_actions = _actions(admin_events)
            has_admin_capacity = any(
                str(e.get("actor_capacity") or "") == "admin"
                or str(e.get("actor_kind") or "") == "super_admin"
                for e in admin_events.get("events") or []
            )
            r.check(
                "super admin sees admin-capacity rows",
                has_admin_capacity or state.get("admin_member_id") is None,
                "admin capacity present" if has_admin_capacity else "admin fixture unavailable; not a leak",
            )
            r.check(
                "super admin sees cross-org member event",
                "user.create" in admin_actions or "auth.login.success" in admin_actions,
                f"admin rows={len(admin_events.get('events') or [])}",
            )
    except Exception as exc:
        r.fail("super-admin visibility", f"{type(exc).__name__}: {exc}")

    # -------------------------------------- admin-capacity semantic event
    try:
        status, settings, _ = lib.api(
            "GET", "/api/v1/admin/audit-settings", token=_token(auth), raise_on_error=False
        )
        if status == 200 and isinstance(settings, dict):
            put_status, _, _ = lib.api(
                "PUT",
                "/api/v1/admin/audit-settings",
                token=_token(auth),
                body=settings,
                raise_on_error=False,
            )
            if put_status == 200:
                time.sleep(1.0)
                all_admin = _admin_all_events(auth, limit=500)
                retention_rows = [
                    e for e in all_admin.get("events") or []
                    if str(e.get("action") or "") == "settings.retention.update"
                ]
                admin_retention = [
                    e for e in retention_rows
                    if str(e.get("actor_capacity") or "") == "admin"
                    or str(e.get("actor_kind") or "") == "super_admin"
                ]
                r.check(
                    "super-admin settings update recorded as admin capacity",
                    bool(admin_retention),
                    f"{len(admin_retention)} admin rows",
                )
                owner_after = _events(owner_auth, owner, limit=500)
                r.check(
                    "owner excludes global admin settings event",
                    "settings.retention.update" not in _actions(owner_after),
                    "global retention event absent from org view",
                )
            else:
                r.skip("admin-capacity semantic event", f"settings PUT HTTP {put_status}")
        else:
            r.skip("admin-capacity semantic event", f"settings GET HTTP {status}")
    except Exception as exc:
        r.fail("admin-capacity semantic event", f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------- cross-tenant gate
    try:
        if super_org == owner:
            r.skip("cross-org denial", "owner and super-admin org are the same")
        else:
            # Use the member token: the real owner account is also a super
            # admin globally, so only the member provides a true non-member
            # cross-tenant probe.
            status, _, _ = lib.api(
                "GET",
                f"/api/v1/orgs/{super_org}/audit-events?limit=1",
                token=_token(member_auth),
                org_id=super_org,
                raise_on_error=False,
            )
            r.check("cross-org denial", status in (401, 403, 404), f"HTTP {status}")
    except Exception as exc:
        r.fail("cross-org denial", f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------ API key gating
    try:
        audit_key = state.get("audit_key")
        standard_key = state.get("standard_key")
        if audit_key:
            status, _, _ = lib.api(
                "GET",
                f"/api/v1/orgs/{owner}/audit-events?limit=1",
                api_key=audit_key,
                org_id=owner,
                raise_on_error=False,
            )
            r.check("audit_reader key can read audit", status == 200, f"HTTP {status}")
            status, _, _ = lib.api(
                "GET",
                "/api/v1/reviews?limit=1",
                api_key=audit_key,
                org_id=owner,
                raise_on_error=False,
            )
            r.check("audit_reader key cannot read reviews", status in (401, 403), f"HTTP {status}")
        else:
            r.skip("audit_reader key", "no audit key in state")
        if standard_key:
            status, _, _ = lib.api(
                "GET",
                f"/api/v1/orgs/{owner}/audit-events?limit=1",
                api_key=standard_key,
                org_id=owner,
                raise_on_error=False,
            )
            r.check("standard key cannot read audit", status in (401, 403), f"HTTP {status}")
        else:
            r.skip("standard key audit denial", "no standard key in state")
    except Exception as exc:
        r.fail("API key gating", f"{type(exc).__name__}: {exc}")

    # ----------------------------------------------------------- exports
    try:
        before = _events(owner_auth, owner, limit=500)
        before_actions_list = _actions(before)
        before_actions = set(before_actions_list)
        for fmt, expected_action in (
            ("csv", "export.audit"),
            ("ndjson", "export.audit"),
            ("package", "export.package"),
        ):
            result = _export(owner_auth, owner, fmt)
            body = result["body"]
            headers = result["headers"]
            r.check(
                f"audit export {fmt}",
                result["status"] == 200 and len(body) > 0,
                f"HTTP {result['status']}, {len(body)} bytes",
            )
            r.check(
                f"audit export {fmt} disposition",
                "content-disposition" in headers,
                headers.get("content-disposition", "missing"),
            )
        time.sleep(1.2)
        after = _events(owner_auth, owner, limit=500)
        after_actions_list = _actions(after)
        after_actions = set(after_actions_list)
        r.check(
            "exports emit sensitive-read audit events",
            after_actions_list.count("export.audit") > before_actions_list.count("export.audit")
            and after_actions_list.count("export.package") > before_actions_list.count("export.package"),
            f"new actions={','.join(sorted(after_actions - before_actions))}",
        )
    except Exception as exc:
        r.fail("audit exports", f"{type(exc).__name__}: {exc}")

    # ----------------------------------------------------------- AI usage
    try:
        window = _window(30)
        status, payload, _ = lib.api(
            "GET",
            f"/api/v1/orgs/{owner}/audit-events/ai-usage?from={lib.urllib.parse.quote(window['from'])}&to={lib.urllib.parse.quote(window['to'])}&limit=50",
            token=_token(owner_auth),
            org_id=owner,
        )
        rows = payload.get("rows") if isinstance(payload, dict) else None
        r.check("AI usage endpoint", status == 200 and isinstance(rows, list), f"HTTP {status}, {len(rows or [])} rows")
        if isinstance(rows, list):
            admin_rows = [x for x in rows if str(x.get("actor_kind") or "") == "super_admin"]
            r.check("owner AI usage excludes super-admin usage", not admin_rows, f"{len(admin_rows)} rows")
            preflight = lib.read_json(lib.PREFLIGHT_PATH, {})
            ai_ready = (
                isinstance(preflight, dict)
                and ((preflight.get("checks") or {}).get("ai") or {}).get("status") == "ready"
            )
            review_report = lib.read_json(lib.STATE_DIR / "review-report.json", {})
            review_ok = isinstance(review_report, dict) and bool(review_report.get("ok"))
            if ai_ready and review_ok:
                r.check(
                    "AI usage reflects the real review",
                    len(rows) > 0,
                    f"{len(rows)} ledger rows for the completed review",
                )
            else:
                r.skip("AI usage reflects the real review", "AI/review capability was not ready")
    except Exception as exc:
        r.fail("AI usage", f"{type(exc).__name__}: {exc}")

    # -------------------------------------------------- integrity & policy
    try:
        # The owner org has historical rows from before the chain/backfill
        # normalization. Report that as a diagnostic, but prove the current
        # write path on a clean org so the test does not confuse old data with
        # a live regression.
        status, owner_report, _ = lib.api(
            "GET",
            f"/api/v1/admin/audit-events/integrity?org_id={owner}&limit=500",
            token=_token(auth),
            raise_on_error=False,
        )
        if status == 200 and isinstance(owner_report, dict) and not owner_report.get("intact"):
            print(
                "  [--] owner-org chain diagnostic: "
                f"broken at seq {owner_report.get('first_broken_seq')} "
                f"({owner_report.get('first_broken_reason')}); historical rows are not rewritten"
            )

        integrity_org = int(state.get("integrity_org_id") or 0)
        if integrity_org and integrity_org != owner:
            key = lib.create_api_key(
                auth,
                integrity_org,
                label=f"e2e-{state.get('run_id')}-integrity-probe",
                kind="standard",
            )
            state["integrity_key_id"] = key["id"]
            lib.save_state(state)
            lib.update_cleanup(
                integrity_key_id=key["id"], integrity_org_id=integrity_org
            )
            time.sleep(0.7)
            status, report, _ = lib.api(
                "GET",
                f"/api/v1/admin/audit-events/integrity?org_id={integrity_org}&limit=500",
                token=_token(auth),
                raise_on_error=False,
            )
            r.check(
                "fresh-org hash chain integrity",
                status == 200
                and isinstance(report, dict)
                and bool(report.get("intact"))
                and int(report.get("verified") or 0) >= 1,
                f"HTTP {status}, verified={report.get('verified') if isinstance(report, dict) else '?'}, rows={report.get('rows') if isinstance(report, dict) else '?'}",
            )
            lib.revoke_api_key(auth, integrity_org, int(key["id"]))
            state.pop("integrity_key_id", None)
            lib.save_state(state)
        else:
            r.skip("fresh-org hash chain integrity", "no separate owner org is available")

        status, settings, _ = lib.api(
            "GET", "/api/v1/admin/audit-settings", token=_token(auth), raise_on_error=False
        )
        r.check("retention settings readable", status == 200 and isinstance(settings, dict), f"HTTP {status}")
        status, _, _ = lib.api(
            "GET", "/api/v1/admin/audit-events?limit=1", token=_token(member_auth), raise_on_error=False
        )
        r.check("member cannot use admin audit endpoint", status in (401, 403, 404), f"HTTP {status}")
    except Exception as exc:
        r.fail("integrity/settings", f"{type(exc).__name__}: {exc}")

    # ------------------------------------------------------ append-only check
    try:
        sample = _events(owner_auth, owner, limit=1)
        events = sample.get("events") or []
        if not events:
            r.skip("append-only trigger", "no sample audit row")
        else:
            event_id = int(events[0]["id"])
            rc, out, err = lib.db_query(
                "BEGIN; UPDATE audit_events SET outcome = 'tampered' WHERE id = $1; ROLLBACK;",
                [str(event_id)],
            )
            combined = (out + err).lower()
            r.check(
                "audit rows reject UPDATE",
                rc != 0 and ("append" in combined or "audit" in combined or "permission" in combined),
                f"psql rc={rc}",
            )
    except Exception as exc:
        r.fail("append-only trigger", f"{type(exc).__name__}: {exc}")

    report = {
        "generated_at": lib.now_iso(),
        "run_id": state.get("run_id"),
        "base_url": lib.base_url(),
        "owner_org_id": owner,
        "super_org_id": super_org,
        "results": r.rows,
        "failures": r.failures,
    }
    lib.write_json(lib.STATE_DIR / "audit-report.json", report)
    return r


def main() -> int:
    lib.load_env()
    parser = argparse.ArgumentParser(description="LiveReview audit E2E assertions")
    parser.add_argument("--json-out", default=str(lib.STATE_DIR / "audit-report.json"))
    args = parser.parse_args()
    lib.secure_dir(lib.STATE_DIR)
    try:
        ctx = setup_context()
    except Exception as exc:
        print(f"Audit E2E setup failed: {type(exc).__name__}: {exc}", file=sys.stderr)
        return 2
    print(f"Audit E2E: {lib.base_url()} / org {ctx['owner_org']} / run {ctx['state'].get('run_id')}")
    results = run_tests(ctx)
    if args.json_out:
        # The runner writes the canonical report; keep the requested path too.
        if Path(args.json_out) != lib.STATE_DIR / "audit-report.json":
            lib.write_json(Path(args.json_out), {
                "results": results.rows,
                "failures": results.failures,
            })
    print(f"Audit E2E: {len(results.rows) - results.failures} passed, {results.failures} failed")
    return 0 if results.failures == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
