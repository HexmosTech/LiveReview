#!/usr/bin/env python3
"""Pre-deployment Environment Variable Verification for LiveReview.

Ensures that deployment env files (.env.prod, .env.staging, .env.prod.low-pricing)
contain all required keys, have non-empty values, and do not contain unregistered new keys.

Key sets are externalized in 'deploy_env_keys.json' relative to this script.

CRITICAL SAFETY RULE:
  Local '.env' is strictly for local workstation development. This script will REJECT
  any attempt to validate or deploy with local '.env' to protect local developer configs.

Works with both 'python3' and 'python' (Python 3.8+).
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


CONFIG_FILE_NAME = "deploy_env_keys.json"


def load_config() -> dict:
    """Load configuration from external JSON."""
    config_path = Path(__file__).parent / CONFIG_FILE_NAME
    if not config_path.exists():
        print(f"❌ ERROR: Key definition config not found: {config_path}", file=sys.stderr)
        sys.exit(1)

    try:
        return json.loads(config_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"❌ ERROR: Failed to parse {config_path}: {exc}", file=sys.stderr)
        sys.exit(1)


def load_mode_keys(config: dict, mode: str) -> set[str]:
    """Compute expected keys for the given mode from config."""
    base_keys = set(config.get("base_keys", []))
    modes = config.get("modes", {})

    if mode not in modes:
        available = ", ".join(modes.keys())
        print(
            f"❌ ERROR: Mode '{mode}' not configured in {CONFIG_FILE_NAME}. Available: {available}",
            file=sys.stderr,
        )
        sys.exit(1)

    mode_cfg = modes[mode]
    add_keys = set(mode_cfg.get("add", []))
    remove_keys = set(mode_cfg.get("remove", []))

    return (base_keys | add_keys) - remove_keys


def strip_enclosing_quotes(val: str) -> str:
    """Safely strip outer enclosing matching quotes without mangling internal quotes."""
    val = val.strip()
    if len(val) >= 2 and ((val.startswith('"') and val.endswith('"')) or (val.startswith("'") and val.endswith("'"))):
        return val[1:-1]
    return val


def parse_env_file(filepath: Path) -> dict[str, str]:
    """Parse key-value pairs from an env file, ignoring comments and blanks."""
    env: dict[str, str] = {}
    content = filepath.read_text(encoding="utf-8")
    for line in content.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, raw_val = line.split("=", 1)
        key = key.strip()
        val = strip_enclosing_quotes(raw_val)
        env[key] = val
    return env


def verify_env(filepath_str: str, mode: str) -> int:
    path = Path(filepath_str)

    # 1. Enforce local .env isolation
    if path.name in (".env", ".env.local"):
        print(
            f"❌ ERROR: scripts/deployment_checks/verify_deploy.py must not be run on {path.name}\n"
            f"   '{path.name}' is strictly for local workstation development and is ignored for deployments.\n"
            "   Use .env.prod, .env.staging, or .env.prod.low-pricing instead.",
            file=sys.stderr,
        )
        return 1

    if not path.exists():
        print(f"❌ ERROR: Environment file not found: {path}", file=sys.stderr)
        return 1

    # 2. Load externalized configuration
    config = load_config()
    expected_keys = load_mode_keys(config, mode)

    parsed = parse_env_file(path)
    found_keys = set(parsed.keys())

    # 3. Check for missing or empty keys
    missing_keys = []
    for k in expected_keys:
        if k not in parsed or not parsed[k]:
            missing_keys.append(k)

    # 4. Check for unregistered new keys
    unregistered_keys = sorted(list(found_keys - expected_keys))

    # 5. Check mode-specific required values (externalized from config)
    value_errors = []
    required_values_cfg = config.get("required_values", {}).get(mode, {})
    for req_k, req_v in required_values_cfg.items():
        actual_val = parsed.get(req_k)
        if actual_val != req_v:
            value_errors.append(
                f"{req_k} must be '{req_v}' in {path.name} (found '{actual_val}')"
            )

    has_error = False

    if missing_keys:
        has_error = True
        print(
            f"\n❌ ERROR: Missing or empty required environment variable(s) in {path}:",
            file=sys.stderr,
        )
        for k in sorted(missing_keys):
            print(f"   - {k}", file=sys.stderr)
        print(
            "\nACTION REQUIRED:\n"
            "This key is required for deployment but is missing or empty.\n"
            "1. If this is a required variable: add it to GitHub repository variables (or update the file and upload via 'make upload-secrets').\n"
            f"2. If this variable is deprecated or removed from code: remove it from 'scripts/deployment_checks/{CONFIG_FILE_NAME}'.\n"
            "3. Refer to docs/deployment/env-verification.md for instructions.",
            file=sys.stderr,
        )

    if unregistered_keys:
        has_error = True
        print(
            f"\n❌ ERROR: Unregistered new environment variable(s) found in {path}:",
            file=sys.stderr,
        )
        for k in unregistered_keys:
            print(f"   - {k}", file=sys.stderr)
        print(
            "\nACTION REQUIRED:\n"
            "A new environment variable was added. To keep deployments safe and prevent production incidents:\n"
            "1. If this is a required variable for deployment:\n"
            f"   - Add '{', '.join(unregistered_keys)}' to 'scripts/deployment_checks/{CONFIG_FILE_NAME}'.\n"
            "   - Ensure it is uploaded to GitHub repository variables ('make upload-secrets').\n"
            "2. If it was added by mistake or is unused, remove it from the file.\n"
            "3. Refer to docs/deployment/env-verification.md for instructions.",
            file=sys.stderr,
        )

    if value_errors:
        has_error = True
        print(f"\n❌ ERROR: Value validation failed for {path}:", file=sys.stderr)
        for err in value_errors:
            print(f"   - {err}", file=sys.stderr)

    if has_error:
        return 1

    print(f"✅ Environment check passed: {path} ({len(found_keys)} keys verified for '{mode}' mode)")
    return 0


def main() -> None:
    parser = argparse.ArgumentParser(description="Verify deployment environment file keys.")
    parser.add_argument(
        "env_file",
        help="Path to the env file (.env.prod, .env.staging, .env.prod.low-pricing)",
    )
    parser.add_argument(
        "positional_mode",
        nargs="?",
        choices=["prod", "staging", "prod-low-pricing"],
        default=None,
        help="Optional positional deployment mode",
    )
    parser.add_argument(
        "--mode",
        choices=["prod", "staging", "prod-low-pricing"],
        default=None,
        help="Deployment mode to validate against (default: prod)",
    )
    args = parser.parse_args()
    mode = args.mode or args.positional_mode or "prod"

    sys.exit(verify_env(args.env_file, mode))


if __name__ == "__main__":
    main()
