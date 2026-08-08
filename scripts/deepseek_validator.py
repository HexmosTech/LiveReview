#!/usr/bin/env python3
"""Minimal DeepSeek API key validator.

This script always prompts for a DeepSeek API key from the terminal, sends a
small "hello world" style request to DeepSeek V3, and reports whether the key
appears valid.
"""

from __future__ import annotations

import getpass
import json
import requests
import sys


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


def _post_validation_request(api_key: str) -> requests.Response:
	"""Send the validation request. Raises on a transport-level failure."""
	payload = {
		"model": MODEL,
		"messages": [
			{"role": "user", "content": "Reply with exactly: hello world"},
		],
		"max_tokens": 16,
		"temperature": 0,
	}
	data = json.dumps(payload).encode("utf-8")
	return requests.post(
		DEEPSEEK_API_URL,
		data=data,
		headers={
			"Content-Type": "application/json",
			"Authorization": f"Bearer {api_key}",
		},
		timeout=20,
	)


def _interpret_response(response: requests.Response) -> tuple[bool, str]:
	"""Turn a completed HTTP response into (is_valid, message).

	Deliberately takes only the response, never the API key.
	"""
	if response.status_code >= 400:
		status_code = response.status_code
		raw = response.text
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
			err_msg = raw or f"HTTP {status_code}"

		if status_code in (401, 403):
			return False, f"Authentication failed ({status_code}): {err_msg}"
		return False, f"API request failed ({status_code}): {err_msg}"

	try:
		parsed = response.json()
	except json.JSONDecodeError:
		return False, "Received non-JSON response from API."

	choices = parsed.get("choices", [])
	if choices and isinstance(choices[0], dict):
		message = choices[0].get("message", {})
		if isinstance(message, dict):
			reply = str(message.get("content", "")).strip()
			if reply:
				return True, reply
	return True, "<empty response text>"


def validate_api_key(api_key: str) -> tuple[bool, str]:
	"""Send a tiny request and return (is_valid, message)."""
	try:
		response = _post_validation_request(api_key)
	except requests.Timeout:
		return False, "Request timed out."
	except requests.RequestException as exc:
		return False, f"Network error: {exc}"

	return _interpret_response(response)


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
