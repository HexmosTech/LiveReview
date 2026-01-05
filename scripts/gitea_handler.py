"""
Experimental Gitea comment workflow exerciser.

Supported flows:
- General PR comment (PAT/API)
- Inline PR comment (browser session + CSRF)
- Reply to an inline comment (browser session + CSRF)
- Fetching comment metadata (author, timestamps, threading)
- Deleting comments (PAT/API) via --delete
- Walking parent chains via in_reply_to

It pulls the Gitea connector (PAT + base_url) from integration_tokens
using DATABASE_URL from the environment, then operates on a target PR URL
(default https://gitea.hexmos.site/megaorg/livereview/pulls/17).
"""

import argparse
import dataclasses
import json
import os
import sys
import textwrap
import time
from typing import Dict, List, Optional, Tuple

import requests

try:
	import psycopg2
except ImportError as exc:  # pragma: no cover - dependency check
	raise SystemExit(
		"psycopg2 is required. Install with `pip install psycopg2-binary`."
	) from exc

from gitea_login import (
	SessionContext,
	create_session,
	load_env as load_browser_env,
	post_inline_comment,
	reply_inline_comment,
	list_inline_comment_ids,
	delete_inline_comment,
)


# -------------------------
# Env loader
# -------------------------


def load_env_file(path: str) -> None:
	"""Load KEY=VALUE lines into os.environ without overwriting existing keys."""
	if not os.path.exists(path):
		raise FileNotFoundError(f"Env file not found: {path}")

	def _strip_quotes(val: str) -> str:
		if (val.startswith('"') and val.endswith('"')) or (
			val.startswith("'") and val.endswith("'")
		):
			return val[1:-1]
		return val

	with open(path, "r", encoding="utf-8") as fh:
		for raw in fh:
			line = raw.strip()
			if not line or line.startswith("#"):
				continue
			if "=" not in line:
				continue
			key, val = line.split("=", 1)
			key = key.strip()
			val = _strip_quotes(val.strip())
			if key and key not in os.environ:
				os.environ[key] = val


# -------------------------
# Data structures
# -------------------------


@dataclasses.dataclass
class GiteaConnector:
	connector_id: int
	org_id: int
	base_url: str
	pat: str


@dataclasses.dataclass
class CommentRefs:
	general_id: Optional[int]
	inline_id: Optional[int]
	reply_id: Optional[int]


@dataclasses.dataclass
class InlineTarget:
	path: str
	line: int
	side: str
	commit_id: Optional[str]


def map_form_side(api_side: str) -> str:
	"""Translate API side values to web-form equivalents."""
	if api_side.upper() in {"RIGHT", "PROPOSED"}:
		return "proposed"
	if api_side.upper() in {"LEFT", "PREVIOUS"}:
		return "previous"
	return "proposed"


def find_comment_by_marker(
	comments: List[Dict], marker: str, path: Optional[str]
) -> Optional[Dict]:
	for c in comments:
		if marker in (c.get("body") or "") and (
			path is None or c.get("path") == path
		):
			return c
	return None


# -------------------------
# DB helpers
# -------------------------


def load_connector_from_db(
	dsn: str, org_id: Optional[int], connector_id: Optional[int]
) -> GiteaConnector:
	"""Fetch the newest Gitea connector (PAT + base_url) from integration_tokens."""

	filters = ["provider = 'gitea'"]
	params: Dict[str, object] = {}

	if org_id is not None:
		filters.append("org_id = %(org_id)s")
		params["org_id"] = org_id

	if connector_id is not None:
		filters.append("id = %(connector_id)s")
		params["connector_id"] = connector_id

	where_clause = " AND ".join(filters)

	query = textwrap.dedent(
		f"""
		SELECT id, org_id, provider_url, pat_token
		FROM integration_tokens
		WHERE {where_clause}
		ORDER BY updated_at DESC
		LIMIT 1
		"""
	)

	with psycopg2.connect(dsn) as conn:
		with conn.cursor() as cur:
			cur.execute(query, params)
			row = cur.fetchone()
			if not row:
				raise RuntimeError("No Gitea connector found in integration_tokens")

	connector_id_db, org_id_db, base_url, pat = row
	base = (base_url or "").rstrip("/")
	if not base:
		raise RuntimeError("Connector base_url is missing")
	if not pat:
		raise RuntimeError("Connector PAT is missing")
	return GiteaConnector(
		connector_id=connector_id_db, org_id=org_id_db, base_url=base, pat=pat
	)


# -------------------------
# Gitea HTTP client
# -------------------------


class GiteaClient:
	def __init__(self, base_url: str, pat: str) -> None:
		self.base_url = base_url.rstrip("/")
		self.pat = pat
		self.session = requests.Session()
		self.session.headers.update(
			{
				"Authorization": f"token {self.pat}",
				"Accept": "application/json",
			}
		)

	# -------- Basic GET helpers
	def get_pull(self, owner: str, repo: str, number: int) -> Dict:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/pulls/{number}"
		return self._request("GET", url)

	def list_pull_files(self, owner: str, repo: str, number: int) -> List[Dict]:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/pulls/{number}/files"
		return self._request("GET", url)

	def list_review_comments(
		self, owner: str, repo: str, number: int
	) -> List[Dict]:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/pulls/{number}/comments"
		try:
			return self._request("GET", url)
		except RuntimeError as exc:
			print(f"Review comments not available: {exc}")
			return []

	def list_issue_comments(self, owner: str, repo: str, number: int) -> List[Dict]:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/issues/{number}/comments"
		return self._request("GET", url)

	def get_review_comment(self, owner: str, repo: str, comment_id: int) -> Dict:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/pulls/comments/{comment_id}"
		return self._request("GET", url)

	def get_issue_comment(self, owner: str, repo: str, comment_id: int) -> Dict:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/issues/comments/{comment_id}"
		return self._request("GET", url)

	# -------- Create / reply
	def create_general_comment(
		self, owner: str, repo: str, number: int, body: str
	) -> Dict:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/issues/{number}/comments"
		return self._request("POST", url, json={"body": body}, expected=(201,))

	def create_inline_comment(
		self,
		owner: str,
		repo: str,
		number: int,
		body: str,
		path: str,
		line: int,
		side: str,
		commit_id: Optional[str],
	) -> Dict:
		payload = {
			"body": body,
			"path": path,
			"line": line,
			"side": side,
		}
		if commit_id:
			payload["commit_id"] = commit_id

		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/pulls/{number}/comments"
		return self._request("POST", url, json=payload, expected=(201,))

	def reply_to_inline(
		self, owner: str, repo: str, number: int, in_reply_to: int, body: str
	) -> Dict:
		payload = {"body": body, "in_reply_to": in_reply_to}
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/pulls/{number}/comments"
		return self._request("POST", url, json=payload, expected=(201,))

	# -------- Delete
	def delete_review_comment(self, owner: str, repo: str, comment_id: int) -> None:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/pulls/comments/{comment_id}"
		self._request("DELETE", url, expected=(204,))

	def delete_issue_comment(self, owner: str, repo: str, comment_id: int) -> None:
		url = f"{self.base_url}/api/v1/repos/{owner}/{repo}/issues/comments/{comment_id}"
		self._request("DELETE", url, expected=(204,))

	# -------- Internal request helper
	def _request(
		self,
		method: str,
		url: str,
		*,
		json: Optional[Dict] = None,
		expected: Tuple[int, ...] = (200,),
	) -> Dict:
		resp = self.session.request(method, url, json=json, timeout=30)
		if resp.status_code not in expected:
			snippet = resp.text[:400]
			raise RuntimeError(
				f"Gitea call failed {method} {url} status={resp.status_code}: {snippet}"
			)
		if resp.status_code == 204:
			return {}
		return resp.json()


def choose_inline_target(
	client: GiteaClient, owner: str, repo: str, number: int, head_sha: Optional[str]
) -> InlineTarget:
	files = client.list_pull_files(owner, repo, number)
	for f in files:
		patch = f.get("patch") or ""
		line = find_first_added_line(patch)
		if line is None:
			line = 1  # fallback when diff has no additions
		commit_id = f.get("sha") or head_sha
		return InlineTarget(path=f["filename"], line=line, side="RIGHT", commit_id=commit_id)
	raise RuntimeError("No suitable file with additions found for inline comment")


def find_first_added_line(patch: str) -> Optional[int]:
	"""Return the first new-line number from a unified diff patch."""

	new_line = None
	old_cursor = 0
	new_cursor = 0
	fallback_line = None

	for raw in patch.splitlines():
		if raw.startswith("@@"):
			header = raw.split("@@")[1].strip()
			try:
				old_chunk, new_chunk = header.split(" ")[:2]
				old_start = int(old_chunk.split(",")[0][1:])
				new_start = int(new_chunk.split(",")[0][1:])
				old_cursor = old_start
				new_cursor = new_start
				fallback_line = fallback_line or new_start
			except (ValueError, IndexError):
				continue
			continue

		if raw.startswith("+"):
			if new_line is None:
				new_line = new_cursor
			new_cursor += 1
		elif raw.startswith("-"):
			old_cursor += 1
		else:
			old_cursor += 1
			new_cursor += 1

		if new_line is not None:
			return new_line

	return new_line or fallback_line


# -------------------------
# PR URL parsing
# -------------------------


def parse_pr_url(pr_url: str) -> Tuple[str, str, int]:
	"""Extract owner, repo, number from a PR URL like .../owner/repo/pulls/17."""

	parts = pr_url.rstrip("/").split("/")
	if len(parts) < 2:
		raise RuntimeError(f"Invalid PR URL: {pr_url}")
	try:
		number = int(parts[-1])
	except ValueError as exc:
		raise RuntimeError(f"Cannot parse PR number from URL: {pr_url}") from exc
	if parts[-2] not in {"pull", "pulls"}:
		raise RuntimeError(f"URL does not look like a pull request: {pr_url}")
	owner = parts[-4]
	repo = parts[-3]
	return owner, repo, number


# -------------------------
# Demo runner
# -------------------------


def run_demo(
	connector: GiteaConnector,
	pr_url: str,
	delete_after: bool,
	inline_form_side: str,
	reply_parent_override: Optional[int],
	org_id: int,
	test_relogin: bool,
) -> None:
	owner, repo, number = parse_pr_url(pr_url)
	client = GiteaClient(connector.base_url, connector.pat)
	inline_form_side = map_form_side(inline_form_side)

	print(f"Using connector id={connector.connector_id} org={org_id} base={connector.base_url}")
	pr = client.get_pull(owner, repo, number)
	head_sha = pr.get("head", {}).get("sha")
	print(f"PR title: {pr.get('title')} head_sha={head_sha}")

	if delete_after:
		delete_all_comments(
			client=client,
			owner=owner,
			repo=repo,
			pr_number=number,
			base_url=connector.base_url,
		)
		return

	if test_relogin:
		test_relogin_flow(
			base_url=connector.base_url,
			owner=owner,
			repo=repo,
			pr_number=number,
			path_hint="cmd/config.go",
			line_hint=3,
			head_sha=head_sha or "",
		)
		return

	target = choose_inline_target(client, owner, repo, number, head_sha)
	print(
		f"Inline target path={target.path} line={target.line} side={target.side} commit={target.commit_id}"
	)

	timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
	general_marker = f"[demo-general {timestamp}]"
	inline_marker = f"[demo-inline {timestamp}]"
	reply_marker = f"[demo-reply {timestamp}]"

	general = client.create_general_comment(
		owner,
		repo,
		number,
		body=general_marker,
	)
	print(f"Created general comment id={general.get('id')}")

	inline_id = None
	reply_id = None
	ctx: Optional[SessionContext] = None
	try:
		ctx = create_session(
			connector.base_url,
			os.environ.get("GITEA_USER", "livereview"),
			os.environ.get("GITEA_PASS", "gitea@12345"),
		)
		if reply_parent_override is None:
			status_inline, meta_inline = post_inline_comment(
				ctx,
				owner=owner,
				repo=repo,
				pr_number=number,
				path=target.path,
				line=target.line,
				side=inline_form_side,
				commit_id=target.commit_id or head_sha or "",
				content=inline_marker,
			)
			print(f"Inline via web form status={status_inline} detail={meta_inline}")
		else:
			print(f"Skipping inline creation; replying to existing id={reply_parent_override}")
	except Exception as exc:
		print(f"Inline attempt failed: {exc}")

	review_comments = client.list_review_comments(owner, repo, number)
	inline_comment = find_comment_by_marker(review_comments, inline_marker, target.path)
	if inline_comment:
		inline_id = inline_comment.get("id")

	parent_id = reply_parent_override or inline_id
	reply_path = target.path
	reply_line = target.line
	reply_commit = target.commit_id or head_sha or ""
	if reply_parent_override:
		try:
			parent_comment = client.get_review_comment(owner, repo, reply_parent_override)
			reply_path = parent_comment.get("path") or reply_path
			reply_line = parent_comment.get("line") or reply_line
			reply_commit = parent_comment.get("commit_id") or reply_commit
		except Exception as exc:
			print(f"Failed to fetch parent {reply_parent_override}: {exc}")
	if ctx and parent_id:
		try:
			status_reply, meta_reply = reply_inline_comment(
				ctx,
				owner=owner,
				repo=repo,
				pr_number=number,
				path=reply_path,
				line=reply_line,
				side=inline_form_side,
				commit_id=reply_commit,
				content=reply_marker,
				parent_comment_id=parent_id,
			)
			print(f"Reply via web form status={status_reply} detail={meta_reply}")
		except Exception as exc:
			print(f"Reply attempt failed: {exc}")

	review_comments = client.list_review_comments(owner, repo, number)
	issue_comments = client.list_issue_comments(owner, repo, number)

	reply_comment = find_comment_by_marker(review_comments, reply_marker, target.path)
	if reply_comment:
		reply_id = reply_comment.get("id")

	created_ids = CommentRefs(
		general_id=general.get("id"), inline_id=inline_id, reply_id=reply_id
	)
	summarize_comments(
		owner, repo, created_ids, review_comments, issue_comments, client
	)

	if delete_after:
		cleanup_comments(owner, repo, created_ids, review_comments, issue_comments, client)


def summarize_comments(
	owner: str,
	repo: str,
	created: CommentRefs,
	review_comments: List[Dict],
	issue_comments: List[Dict],
	client: GiteaClient,
) -> None:
	review_index = {c.get("id"): c for c in review_comments}

	print("\nReview comments (id, user, path, line, in_reply_to, created_at):")
	for c in review_comments:
		print(
			f" - {c.get('id')} user={c.get('user', {}).get('login')} "
			f"path={c.get('path')} line={c.get('line')} "
			f"in_reply_to={c.get('in_reply_to')} created={c.get('created_at')}"
		)

	print("\nIssue comments (general) (id, user, created_at):")
	for c in issue_comments:
		print(
			f" - {c.get('id')} user={c.get('user', {}).get('login')} "
			f"created={c.get('created_at')}"
		)

	if created.reply_id:
		print("\nParent chain for reply (child -> parents):")
		chain = build_parent_chain(created.reply_id, review_index)
		print(" -> ".join(chain))

	print("\nSingle-comment fetch tests:")
	if created.inline_id:
		c = client.get_review_comment(owner, repo, created.inline_id)
		print(f" inline {created.inline_id} path={c.get('path')} created={c.get('created_at')}")
	if created.general_id:
		c = client.get_issue_comment(owner, repo, created.general_id)
		print(f" general {created.general_id} created={c.get('created_at')}")


def build_parent_chain(comment_id: Optional[int], index: Dict[int, Dict]) -> List[str]:
	chain: List[str] = []
	current = comment_id
	while current and current in index:
		chain.append(str(current))
		parent = index[current].get("in_reply_to")
		current = parent
	return chain


def cleanup_comments(
	owner: str,
	repo: str,
	created: CommentRefs,
	review_comments: List[Dict],
	issue_comments: List[Dict],
	client: GiteaClient,
) -> None:
	print("\nCleanup enabled; deleting demo comments...")
	review_ids = set()
	issue_ids = set()
	if created.reply_id:
		review_ids.add(created.reply_id)
	if created.inline_id:
		review_ids.add(created.inline_id)
	if created.general_id:
		issue_ids.add(created.general_id)

	def _is_demo(body: Optional[str]) -> bool:
		if not body:
			return False
		return any(token in body for token in ("[demo", "browser-form comment"))

	for c in review_comments:
		if _is_demo(c.get("body")):
			review_ids.add(c.get("id"))
	for c in issue_comments:
		if _is_demo(c.get("body")):
			issue_ids.add(c.get("id"))

	if not review_ids and not issue_ids:
		print(" No demo comments found to delete.")
		return

	for cid in sorted(r for r in review_ids if r):
		client.delete_review_comment(owner, repo, cid)
		print(f" deleted review comment {cid}")
	for cid in sorted(i for i in issue_ids if i):
		client.delete_issue_comment(owner, repo, cid)
		print(f" deleted issue comment {cid}")


def delete_all_comments(
	client: GiteaClient,
	owner: str,
	repo: str,
	pr_number: int,
	base_url: str,
) -> None:
	print("Delete-only mode: removing all issue and inline comments...")

	# Delete all issue comments via API.
	try:
		issue_comments = client.list_issue_comments(owner, repo, pr_number)
		for c in issue_comments:
			cid = c.get("id")
			if cid:
				client.delete_issue_comment(owner, repo, cid)
				print(f" deleted issue comment {cid}")
	except Exception as exc:
		print(f"Issue comment deletion failed: {exc}")

	# Attempt review comment deletion via API (may 404 on this instance).
	try:
		review_comments = client.list_review_comments(owner, repo, pr_number)
		for c in review_comments:
			cid = c.get("id")
			if cid:
				client.delete_review_comment(owner, repo, cid)
				print(f" deleted review comment {cid}")
	except Exception as exc:
		print(f"Review comment API deletion failed: {exc}")

	# Browser-session inline deletion as fallback.
	try:
		ctx = create_session(
			base_url,
			os.environ.get("GITEA_USER", "livereview"),
			os.environ.get("GITEA_PASS", "gitea@12345"),
		)
		inline_ids = list_inline_comment_ids(ctx, owner, repo, pr_number)
		if inline_ids:
			for cid in inline_ids:
				status, used_url = delete_inline_comment(
					ctx, owner, repo, cid, pr_number=pr_number
				)
				print(
					f" deleted inline comment {cid} via browser session status={status} url={used_url}"
				)
		else:
			print(" no inline comments found via browser session scrape")
	except Exception as exc:
		print(f"Browser-session inline deletion failed: {exc}")


def test_relogin_flow(
	base_url: str,
	owner: str,
	repo: str,
	pr_number: int,
	path_hint: str,
	line_hint: int,
	head_sha: str,
) -> None:
	"""Validate relogin by forcing cookie expiry before GET and POST."""
	print("Starting relogin test...")
	ctx = create_session(
		base_url,
		os.environ.get("GITEA_USER", "livereview"),
		os.environ.get("GITEA_PASS", "gitea@12345"),
	)

	baseline_ids = list_inline_comment_ids(ctx, owner, repo, pr_number)
	print(f" baseline inline ids: {baseline_ids}")

	# Force expiry then GET
	ctx.session.cookies.pop("gitea_incredible", None)
	ids_after_relogin = list_inline_comment_ids(ctx, owner, repo, pr_number)
	print(f" after forced expiry, fetched inline ids (should succeed): {ids_after_relogin}")

	# Force expiry then POST
	ctx.session.cookies.pop("gitea_incredible", None)
	marker = f"[relogin-test {time.strftime('%H:%M:%S')}]"
	status_post, meta_post = post_inline_comment(
		ctx,
		owner=owner,
		repo=repo,
		pr_number=pr_number,
		path=path_hint,
		line=line_hint,
		side="proposed",
		commit_id=head_sha,
		content=marker,
	)
	print(f" post status={status_post} meta={meta_post}")

	# Identify and remove newly created inline(s)
	ids_after_post = list_inline_comment_ids(ctx, owner, repo, pr_number)
	new_ids = [cid for cid in ids_after_post if cid not in baseline_ids]
	if new_ids:
		print(f" cleaning up new inline ids: {new_ids}")
		for cid in new_ids:
			status_del, used_url = delete_inline_comment(
				ctx, owner, repo, cid, pr_number=pr_number
			)
			print(f"  deleted inline {cid} status={status_del} url={used_url}")
	else:
		print(" no new inline ids detected after post")

	print("Relogin test complete.")

def main() -> None:
	parser = argparse.ArgumentParser(description="Gitea comment workflow demo")
	parser.add_argument(
		"--pr-url",
		default="https://gitea.hexmos.site/megaorg/livereview/pulls/17",
		help="Target pull request URL",
	)
	parser.add_argument(
		"--inline-side",
		default="proposed",
		choices=["proposed", "previous"],
		help="Side for inline/reply comments (web form)",
	)
	parser.add_argument(
		"--reply-parent",
		type=int,
		help="Existing inline comment id to reply to (skip creating a new inline)",
	)
	parser.add_argument(
		"--delete",
		action="store_true",
		help="Delete created comments at the end",
	)
	parser.add_argument(
		"--test-relogin",
		action="store_true",
		help="Run relogin test (forces cookie expiry, GET+POST, cleans up new inline)",
	)
	args = parser.parse_args()

	# Prefer production .env.prod; fallback to local .env.
	script_dir = os.path.dirname(__file__)
	prod_env = os.path.abspath(os.path.join(script_dir, os.pardir, ".env.prod"))
	local_env = os.path.abspath(os.path.join(script_dir, os.pardir, ".env"))

	if os.path.exists(prod_env):
		load_env_file(prod_env)
	elif os.path.exists(local_env):
		load_env_file(local_env)
	else:
		raise SystemExit("No .env or .env.prod found next to repository root")

	# Also load browser session env (same files, but tolerant) for GITEA_USER/PASS.
	load_browser_env()

	dsn = os.environ.get("DATABASE_URL")
	if not dsn:
		raise SystemExit("DATABASE_URL is required in environment")

	# Infer the newest Gitea connector automatically.
	connector = load_connector_from_db(dsn, org_id=None, connector_id=None)

	run_demo(
		connector=connector,
		pr_url=args.pr_url,
		delete_after=args.delete,
		inline_form_side=args.inline_side,
		reply_parent_override=args.reply_parent,
		test_relogin=args.test_relogin,
		org_id=connector.org_id,
	)


if __name__ == "__main__":
	try:
		main()
	except Exception as exc:  # pragma: no cover - CLI guard
		print(f"Error: {exc}", file=sys.stderr)
		sys.exit(1)

