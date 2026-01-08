"""Smoke-tested Gitea webhook helper snippets.

This file documents the exact API calls and live responses that worked against
https://gitea.hexmos.site using the LiveReview bot token. Use this as the
single source of truth when porting to Go.

Endpoints used (Authorization: token <PAT>):
- GET  /api/v1/user                 -> 200
- GET  /api/v1/user/repos           -> 200
- GET  /api/v1/repos/<owner>/<repo>/hooks -> 200
- POST /api/v1/repos/<owner>/<repo>/hooks -> 201
- DELETE /api/v1/repos/<owner>/<repo>/hooks/<id> -> 204

Webhook creation payload that succeeded:
{
    "type": "gitea",
    "config": {
        "url": "https://example.com/webhook-test",
        "content_type": "json",
        "secret": "abc123"
    },
    "events": ["pull_request", "issue_comment"],
    "active": true
}

Live response samples (2026-01-08):
- GET /user (200):
  {"id":2,"login":"livereview","login_name":"","source_id":0,"full_name":"","email":"livereveiewbot@gmail.com","avatar_url":"https://gitea.hexmos.site/avatars/dd58054c086f0375c2440173ba52a163","html_url":"https://gitea.hexmos.site/livereview","language":"en-US","is_admin":false,"last_login":"2026-01-05T10:52:59Z","created":"2026-01-04T05:32:59Z","restricted":false,"active":true,"prohibit_login":false,"location":"","website":"","description":"","visibility":"private","followers_count":0,"following_count":0,"starred_repos_count":0,"username":"livereview"}

- GET /user/repos (200):
  single repo example: {"id":2,"owner":{"id":3,"login":"megaorg",...},"full_name":"megaorg/livereview","permissions":{"admin":true,"push":true,"pull":true},"has_pull_requests":true,"has_issues":true,...}

- POST /repos/megaorg/livereview/hooks (201):
  {"id":2,"type":"gitea","branch_filter":"","config":{"content_type":"json","url":"https://example.com/webhook-test"},"events":["pull_request_comment","pull_request_review_request","pull_request_label","pull_request_milestone","pull_request","pull_request_assign","pull_request_review","pull_request_sync","issue_comment"],"authorization_header":"","active":true,"updated_at":"2026-01-08T06:10:58Z","created_at":"2026-01-08T06:10:58Z"}

- DELETE /repos/megaorg/livereview/hooks/2 (204): empty body

Notes:
- Events auto-expand when you send ["pull_request", "issue_comment"].
- Secret is accepted but not echoed in response.
- List hooks returns the same shape as the create response (array form).
"""

import requests
from datetime import datetime
from pathlib import Path

# Snapshot responses for quick reference during Go porting.
sample_user_resp = {
    "id": 2,
    "login": "livereview",
    "email": "livereveiewbot@gmail.com",
    "avatar_url": "https://gitea.hexmos.site/avatars/dd58054c086f0375c2440173ba52a163",
    "html_url": "https://gitea.hexmos.site/livereview",
    "language": "en-US",
    "is_admin": False,
    "last_login": "2026-01-05T10:52:59Z",
    "created": "2026-01-04T05:32:59Z",
}

sample_repo_resp = {
    "id": 2,
    "owner": {"id": 3, "login": "megaorg"},
    "full_name": "megaorg/livereview",
    "permissions": {"admin": True, "push": True, "pull": True},
    "has_pull_requests": True,
    "has_issues": True,
}

sample_hook_create_resp = {
    "id": 2,
    "type": "gitea",
    "config": {"content_type": "json", "url": "https://example.com/webhook-test"},
    "events": [
        "pull_request_comment",
        "pull_request_review_request",
        "pull_request_label",
        "pull_request_milestone",
        "pull_request",
        "pull_request_assign",
        "pull_request_review",
        "pull_request_sync",
        "issue_comment",
    ],
    "active": True,
}

sample_hook_list_resp = [sample_hook_create_resp]

base_url = "https://gitea.hexmos.site"
uname = "livereview"
pwd = "gitea@12345"
access_token = "77d844a025c45817b0e7cc0ccaa90451f98da8c6"

log_path = Path(__file__).with_suffix(".log")


def log_request(name: str, method: str, url: str, payload, status: int, text: str):
    timestamp = datetime.utcnow().isoformat()
    entry = (
        f"\n[{timestamp}] {name}\n"
        f"METHOD: {method}\nURL: {url}\n"
        f"PAYLOAD: {payload}\n"
        f"STATUS: {status}\n"
        f"RESPONSE: {text}\n"
    )
    with log_path.open("a", encoding="utf-8") as f:
        f.write(entry)


def auth_headers():
    return {"Authorization": f"token {access_token}"}


def get_user():
    url = f"{base_url}/api/v1/user"
    resp = requests.get(url, headers=auth_headers())
    print("GET /user", resp.status_code)
    print(resp.text)
    log_request("get_user", "GET", url, None, resp.status_code, resp.text)


def list_repos():
    url = f"{base_url}/api/v1/user/repos"
    resp = requests.get(url, headers=auth_headers())
    print("GET /user/repos", resp.status_code)
    print(resp.json())
    log_request("list_repos", "GET", url, None, resp.status_code, resp.text)


def list_hooks(repo_full_name: str):
    url = f"{base_url}/api/v1/repos/{repo_full_name}/hooks"
    resp = requests.get(url, headers=auth_headers())
    print("GET hooks", repo_full_name, resp.status_code)
    print(resp.json())
    log_request("list_hooks", "GET", url, None, resp.status_code, resp.text)


def create_hook(repo_full_name: str, target_url: str):
    payload = {
        "type": "gitea",
        "config": {
            "url": target_url,
            "content_type": "json",
            "secret": "abc123",
        },
        "events": ["pull_request", "issue_comment"],
        "active": True,
    }
    url = f"{base_url}/api/v1/repos/{repo_full_name}/hooks"
    resp = requests.post(url, headers=auth_headers(), json=payload)
    print("POST hook", repo_full_name, resp.status_code)
    print(resp.text)
    log_request("create_hook", "POST", url, payload, resp.status_code, resp.text)
    return resp.json() if resp.status_code == 201 else None


def delete_hook(repo_full_name: str, hook_id: int):
    url = f"{base_url}/api/v1/repos/{repo_full_name}/hooks/{hook_id}"
    resp = requests.delete(url, headers=auth_headers())
    print("DELETE hook", repo_full_name, hook_id, resp.status_code)
    print(resp.text)
    log_request("delete_hook", "DELETE", url, None, resp.status_code, resp.text)


if __name__ == "__main__":
    repo = "megaorg/livereview"

    print("-- user --")
    get_user()

    print("-- repos --")
    list_repos()

    print("-- hooks before --")
    list_hooks(repo)

    print("-- create hook --")
    created = create_hook(repo, "https://example.com/webhook-test")

    print("-- hooks after create --")
    list_hooks(repo)

    if created and "id" in created:
        print("-- delete hook --")
        delete_hook(repo, created["id"])

    print("-- hooks after delete --")
    list_hooks(repo)
