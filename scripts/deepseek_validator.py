#!/usr/bin/env python3
"""Minimal DeepSeek API key validator.

This script always prompts for a DeepSeek API key from the terminal, sends a
small "hello world" style request to DeepSeek V3, and reports whether the key
appears valid.
"""

from __future__ import annotations

import getpass
import json
import sys
import urllib.error
import urllib.request


DEEPSEEK_API_URL = "https://api.deepseek.com/chat/completions"
# DeepSeek V3 is exposed via this model identifier.
MODEL = "deepseek-chat"


def request_api_key() -> str:
	"""Get API key from terminal without echoing it."""
	key = getpass.getpass("Enter your DeepSeek API key: ").strip()
	if not key:
		print("ERROR: API key cannot be empty.", file=sys.stderr)
		sys.exit(2)
	return key


def validate_api_key(api_key: str) -> tuple[bool, str]:
	"""Send a tiny request and return (is_valid, message)."""
	payload = {
		"model": MODEL,
		"messages": [
			{"role": "user", "content": "Reply with exactly: hello world"},
		],
		"max_tokens": 16,
		"temperature": 0,
	}

	data = json.dumps(payload).encode("utf-8")
	req = urllib.request.Request(
		DEEPSEEK_API_URL,
		data=data,
		headers={
			"Content-Type": "application/json",
			"Authorization": f"Bearer {api_key}",
		},
		method="POST",
	)

	try:
		with urllib.request.urlopen(req, timeout=20) as response:
			body = response.read().decode("utf-8", errors="replace")
			parsed = json.loads(body)
			choices = parsed.get("choices", [])
			if choices and isinstance(choices[0], dict):
				message = choices[0].get("message", {})
				if isinstance(message, dict):
					reply = str(message.get("content", "")).strip()
					if reply:
						return True, reply
			return True, "<empty response text>"
	except urllib.error.HTTPError as exc:
		raw = exc.read().decode("utf-8", errors="replace")
		try:
			err_obj = json.loads(raw)
		except json.JSONDecodeError:
			err_obj = {}

		err_msg = (
			err_obj.get("error", {}).get("message")
			if isinstance(err_obj.get("error"), dict)
			else None
		)
		if not err_msg:
			err_msg = raw or str(exc)

		if exc.code in (401, 403):
			return False, f"Authentication failed ({exc.code}): {err_msg}"
		return False, f"API request failed ({exc.code}): {err_msg}"
	except urllib.error.URLError as exc:
		return False, f"Network error: {exc.reason}"
	except TimeoutError:
		return False, "Request timed out."
	except json.JSONDecodeError:
		return False, "Received non-JSON response from API."


def main() -> int:
	print("DeepSeek API key validator (DeepSeek V3 via deepseek-chat)")
	api_key = request_api_key()
	ok, detail = validate_api_key(api_key)

	if ok:
		print("VALID API KEY")
		print(f"Model reply: {detail}")
		return 0

	print("INVALID API KEY")
	print(detail, file=sys.stderr)
	return 1


if __name__ == "__main__":
	raise SystemExit(main())
