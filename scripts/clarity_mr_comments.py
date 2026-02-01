#!/usr/bin/env python3
"""
Prototype: Post a single inline comment to a GitLab MR at a specific file and line.

- Reads base URL, token, and MR URL from local constants (mirror delete_all_comments.py).
- Fetches latest MR version SHAs (base/start/head).
- Fetches MR changes and parses the unified diff for the target file to classify the target line
  as added (+), deleted (-), or context (space), then builds the correct position payload:
  - added  → position[new_line]
  - deleted→ position[old_line]
  - context→ both position[old_line] and position[new_line]

Usage:
  python clarity_mr_comments.py --file liveapi-backend/qmanager/repodag.go --line 285 \
	  --text "Prototype inline comment"

Optional:
  --side new|old   # bypass detection: force new_line or old_line

This script avoids printing secrets and prints server responses only when needed.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from typing import Dict, Optional, Tuple

import requests


# --- Access details (mirrored from delete_all_comments.py) ---
# NOTE: Keep these in sync with scripts/delete_all_comments.py; avoid logging the token.
URL = "https://git.apps.hexmos.com"
# Test token for git.apps.hexmos.com - hardcoded for testing only
TOKEN = "REDACTED_GITLAB_PAT_6"
MR_URL = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/426"


def extract_mr_info(mr_url: str) -> Tuple[Optional[str], Optional[str]]:
	pattern = r"https://[^/]+/([^/]+/[^/]+)/-/merge_requests/(\d+)"
	m = re.match(pattern, mr_url)
	if not m:
		return None, None
	return m.group(1), m.group(2)


def encode_project_path(project_path: str) -> str:
	return project_path.replace("/", "%2F")


def get_mr_versions(base_url: str, project_path: str, mr_id: str, headers: Dict[str, str]) -> Optional[Dict]:
	versions_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/versions"
	r = requests.get(versions_url, headers=headers)
	if r.status_code == 200:
		arr = r.json()
		if arr:
			return arr[0]
	print(f"[ERROR] Failed to fetch MR versions ({r.status_code}): {r.text[:300]}")
	return None


def get_mr_changes(base_url: str, project_path: str, mr_id: str, headers: Dict[str, str]) -> Optional[Dict]:
	changes_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/changes"
	r = requests.get(changes_url, headers=headers)
	if r.status_code != 200:
		print(f"[ERROR] Failed to fetch MR changes ({r.status_code}): {r.text[:300]}")
		return None
	return r.json()


def find_change_for_file(changes_payload: Dict, file_path: str) -> Optional[Dict]:
	changes_list = changes_payload.get("changes", changes_payload)
	if isinstance(changes_list, dict):
		changes_list = changes_list.get("changes", [])
	for ch in changes_list:
		if ch.get("new_path") == file_path or ch.get("old_path") == file_path:
			return ch
	return None


def find_first_added_line(diff_text: str) -> Optional[int]:
	"""Return the first new_line number for an added ('+') line in the diff text."""
	if not diff_text:
		return None
	lines = diff_text.splitlines()
	old_ln = new_ln = None
	hunk_header_re = re.compile(r"^@@ -(?P<old_start>\d+)(?:,(?P<old_count>\d+))? \+(?P<new_start>\d+)(?:,(?P<new_count>\d+))? @@")
	for line in lines:
		if line.startswith("@@ "):
			m = hunk_header_re.match(line)
			if not m:
				old_ln = new_ln = None
				continue
			old_ln = int(m.group("old_start"))
			new_ln = int(m.group("new_start"))
			continue
		if old_ln is None or new_ln is None:
			continue
		if not line:
			continue
		prefix = line[0]
		if prefix == ' ':
			old_ln += 1
			new_ln += 1
		elif prefix == '+':
			return new_ln
		elif prefix == '-':
			old_ln += 1
	return None


def classify_line_in_diff(diff_text: str, target_line: int) -> Optional[Tuple[str, int, int]]:
	"""
	Parse a unified diff (GitLab change["diff"]) to determine if target_line refers to:
	  - added line → returns ("added", old_line, new_line)
	  - deleted line → returns ("deleted", old_line, new_line)
	  - context line → returns ("context", old_line, new_line)
	If ambiguous or not found, return None.
	"""
	if not diff_text:
		return None

	lines = diff_text.splitlines()
	# State for current hunk
	old_ln = new_ln = None

	hunk_header_re = re.compile(r"^@@ -(?P<old_start>\d+)(?:,(?P<old_count>\d+))? \+(?P<new_start>\d+)(?:,(?P<new_count>\d+))? @@")

	for line in lines:
		if line.startswith("@@ "):
			m = hunk_header_re.match(line)
			if not m:
				old_ln = new_ln = None
				continue
			old_ln = int(m.group("old_start"))
			new_ln = int(m.group("new_start"))
			continue

		if old_ln is None or new_ln is None:
			continue  # not inside a hunk

		if not line:
			continue

		prefix = line[0]
		# Hunk body line classification
		if prefix == ' ':
			# Context: both sides advance
			if target_line == new_ln or target_line == old_ln:
				return ("context", old_ln, new_ln)
			old_ln += 1
			new_ln += 1
		elif prefix == '+':
			# Added: only new advances
			if target_line == new_ln:
				return ("added", old_ln, new_ln)
			new_ln += 1
		elif prefix == '-':
			# Deleted: only old advances
			if target_line == old_ln:
				return ("deleted", old_ln, new_ln)
			old_ln += 1
		else:
			# Unrecognized; skip
			continue

	return None


def create_inline_comment(
	base_url: str,
	project_path: str,
	mr_id: str,
	headers: Dict[str, str],
	version_info: Dict,
	change: Dict,
	file_path: str,
	target_line: int,
	comment_text: str,
	force_side: Optional[str] = None,
) -> Optional[Dict]:
	diff_text = change.get("diff", "")

	kind_old_new = None
	if force_side in ("new", "old"):
		# Bypass detection: treat as added (new) or deleted (old)
		if force_side == "new":
			kind_old_new = ("added", None, target_line)
		else:
			kind_old_new = ("deleted", target_line, None)
	else:
		cls = classify_line_in_diff(diff_text, target_line)
		if cls is None:
			# Fallback to first added line in this file's diff
			fallback_new = find_first_added_line(diff_text)
			if fallback_new is not None:
				print(f"[WARN] Target line not in diff; falling back to first added line: {fallback_new}")
				kind_old_new = ("added", None, fallback_new)
			else:
				print("[WARN] Could not classify and no added lines found; defaulting to 'new' side at provided line.")
				kind_old_new = ("added", None, target_line)
		else:
			kind_old_new = cls

	kind, old_line_num, new_line_num = kind_old_new

	payload = {
		"body": comment_text,
		"position": {
			"position_type": "text",
			"base_sha": version_info["base_commit_sha"],
			"head_sha": version_info["head_commit_sha"],
			"start_sha": version_info["start_commit_sha"],
			"old_path": change.get("old_path", file_path),
			"new_path": change.get("new_path", file_path),
		},
	}

	if kind == "added":
		payload["position"]["new_line"] = int(new_line_num if new_line_num is not None else target_line)
	elif kind == "deleted":
		payload["position"]["old_line"] = int(old_line_num if old_line_num is not None else target_line)
	else:  # context
		# Use both; if one side is None (rare), fall back to target_line
		payload["position"]["old_line"] = int(old_line_num if old_line_num is not None else target_line)
		payload["position"]["new_line"] = int(new_line_num if new_line_num is not None else target_line)

	discussions_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/discussions"
	r = requests.post(discussions_url, json=payload, headers=headers)
	if r.status_code in (200, 201):
		print("[OK] Inline comment created.")
		return r.json()
	print(f"[ERROR] Failed to create inline comment ({r.status_code}): {r.text[:400]}")
	print("[DEBUG] Payload sent:")
	print(json.dumps(payload, indent=2))
	return None


def main(argv: Optional[list] = None) -> int:
	p = argparse.ArgumentParser(description="Post a single inline comment to a GitLab MR.")
	p.add_argument("--file", required=False, default="liveapi-backend/qmanager/repodag.go",
				   help="File path inside the repo as it appears in MR changes")
	p.add_argument("--line", type=int, required=False, default=285,
				   help="Target line number as visible in MR diff")
	p.add_argument("--text", required=False, default="Prototype inline comment (clarity)",
				   help="Comment text")
	p.add_argument("--side", choices=["new", "old"], required=False,
				   help="Force side (new for added, old for deleted) to bypass detection")
	args = p.parse_args(argv)

	project_path, mr_id = extract_mr_info(MR_URL)
	if not project_path or not mr_id:
		print(f"[FATAL] Could not extract project/MR from MR_URL={MR_URL}")
		return 2

	enc_project = encode_project_path(project_path)
	headers = {
		"PRIVATE-TOKEN": TOKEN,
		"Content-Type": "application/json",
	}

	version_info = get_mr_versions(URL, enc_project, mr_id, headers)
	if not version_info:
		return 2

	changes = get_mr_changes(URL, enc_project, mr_id, headers)
	if not changes:
		return 2

	change = find_change_for_file(changes, args.file)
	if not change:
		# Fallback to the first changed file
		ch_list = changes.get("changes", [])
		if not ch_list:
			print(f"[FATAL] No changes found in MR.")
			return 2
		change = ch_list[0]
		print(f"[WARN] File not found in MR changes: {args.file}. Falling back to first changed file: {change.get('new_path')}")
		args.file = change.get("new_path") or change.get("old_path")

	res = create_inline_comment(
		URL,
		enc_project,
		mr_id,
		headers,
		version_info,
		change,
		args.file,
		args.line,
		args.text,
		force_side=args.side,
	)
	if not res:
		return 1

	print("Discussion created: ")
	print(json.dumps(res, indent=2)[:800])
	return 0


if __name__ == "__main__":
	sys.exit(main())

