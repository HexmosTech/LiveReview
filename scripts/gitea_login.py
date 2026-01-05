"""
Browser-style Gitea login helpers for inline comments and replies.

Public helpers (import-safe):
- load_env() -> populate os.environ from .env.prod/.env.
- create_session() -> authenticated requests.Session + CSRF token.
- post_inline_comment() -> create an inline comment via web form.
- reply_inline_comment() -> reply to an existing inline comment via web form.

Running this file directly keeps the previous demo behavior: it logs in
with defaults and posts an inline comment.
"""

import json
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Optional, Tuple

import requests


# ---------- Defaults (overridable via env) ----------
BASE_URL = os.environ.get("GITEA_BASE_URL", "https://gitea.hexmos.site").rstrip("/")
USERNAME = os.environ.get("GITEA_USER", "livereview")
PASSWORD = os.environ.get("GITEA_PASS", "gitea@12345")
PR_OWNER = os.environ.get("GITEA_OWNER", "megaorg")
PR_REPO = os.environ.get("GITEA_REPO", "livereview")
PR_NUMBER = int(os.environ.get("GITEA_PR", "17"))
PR_FILE_PATH = os.environ.get("GITEA_FILE", "cmd/config.go")
PR_LINE = int(os.environ.get("GITEA_LINE", "3"))
PR_SIDE = os.environ.get("GITEA_SIDE", "proposed")  # proposed|previous
PR_COMMIT = os.environ.get(
	"GITEA_COMMIT", "91650c1bd33f153479777fdd98ae62b6c9035cb8"
)
COMMENT_TEXT = os.environ.get("GITEA_COMMENT", "[demo] browser-form comment")

SESSION_DUMP = Path(__file__).with_name("gitea_session.json")


@dataclass
class SessionContext:
	session: requests.Session
	csrf: str
	base_url: str
	user: str
	password: str
	cookies: Dict[str, str]


# ---------- Env loader ----------


def load_env() -> None:
	"""Load .env.prod then .env without clobbering existing vars."""
	script_dir = Path(__file__).resolve().parent
	for candidate in (script_dir / "../.env.prod", script_dir / "../.env"):
		if not candidate.exists():
			continue
		for raw in candidate.read_text().splitlines():
			line = raw.strip()
			if not line or line.startswith("#") or "=" not in line:
				continue
			k, v = line.split("=", 1)
			k = k.strip()
			v = v.strip().strip('"').strip("'")
			os.environ.setdefault(k, v)


# ---------- Helpers ----------


def extract_csrf_from_html(html: str) -> str:
	m = re.search(r'name="_csrf"\s+value="([^"]+)"', html)
	return m.group(1) if m else ""


def dump_session(data: dict) -> None:
	SESSION_DUMP.write_text(json.dumps(data, indent=2))


# ---------- Public API ----------


def create_session(base_url: str, username: str, password: str) -> SessionContext:
	session = requests.Session()
	session.headers.update({"User-Agent": "lr-browser-bot"})

	login_url = f"{base_url}/user/login"
	r = session.get(login_url, timeout=20)
	r.raise_for_status()
	csrf_page = extract_csrf_from_html(r.text) or session.cookies.get("_csrf", "")
	if not csrf_page:
		raise RuntimeError("Unable to find CSRF token on login page")

	payload = {
		"_csrf": csrf_page,
		"user_name": username,
		"password": password,
		"remember": "on",
	}

	r = session.post(login_url, data=payload, allow_redirects=False, timeout=20)
	if r.status_code not in (302, 303, 200):
		raise RuntimeError(f"Login failed status={r.status_code} body={r.text[:200]}")

	csrf_cookie = session.cookies.get("_csrf", "")
	auth_cookie = session.cookies.get("gitea_incredible", "")
	if not auth_cookie:
		raise RuntimeError("Session cookie gitea_incredible missing after login")

	ctx = SessionContext(
		session=session,
		csrf=csrf_cookie,
		base_url=base_url,
		user=username,
		password=password,
		cookies={k: v for k, v in session.cookies.get_dict().items()},
	)
	dump_session(
		{
			"base_url": ctx.base_url,
			"user": ctx.user,
			"csrf": ctx.csrf,
			"cookies": ctx.cookies,
		}
	)
	return ctx


def _comment_url(base_url: str, owner: str, repo: str, pr_number: int) -> str:
	return f"{base_url}/{owner}/{repo}/pulls/{pr_number}/files/reviews/comments"


def post_inline_comment(
	ctx: SessionContext,
	owner: str,
	repo: str,
	pr_number: int,
	path: str,
	line: int,
	side: str,
	commit_id: str,
	content: str,
) -> Tuple[int, Dict[str, str]]:
	comment_url = _comment_url(ctx.base_url, owner, repo, pr_number)
	form = {
		"_csrf": ctx.csrf,
		"origin": "diff",
		"latest_commit_id": commit_id,
		"side": side,
		"line": str(line),
		"path": path,
		"diff_start_cid": "",
		"diff_end_cid": "",
		"diff_base_cid": "",
		"content": content,
		"single_review": "true",
	}
	r = _post_with_relogin(ctx, comment_url, form)
	return r.status_code, {
		"location": r.headers.get("Location"),
		"text_snippet": r.text[:200],
	}


def reply_inline_comment(
	ctx: SessionContext,
	owner: str,
	repo: str,
	pr_number: int,
	path: str,
	line: int,
	side: str,
	commit_id: str,
	content: str,
	parent_comment_id: int,
) -> Tuple[int, Dict[str, str]]:
	comment_url = _comment_url(ctx.base_url, owner, repo, pr_number)
	form = {
		"_csrf": ctx.csrf,
		"origin": "diff",
		"latest_commit_id": commit_id,
		"side": side,
		"line": str(line),
		"path": path,
		"diff_start_cid": "",
		"diff_end_cid": "",
		"diff_base_cid": "",
		"content": content,
		"reply": str(parent_comment_id),
		"single_review": "true",
	}
	r = _post_with_relogin(ctx, comment_url, form)
	return r.status_code, {
		"location": r.headers.get("Location"),
		"text_snippet": r.text[:200],
	}


def list_inline_comment_ids(
	ctx: SessionContext, owner: str, repo: str, pr_number: int
) -> list[int]:
	"""Scrape PR files page for inline comment ids (data-comment-id attributes)."""
	url = f"{ctx.base_url}/{owner}/{repo}/pulls/{pr_number}/files"
	r = _get_with_relogin(ctx, url)
	r.raise_for_status()
	ids = set(int(m) for m in re.findall(r'data-comment-id="(\d+)"', r.text))
	return sorted(ids)


def delete_inline_comment(
	ctx: SessionContext, owner: str, repo: str, comment_id: int, pr_number: Optional[int] = None
) -> Tuple[int, str]:
	"""Try multiple known delete endpoints until one works."""
	form = {"_csrf": ctx.csrf}
	candidates = [
		f"{ctx.base_url}/{owner}/{repo}/pulls/comments/{comment_id}/delete",
	]
	if pr_number is not None:
		candidates.append(
			f"{ctx.base_url}/{owner}/{repo}/pulls/{pr_number}/comments/{comment_id}/delete"
		)
	# Legacy/comment route
	candidates.append(f"{ctx.base_url}/{owner}/{repo}/comments/{comment_id}/delete")

	last_status = 0
	last_url = ""
	for url in candidates:
		last_url = url
		r = _post_with_relogin(ctx, url, form)
		last_status = r.status_code
		if r.status_code in (200, 204, 302, 303):
			return r.status_code, url
	return last_status, last_url


# ---------- Session reliability helpers ----------


def _should_relogin(resp: requests.Response) -> bool:
	loc = (resp.headers.get("Location") or "").lower()
	body_snip = (resp.text or "")[:500].lower()
	if resp.status_code in (401, 403):
		return True
	if resp.status_code in (301, 302, 303, 307, 308) and "user/login" in loc:
		return True
	if resp.status_code == 200 and ("user/login" in body_snip or "sign in" in body_snip):
		return True
	return False


def _refresh_session(ctx: SessionContext) -> None:
	new_ctx = create_session(ctx.base_url, ctx.user, ctx.password)
	ctx.session = new_ctx.session
	ctx.csrf = new_ctx.csrf
	ctx.cookies = new_ctx.cookies


def _post_with_relogin(ctx: SessionContext, url: str, form: Dict[str, str]) -> requests.Response:
	def _do() -> requests.Response:
		form["_csrf"] = ctx.csrf
		headers = {"X-CSRF-Token": ctx.csrf, "Referer": f"{ctx.base_url}/user/login"}
		return ctx.session.post(url, data=form, headers=headers, timeout=20)

	r = _do()
	if _should_relogin(r):
		_refresh_session(ctx)
		r = _do()
	return r


def _get_with_relogin(ctx: SessionContext, url: str) -> requests.Response:
	r = ctx.session.get(url, timeout=20)
	if _should_relogin(r):
		_refresh_session(ctx)
		r = ctx.session.get(url, timeout=20)
	return r


# ---------- CLI demo ----------


def main() -> None:
	load_env()
	ctx = create_session(BASE_URL, USERNAME, PASSWORD)
	status, meta = post_inline_comment(
		ctx,
		owner=PR_OWNER,
		repo=PR_REPO,
		pr_number=PR_NUMBER,
		path=PR_FILE_PATH,
		line=PR_LINE,
		side=PR_SIDE,
		commit_id=PR_COMMIT,
		content=COMMENT_TEXT,
	)
	session_info = {
		"base_url": ctx.base_url,
		"user": ctx.user,
		"csrf": ctx.csrf,
		"cookies": ctx.cookies,
		"comment_result": {"status": status, **meta},
	}
	dump_session(session_info)
	print("Login OK; comment POST status", status)


if __name__ == "__main__":
	main()
