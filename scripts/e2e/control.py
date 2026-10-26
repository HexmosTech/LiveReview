#!/usr/bin/env python3
"""Lifecycle control plane for the local LiveReview E2E harness."""

from __future__ import annotations

import argparse
import hashlib
import os
import secrets
import shutil
import sys
from pathlib import Path
from typing import Any, Dict, Optional

sys.path.insert(0, str(Path(__file__).resolve().parent))

import e2e_lib as lib


def _safe_auth_summary(auth: Dict[str, Any]) -> str:
    user = auth.get("user") or {}
    orgs = auth.get("organizations") or []
    email = user.get("email") or auth.get("email") or "(unknown)"
    roles = ", ".join(f"{o.get('id')}:{o.get('role')}" for o in orgs[:6])
    return f"{email} ({roles or 'no organizations'})"


def _hash_file(path: Path) -> Optional[str]:
    try:
        return hashlib.sha256(path.read_bytes()).hexdigest()
    except OSError:
        return None


def _user_lrc_path() -> Path:
    home = Path(os.environ.get("HOME") or str(Path.home()))
    return home / ".lrc.toml"


def _lrc_content(api_key: str, org_id: int, auth: Dict[str, Any]) -> str:
    tokens = auth.get("tokens") or {}
    email = (auth.get("user") or {}).get("email") or lib.cfg("E2E_EMAIL", "")
    return "\n".join(
        [
            f'api_key = "{api_key}"',
            f'api_url = "{lib.api_url()}"',
            f'org_id = "{org_id}"',
            f'user_email = "{email}"',
            f'jwt = "{tokens.get("access_token", "")}"',
            f'refresh_token = "{tokens.get("refresh_token", "")}"',
            "",
        ]
    )


def _recover_user_lrc() -> None:
    journal = lib.read_json(lib.LRC_JOURNAL_PATH)
    if not isinstance(journal, dict):
        return
    if not journal.get("active"):
        try:
            lib.LRC_JOURNAL_PATH.unlink()
        except FileNotFoundError:
            pass
        shutil.rmtree(lib.LRC_BACKUP_DIR, ignore_errors=True)
        return
    target = Path(journal.get("target") or _user_lrc_path())
    backup = Path(journal.get("backup") or "")
    existed = bool(journal.get("existed"))
    try:
        if existed and backup.exists():
            shutil.copy2(backup, target)
            os.chmod(target, 0o600)
            print(f"Recovered original {target} from the E2E journal.")
        elif not existed:
            try:
                target.unlink()
            except FileNotFoundError:
                pass
            print(f"Removed the temporary E2E lrc config at {target}; it did not exist before.")
    finally:
        journal["active"] = False
        journal["recovered_at"] = lib.now_iso()
        # Keep the journal only while a write is active. Once restored, remove
        # the journal and backup so an idle repo has no lingering config copy.
        try:
            lib.LRC_JOURNAL_PATH.unlink()
        except FileNotFoundError:
            pass
        shutil.rmtree(lib.LRC_BACKUP_DIR, ignore_errors=True)


def _write_user_lrc(api_key: str, org_id: int, auth: Dict[str, Any]) -> None:
    target = _user_lrc_path()
    if target.exists():
        digest = _hash_file(target) or "unreadable"
        backup = lib.LRC_BACKUP_DIR / f"lrc-{digest[:12]}.toml"
        lib.secure_dir(backup.parent)
        shutil.copy2(target, backup)
        os.chmod(backup, 0o600)
        existed = True
    else:
        backup = lib.LRC_BACKUP_DIR / "lrc-missing.toml"
        existed = False
    lib.write_json(
        lib.LRC_JOURNAL_PATH,
        {
            "active": True,
            "target": str(target),
            "backup": str(backup),
            "existed": existed,
            "original_sha256": _hash_file(target),
            "created_at": lib.now_iso(),
            "note": "Written only when E2E_USE_USER_LRC=1; restored by dev-down and every later up/status.",
        },
    )
    lib.secure_write(target, _lrc_content(api_key, org_id, auth))


def _provision_lrc(api_key: str, org_id: int, auth: Dict[str, Any], state: Dict[str, Any]) -> None:
    if lib.cfg("E2E_USE_USER_LRC", "0") == "1":
        _write_user_lrc(api_key, org_id, auth)
        state["lrc_mode"] = "user"
        state["lrc_config"] = str(_user_lrc_path())
    else:
        lib.write_lrc_home(api_key, org_id, auth)
        state["lrc_mode"] = "sandbox"
        state["lrc_config"] = str(lib.LRC_HOME / ".lrc.toml")


def _member_password() -> str:
    alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
    return "E2e-" + "".join(secrets.choice(alphabet) for _ in range(20)) + "!7"


def cmd_up(args: argparse.Namespace) -> int:
    lib.load_env()
    prior_journal = lib.read_json(lib.LRC_JOURNAL_PATH, {})
    had_journal = isinstance(prior_journal, dict) and bool(prior_journal.get("active"))
    _recover_user_lrc()  # recovers a prior temp-mode write before doing anything else
    lib.secure_dir(lib.STATE_DIR)

    existing = lib.load_state()
    if existing and not args.fresh and not had_journal:
        auth = lib.get_auth()
        lrc_path = Path(str(existing.get("lrc_config") or ""))
        lrc_ok = bool(lrc_path and lrc_path.exists())
        fixtures_ok = bool(existing.get("member_auth") and existing.get("owner_auth"))
        if auth and lib.validate_access(auth) and lrc_ok and fixtures_ok:
            print(f"E2E already provisioned for run {existing.get('run_id')}.")
            _print_summary(existing, auth)
            return 0
        # A stale state with no usable auth or missing lrc config: clean it up,
        # then reprovision.
        notes = lib.cleanup_state()
        for note in notes:
            print(f"  cleanup: {note}")

    if existing and args.fresh:
        notes = lib.cleanup_state()
        for note in notes:
            print(f"  cleanup: {note}")

    try:
        auth = lib.ensure_auth(
            interactive=args.interactive,
            allow_browser=args.interactive,
            headed=not args.headless,
        )
    except lib.AuthRequired as exc:
        if args.optional:
            print(f"E2E not provisioned: {exc}")
            return 0
        print(f"E2E auth required: {exc}", file=sys.stderr)
        return 2

    orgs = lib.select_orgs(auth)
    run = lib.run_id()
    owner_org = orgs["owner"]
    super_org = orgs["super"]

    print(f"Provisioning E2E run {run} (base={lib.base_url()}, api={lib.api_url()})")
    print(f"Owner org: {owner_org}; super-admin org: {super_org}")

    integrity_org = 0
    for org in auth.get("organizations") or []:
        try:
            oid = int(org.get("id"))
        except (TypeError, ValueError):
            continue
        if oid != owner_org and str(org.get("role") or "") == "owner":
            integrity_org = oid
            break

    state: Dict[str, Any] = {
        "run_id": run,
        "created_at": lib.now_iso(),
        "base_url": lib.base_url(),
        "api_url": lib.api_url(),
        "owner_org_id": owner_org,
        "super_org_id": super_org,
        "integrity_org_id": integrity_org,
        "user_lrc_sha256_before": _hash_file(_user_lrc_path()),
    }
    lib.save_state(state)
    lib.update_cleanup(run_id=run, owner_org_id=owner_org)

    try:
        standard = lib.create_api_key(
            auth, owner_org, label=f"e2e-{run}-standard", kind="standard"
        )
        state["standard_key_id"] = standard["id"]
        state["standard_key"] = standard["plain_key"]
        lib.save_state(state)
        lib.update_cleanup(standard_key_id=standard["id"])

        audit_key = lib.create_api_key(
            auth, owner_org, label=f"e2e-{run}-audit-reader", kind="audit_reader"
        )
        state["audit_key_id"] = audit_key["id"]
        state["audit_key"] = audit_key["plain_key"]
        lib.save_state(state)
        lib.update_cleanup(audit_key_id=audit_key["id"])

        member_email = f"e2e+{run}@example.com"
        member_password = _member_password()
        member = lib.create_member(
            auth, owner_org, email=member_email, password=member_password, run=run
        )
        state["member_id"] = member["id"]
        state["member_email"] = member_email
        lib.save_state(state)
        lib.update_cleanup(member_id=member["id"])

        # The generated password is used once and deliberately never persisted.
        # Only the resulting session/refresh tokens are kept.
        member_auth = lib.login_member(member_email, member_password)
        state["member_auth"] = member_auth

        # The supplied real account is a global super admin, so acting in org
        # 151 still resolves as super_admin. Create a pure owner fixture to
        # exercise the real owner-scope (which intentionally excludes support
        # actions) rather than testing a super admin wearing an owner hat.
        owner_email = f"e2e-owner+{run}@example.com"
        owner_password = _member_password()
        owner_user = lib.create_member(
            auth,
            owner_org,
            email=owner_email,
            password=owner_password,
            run=run,
            role_id=2,
            first_name="E2E Owner",
        )
        state["owner_member_id"] = owner_user["id"]
        state["owner_email"] = owner_email
        lib.save_state(state)
        lib.update_cleanup(owner_member_id=owner_user["id"])
        owner_auth = lib.login_member(owner_email, owner_password)
        state["owner_auth"] = owner_auth

        # Submit the real review as the pure owner, not the global super admin:
        # owner AI-usage queries intentionally hide super-admin work, so using
        # the super admin's key would make the owner tab legitimately empty.
        owner_key = lib.create_api_key(
            owner_auth,
            owner_org,
            label=f"e2e-{run}-owner-lrc",
            kind="standard",
        )
        state["lrc_key_id"] = owner_key["id"]
        state["lrc_key"] = owner_key["plain_key"]
        lib.save_state(state)
        lib.update_cleanup(lrc_key_id=owner_key["id"])

        admin_member: Optional[Dict[str, Any]] = None
        try:
            admin_email = f"e2e-admin+{run}@example.com"
            admin_password = _member_password()
            admin_member = lib.admin_create_member(
                auth, owner_org, email=admin_email, password=admin_password, run=run
            )
            state["admin_member_id"] = admin_member["id"]
            state["admin_member_email"] = admin_member["email"]
            lib.save_state(state)
            lib.update_cleanup(admin_member_id=admin_member["id"])
        except Exception as exc:
            # The admin-capacity assertion is valuable but should not prevent
            # the rest of the harness from running on an account or deployment
            # where the admin invitation path is unavailable.
            print(f"  note: admin-created member fixture unavailable ({type(exc).__name__}).")

        _provision_lrc(owner_key["plain_key"], owner_org, auth, state)

        lib.playwright_state(owner_auth, owner_org, lib.PLAYWRIGHT_STATE_PATH)
        lib.playwright_state(auth, super_org, lib.PLAYWRIGHT_STATE_SUPER_PATH)
        lib.playwright_state(member_auth, owner_org, lib.PLAYWRIGHT_STATE_MEMBER_PATH)

        lib.save_state(state)
        lib.write_run_env(state)
        _print_summary(state, auth)
        return 0
    except Exception:
        # Never leave ephemeral keys or a temp member behind when provisioning
        # fails halfway through.
        notes = lib.cleanup_state()
        for note in notes:
            print(f"  cleanup: {note}", file=sys.stderr)
        raise


def _print_summary(state: Dict[str, Any], auth: Dict[str, Any]) -> None:
    print("")
    print("  E2E ready")
    print(f"    run:        {state.get('run_id')}")
    print(f"    browser:    {lib.base_url()}")
    print(f"    api:        {lib.api_url()}")
    print(f"    auth:       {_safe_auth_summary(auth)}")
    print(f"    lrc mode:   {state.get('lrc_mode')} ({state.get('lrc_config')})")
    print(f"    lrc shell:  source {lib.RUN_ENV_PATH}")
    print(f"    lrc wrapper:{lib.ROOT / 'scripts/e2e/lrc.sh'}")
    print(f"    tests:      make e2e")
    print("")


def cmd_down(args: argparse.Namespace) -> int:
    lib.load_env()
    notes = lib.cleanup_state(keep_auth=not args.forget_auth)
    _recover_user_lrc()
    if notes:
        for note in notes:
            print(f"  cleanup: {note}")
    else:
        print("E2E cleanup complete.")
    if args.forget_auth:
        print("Cached E2E session removed; run `make e2e-auth` before the next run.")
    return 0


def cmd_status(args: argparse.Namespace) -> int:
    lib.load_env()
    state = lib.load_state()
    auth = lib.get_auth()
    print(f"Base URL: {lib.base_url()}")
    print(f"API URL:  {lib.api_url()}")
    print(f"State:    {lib.STATE_DIR}")
    print(f"API:      {'up' if lib.wait_http(lib.api_url() + '/health', 2) else 'down'}")
    if not state:
        print("E2E state: not provisioned (run `make dev-up` or `make e2e-up`)")
    else:
        print(f"E2E run:  {state.get('run_id')} (created {state.get('created_at')})")
        print(f"Owner org: {state.get('owner_org_id')}  super org: {state.get('super_org_id')}")
        print(f"lrc:      {state.get('lrc_mode')} -> {state.get('lrc_config')}")
        print(f"member:   {state.get('member_email')}")
    if auth:
        print(f"Auth:     {'valid' if lib.validate_access(auth) else 'expired'} ({_safe_auth_summary(auth)})")
    else:
        print("Auth:     none (run `make e2e-auth`)")
    if lib.LRC_JOURNAL_PATH.exists():
        journal = lib.read_json(lib.LRC_JOURNAL_PATH, {})
        if journal.get("active"):
            print(f"WARNING: temporary user-lrc journal is active: {journal.get('target')}")
    return 0


def cmd_auth(args: argparse.Namespace) -> int:
    lib.load_env()
    lib.secure_dir(lib.STATE_DIR)
    try:
        # Explicit auth refresh must not silently reuse whatever member token
        # happens to be cached.
        email = lib.cfg("E2E_EMAIL")
        password = lib.cfg("E2E_PASSWORD")
        if email and password:
            lib.password_login(email, password, save=True)
            # Reload from disk rather than passing the login response object
            # into the summary print.  The cached file contains no password.
            auth = lib.get_auth()
            if not auth:
                raise lib.AuthRequired("Password login succeeded but no cached session was written.")
        else:
            auth = lib.browser_auth(headed=not args.headless)
    except lib.AuthRequired as exc:
        print(str(exc), file=sys.stderr)
        return 2
    orgs = lib.select_orgs(auth)
    # _safe_auth_summary only emits email + org roles; it never touches tokens.
    print(f"Authenticated: {_safe_auth_summary(auth)}")  # codeql[py/clear-text-logging-sensitive-data]
    print(f"Selected owner org {orgs['owner']}, super-admin org {orgs['super']}.")
    print(f"Cached at {lib.AUTH_PATH} (mode 0600).")
    return 0


def cmd_doctor(args: argparse.Namespace) -> int:
    lib.load_env()
    problems = 0
    fixes = []

    def check(ok: bool, label: str, detail: str = "") -> None:
        nonlocal problems
        print(f"  [{'ok' if ok else '!!'}] {label}" + (f": {detail}" if detail else ""))
        if not ok:
            problems += 1

    print("E2E doctor")
    check(bool(shutil.which("node")), "node available")
    check(bool(shutil.which("git")), "git available")
    check(bool(shutil.which("psql")), "psql available")
    check(bool(shutil.which("lrc")), "lrc available", shutil.which("lrc") or "")
    check(
        lib.cfg("E2E_EMAIL") is not None,
        "E2E_EMAIL configured",
        lib.mask(lib.cfg("E2E_EMAIL")) if lib.cfg("E2E_EMAIL") else "set it in .env.e2e.local",
    )
    check(
        lib.STATE_DIR.exists() and (lib.STATE_DIR.stat().st_mode & 0o077) == 0,
        "state directory private",
        str(lib.STATE_DIR),
    )
    if lib.STATE_DIR.exists():
        lib.secure_dir(lib.STATE_DIR)

    e2e_env = lib.ROOT / ".env.e2e.local"
    if e2e_env.exists():
        mode = e2e_env.stat().st_mode & 0o777
        check(mode == 0o600, ".env.e2e.local mode 0600", oct(mode))
        if mode != 0o600:
            os.chmod(e2e_env, 0o600)
            fixes.append("chmod 600 .env.e2e.local")
        try:
            proc = lib.run(["git", "check-ignore", "-q", str(e2e_env)], check=False)
            check(proc.returncode == 0, ".env.e2e.local is git-ignored")
        except Exception:
            check(False, ".env.e2e.local git-ignore check")
    else:
        print("  [--] .env.e2e.local not present; copy scripts/e2e/e2e.env.example")

    auth = lib.get_auth()
    if auth:
        check(lib.validate_access(auth), "cached session valid", _safe_auth_summary(auth))
    else:
        print("  [--] no cached session (run `make e2e-auth`)")

    state = lib.load_state()
    if state:
        print(f"  [ok] run state present: {state.get('run_id')}")
        check((lib.LRC_HOME / ".lrc.toml").exists() or state.get("lrc_mode") == "user", "lrc config provisioned")
        before = state.get("user_lrc_sha256_before")
        after = _hash_file(_user_lrc_path())
        if state.get("lrc_mode") == "sandbox":
            check(before == after, "real ~/.lrc.toml untouched")
    else:
        print("  [--] no E2E run state (run `make dev-up` or `make e2e-up`)")

    journal = lib.read_json(lib.LRC_JOURNAL_PATH, {})
    if isinstance(journal, dict) and journal.get("active"):
        print(f"  [!!] temporary user-lrc write is active: {journal.get('target')}")
        problems += 1

    if fixes:
        print("Fixes applied: " + ", ".join(fixes))
    print(f"Doctor result: {'ok' if problems == 0 else str(problems) + ' issue(s)'}")
    return 0 if problems == 0 else 1


def cmd_sync_ui(args: argparse.Namespace) -> int:
    """Refresh Playwright storageState for the current E2E_BASE_URL origin.

    This makes `E2E_BASE_URL=https://manual-talent3... make e2e` work without
    re-provisioning keys: the cached tokens and fixtures are reused, but their
    localStorage entries are written for the requested origin.
    """
    lib.load_env()
    state = lib.load_state()
    auth = lib.get_auth()
    if (
        not state
        or not auth
        or not lib.validate_access(auth)
        or not state.get("owner_email")
        or not state.get("member_email")
    ):
        print("No valid E2E run/auth to sync.", file=sys.stderr)
        return 2
    owner_org = int(state["owner_org_id"])
    super_org = int(state.get("super_org_id") or owner_org)
    try:
        owner_auth = lib.ensure_fixture_auth(state.get("owner_auth"), "owner")
        member_auth = lib.ensure_fixture_auth(state.get("member_auth"), "member")
    except lib.AuthRequired as exc:
        print(str(exc), file=sys.stderr)
        return 2
    state["owner_auth"] = owner_auth
    state["member_auth"] = member_auth
    lib.save_state(state)
    lib.playwright_state(owner_auth, owner_org, lib.PLAYWRIGHT_STATE_PATH)
    lib.playwright_state(auth, super_org, lib.PLAYWRIGHT_STATE_SUPER_PATH)
    lib.playwright_state(member_auth, owner_org, lib.PLAYWRIGHT_STATE_MEMBER_PATH)
    print(f"Playwright state synced for {lib.ui_origin()} (owner/member/super).")
    return 0


def cmd_env(args: argparse.Namespace) -> int:
    lib.load_env()
    if not lib.RUN_ENV_PATH.exists():
        print("No E2E run.env yet. Run `make dev-up NICEURL=niceurl3` or `make e2e-up`.", file=sys.stderr)
        return 2
    print(str(lib.RUN_ENV_PATH))
    return 0


def cmd_clean(_args: argparse.Namespace) -> int:
    lib.load_env()
    notes = lib.cleanup_state(keep_auth=True)
    _recover_user_lrc()
    for note in notes:
        print(f"  cleanup: {note}")
    print("E2E cleanup complete.")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="LiveReview local E2E lifecycle")
    sub = parser.add_subparsers(dest="command", required=True)

    up = sub.add_parser("up", help="provision E2E fixtures, keys, and lrc sandbox")
    up.add_argument("--interactive", action="store_true", help="allow an interactive browser login")
    up.add_argument("--headless", action="store_true", help="do not open a headed browser")
    up.add_argument("--optional", action="store_true", help="return success when auth is not configured")
    up.add_argument("--fresh", action="store_true", help="clean up and reprovision the current run")
    up.set_defaults(func=cmd_up)

    down = sub.add_parser("down", help="clean up fixtures, keys, and lrc sandbox")
    down.add_argument("--forget-auth", action="store_true", help="also remove the cached browser session")
    down.set_defaults(func=cmd_down)

    status = sub.add_parser("status", help="show E2E status")
    status.set_defaults(func=cmd_status)

    auth = sub.add_parser("auth", help="authenticate and cache a session")
    auth.add_argument("--headless", action="store_true")
    auth.set_defaults(func=cmd_auth)

    doctor = sub.add_parser("doctor", help="check harness prerequisites and permissions")
    doctor.set_defaults(func=cmd_doctor)

    env = sub.add_parser("env", help="print the generated run.env path")
    env.set_defaults(func=cmd_env)

    sync_ui = sub.add_parser("sync-ui", help="rewrite Playwright state for the current base URL")
    sync_ui.set_defaults(func=cmd_sync_ui)

    clean = sub.add_parser("clean", help="clean up without touching the cached session")
    clean.set_defaults(func=cmd_clean)
    return parser


def main() -> int:
    lib.load_env()
    parser = build_parser()
    args = parser.parse_args()
    return int(args.func(args))


if __name__ == "__main__":
    raise SystemExit(main())
