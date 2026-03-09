#!/usr/bin/env python3
"""Simple OpenAI API key validator.

Usage examples:
  python scripts/openai_validator.py
  python scripts/openai_validator.py --model o4-mini
  OPENAI_API_KEY=sk-... python scripts/openai_validator.py --verbose
"""

from __future__ import annotations

import argparse
import getpass
import json
import os
import socket
import sys
import traceback
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any, Optional


DEFAULT_MODEL = "o4-mini"
DEFAULT_BASE_URL = "https://api.openai.com/v1"


@dataclass
class ValidationResult:
	ok: bool
	message: str
	request_id: Optional[str] = None
	status_code: Optional[int] = None
	response_body: Optional[dict[str, Any]] = None


def _masked_key(api_key: str) -> str:
	if len(api_key) <= 8:
		return "*" * len(api_key)
	return f"{api_key[:4]}...{api_key[-4:]}"


def _extract_output_text(payload: dict[str, Any]) -> str:
	# Newer responses API often includes this directly.
	output_text = payload.get("output_text")
	if isinstance(output_text, str) and output_text.strip():
		return output_text.strip()

	# Fallback traversal for nested output content.
	output = payload.get("output")
	if isinstance(output, list):
		texts: list[str] = []
		for item in output:
			if not isinstance(item, dict):
				continue

			# Some responses include item-level text fields.
			item_text = item.get("text")
			if isinstance(item_text, str) and item_text.strip():
				texts.append(item_text)

			content = item.get("content")
			if not isinstance(content, list):
				continue
			for chunk in content:
				if not isinstance(chunk, dict):
					continue
				chunk_type = chunk.get("type")

				# Common format: {"type":"output_text","text":"..."}
				if chunk_type in {"output_text", "text"} and isinstance(chunk.get("text"), str):
					texts.append(chunk["text"])

				# Alternative format: {"type":"output_text","text":{"value":"..."}}
				text_obj = chunk.get("text")
				if isinstance(text_obj, dict):
					value = text_obj.get("value")
					if isinstance(value, str) and value.strip():
						texts.append(value)

				# Another variant: {"type":"message","content":[{"type":"text","text":"..."}]}
				nested = chunk.get("content")
				if isinstance(nested, list):
					for nested_chunk in nested:
						if not isinstance(nested_chunk, dict):
							continue
						nested_text = nested_chunk.get("text")
						if isinstance(nested_text, str) and nested_text.strip():
							texts.append(nested_text)
						elif isinstance(nested_text, dict):
							value = nested_text.get("value")
							if isinstance(value, str) and value.strip():
								texts.append(value)
		if texts:
			return "\n".join(texts).strip()

	return ""


def _response_fallback_preview(payload: dict[str, Any]) -> str:
	"""Return compact, human-readable details when text extraction fails."""
	parts: list[str] = []
	for key in ("id", "model", "object", "status"):
		value = payload.get(key)
		if value is not None:
			parts.append(f"{key}={value}")

	usage = payload.get("usage")
	if isinstance(usage, dict):
		input_toks = usage.get("input_tokens")
		output_toks = usage.get("output_tokens")
		if input_toks is not None or output_toks is not None:
			parts.append(f"tokens(in={input_toks}, out={output_toks})")

	if not parts:
		keys_preview = ", ".join(sorted(payload.keys())[:8])
		if keys_preview:
			parts.append(f"top-level keys: {keys_preview}")

	raw = json.dumps(payload, sort_keys=True)
	return f"{'; '.join(parts)} | raw: {raw[:400]}"


def _friendly_error_help(status_code: int, error_payload: Optional[dict[str, Any]]) -> str:
	if status_code == 401:
		return "Authentication failed. Verify the API key is correct and active."
	if status_code == 403:
		return "Access forbidden. The key may lack permission for this project or model."
	if status_code == 404:
		return "Endpoint or model not found. Check base URL and model name."
	if status_code == 429:
		return "Rate limit or quota exceeded. Check billing, quota, and retry later."
	if 500 <= status_code <= 599:
		return "OpenAI service error. Retry shortly."

	if error_payload and isinstance(error_payload.get("error"), dict):
		msg = error_payload["error"].get("message")
		if isinstance(msg, str) and msg:
			return msg

	return "Request failed. Inspect error details below."


def validate_openai_key(api_key: str, model: str, base_url: str, timeout: float) -> ValidationResult:
	url = f"{base_url.rstrip('/')}/responses"
	body = {
		"model": model,
		"input": "Say hello world exactly.",
		"max_output_tokens": 20,
	}
	payload = json.dumps(body).encode("utf-8")

	request = urllib.request.Request(
		url=url,
		data=payload,
		method="POST",
		headers={
			"Authorization": f"Bearer {api_key}",
			"Content-Type": "application/json",
		},
	)

	try:
		with urllib.request.urlopen(request, timeout=timeout) as response:
			raw = response.read().decode("utf-8", errors="replace")
			parsed: dict[str, Any]
			try:
				parsed = json.loads(raw)
			except json.JSONDecodeError:
				parsed = {"raw": raw}

			text = _extract_output_text(parsed)
			if text:
				msg = f"Success: API key is valid. Model replied: {text!r}"
			else:
				preview = _response_fallback_preview(parsed)
				msg = (
					"Success: API key is valid, but output text was not found in expected fields. "
					f"Response preview: {preview}"
				)

			return ValidationResult(
				ok=True,
				message=msg,
				request_id=response.headers.get("x-request-id"),
				status_code=response.status,
				response_body=parsed,
			)

	except urllib.error.HTTPError as exc:
		raw_err = exc.read().decode("utf-8", errors="replace")
		parsed_err: Optional[dict[str, Any]] = None
		try:
			parsed = json.loads(raw_err)
			if isinstance(parsed, dict):
				parsed_err = parsed
		except json.JSONDecodeError:
			parsed_err = {"raw": raw_err}

		help_text = _friendly_error_help(exc.code, parsed_err)
		return ValidationResult(
			ok=False,
			message=f"HTTP {exc.code}: {help_text}",
			request_id=exc.headers.get("x-request-id") if exc.headers else None,
			status_code=exc.code,
			response_body=parsed_err,
		)

	except urllib.error.URLError as exc:
		reason = exc.reason
		if isinstance(reason, socket.timeout):
			detail = "Request timed out. Try a larger --timeout or check network connectivity."
		else:
			detail = f"Network error: {reason}"
		return ValidationResult(ok=False, message=detail)

	except Exception as exc:  # Defensive catch to print full diagnostic context.
		return ValidationResult(
			ok=False,
			message=f"Unexpected error: {exc.__class__.__name__}: {exc}",
			response_body={"traceback": traceback.format_exc()},
		)


def _parse_args() -> argparse.Namespace:
	parser = argparse.ArgumentParser(description="Validate an OpenAI API key with a hello-world request.")
	parser.add_argument("--model", default=DEFAULT_MODEL, help=f"Model to test (default: {DEFAULT_MODEL})")
	parser.add_argument("--base-url", default=DEFAULT_BASE_URL, help="OpenAI API base URL")
	parser.add_argument("--timeout", type=float, default=30.0, help="HTTP timeout in seconds")
	parser.add_argument(
		"--verbose",
		action="store_true",
		help="Print full response/error JSON for debugging",
	)
	return parser.parse_args()


def _read_api_key() -> str:
	env_key = os.getenv("OPENAI_API_KEY", "").strip()
	if env_key:
		return env_key

	print("OPENAI_API_KEY is not set.")
	print("Paste your OpenAI API key (input is hidden):")
	return getpass.getpass("API key: ").strip()


def main() -> int:
	args = _parse_args()
	api_key = _read_api_key()

	if not api_key:
		print("ERROR: No API key provided.", file=sys.stderr)
		return 2

	print(f"Testing OpenAI key {_masked_key(api_key)} with model '{args.model}'...")
	result = validate_openai_key(api_key, args.model, args.base_url, args.timeout)

	print(result.message)
	if result.status_code is not None:
		print(f"Status code: {result.status_code}")
	if result.request_id:
		print(f"Request ID: {result.request_id}")

	if args.verbose and result.response_body is not None:
		print("Details:")
		print(json.dumps(result.response_body, indent=2, sort_keys=True))

	return 0 if result.ok else 1


if __name__ == "__main__":
	raise SystemExit(main())