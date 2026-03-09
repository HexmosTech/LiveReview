#!/usr/bin/env python3
"""Claude API key validator.

This script prompts for an Anthropic API key from the terminal, shows a
curated list of Claude models, lets the user choose one, and sends a small
"hello world" style request.
"""

from __future__ import annotations

import getpass
import json
import sys
import urllib.error
import urllib.request


ANTHROPIC_API_URL = "https://api.anthropic.com/v1/messages"
ANTHROPIC_VERSION = "2023-06-01"
AVAILABLE_MODELS = [
	"claude-haiku-4-5-20251001",
	"claude-opus-4-1-20250805",
	"claude-opus-4-20250514",
	"claude-opus-4-5-20251101",
	"claude-opus-4-6",
	"claude-sonnet-4-20250514",
	"claude-sonnet-4-5-20250929",
	"claude-sonnet-4-6",
]
DEFAULT_MODEL_INDEX = 0


def request_api_key() -> str:
	"""Get API key from terminal without echoing it."""
	key = getpass.getpass("Enter your Anthropic API key: ").strip()
	if not key:
		print("ERROR: API key cannot be empty.", file=sys.stderr)
		sys.exit(2)
	return key


def choose_model(models: list[str]) -> str:
	"""Prompt user to select model by index or provide custom model ID."""
	if not models:
		return input("No model list available. Enter model ID manually: ").strip()

	# DEFAULT_MODEL_INDEX is zero-based; CLI choices are one-based.
	default_choice = DEFAULT_MODEL_INDEX + 1 if 0 <= DEFAULT_MODEL_INDEX < len(models) else 1

	print("\nAvailable models:")
	for idx, model in enumerate(models, start=1):
		default_mark = " (default)" if idx == default_choice else ""
		print(f"  {idx}. {model}{default_mark}")

	while True:
		raw_choice = input(
			"Select model by number, press Enter for default, or type model ID: "
		).strip()
		if not raw_choice:
			return models[default_choice - 1]
		if raw_choice.isdigit():
			selected = int(raw_choice)
			if 1 <= selected <= len(models):
				return models[selected - 1]
			print(f"Please enter a number between 1 and {len(models)}.")
			continue
		return raw_choice


def validate_api_key(api_key: str, model: str) -> tuple[str, str]:
	"""Send a tiny request and return (status, message)."""
	payload = {
		"model": model,
		"max_tokens": 16,
		"messages": [{"role": "user", "content": "Reply with exactly: hello world"}],
	}

	data = json.dumps(payload).encode("utf-8")
	req = urllib.request.Request(
		ANTHROPIC_API_URL,
		data=data,
		headers={
			"Content-Type": "application/json",
			"x-api-key": api_key,
			"anthropic-version": ANTHROPIC_VERSION,
		},
		method="POST",
	)

	try:
		with urllib.request.urlopen(req, timeout=20) as response:
			body = response.read().decode("utf-8")
			parsed = json.loads(body)
			text_parts = [
				item.get("text", "")
				for item in parsed.get("content", [])
				if isinstance(item, dict) and item.get("type") == "text"
			]
			model_reply = " ".join(part.strip() for part in text_parts if part.strip())
			if not model_reply:
				model_reply = "<empty response text>"
			return "valid", model_reply
	except urllib.error.HTTPError as exc:
		raw = exc.read().decode("utf-8", errors="replace")
		try:
			err_obj = json.loads(raw)
			err_msg = err_obj.get("error", {}).get("message", raw)
		except json.JSONDecodeError:
			err_msg = raw or str(exc)

		if exc.code in (401, 403):
			return "invalid", f"Authentication failed ({exc.code}): {err_msg}"

		# 404 with a model message usually means the key is accepted but
		# the configured model is unavailable or misspelled for this account.
		err_msg_lower = str(err_msg).lower()
		if exc.code == 404 and "model" in err_msg_lower:
			return "model_unavailable", f"Model unavailable ({exc.code}): {err_msg}"

		return "error", f"API request failed ({exc.code}): {err_msg}"
	except urllib.error.URLError as exc:
		return "error", f"Network error: {exc.reason}"
	except TimeoutError:
		return "error", "Request timed out."
	except json.JSONDecodeError:
		return "error", "Received non-JSON response from API."


def main() -> int:
	print("Claude API key validator")

	api_key = request_api_key()
	model = choose_model(AVAILABLE_MODELS)
	if not model:
		print("INVALID INPUT")
		print("Model cannot be empty.", file=sys.stderr)
		return 2

	status, detail = validate_api_key(api_key, model)

	if status == "valid":
		print("VALID API KEY")
		print(f"Model used: {model}")
		print(f"Model reply: {detail}")
		return 0

	if status == "model_unavailable":
		print("API KEY ACCEPTED, BUT SELECTED MODEL IS UNAVAILABLE")
		print(detail, file=sys.stderr)
		return 3

	print("INVALID API KEY")
	print(detail, file=sys.stderr)
	return 1


if __name__ == "__main__":
	raise SystemExit(main())
