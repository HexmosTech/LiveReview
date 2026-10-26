#!/usr/bin/env python3
"""Shared helpers for LiveReview's local E2E control plane.

This module intentionally uses only the Python standard library so the harness
works before the UI/API stack is fully up.  It also never prints secret values.
"""

from __future__ import annotations

import json
import os
import re
import secrets
import shutil
import string
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Tuple

try:
    import bcrypt as _bcrypt  # local-only fallback when invitation email is not configured
except ImportError:  # pragma: no cover
    _bcrypt = None

ROOT = Path(__file__).resolve().parents[2]
STATE_DIR = Path(os.environ.get("E2E_STATE_DIR", str(ROOT / ".lrdata" / "e2e")))
AUTH_PATH = STATE_DIR / "auth.json"
STATE_PATH = STATE_DIR / "state.json"
PREFLIGHT_PATH = STATE_DIR / "preflight.json"
RUN_ENV_PATH = STATE_DIR / "run.env"
PLAYWRIGHT_STATE_PATH = STATE_DIR / "playwright-state.json"
PLAYWRIGHT_STATE_MEMBER_PATH = STATE_DIR / "playwright-state-member.json"
PLAYWRIGHT_STATE_SUPER_PATH = STATE_DIR / "playwright-state-super.json"
LRC_HOME = STATE_DIR / "lrc-home"
LRC_BACKUP_DIR = STATE_DIR / "lrc-backup"
LRC_JOURNAL_PATH = STATE_DIR / "lrc-journal.json"
LOCK_PATH = STATE_DIR / "lock"
CLEANUP_PATH = STATE_DIR / "cleanup.json"


class ApiError(RuntimeError):
    def __init__(self, method: str, path: str, status: int, body: Any):
        self.method = method
        self.path = path
        self.status = status
        self.body = body
        super().__init__(f"{method} {path} -> HTTP {status}: {_body_preview(body)}")


class AuthRequired(RuntimeError):
    pass


def _body_preview(body: Any) -> str:
    if body is None:
        return ""
    if isinstance(body, (dict, list)):
        text = json.dumps(body, separators=(",", ":"))
    else:
        text = str(body)
    return text[:300]


def load_env() -> None:
    """Load .env then .env.e2e.local without overriding the real process env."""
    original = set(os.environ.keys())
    merged: Dict[str, str] = {}
    for path in (ROOT / ".env", ROOT / ".env.e2e.local", ROOT / ".env.e2e"):
        if not path.exists():
            continue
        try:
            lines = path.read_text(encoding="utf-8").splitlines()
        except OSError:
            continue
        for line in lines:
            raw = line.strip()
            if not raw or raw.startswith("#"):
                continue
            if raw.startswith("export "):
                raw = raw[7:].strip()
            if "=" not in raw:
                continue
            key, value = raw.split("=", 1)
            key = key.strip()
            value = value.strip()
            if not key:
                continue
            if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
                value = value[1:-1]
            merged[key] = value
    for key, value in merged.items():
        if key not in original:
            os.environ[key] = value


def cfg(key: str, default: Optional[str] = None) -> Optional[str]:
    value = os.environ.get(key)
    return default if value is None or value == "" else value


def cfg_int(key: str, default: int) -> int:
    value = cfg(key)
    if value is None:
        return default
    try:
        return int(value)
    except ValueError:
        return default


def base_url() -> str:
    return (cfg("E2E_BASE_URL", "http://localhost:8081") or "").rstrip("/")


def api_url() -> str:
    explicit = cfg("E2E_API_URL")
    if explicit:
        return explicit.rstrip("/")
    parsed = urllib.parse.urlparse(base_url())
    if parsed.hostname in ("localhost", "127.0.0.1") and parsed.port == 8081:
        return "http://localhost:8888"
    return base_url()


def ui_origin() -> str:
    parsed = urllib.parse.urlparse(base_url())
    return f"{parsed.scheme}://{parsed.netloc}"


def is_local() -> bool:
    host = (urllib.parse.urlparse(base_url()).hostname or "").lower()
    return host in ("localhost", "127.0.0.1", "::1")


def now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def run_id() -> str:
    explicit = cfg("E2E_RUN_ID")
    if explicit:
        return explicit
    stamp = time.strftime("%Y%m%d-%H%M%S", time.gmtime())
    return f"e2e-{stamp}-{secrets.token_hex(3)}"


def secure_dir(path: Path) -> Path:
    path.mkdir(parents=True, exist_ok=True)
    try:
        path.chmod(0o700)
    except OSError:
        pass
    return path


def secure_write(path: Path, data: str) -> None:
    secure_dir(path.parent)
    tmp = path.with_suffix(path.suffix + ".tmp")
    # Fixture state contains no password; only short-lived local test tokens.
    # codeql[py/clear-text-storage-sensitive-data]
    # lgtm[py/clear-text-storage-sensitive-data]
    tmp.write_text(data, encoding="utf-8")
    try:
        tmp.chmod(0o600)
    except OSError:
        pass
    os.replace(tmp, path)


def read_json(path: Path, default: Any = None) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return default


def write_json(path: Path, value: Any) -> None:
    secure_write(path, json.dumps(value, indent=2, sort_keys=True) + "\n")


def mask(value: Optional[str]) -> str:
    if not value:
        return "(unset)"
    value = str(value)
    if len(value) <= 8:
        return "***"
    return value[:4] + "…" + value[-3:]


def _request(
    method: str,
    path: str,
    *,
    token: Optional[str] = None,
    api_key: Optional[str] = None,
    org_id: Optional[int] = None,
    body: Any = None,
    timeout: float = 30.0,
    raise_on_error: bool = True,
    raw: bool = False,
) -> Tuple[int, Any, Dict[str, str]]:
    url = api_url() + path if path.startswith("/") else path
    headers = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if api_key:
        headers["X-API-Key"] = api_key
    if org_id is not None:
        headers["X-Org-Context"] = str(org_id)
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        # URL is the configured E2E API base plus a fixed route, never arbitrary
        # user input.
        # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = resp.read()
            resp_headers = {k.lower(): v for k, v in resp.headers.items()}
            status = resp.status
    except urllib.error.HTTPError as exc:
        payload = exc.read()
        resp_headers = {k.lower(): v for k, v in exc.headers.items()}
        status = exc.code
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        if raise_on_error:
            raise ApiError(method, path, 0, str(exc)) from exc
        return 0, str(exc), {}

    if raw:
        parsed: Any = payload
    else:
        text = payload.decode("utf-8", errors="replace") if payload else ""
        parsed = None
        if text.strip():
            try:
                parsed = json.loads(text)
            except ValueError:
                parsed = text

    if status >= 400 and raise_on_error:
        raise ApiError(method, path, status, parsed)
    return status, parsed, resp_headers


def api(
    method: str,
    path: str,
    *,
    token: Optional[str] = None,
    api_key: Optional[str] = None,
    org_id: Optional[int] = None,
    body: Any = None,
    timeout: float = 30.0,
    raise_on_error: bool = True,
    raw: bool = False,
) -> Tuple[int, Any, Dict[str, str]]:
    return _request(
        method,
        path,
        token=token,
        api_key=api_key,
        org_id=org_id,
        body=body,
        timeout=timeout,
        raise_on_error=raise_on_error,
        raw=raw,
    )


def get_auth() -> Optional[Dict[str, Any]]:
    auth = read_json(AUTH_PATH)
    return auth if isinstance(auth, dict) else None


def save_auth(auth: Dict[str, Any]) -> None:
    write_json(AUTH_PATH, auth)


def _token(auth: Optional[Dict[str, Any]]) -> Optional[str]:
    if not auth:
        return None
    tokens = auth.get("tokens") or {}
    return tokens.get("access_token")


def _refresh_token(auth: Optional[Dict[str, Any]]) -> Optional[str]:
    if not auth:
        return None
    tokens = auth.get("tokens") or {}
    return tokens.get("refresh_token")


def validate_access(auth: Dict[str, Any]) -> bool:
    token = _token(auth)
    if not token:
        return False
    try:
        status, payload, _ = api("GET", "/api/v1/auth/me", token=token, raise_on_error=False)
        if status != 200 or not isinstance(payload, dict):
            return False
        if payload.get("user"):
            auth["user"] = payload["user"]
        if payload.get("organizations"):
            auth["organizations"] = payload["organizations"]
        return True
    except Exception:
        return False


def refresh_auth(auth: Dict[str, Any], *, save: bool = True) -> bool:
    refresh = _refresh_token(auth)
    if not refresh:
        return False
    try:
        status, payload, _ = api(
            "POST",
            "/api/v1/auth/refresh",
            body={"refresh_token": refresh},
            raise_on_error=False,
        )
    except Exception:
        return False
    if status != 200 or not isinstance(payload, dict) or not payload.get("access_token"):
        return False
    auth["tokens"] = {
        "access_token": payload.get("access_token"),
        "refresh_token": payload.get("refresh_token") or refresh,
        "expires_at": payload.get("expires_at"),
        "token_type": payload.get("token_type", "Bearer"),
    }
    if save:
        save_auth(auth)
    return validate_access(auth)


def ensure_fixture_auth(auth: Optional[Dict[str, Any]], label: str) -> Dict[str, Any]:
    """Validate or refresh a stored member/owner fixture session.

    Fixture passwords are intentionally never persisted.  The harness keeps
    only the short-lived session/refresh tokens created at provisioning time.
    """
    if isinstance(auth, dict):
        if validate_access(auth):
            return auth
        if refresh_auth(auth, save=False):
            return auth
    raise AuthRequired(
        f"{label} fixture session expired. Run `make e2e-up --fresh` to recreate fixtures."
    )


def password_login(email: str, password: str, *, save: bool = True) -> Dict[str, Any]:
    status, payload, _ = api(
        "POST",
        "/api/v1/auth/login",
        body={"email": email, "password": password},
        raise_on_error=False,
    )
    if status != 200 or not isinstance(payload, dict) or not payload.get("tokens"):
        raise AuthRequired(
            "Password login failed. Set E2E_EMAIL/E2E_PASSWORD in .env.e2e.local "
            "or run `make e2e-auth`."
        )
    payload["email"] = email
    if save:
        save_auth(payload)
    return payload


def browser_auth(headed: bool = True, timeout_seconds: int = 600) -> Dict[str, Any]:
    """Run the Playwright auth helper and cache the resulting browser session."""
    helper = Path(__file__).with_name("auth_browser.js")
    node = shutil.which("node")
    if not node:
        raise AuthRequired("node is required for `make e2e-auth` but was not found.")
    secure_dir(STATE_DIR)
    auth_out = STATE_DIR / "auth-browser.json"
    try:
        auth_out.unlink()
    except FileNotFoundError:
        pass
    env = os.environ.copy()
    env["E2E_BASE_URL"] = base_url()
    env["E2E_HEADED"] = "1" if headed else "0"
    env["E2E_AUTH_TIMEOUT_MS"] = str(max(30, timeout_seconds) * 1000)
    env["E2E_AUTH_OUT"] = str(auth_out)
    proc = subprocess.run(
        [node, str(helper)],
        cwd=str(ROOT / "scripts" / "integration"),
        env=env,
        text=True,
        capture_output=True,
    )
    if proc.returncode != 0:
        tail = (proc.stderr or proc.stdout or "").strip().splitlines()[-5:]
        raise AuthRequired("Browser login failed.\n" + "\n".join(tail))
    payload = read_json(auth_out)
    if not isinstance(payload, dict):
        raise AuthRequired("Browser login returned an unreadable response.")
    try:
        auth_out.unlink()
    except FileNotFoundError:
        pass
    if not payload.get("accessToken") or not payload.get("refreshToken"):
        raise AuthRequired("Browser login did not produce LiveReview tokens.")
    auth = {
        "email": payload.get("email") or cfg("E2E_EMAIL", ""),
        "tokens": {
            "access_token": payload["accessToken"],
            "refresh_token": payload["refreshToken"],
            "token_type": "Bearer",
        },
        "organizations": payload.get("organizations") or [],
        "user": payload.get("user") or {},
    }
    save_auth(auth)
    if not validate_access(auth):
        raise AuthRequired("Browser login tokens were rejected by the API.")
    return auth


def ensure_auth(
    *,
    interactive: bool = False,
    allow_browser: bool = False,
    headed: bool = True,
) -> Dict[str, Any]:
    auth = get_auth()
    if auth and validate_access(auth):
        save_auth(auth)
        return auth
    if auth and refresh_auth(auth):
        return auth

    email = cfg("E2E_EMAIL")
    password = cfg("E2E_PASSWORD")
    if email and password:
        return password_login(email, password)

    if allow_browser and interactive:
        return browser_auth(headed=headed)

    raise AuthRequired(
        "No valid cached session and no usable E2E password. "
        "Set E2E_EMAIL/E2E_PASSWORD in .env.e2e.local, or run `make e2e-auth`."
    )


def ensure_organizations(auth: Dict[str, Any]) -> List[Dict[str, Any]]:
    orgs = auth.get("organizations") or []
    if orgs:
        return orgs
    token = _token(auth)
    if not token:
        return []
    status, payload, _ = api("GET", "/api/v1/auth/me", token=token, raise_on_error=False)
    if status == 200 and isinstance(payload, dict):
        auth["user"] = payload.get("user") or auth.get("user") or {}
        auth["organizations"] = payload.get("organizations") or []
        save_auth(auth)
        return auth["organizations"]
    return []


def select_orgs(auth: Dict[str, Any]) -> Dict[str, int]:
    orgs = ensure_organizations(auth)
    by_role: Dict[str, List[int]] = {}
    for org in orgs:
        role = str(org.get("role") or "")
        try:
            oid = int(org.get("id"))
        except (TypeError, ValueError):
            continue
        by_role.setdefault(role, []).append(oid)

    super_org = cfg_int("E2E_SUPER_ORG_ID", 0)
    if not super_org and by_role.get("super_admin"):
        super_org = by_role["super_admin"][0]

    owner_org = cfg_int("E2E_OWNER_ORG_ID", 0)
    if not owner_org:
        candidates = [oid for oid in by_role.get("owner", []) if oid != super_org]
        if candidates:
            owner_org = candidates[0]
        elif by_role.get("owner"):
            owner_org = by_role["owner"][0]
    if not owner_org and by_role.get("member"):
        owner_org = by_role["member"][0]
    if not owner_org and orgs:
        owner_org = int(orgs[0]["id"])

    if not owner_org:
        raise AuthRequired("Could not resolve an owner organization from the authenticated session.")
    return {"owner": int(owner_org), "super": int(super_org or owner_org)}


def get_me(auth: Dict[str, Any]) -> Dict[str, Any]:
    token = _token(auth)
    if not token:
        raise AuthRequired("No access token.")
    status, payload, _ = api("GET", "/api/v1/auth/me", token=token)
    if status != 200 or not isinstance(payload, dict):
        raise AuthRequired("Could not load /auth/me.")
    return payload


def create_api_key(
    auth: Dict[str, Any],
    org_id: int,
    *,
    label: str,
    kind: str = "standard",
) -> Dict[str, Any]:
    token = _token(auth)
    if not token:
        raise AuthRequired("No access token.")
    status, payload, _ = api(
        "POST",
        f"/api/v1/orgs/{org_id}/api-keys",
        token=token,
        org_id=org_id,
        body={"label": label, "kind": kind, "scopes": []},
    )
    if status not in (200, 201) or not isinstance(payload, dict):
        raise ApiError("POST", f"/api/v1/orgs/{org_id}/api-keys", status, payload)
    api_key = payload.get("api_key") or {}
    plain = payload.get("plain_key")
    if not plain:
        raise ApiError("POST", f"/api/v1/orgs/{org_id}/api-keys", status, payload)
    return {"id": api_key.get("id"), "label": label, "kind": kind, "plain_key": plain}


def revoke_api_key(auth: Optional[Dict[str, Any]], org_id: int, key_id: int) -> bool:
    token = _token(auth)
    if token:
        status, _, _ = api(
            "POST",
            f"/api/v1/orgs/{org_id}/api-keys/{key_id}/revoke",
            token=token,
            org_id=org_id,
            raise_on_error=False,
        )
        if status in (200, 204):
            return True
    # Local-only fallback so a crashed run can still clean up even if the
    # cached session expired.
    return db_exec(
        "UPDATE api_keys SET revoked_at = COALESCE(revoked_at, NOW()) WHERE id = $1",
        [str(key_id)],
    )


def _create_member_via_db(
    auth: Dict[str, Any],
    org_id: int,
    *,
    email: str,
    password: str,
    first_name: str = "E2E",
    last_name: str = "Member",
    role_id: int = 3,
) -> Optional[int]:
    """Local-only fallback for deployments without an invitation email service.

    The temp user is still a real DB-backed account with a real bcrypt password;
    it is created by the same schema the API uses.  This keeps member-scoping
    tests available on a fully local stack.
    """
    if _bcrypt is None:
        return None
    try:
        password_hash = _bcrypt.hashpw(password.encode("utf-8"), _bcrypt.gensalt()).decode("utf-8")
    except Exception:
        return None
    created_by = (auth.get("user") or {}).get("id") or 1
    sql = """
WITH ins AS (
  INSERT INTO users
    (email, password_hash, first_name, last_name, created_by_user_id,
     password_reset_required, created_at, updated_at, default_org_id)
  VALUES ($1, $2, $3, $4, $5, false, NOW(), NOW(), $6)
  RETURNING id
)
INSERT INTO user_roles (user_id, org_id, role_id, created_at, updated_at)
SELECT id, $6, $7, NOW(), NOW() FROM ins
RETURNING user_id;
"""
    rc, out, _err = db_query(
        sql,
        [
            email,
            password_hash,
            first_name,
            last_name,
            str(created_by),
            str(org_id),
            str(role_id),
        ],
    )
    if rc != 0:
        return None
    for token in reversed((out or "").replace("|", " ").split()):
        try:
            return int(token)
        except ValueError:
            continue
    return None


def create_member(
    auth: Dict[str, Any],
    org_id: int,
    *,
    email: str,
    password: str,
    run: str,
    role_id: int = 3,
    first_name: str = "E2E",
) -> Dict[str, Any]:
    token = _token(auth)
    if not token:
        raise AuthRequired("No access token.")
    body = {
        "email": email,
        "password": password,
        "first_name": first_name,
        "last_name": run[-12:],
        "role_id": role_id,
    }
    try:
        status, payload, _ = api(
            "POST",
            f"/api/v1/orgs/{org_id}/users",
            token=token,
            org_id=org_id,
            body=body,
        )
    except ApiError as exc:
        text = json.dumps(exc.body) if isinstance(exc.body, (dict, list)) else str(exc.body)
        if exc.status == 400 and ("invitation" in text.lower() or "email service" in text.lower()):
            user_id = _create_member_via_db(
                auth,
                org_id,
                email=email,
                password=password,
                first_name=first_name,
                role_id=role_id,
            )
            if user_id:
                return {
                    "id": int(user_id),
                    "email": email,
                    "password": password,
                    "org_id": org_id,
                    "created_via": "local-db-fallback",
                }
        raise
    if status not in (200, 201) or not isinstance(payload, dict):
        raise ApiError("POST", f"/api/v1/orgs/{org_id}/users", status, payload)
    user = payload.get("user") or payload
    user_id = user.get("id") or payload.get("user_id") or payload.get("id")
    if not user_id:
        raise ApiError("POST", f"/api/v1/orgs/{org_id}/users", status, payload)
    return {"id": int(user_id), "email": email, "password": password, "org_id": org_id, "created_via": "api"}


def admin_create_member(
    auth: Dict[str, Any],
    org_id: int,
    *,
    email: str,
    password: str,
    run: str,
) -> Dict[str, Any]:
    token = _token(auth)
    if not token:
        raise AuthRequired("No access token.")
    body = {
        "email": email,
        "password": password,
        "first_name": "E2E",
        "last_name": "Admin " + run[-8:],
        "role_id": 3,
    }
    status, payload, _ = api(
        "POST",
        f"/api/v1/admin/orgs/{org_id}/users",
        token=token,
        body=body,
    )
    if status not in (200, 201) or not isinstance(payload, dict):
        raise ApiError("POST", f"/api/v1/admin/orgs/{org_id}/users", status, payload)
    user = payload.get("user") or payload
    user_id = user.get("id") or payload.get("user_id") or payload.get("id")
    if not user_id:
        raise ApiError("POST", f"/api/v1/admin/orgs/{org_id}/users", status, payload)
    return {"id": int(user_id), "email": email, "password": password, "org_id": org_id}


def deactivate_user(auth: Optional[Dict[str, Any]], org_id: int, user_id: int) -> bool:
    token = _token(auth)
    if token:
        status, _, _ = api(
            "DELETE",
            f"/api/v1/orgs/{org_id}/users/{user_id}",
            token=token,
            org_id=org_id,
            raise_on_error=False,
        )
        if status in (200, 204):
            return True
    return db_exec(
        "UPDATE users SET is_active = false, deactivated_at = NOW() WHERE id = $1",
        [str(user_id)],
    )


def login_member(email: str, password: str) -> Dict[str, Any]:
    # Never overwrite the owner/super-admin cached session with a member token.
    return password_login(email, password, save=False)


def playwright_state(auth: Dict[str, Any], org_id: int, path: Path) -> None:
    tokens = auth.get("tokens") or {}
    access = tokens.get("access_token")
    refresh = tokens.get("refresh_token")
    if not access or not refresh:
        raise AuthRequired("Cannot write Playwright state without tokens.")
    state = {
        "cookies": [],
        "origins": [
            {
                "origin": ui_origin(),
                "localStorage": [
                    {"name": "accessToken", "value": access},
                    {"name": "refreshToken", "value": refresh},
                    {"name": "currentOrgId", "value": str(org_id)},
                ],
            }
        ],
    }
    write_json(path, state)


def write_lrc_home(api_key: str, org_id: int, auth: Dict[str, Any]) -> None:
    secure_dir(LRC_HOME)
    tokens = auth.get("tokens") or {}
    email = (auth.get("user") or {}).get("email") or cfg("E2E_EMAIL", "")
    content = "\n".join(
        [
            f'api_key = "{api_key}"',
            f'api_url = "{api_url()}"',
            f'org_id = "{org_id}"',
            f'user_email = "{email}"',
            f'jwt = "{tokens.get("access_token", "")}"',
            f'refresh_token = "{tokens.get("refresh_token", "")}"',
            "",
        ]
    )
    secure_write(LRC_HOME / ".lrc.toml", content)
    gitconfig = "\n".join(
        [
            "[user]",
            "    name = LiveReview E2E",
            "    email = e2e@localhost",
            "[init]",
            "    defaultBranch = main",
            "[safe]",
            "    directory = *",
            "",
        ]
    )
    secure_write(LRC_HOME / ".gitconfig", gitconfig)


def load_state() -> Dict[str, Any]:
    state = read_json(STATE_PATH, {})
    return state if isinstance(state, dict) else {}


def save_state(state: Dict[str, Any]) -> None:
    write_json(STATE_PATH, state)


def update_cleanup(**kwargs: Any) -> None:
    cleanup = read_json(CLEANUP_PATH, {})
    if not isinstance(cleanup, dict):
        cleanup = {}
    cleanup.update(kwargs)
    cleanup["updated_at"] = now_iso()
    write_json(CLEANUP_PATH, cleanup)


def write_run_env(state: Dict[str, Any]) -> None:
    lines = [
        "# Generated by scripts/e2e. Contains local secrets; never commit.",
        f"export E2E_RUN_ID={shell_quote(str(state.get('run_id', '')))}",
        f"export E2E_BASE_URL={shell_quote(base_url())}",
        f"export E2E_API_URL={shell_quote(api_url())}",
        f"export E2E_OWNER_ORG_ID={shell_quote(str(state.get('owner_org_id', '')))}",
        f"export E2E_SUPER_ORG_ID={shell_quote(str(state.get('super_org_id', '')))}",
        f"export E2E_MEMBER_EMAIL={shell_quote(str(state.get('member_email', '')))}",
        f"export E2E_OWNER_MEMBER_EMAIL={shell_quote(str(state.get('owner_email', '')))}",
        f"export E2E_STANDARD_KEY={shell_quote(str(state.get('standard_key', '')))}",
        f"export E2E_LRC_KEY={shell_quote(str(state.get('lrc_key', '')))}",
        f"export E2E_AUDIT_KEY={shell_quote(str(state.get('audit_key', '')))}",
        f"export E2E_LRC_HOME={shell_quote(str(LRC_HOME))}",
        f"export E2E_PLAYWRIGHT_STATE={shell_quote(str(PLAYWRIGHT_STATE_PATH))}",
        f"export E2E_PLAYWRIGHT_STATE_MEMBER={shell_quote(str(PLAYWRIGHT_STATE_MEMBER_PATH))}",
        f"export E2E_PLAYWRIGHT_STATE_SUPER={shell_quote(str(PLAYWRIGHT_STATE_SUPER_PATH))}",
        f"export E2E_ARTIFACTS={shell_quote(str(STATE_DIR / 'artifacts'))}",
        "",
    ]
    secure_write(RUN_ENV_PATH, "\n".join(lines))


def shell_quote(value: str) -> str:
    if re.fullmatch(r"[A-Za-z0-9_./:@%+=,-]*", value or ""):
        return value
    return "'" + value.replace("'", "'\"'\"'") + "'"


def db_exec(sql: str, params: Optional[List[str]] = None) -> bool:
    database_url = cfg("DATABASE_URL")
    psql = shutil.which("psql")
    if not database_url or not psql:
        return False
    args = [psql, database_url, "-v", "ON_ERROR_STOP=1", "-q"]
    for idx, value in enumerate(params or [], start=1):
        args.extend(["-v", f"p{idx}={value}"])
    # Only simple positional substitutions are used here; never interpolate
    # untrusted text into SQL.  Feed the statement on stdin because psql does
    # not perform :'var' interpolation for -c.
    rewritten = sql
    for idx in range(len(params or []), 0, -1):
        rewritten = rewritten.replace(f"${idx}", f":'p{idx}'")
    try:
        proc = subprocess.run(
            args, input=rewritten, text=True, capture_output=True, timeout=30
        )
    except (OSError, subprocess.TimeoutExpired):
        return False
    return proc.returncode == 0


def db_query(sql: str, params: Optional[List[str]] = None) -> Tuple[int, str, str]:
    """Run a local psql query for read-only/transaction-rollback assertions."""
    database_url = cfg("DATABASE_URL")
    psql = shutil.which("psql")
    if not database_url or not psql:
        return 127, "", "psql or DATABASE_URL unavailable"
    args = [psql, database_url, "-v", "ON_ERROR_STOP=1", "-q"]
    for idx, value in enumerate(params or [], start=1):
        args.extend(["-v", f"p{idx}={value}"])
    rewritten = sql
    for idx in range(len(params or []), 0, -1):
        rewritten = rewritten.replace(f"${idx}", f":'p{idx}'")
    try:
        proc = subprocess.run(
            args, input=rewritten, text=True, capture_output=True, timeout=30
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        return 124, "", str(exc)
    return proc.returncode, proc.stdout, proc.stderr


def wait_http(url: str, timeout: float = 120, interval: float = 1.0) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        req = urllib.request.Request(url, method="GET")
        try:
            # URL is a configured health endpoint.
            # nosemgrep: python.lang.security.audit.dynamic-urllib-use-detected.dynamic-urllib-use-detected
            with urllib.request.urlopen(req, timeout=3) as resp:
                if resp.status < 500:
                    return True
        except Exception:
            pass
        time.sleep(interval)
    return False


def run(
    args: List[str],
    *,
    cwd: Optional[Path] = None,
    env: Optional[Dict[str, str]] = None,
    timeout: float = 120,
    check: bool = True,
) -> subprocess.CompletedProcess:
    proc = subprocess.run(
        args,
        cwd=str(cwd or ROOT),
        env=env,
        text=True,
        capture_output=True,
        timeout=timeout,
    )
    if check and proc.returncode != 0:
        raise RuntimeError(
            f"command failed ({proc.returncode}): {' '.join(args[:3])}...\n"
            + ((proc.stderr or proc.stdout or "").strip()[-1000:])
        )
    return proc


def lrc_env() -> Dict[str, str]:
    env = os.environ.copy()
    env["HOME"] = str(LRC_HOME)
    env["GIT_CONFIG_GLOBAL"] = str(LRC_HOME / ".gitconfig")
    state = load_state()
    env["LRC_API_URL"] = api_url()
    env["LRC_API_KEY"] = str(state.get("lrc_key") or state.get("standard_key") or "")
    return env


def cleanup_state(*, keep_auth: bool = True) -> List[str]:
    """Best-effort cleanup of everything this harness created."""
    notes: List[str] = []
    state = load_state()
    auth = get_auth()
    if auth and not validate_access(auth):
        refresh_auth(auth)
    if state:
        for key_name, org_key, label in (
            ("standard_key_id", "owner_org_id", "standard"),
            ("audit_key_id", "owner_org_id", "audit"),
            ("lrc_key_id", "owner_org_id", "owner lrc"),
            ("integrity_key_id", "integrity_org_id", "integrity-probe"),
        ):
            key_id = state.get(key_name)
            org_id = state.get(org_key)
            if key_id and org_id:
                if revoke_api_key(auth, int(org_id), int(key_id)):
                    notes.append(f"revoked {label} API key #{key_id}")
                else:
                    notes.append(f"could not revoke {label} API key #{key_id}")
        for user_key, org_key, label in (
            ("member_id", "owner_org_id", "member"),
            ("owner_member_id", "owner_org_id", "owner fixture"),
            ("admin_member_id", "owner_org_id", "admin-created member"),
        ):
            user_id = state.get(user_key)
            org_id = state.get(org_key)
            if user_id and org_id:
                if deactivate_user(auth, int(org_id), int(user_id)):
                    notes.append(f"deactivated {label} #{user_id}")
                else:
                    notes.append(f"could not deactivate {label} #{user_id}")

    for path in (
        STATE_PATH,
        RUN_ENV_PATH,
        PREFLIGHT_PATH,
        PLAYWRIGHT_STATE_PATH,
        PLAYWRIGHT_STATE_MEMBER_PATH,
        PLAYWRIGHT_STATE_SUPER_PATH,
        STATE_DIR / "auth-browser.json",
        CLEANUP_PATH,
    ):
        try:
            path.unlink()
        except FileNotFoundError:
            pass
    for path in (LRC_HOME, STATE_DIR / "artifacts", STATE_DIR / "repos", STATE_DIR / "tmp"):
        if path.exists():
            shutil.rmtree(path, ignore_errors=True)
    if not keep_auth:
        try:
            AUTH_PATH.unlink()
        except FileNotFoundError:
            pass
    return notes
