#!/usr/bin/env python3
"""
livereviewbot.py

Sections:
1) Input (GitLab base URL, PAT, project URL/path)
2) Install Webhook
3) Listen to webhook event and print it
"""
import os
import sys
import json
import urllib.parse

from flask import Flask, request, abort
import httpx

# ---------------------------------------------------------------------
# 1) INPUT  (hardcode here)
# ---------------------------------------------------------------------
GITLAB_BASE_URL   = "https://git.apps.hexmos.com"
# GITLAB_PAT        = "REDACTED_GITLAB_PAT_3"     # scope: api
GITLAB_PAT = "REDACTED_GITLAB_PAT_5"
GITLAB_PROJECT    = "hexmos/liveapi"        # namespace/path or numeric id
WEBHOOK_SECRET    = "super-secret-string"        # used to verify incoming webhooks
PUBLIC_ENDPOINT   = "https://talented-manually-turkey.ngrok-free.app/api/gitlab-hook"  # where GitLab can reach you

# Event types you want GitLab to send (adjust as needed)
WEBHOOK_EVENTS = {
    "push_events": False,
    "issues_events": False,
    "merge_requests_events": True,
    "tag_push_events": False,
    "note_events": True,
    "job_events": False,
    "pipeline_events": False
}

# ---------------------------------------------------------------------
# 2) INSTALL WEBHOOK
# ---------------------------------------------------------------------
def gitlab_client():
    return httpx.Client(
        base_url=GITLAB_BASE_URL,
        headers={"PRIVATE-TOKEN": GITLAB_PAT},
        timeout=30.0
    )

def get_project_id(client, project):
    """
    Resolve path -> numeric id so the rest of the calls are unambiguous.
    """
    if str(project).isdigit():
        return int(project)
    encoded = urllib.parse.quote_plus(project)
    r = client.get(f"/api/v4/projects/{encoded}")
    r.raise_for_status()
    return r.json()["id"]

def webhook_exists(client, project_id, url):
    r = client.get(f"/api/v4/projects/{project_id}/hooks")
    r.raise_for_status()
    for h in r.json():
        if h["url"] == url:
            return h["id"]
    return None

def check_permissions():
    """Check if current user has sufficient permissions for webhook management"""
    with gitlab_client() as c:
        try:
            pid = get_project_id(c, GITLAB_PROJECT)
            
            # Get project details to check permissions
            r = c.get(f"/api/v4/projects/{pid}")
            r.raise_for_status()
            project = r.json()
            
            # Get user info
            user_r = c.get("/api/v4/user")
            user_r.raise_for_status()
            user = user_r.json()
            
            print(f"User: {user.get('username')}")
            print(f"Project: {project.get('name_with_namespace')}")
            
            # Check permissions
            permissions = project.get('permissions', {})
            project_access = permissions.get('project_access')
            group_access = permissions.get('group_access')
            
            access_level = 0
            access_source = "none"
            
            if project_access and project_access.get('access_level'):
                access_level = max(access_level, project_access['access_level'])
                access_source = "project"
            
            if group_access and group_access.get('access_level'):
                if group_access['access_level'] > access_level:
                    access_level = group_access['access_level']
                    access_source = "group"
            
            access_names = {
                10: "Guest",
                20: "Reporter", 
                30: "Developer",
                40: "Maintainer",
                50: "Owner"
            }
            
            access_name = access_names.get(access_level, f"Unknown({access_level})")
            print(f"Your access level: {access_level} ({access_name}) via {access_source}")
            
            if access_level >= 40:
                print("✅ You have sufficient permissions for webhook management")
                return True
            else:
                print("❌ You need Maintainer (40) or Owner (50) permissions to manage webhooks")
                print("   Current Developer (30) permissions only allow reading webhooks")
                return False
                
        except Exception as e:
            print(f"Error checking permissions: {e}")
            return False

def install_webhook():
    """
    Creates (or idempotently reuses) a project webhook that sends MR + note events
    to our listener, guarded by WEBHOOK_SECRET.
    """
    # Check permissions first
    if not check_permissions():
        print("\n💡 Solution: Ask a project Maintainer or Owner to:")
        print("   1. Run this script for you, OR")
        print("   2. Grant you Maintainer permissions on the project")
        return
        
    with gitlab_client() as c:
        pid = get_project_id(c, GITLAB_PROJECT)
        existing_id = webhook_exists(c, pid, PUBLIC_ENDPOINT)
        payload = {
            "url": PUBLIC_ENDPOINT,
            "token": WEBHOOK_SECRET,
            **WEBHOOK_EVENTS,
            "enable_ssl_verification": True,
        }

        if existing_id:
            # Update to keep it consistent with our current config
            r = c.put(f"/api/v4/projects/{pid}/hooks/{existing_id}", data=payload)
            r.raise_for_status()
            print(f"[webhook] Updated existing hook #{existing_id}")
        else:
            r = c.post(f"/api/v4/projects/{pid}/hooks", data=payload)
            r.raise_for_status()
            print(f"[webhook] Created hook #{r.json()['id']}")

# ---------------------------------------------------------------------
# 3) LISTEN TO WEBHOOK EVENT AND PRINT IT
# ---------------------------------------------------------------------
app = Flask(__name__)

@app.post("/api/gitlab-hook")
def gitlab_hook():
    # Reject anything that doesn't present our shared secret
    if request.headers.get("X-Gitlab-Token") != WEBHOOK_SECRET:
        abort(401)

    kind = request.headers.get("X-Gitlab-Event", "unknown")
    try:
        payload = request.get_json(force=True, silent=False)
    except Exception:
        abort(400)

    # Print *exactly* what came in, nicely formatted
    print("\n=== Incoming GitLab Webhook ===")
    print(f"Event: {kind}")
    print(json.dumps(payload, indent=2, sort_keys=True))
    print("=== End ===\n", flush=True)

    return {"ok": True}

def run_server():
    # Run the Flask dev server. In production, put this behind a real web server.
    app.run(host="0.0.0.0", port=8888, debug=False)

# ---------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------
if __name__ == "__main__":
    # Cheap CLI so you can choose to only register or only listen.
    #   python livereviewbot.py install-webhook
    #   python livereviewbot.py listen
    #   python livereviewbot.py install-webhook && python livereviewbot.py listen
    if len(sys.argv) < 2:
        print("Usage: python livereviewbot.py [install-webhook|listen|check-permissions]")
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "install-webhook":
        install_webhook()
    elif cmd == "listen":
        run_server()
    elif cmd == "check-permissions":
        check_permissions()
    else:
        print(f"Unknown command: {cmd}")
        sys.exit(1)
