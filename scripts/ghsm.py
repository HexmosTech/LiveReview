#!/usr/bin/env python3

"""GitHub Secret Manager for LiveReview env files.

Backs up selected env files to GitHub repository variables and restores
them safely with local timestamped backups.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
import shutil
import subprocess
import sys
from typing import Iterable


MANIFEST_VAR = "GHSM_FILES_MANIFEST_V1"
MAX_VAR_BODY_BYTES = 47000


@dataclass(frozen=True)
class TrackedFile:
	rel_path: str
	variable_name: str


TRACKED_FILES: tuple[TrackedFile, ...] = (
	TrackedFile(rel_path=".env", variable_name="GHSM_FILE_ENV"),
	TrackedFile(rel_path=".env.prod", variable_name="GHSM_FILE_ENV_PROD"),
	TrackedFile(rel_path=".env.prod.low-pricing", variable_name="GHSM_FILE_ENV_PROD_LOW_PRICING"),
	TrackedFile(rel_path="ui/.env.prod", variable_name="GHSM_FILE_UI_ENV_PROD"),
	TrackedFile(rel_path="debug/.env", variable_name="GHSM_FILE_DEBUG_ENV"),
	TrackedFile(rel_path="scripts/.env", variable_name="GHSM_FILE_SCRIPTS_ENV"),
)


def _run_gh(args: list[str], repo: str) -> subprocess.CompletedProcess[str]:
	cmd = ["gh", *args, "--repo", repo]
	return subprocess.run(cmd, check=False, text=True, capture_output=True)


def _ensure_gh_exists() -> None:
	if shutil.which("gh") is None:
		print("error: gh CLI not found in PATH", file=sys.stderr)
		sys.exit(1)


def _sha256_bytes(data: bytes) -> str:
	return hashlib.sha256(data).hexdigest()


def _repo_root() -> Path:
	return Path.cwd()


def _tracked_path(root: Path, tracked: TrackedFile) -> Path:
	return root / tracked.rel_path


def _atomic_write_bytes(path: Path, content: bytes) -> None:
	path.parent.mkdir(parents=True, exist_ok=True)
	tmp_path = path.with_suffix(path.suffix + ".ghsm.tmp")
	with tmp_path.open("wb") as handle:
		handle.write(content)
		handle.flush()
		os.fsync(handle.fileno())
	os.replace(tmp_path, path)


def _create_backup(root: Path, file_paths: Iterable[Path]) -> Path:
	timestamp = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%SZ")
	backup_root = root / ".ghsm-backups" / timestamp
	copied_any = False
	for file_path in file_paths:
		if not file_path.exists():
			continue
		rel = file_path.relative_to(root)
		dest = backup_root / rel
		dest.parent.mkdir(parents=True, exist_ok=True)
		shutil.copy2(file_path, dest)
		copied_any = True

	if not copied_any:
		backup_root.mkdir(parents=True, exist_ok=True)

	return backup_root


def cmd_list_files() -> int:
	root = _repo_root()
	print("Tracked files:")
	for tracked in TRACKED_FILES:
		path = _tracked_path(root, tracked)
		status = "exists" if path.exists() else "missing"
		print(f"- {tracked.rel_path} [{status}] -> {tracked.variable_name}")
	print(f"Manifest variable: {MANIFEST_VAR}")
	return 0


def cmd_upload(repo: str, dry_run: bool) -> int:
	_ensure_gh_exists()
	root = _repo_root()

	file_payloads: list[dict[str, object]] = []
	for tracked in TRACKED_FILES:
		path = _tracked_path(root, tracked)
		if not path.exists():
			print(f"error: missing required file: {tracked.rel_path}", file=sys.stderr)
			return 1

		raw = path.read_bytes()
		b64 = base64.b64encode(raw).decode("ascii")
		if len(b64.encode("utf-8")) > MAX_VAR_BODY_BYTES:
			print(
				(
					"error: encoded file too large for GitHub variable body: "
					f"{tracked.rel_path} ({len(b64)} bytes base64)"
				),
				file=sys.stderr,
			)
			return 1

		file_payloads.append(
			{
				"path": tracked.rel_path,
				"variable": tracked.variable_name,
				"sha256": _sha256_bytes(raw),
				"size": len(raw),
				"encoding": "base64",
				"value": b64,
			}
		)

	manifest = {
		"version": 1,
		"updated_at": datetime.now(timezone.utc).isoformat(),
		"files": [
			{
				"path": item["path"],
				"variable": item["variable"],
				"sha256": item["sha256"],
				"size": item["size"],
				"encoding": item["encoding"],
			}
			for item in file_payloads
		],
	}
	manifest_body = json.dumps(manifest, separators=(",", ":"), sort_keys=True)

	print(f"Uploading {len(file_payloads)} tracked file(s) to {repo}...")
	for item in file_payloads:
		variable_name = str(item["variable"])
		if dry_run:
			print(f"[dry-run] gh variable set {variable_name} --repo {repo}")
			continue
		result = _run_gh(["variable", "set", variable_name, "--body", str(item["value"])], repo)
		if result.returncode != 0:
			print(f"error: failed to upload {variable_name}: {result.stderr.strip()}", file=sys.stderr)
			return 1
		print(f"- uploaded {item['path']} -> {variable_name}")

	if dry_run:
		print(f"[dry-run] gh variable set {MANIFEST_VAR} --repo {repo}")
		print("Dry run complete.")
		return 0

	manifest_result = _run_gh(["variable", "set", MANIFEST_VAR, "--body", manifest_body], repo)
	if manifest_result.returncode != 0:
		print(
			f"error: failed to upload manifest {MANIFEST_VAR}: {manifest_result.stderr.strip()}",
			file=sys.stderr,
		)
		return 1

	print(f"Manifest updated: {MANIFEST_VAR}")
	return 0


def _fetch_variable(repo: str, variable_name: str) -> str:
	result = _run_gh(["variable", "get", variable_name], repo)
	if result.returncode != 0:
		raise RuntimeError(f"failed to fetch {variable_name}: {result.stderr.strip()}")
	return result.stdout.strip()


def cmd_download(repo: str, dry_run: bool) -> int:
	_ensure_gh_exists()
	root = _repo_root()

	try:
		manifest_raw = _fetch_variable(repo, MANIFEST_VAR)
	except RuntimeError as exc:
		print(f"error: {exc}", file=sys.stderr)
		return 1

	try:
		manifest = json.loads(manifest_raw)
	except json.JSONDecodeError as exc:
		print(f"error: manifest is not valid JSON: {exc}", file=sys.stderr)
		return 1

	files = manifest.get("files")
	if not isinstance(files, list):
		print("error: manifest is missing 'files' list", file=sys.stderr)
		return 1

	expected_map = {item.rel_path: item for item in TRACKED_FILES}
	manifest_map: dict[str, dict[str, object]] = {}
	for entry in files:
		if not isinstance(entry, dict):
			continue
		rel_path = entry.get("path")
		if isinstance(rel_path, str):
			manifest_map[rel_path] = entry

	missing = [path for path in expected_map if path not in manifest_map]
	if missing:
		print(f"error: manifest missing tracked file(s): {', '.join(missing)}", file=sys.stderr)
		return 1

	targets = [_tracked_path(root, tracked) for tracked in TRACKED_FILES]
	backup_root = _create_backup(root, targets)

	print(f"Restoring tracked files from {repo}...")
	if dry_run:
		print(f"[dry-run] backup directory would be: {backup_root}")

	for tracked in TRACKED_FILES:
		entry = manifest_map[tracked.rel_path]
		variable_name = entry.get("variable")
		sha256_expected = entry.get("sha256")
		if not isinstance(variable_name, str):
			print(f"error: invalid manifest variable for {tracked.rel_path}", file=sys.stderr)
			return 1

		try:
			encoded = _fetch_variable(repo, variable_name)
		except RuntimeError as exc:
			print(f"error: {exc}", file=sys.stderr)
			return 1

		try:
			raw = base64.b64decode(encoded, validate=True)
		except Exception as exc:  # noqa: BLE001
			print(f"error: invalid base64 payload for {tracked.rel_path}: {exc}", file=sys.stderr)
			return 1

		if isinstance(sha256_expected, str):
			digest = _sha256_bytes(raw)
			if digest != sha256_expected:
				print(
					(
						"error: integrity check failed for "
						f"{tracked.rel_path} (expected {sha256_expected}, got {digest})"
					),
					file=sys.stderr,
				)
				return 1

		target = _tracked_path(root, tracked)
		if dry_run:
			print(f"[dry-run] restore {tracked.rel_path} from {variable_name}")
			continue

		_atomic_write_bytes(target, raw)
		print(f"- restored {tracked.rel_path}")

	if not dry_run:
		print(f"Local backup saved at: {backup_root}")
	return 0


def build_parser() -> argparse.ArgumentParser:
	parser = argparse.ArgumentParser(description="Backup and restore env files via GitHub variables.")
	parser.add_argument(
		"--repo",
		default="HexmosTech/LiveReview",
		help="GitHub repository in owner/name format (default: HexmosTech/LiveReview)",
	)

	subparsers = parser.add_subparsers(dest="command", required=True)

	subparsers.add_parser("list-files", help="List tracked files and mapped GitHub variables")

	upload = subparsers.add_parser("upload", help="Upload tracked files to GitHub variables")
	upload.add_argument("--dry-run", action="store_true", help="Show actions without changing GitHub")

	download = subparsers.add_parser("download", help="Download tracked files from GitHub variables")
	download.add_argument("--dry-run", action="store_true", help="Show actions without writing local files")

	return parser


def main() -> int:
	parser = build_parser()
	args = parser.parse_args()

	if args.command == "list-files":
		return cmd_list_files()
	if args.command == "upload":
		return cmd_upload(repo=args.repo, dry_run=args.dry_run)
	if args.command == "download":
		return cmd_download(repo=args.repo, dry_run=args.dry_run)

	parser.error("unknown command")
	return 2


if __name__ == "__main__":
	raise SystemExit(main())
