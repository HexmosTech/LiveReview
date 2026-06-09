import os
import sys
import json
import time
import urllib.request
import urllib.error

def submit_review(api_url, api_key, payload_b64, index):
    url = f"{api_url}/api/v1/diff-review"
    headers = {
        "Content-Type": "application/json",
        "X-API-Key": api_key,
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    }
    data = json.dumps({
        "diff_zip_base64": payload_b64,
        "repo_name": f"load-test-repo-{index}"
    }).encode("utf-8")

    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req) as resp:
            body = resp.read().decode("utf-8")
            res_json = json.loads(body)
            review_id = res_json.get("review_id")
            return {"status": "success", "review_id": review_id, "index": index}
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8")
        return {"status": "error", "code": e.code, "body": body, "index": index}
    except Exception as e:
        return {"status": "error", "error": str(e), "index": index}

def check_status(api_url, api_key, review_id):
    url = f"{api_url}/api/v1/diff-review/{review_id}"
    headers = {
        "X-API-Key": api_key,
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    }
    req = urllib.request.Request(url, headers=headers, method="GET")
    start = time.time()
    try:
        with urllib.request.urlopen(req) as resp:
            body = resp.read().decode("utf-8")
            latency = time.time() - start
            res_json = json.loads(body)
            return res_json.get("status"), latency
    except Exception as e:
        latency = time.time() - start
        return f"error: {e}", latency

from datetime import datetime

def parse_iso_time(t_str):
    if not t_str:
        return None
    t_str = t_str.replace('Z', '+00:00')
    try:
        return datetime.fromisoformat(t_str)
    except Exception:
        return None

def fetch_and_save_logs(api_url, api_key, review_id, logs_dir):
    url = f"{api_url}/api/v1/diff-review/{review_id}/events?limit=1000"
    headers = {
        "X-API-Key": api_key,
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    }
    req = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req) as resp:
            body = resp.read().decode("utf-8")
            res_json = json.loads(body)
            events = res_json.get("events") or []
            
            start_time = None
            first_comment_time = None
            
            # Format logs
            logs_lines = []
            for i, event in enumerate(events):
                event_type = event.get("type", "")
                event_level = event.get("level") or "info"
                event_time_str = event.get("time", "")
                batch_id = event.get("batchId")
                data = event.get("data") or {}

                if isinstance(data, str):
                    try:
                        data = json.loads(data)
                    except Exception:
                        pass

                if event_time_str:
                    event_time = parse_iso_time(event_time_str)
                    if event_time:
                        if start_time is None:
                            start_time = event_time
                        
                        has_comments = False
                        if event_type == "batch":
                            status = data.get("status", "")
                            if status == "completed":
                                comment_count = data.get("commentCount")
                                if comment_count is None:
                                    # Fallback: check comments list if present
                                    comment_count = len(data.get("comments") or [])
                                if comment_count and comment_count > 0:
                                    has_comments = True
                        elif event_type == "completion":
                            comment_count = data.get("commentCount", 0)
                            if comment_count and comment_count > 0:
                                has_comments = True
                        
                        if has_comments and first_comment_time is None:
                            first_comment_time = event_time

                message = ""
                if event_type == "log":
                    message = data.get("message") or ""
                elif event_type == "batch":
                    status = data.get("status", "")
                    if status == "processing":
                        file_count = data.get("fileCount", 0)
                        message = f"Batch {batch_id or 'unknown'} started: processing {file_count} file{'s' if file_count != 1 else ''}"
                    elif status == "completed":
                        comment_count = data.get("commentCount")
                        if comment_count is None:
                            comment_count = data.get("fileCount")
                        if comment_count is None:
                            comment_count = 0
                        message = f"Batch {batch_id or 'unknown'} completed: generated {comment_count} comment{'s' if comment_count != 1 else ''}"
                    else:
                        message = f"Batch {batch_id or 'unknown'}: {status or 'unknown status'}"
                elif event_type == "status":
                    message = f"Status: {data.get('status') or 'unknown'}"
                elif event_type == "artifact":
                    message = f"Generated: {data.get('kind') or 'artifact'}" if data.get("url") else f"Artifact: {data.get('kind') or 'unknown'}"
                elif event_type == "completion":
                    comment_count = data.get("commentCount", 0)
                    message = data.get("resultSummary") or f"Process completed with {comment_count} comment{'s' if comment_count != 1 else ''}"
                else:
                    message = data.get("message") or json.dumps(data)

                details = ""
                details_dict = {}
                for k, v in data.items():
                    if k in ("message", "resultSummary") or v is None:
                        continue
                    details_dict[k] = v
                if batch_id:
                    details_dict["batchId"] = batch_id

                if details_dict:
                    details = "\n  Details: " + json.dumps(details_dict, indent=2).replace("\n", "\n  ")

                header = f"[{i + 1}] {event_time_str} - {event_type.upper()} - {event_level.upper()}"
                logs_lines.append(f"{header}\n  {message}{details}")

            log_content = "\n\n".join(logs_lines)
            
            # Write to log file
            log_file_path = os.path.join(logs_dir, f"{review_id}.log")
            with open(log_file_path, "w", encoding="utf-8") as f:
                f.write(log_content)
            print(f"  [✓] Wrote logs to {log_file_path}")
            
            if start_time and first_comment_time:
                return (first_comment_time - start_time).total_seconds()
            return None
    except Exception as e:
        print(f"  [-] Failed to fetch/save events for review {review_id}: {e}", file=sys.stderr)
        return None
