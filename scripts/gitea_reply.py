"""One-off inline reply helper for Gitea.

Hardcoded to the failing PR/comment from the latest log so we can verify
reply mechanics outside the main app. Uses the browser-form flow from
gitea_login.py (username/password + CSRF) instead of PAT.
"""

import os
import time
from typing import Dict, List, Optional, Tuple

import requests

from gitea_login import (
	load_env,
	create_session,
	reply_inline_comment,
)


# --- Hardcoded context from gitea_webhooks.txt ---
BASE_URL = "https://gitea.hexmos.site"
OWNER = "megaorg"
REPO = "livereview"
PR_NUMBER = 17
COMMENT_PATH = "cmd/config.go"

# Reply payload
REPLY_BODY = "[scripted check] inline reply via gitea_reply.py"


def create_pat(username: str, password: str) -> Tuple[int, Optional[str], Dict[str, str]]:
	"""Create a short-lived PAT for fetching thread metadata."""
	token_name = f"tmp-inline-reply-{int(time.time())}"
	token_payload = {
		"name": token_name,
		"scopes": ["write:repository"],
	}
	token_resp = requests.post(
		f"{BASE_URL}/api/v1/users/{username}/tokens",
		auth=(username, password),
		json=token_payload,
		headers={"Accept": "application/json"},
		timeout=20,
	)

	if token_resp.status_code not in (200, 201):
		return token_resp.status_code, None, {"error": "failed to create PAT"}

	pat = (token_resp.json() or {}).get("sha1")
	if not pat:
		return 500, None, {"error": "PAT missing in response"}

	return 201, pat, {}


def fetch_thread(pat: str) -> Tuple[Optional[int], Optional[int], Dict[str, str]]:
	"""Return (parent_comment_id, review_id) for the latest inline comment on path."""

	reviews_url = f"{BASE_URL}/api/v1/repos/{OWNER}/{REPO}/pulls/{PR_NUMBER}/reviews"
	reviews_resp = requests.get(
		reviews_url,
		headers={"Accept": "application/json", "Authorization": f"token {pat}"},
		timeout=20,
	)
	if reviews_resp.status_code != 200:
		return None, None, {
			"mode": "list-reviews-failed",
			"status": str(reviews_resp.status_code),
			"body": reviews_resp.text[:500],
		}

	comments: List[Dict] = []
	for review in reviews_resp.json() or []:
		review_id = review.get("id")
		url = f"{BASE_URL}/api/v1/repos/{OWNER}/{REPO}/pulls/{PR_NUMBER}/reviews/{review_id}/comments"
		resp = requests.get(
			url,
			headers={"Accept": "application/json", "Authorization": f"token {pat}"},
			timeout=20,
		)
		if resp.status_code == 200:
			for c in resp.json() or []:
				c["_review_id"] = review_id
				comments.append(c)

	comments = [c for c in comments if c.get("path") == COMMENT_PATH]
	if not comments:
		return None, None, {"mode": "no-comments", "note": "no inline comments found on path"}

	comments.sort(key=lambda c: c.get("created_at", ""))
	parent = comments[-1]
	return parent.get("id"), parent.get("_review_id"), {
		"mode": "thread-found",
		"parent_id": str(parent.get("id")),
		"review_id": str(parent.get("_review_id")),
		"commit_id": str(parent.get("commit_id")),
		"position": str(parent.get("position", "")),
		"body": parent.get("body", "")[:200],
	}


def post_reply() -> Tuple[int, Dict[str, str]]:
	"""Login via form auth and POST inline comment using multipart form."""
	load_env()

	username = os.environ.get("GITEA_USER", "livereview")
	password = os.environ.get("GITEA_PASS", "gitea@12345")

	# Create PAT to fetch review thread metadata
	pat_status, pat, pat_meta = create_pat(username, password)
	if not pat:
		return pat_status, pat_meta

	parent_id, review_id, thread_meta = fetch_thread(pat)
	if not parent_id or not review_id:
		return 500, {"mode": "no-parent", **(thread_meta or {}), **pat_meta}

	# Get line position
	line = thread_meta.get("position")
	if not line:
		return 400, {"mode": "no-position", "error": "Cannot post inline comment without line position", **thread_meta}

	# Use multipart form (the only working approach)
	ctx = create_session(BASE_URL, username, password)
	comment_url = f"{BASE_URL}/{OWNER}/{REPO}/pulls/{PR_NUMBER}/files/reviews/comments"
	form_fields = {
		"_csrf": ctx.csrf,
		"origin": "timeline",
		"latest_commit_id": "",
		"side": "proposed",
		"line": str(line),
		"path": COMMENT_PATH,
		"diff_start_cid": "",
		"diff_end_cid": "",
		"diff_base_cid": "",
		"content": REPLY_BODY,
		"reply": str(review_id),
		"single_review": "true",
	}

	# Force multipart by using files with (None, value) tuples
	multipart = {k: (None, v) for k, v in form_fields.items()}
	form_resp = ctx.session.post(
		comment_url,
		files=multipart,
		headers={
			"X-CSRF-Token": ctx.csrf,
			"Referer": f"{BASE_URL}/user/login",
			"Accept": "*/*",
		},
		timeout=20,
	)

	return form_resp.status_code, {
		"mode": "multipart",
		**pat_meta,
		**thread_meta,
		"form_status": form_resp.status_code,
		"form_body": form_resp.text[:1200],
		"form_url": comment_url,
	}


def main() -> None:
	status, meta = post_reply()
	print(f"reply status={status}")
	print(meta)


if __name__ == "__main__":
	main()
