#!/usr/bin/env python3
"""Submit a real review through the local lrc sandbox and verify the result."""

from __future__ import annotations

import argparse
import json
import shutil
import sys
import time
from pathlib import Path
from typing import Any, Dict, List, Optional

sys.path.insert(0, str(Path(__file__).resolve().parent))

import e2e_lib as lib


def _git(repo: Path, args: List[str], env: Dict[str, str]) -> None:
    lib.run(["git", *args], cwd=repo, env=env, timeout=30)


def _make_repo(run_id: str) -> Path:
    repo = lib.STATE_DIR / "repos" / run_id
    if repo.exists():
        shutil.rmtree(repo, ignore_errors=True)
    lib.secure_dir(repo.parent)
    repo.mkdir(parents=True, exist_ok=True)
    env = lib.lrc_env()
    env.update(
        {
            "GIT_AUTHOR_NAME": "LiveReview E2E",
            "GIT_AUTHOR_EMAIL": "e2e@localhost",
            "GIT_COMMITTER_NAME": "LiveReview E2E",
            "GIT_COMMITTER_EMAIL": "e2e@localhost",
        }
    )
    _git(repo, ["init", "-q", "-b", "main"], env)
    _git(repo, ["config", "user.name", "LiveReview E2E"], env)
    _git(repo, ["config", "user.email", "e2e@localhost"], env)
    (repo / "hello.txt").write_text("hello\n", encoding="utf-8")
    _git(repo, ["add", "hello.txt"], env)
    _git(repo, ["commit", "-q", "-m", "initial"], env)
    (repo / "hello.txt").write_text("hello\nworld\n", encoding="utf-8")
    _git(repo, ["add", "hello.txt"], env)
    return repo


def _read_preflight() -> Dict[str, Any]:
    value = lib.read_json(lib.PREFLIGHT_PATH, {})
    return value if isinstance(value, dict) else {}


def _review_rows(auth: Dict[str, Any], org_id: int, repo_name: str) -> List[Dict[str, Any]]:
    token = (auth.get("tokens") or {}).get("access_token")
    status, payload, _ = lib.api(
        "GET",
        f"/api/v1/reviews?search={lib.urllib.parse.quote(repo_name)}&per_page=20&sort=last_activity&order=desc",
        token=token,
        org_id=org_id,
    )
    if status != 200 or not isinstance(payload, dict):
        return []
    return [r for r in payload.get("reviews") or [] if isinstance(r, dict)]


def run_review(strict: bool) -> Dict[str, Any]:
    state = lib.load_state()
    auth = lib.get_auth()
    if not state:
        raise RuntimeError("No E2E state. Run `make dev-up NICEURL=niceurl3` or `make e2e-up` first.")
    if not auth or not lib.validate_access(auth):
        auth = lib.ensure_auth(interactive=False, allow_browser=False)
    org_id = int(state["owner_org_id"])
    run_id = str(state["run_id"])
    repo_name = f"e2e/{run_id}"
    repo = _make_repo(run_id + "-review")
    env = lib.lrc_env()
    env.update(
        {
            "LRC_REPO_NAME": repo_name,
            "LRC_NO_SERVE": "1",
            "LRC_OUTPUT": "json",
            "LRC_STAGED": "1",
            "LRC_BLOCKING_REVIEW": "false",
        }
    )
    wrapper = Path(__file__).with_name("lrc.sh")
    started = time.time()
    proc = lib.run(
        [str(wrapper), "review", "--no-serve", "--output", "json", "--staged"],
        cwd=repo,
        env=env,
        timeout=420,
        check=False,
    )
    elapsed = int(time.time() - started)
    result: Dict[str, Any] = {
        "ok": False,
        "repo_name": repo_name,
        "elapsed_sec": elapsed,
        "lrc_rc": proc.returncode,
        "review_id": None,
        "status": None,
        "detail": None,
    }
    if proc.returncode != 0:
        tail = (proc.stderr or proc.stdout or "").strip().splitlines()[-8:]
        result["detail"] = "lrc failed: " + " | ".join(tail[-3:])
        return result

    parsed: Optional[Dict[str, Any]] = None
    stdout = (proc.stdout or "").strip()
    if stdout:
        # The JSON document is the last stdout value. Ignore any progress
        # lines that lrc may have printed before it.
        try:
            parsed = json.loads(stdout)
        except ValueError:
            first = stdout.find("{")
            last = stdout.rfind("}")
            if first >= 0 and last > first:
                try:
                    parsed = json.loads(stdout[first : last + 1])
                except ValueError:
                    parsed = None
    if isinstance(parsed, dict):
        result["status"] = parsed.get("status")
        result["detail"] = str(parsed.get("summary") or "")[:200]

    # The review response deliberately omits review_id; resolve it through the
    # list API using the unique repo name.
    rows: List[Dict[str, Any]] = []
    deadline = time.time() + 30
    while time.time() < deadline and not rows:
        rows = _review_rows(auth, org_id, repo_name)
        if not rows:
            time.sleep(1)
    if rows:
        row = rows[0]
        result["review_id"] = row.get("id")
        result["status"] = row.get("status") or result["status"]
        result["ok"] = True
        result["detail"] = f"review {row.get('id')} status={row.get('status')}"
    else:
        # Some server versions report success before the list index catches up.
        if isinstance(parsed, dict) and parsed.get("status") in ("completed", "processing"):
            result["ok"] = True
            result["detail"] = f"lrc reported {parsed.get('status')}; review not yet in list index"
        else:
            result["detail"] = "lrc exited 0 but no review row was found"
    return result


def main() -> int:
    lib.load_env()
    parser = argparse.ArgumentParser(description="LiveReview lrc review E2E")
    parser.add_argument("--strict", action="store_true", help="fail if AI preflight is broken")
    args = parser.parse_args()
    lib.secure_dir(lib.STATE_DIR)
    preflight = _read_preflight()
    ai = (preflight.get("checks") or {}).get("ai") or {}
    if ai.get("status") not in ("ready", None):
        msg = f"AI preflight is {ai.get('status')}: {ai.get('detail')}"
        if args.strict:
            print(f"Review E2E blocked: {msg}")
            return 1
        print(f"Review E2E skipped: {msg}")
        return 0
    try:
        result = run_review(args.strict)
    except Exception as exc:
        print(f"Review E2E failed: {type(exc).__name__}: {exc}", file=sys.stderr)
        return 1
    lib.write_json(lib.STATE_DIR / "review-report.json", result)
    if result["ok"]:
        print(f"Review E2E: {result['detail']} ({result['elapsed_sec']}s)")
        return 0
    print(f"Review E2E failed: {result['detail']}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
