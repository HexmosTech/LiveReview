#!/usr/bin/env python3
"""Sync production PostgreSQL database into local docker PostgreSQL.

Usage (run from scripts directory):
  python3 duplicate_prod_db.py --dry-run
  python3 duplicate_prod_db.py --force

Behavior:
- Reads source DB from ../.env.prod (DATABASE_URL)
- Reads destination DB from ../.env (DATABASE_URL)
- Starts local docker PostgreSQL via ../pgctl.sh start when needed
- Creates destination DB when missing
- Requires --force to overwrite an existing local DB with user tables
"""

from __future__ import annotations

import argparse
import shlex
import shutil
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import parse_qsl, urlencode, urlparse, urlunparse


class SyncError(Exception):
    """Raised when sync workflow fails."""


@dataclass
class DbUrl:
    raw: str
    scheme: str
    username: str
    password: str
    hostname: str
    port: int
    dbname: str
    query: str

    @classmethod
    def from_url(cls, raw_url: str) -> "DbUrl":
        raw_url = raw_url.strip().strip('"').strip("'")
        if raw_url.startswith("postgres://"):
            normalized = "postgresql://" + raw_url[len("postgres://") :]
        else:
            normalized = raw_url

        parsed = urlparse(normalized)
        if parsed.scheme not in {"postgresql", "postgres"}:
            raise SyncError(f"Unsupported DATABASE_URL scheme: {parsed.scheme}")
        if not parsed.hostname:
            raise SyncError("DATABASE_URL is missing host")
        if not parsed.username:
            raise SyncError("DATABASE_URL is missing username")
        if not parsed.path or parsed.path == "/":
            raise SyncError("DATABASE_URL is missing database name")

        dbname = parsed.path.lstrip("/")
        if not dbname:
            raise SyncError("DATABASE_URL is missing database name")

        return cls(
            raw=normalized,
            scheme="postgresql",
            username=parsed.username,
            password=parsed.password or "",
            hostname=parsed.hostname,
            port=parsed.port or 5432,
            dbname=dbname,
            query=parsed.query,
        )

    def as_url(self, dbname: str | None = None) -> str:
        name = self.dbname if dbname is None else dbname
        netloc = self.hostname
        if self.username:
            userinfo = self.username
            if self.password:
                userinfo += f":{self.password}"
            netloc = f"{userinfo}@{netloc}"
        if self.port:
            netloc = f"{netloc}:{self.port}"

        query = self.query
        if query:
            query = urlencode(parse_qsl(query, keep_blank_values=True), doseq=True)

        return urlunparse((self.scheme, netloc, f"/{name}", "", query, ""))


def read_database_url(env_file: Path) -> str:
    if not env_file.exists():
        raise SyncError(f"Env file not found: {env_file}")

    for line in env_file.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("DATABASE_URL="):
            return line.split("=", 1)[1].strip()

    raise SyncError(f"DATABASE_URL not found in {env_file}")


def quote_identifier(name: str) -> str:
    return '"' + name.replace('"', '""') + '"'


class SyncRunner:
    def __init__(self, force: bool, dry_run: bool, verbose: bool) -> None:
        self.force = force
        self.dry_run = dry_run
        self.verbose = verbose

        self.scripts_dir = Path(__file__).resolve().parent
        self.repo_root = self.scripts_dir.parent
        self.prod_env = self.repo_root / ".env.prod"
        self.local_env = self.repo_root / ".env"
        self.pgctl = self.repo_root / "pgctl.sh"

        self.prod_db = DbUrl.from_url(read_database_url(self.prod_env))
        self.local_db = DbUrl.from_url(read_database_url(self.local_env))

    def log(self, msg: str) -> None:
        print(msg)

    def require_commands(self) -> None:
        missing = [
            name
            for name in ["psql", "pg_dump", "pg_restore"]
            if shutil.which(name) is None
        ]
        if missing:
            raise SyncError(
                "Missing required PostgreSQL client tools: "
                + ", ".join(missing)
                + ". Install first (Ubuntu/WSL): sudo apt update && sudo apt install -y postgresql-client"
            )

    def run_cmd(
        self,
        cmd: list[str],
        env: dict[str, str] | None = None,
        check: bool = True,
        stream_output: bool = False,
    ) -> subprocess.CompletedProcess[str]:
        if self.verbose or self.dry_run:
            self.log(f"$ {' '.join(shlex.quote(p) for p in cmd)}")

        if self.dry_run:
            return subprocess.CompletedProcess(cmd, 0, "", "")

        try:
            if stream_output:
                return subprocess.run(
                    cmd,
                    env=env,
                    cwd=str(self.repo_root),
                    text=True,
                    check=check,
                )

            return subprocess.run(
                cmd,
                env=env,
                cwd=str(self.repo_root),
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                check=check,
            )
        except FileNotFoundError as exc:
            raise SyncError(f"Command not found: {cmd[0]}") from exc
        except subprocess.CalledProcessError as exc:
            stderr = (exc.stderr or "").strip()
            stdout = (exc.stdout or "").strip()
            details = stderr or stdout or "(no command output)"
            raise SyncError(f"Command failed: {' '.join(cmd)}\n{details}") from exc

    def is_database_ready(self, db_url: str) -> bool:
        if shutil.which("pg_isready") is not None:
            cmd = ["pg_isready", "-d", db_url]
            try:
                result = self.run_cmd(cmd, check=False)
                return result.returncode == 0
            except SyncError:
                return False

        if shutil.which("psql") is not None:
            cmd = ["psql", db_url, "-t", "-A", "-c", "SELECT 1;"]
            try:
                result = self.run_cmd(cmd, check=False)
                return result.returncode == 0
            except SyncError:
                return False

        return False

    def wait_for_local_ready(self, timeout_seconds: int = 180) -> None:
        self.log("Checking local database readiness...")
        maintenance_url = self.local_db.as_url("postgres")
        if self.is_database_ready(maintenance_url):
            self.log("Local database is reachable.")
            return

        self.log("Local database is not reachable. Starting local postgres via ../pgctl.sh start...")
        if not self.pgctl.exists():
            raise SyncError(f"Expected pgctl script not found: {self.pgctl}")

        try:
            self.run_cmd(["./pgctl.sh", "start"], stream_output=True)
        except SyncError as exc:
            message = str(exc)
            if "docker API at unix:///var/run/docker.sock" in message and "permission denied" in message.lower():
                raise SyncError(
                    "Cannot start local postgres because Docker socket access is denied. "
                    "Fix Docker permissions for this user, then re-run."
                ) from exc
            raise

        if self.dry_run:
            self.log("[dry-run] Skipping readiness wait.")
            return

        deadline = time.time() + timeout_seconds
        while time.time() < deadline:
            if self.is_database_ready(maintenance_url):
                self.log("Local database started and reachable.")
                return
            time.sleep(2)

        raise SyncError("Local database did not become ready in time after pgctl start")

    def psql_scalar(self, db_url: str, sql: str) -> str:
        cmd = ["psql", db_url, "-t", "-A", "-c", sql]
        result = self.run_cmd(cmd)
        return (result.stdout or "").strip()

    def ensure_local_database_exists(self) -> None:
        self.log(f"Ensuring destination database exists: {self.local_db.dbname}")
        maintenance_url = self.local_db.as_url("postgres")
        escaped_name = self.local_db.dbname.replace("'", "''")
        check_sql = (
            "SELECT 1 FROM pg_database "
            f"WHERE datname = '{escaped_name}';"
        )
        exists = self.psql_scalar(maintenance_url, check_sql)
        if exists == "1":
            self.log("Destination database already exists.")
            return

        create_sql = f"CREATE DATABASE {quote_identifier(self.local_db.dbname)};"
        self.run_cmd(["psql", maintenance_url, "-v", "ON_ERROR_STOP=1", "-c", create_sql])
        self.log("Destination database created.")

    def local_has_user_tables(self) -> bool:
        sql = (
            "SELECT COUNT(*) FROM pg_catalog.pg_tables "
            "WHERE schemaname NOT IN ('pg_catalog', 'information_schema');"
        )
        value = self.psql_scalar(self.local_db.as_url(), sql)
        try:
            return int(value or "0") > 0
        except ValueError as exc:
            raise SyncError(f"Unexpected table count result: {value}") from exc

    def reset_local_database(self) -> None:
        self.log("Force mode enabled: replacing local database.")
        maintenance_url = self.local_db.as_url("postgres")
        escaped_name = self.local_db.dbname.replace("'", "''")

        terminate_sql = (
            "SELECT pg_terminate_backend(pid) "
            "FROM pg_stat_activity "
            f"WHERE datname = '{escaped_name}' AND pid <> pg_backend_pid();"
        )
        drop_sql = f"DROP DATABASE IF EXISTS {quote_identifier(self.local_db.dbname)};"
        create_sql = f"CREATE DATABASE {quote_identifier(self.local_db.dbname)};"

        self.run_cmd(["psql", maintenance_url, "-v", "ON_ERROR_STOP=1", "-c", terminate_sql])
        self.run_cmd(["psql", maintenance_url, "-v", "ON_ERROR_STOP=1", "-c", drop_sql])
        self.run_cmd(["psql", maintenance_url, "-v", "ON_ERROR_STOP=1", "-c", create_sql])
        self.log("Destination database dropped and recreated.")

    def dump_and_restore(self) -> None:
        self.log("Syncing production database into local database...")
        with tempfile.NamedTemporaryFile(prefix="prod_db_", suffix=".dump", delete=False) as temp_file:
            dump_path = Path(temp_file.name)

        try:
            dump_cmd = [
                "pg_dump",
                "--format=custom",
                "--no-owner",
                "--no-privileges",
                "--verbose",
                "--dbname",
                self.prod_db.as_url(),
                "--file",
                str(dump_path),
            ]
            self.log("Running pg_dump (live output)...")
            self.run_cmd(dump_cmd, stream_output=True)

            restore_cmd = [
                "pg_restore",
                "--no-owner",
                "--no-privileges",
                "--exit-on-error",
                "--verbose",
                "--dbname",
                self.local_db.as_url(),
                str(dump_path),
            ]
            self.log("Running pg_restore (live output)...")
            self.run_cmd(restore_cmd, stream_output=True)
        finally:
            if dump_path.exists() and not self.dry_run:
                dump_path.unlink()

    def run(self) -> int:
        self.log("Starting production -> local database sync...")
        self.log(f"Source:      {self.prod_db.hostname}:{self.prod_db.port}/{self.prod_db.dbname}")
        self.log(f"Destination: {self.local_db.hostname}:{self.local_db.port}/{self.local_db.dbname}")

        self.require_commands()
        self.wait_for_local_ready()
        self.ensure_local_database_exists()

        if self.local_has_user_tables() and not self.force:
            raise SyncError(
                "Local database already contains tables. "
                "Re-run with --force to replace local data with production snapshot."
            )

        if self.force:
            self.reset_local_database()

        self.dump_and_restore()
        self.log("Sync completed successfully.")
        return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Sync production PostgreSQL database to local docker PostgreSQL"
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Destroy and replace existing local database data",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print planned commands without making changes",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Print commands before execution",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        runner = SyncRunner(force=args.force, dry_run=args.dry_run, verbose=args.verbose)
        return runner.run()
    except SyncError as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
