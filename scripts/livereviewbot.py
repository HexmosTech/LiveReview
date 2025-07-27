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

def process_livereviewbot_mention(payload, kind):
    """
    Check if this is a note event mentioning @livereviewbot and extract relevant info
    """
    print(f"\n🔍 [STEP 1] Processing mention detection...")
    print(f"    📋 Event kind received: '{kind}'")
    print(f"    🎯 Expected kind: 'note hooks' (case-insensitive)")
    
    if kind.lower() != "note hook":
        print(f"    ❌ Event kind mismatch: '{kind.lower()}' != 'note hooks'")
        print(f"    🚫 Skipping mention processing")
        return None
    print(f"    ✅ Event kind matches!")
    
    print(f"\n🔍 [STEP 2] Checking event_type in payload...")
    event_type = payload.get("event_type")
    print(f"    📋 Event type found: '{event_type}'")
    print(f"    🎯 Expected: 'note'")
    
    if event_type != "note":
        print(f"    ❌ Event type mismatch: '{event_type}' != 'note'")
        print(f"    🚫 Skipping mention processing")
        return None
    print(f"    ✅ Event type matches!")
    
    print(f"\n🔍 [STEP 3] Extracting note content...")
    object_attributes = payload.get("object_attributes", {})
    note_text = object_attributes.get("note", "")
    description = object_attributes.get("description", "")
    print(f"    📝 Note text: '{note_text}'")
    print(f"    📝 Description: '{description}'")
    
    print(f"\n🔍 [STEP 4] Checking for @livereviewbot mention...")
    text_to_check = f"{note_text} {description}".lower()
    print(f"    🔤 Combined text (lowercase): '{text_to_check}'")
    print(f"    🎯 Looking for: '@livereviewbot'")
    
    if "@livereviewbot" not in text_to_check:
        print(f"    ❌ No @livereviewbot mention found")
        print(f"    🚫 Skipping mention processing")
        return None
    print(f"    ✅ @livereviewbot mention found!")
    
    print(f"\n🔍 [STEP 5] Extracting user information...")
    user = payload.get("user", {})
    user_id = user.get("id")
    username = user.get("username")
    user_name = user.get("name")
    user_email = user.get("email")
    print(f"    👤 User ID: {user_id}")
    print(f"    👤 Username: {username}")
    print(f"    👤 Name: {user_name}")
    print(f"    👤 Email: {user_email}")
    
    print(f"\n🔍 [STEP 6] Extracting project information...")
    project = payload.get("project", {})
    project_id = project.get("id")
    project_name = project.get("name")
    project_path = project.get("path_with_namespace")
    project_url = project.get("web_url")
    print(f"    🏗️  Project ID: {project_id}")
    print(f"    🏗️  Project name: {project_name}")
    print(f"    🏗️  Project path: {project_path}")
    print(f"    🏗️  Project URL: {project_url}")
    
    print(f"\n🔍 [STEP 7] Building mention_info object...")
    mention_info = {
        "event_type": "livereviewbot_mention",
        "note_id": object_attributes.get("id"),
        "note_url": object_attributes.get("url"),
        "note_text": note_text,
        "note_description": description,
        "created_at": object_attributes.get("created_at"),
        "author": {
            "id": user_id,
            "username": username,
            "name": user_name,
            "email": user_email
        },
        "project": {
            "id": project_id,
            "name": project_name,
            "path_with_namespace": project_path,
            "web_url": project_url
        }
    }
    print(f"    📦 Basic mention_info created")
    print(f"    🆔 Note ID: {mention_info['note_id']}")
    print(f"    🔗 Note URL: {mention_info['note_url']}")
    print(f"    📅 Created at: {mention_info['created_at']}")
    
    print(f"\n🔍 [STEP 8] Checking for merge request information...")
    merge_request = payload.get("merge_request", {})
    noteable_type = object_attributes.get("noteable_type")
    print(f"    📋 Noteable type: '{noteable_type}'")
    print(f"    🎯 Expected for MR: 'MergeRequest'")
    print(f"    📦 Merge request exists: {bool(merge_request)}")
    
    if merge_request and noteable_type == "MergeRequest":
        print(f"    ✅ This is a merge request comment!")
        mr_id = merge_request.get("id")
        mr_iid = merge_request.get("iid")
        mr_title = merge_request.get("title")
        mr_state = merge_request.get("state")
        source_branch = merge_request.get("source_branch")
        target_branch = merge_request.get("target_branch")
        mr_url = merge_request.get("url")
        
        print(f"    🔀 MR ID: {mr_id}")
        print(f"    🔀 MR IID: #{mr_iid}")
        print(f"    🔀 MR Title: '{mr_title}'")
        print(f"    🔀 MR State: {mr_state}")
        print(f"    🔀 Source Branch: {source_branch}")
        print(f"    🔀 Target Branch: {target_branch}")
        print(f"    🔀 MR URL: {mr_url}")
        
        mention_info["merge_request"] = {
            "id": mr_id,
            "iid": mr_iid,
            "title": mr_title,
            "description": merge_request.get("description"),
            "state": mr_state,
            "source_branch": source_branch,
            "target_branch": target_branch,
            "url": mr_url,
            "author_id": merge_request.get("author_id"),
            "assignee_ids": merge_request.get("assignee_ids", []),
            "reviewer_ids": merge_request.get("reviewer_ids", []),
            "last_commit": merge_request.get("last_commit", {})
        }
        print(f"    📦 Merge request info added to mention_info")
    else:
        if not merge_request:
            print(f"    ⚠️  No merge request data found")
        if noteable_type != "MergeRequest":
            print(f"    ⚠️  Not a merge request comment (type: {noteable_type})")
        print(f"    📝 This appears to be a general project comment")
    
    print(f"\n✅ [STEP 9] Mention processing completed successfully!")
    print(f"    📊 Final mention_info keys: {list(mention_info.keys())}")
    return mention_info

@app.post("/api/gitlab-hook")
def gitlab_hook():
    print(f"\n🌐 === Webhook Request Received ===")
    
    # Reject anything that doesn't present our shared secret
    token = request.headers.get("X-Gitlab-Token")
    print(f"🔐 Token validation...")
    print(f"    📋 Received token: {'[PRESENT]' if token else '[MISSING]'}")
    print(f"    🎯 Expected token: {'[CONFIGURED]' if WEBHOOK_SECRET else '[NOT CONFIGURED]'}")
    
    if token != WEBHOOK_SECRET:
        print(f"    ❌ Token validation failed!")
        print(f"    🚫 Aborting with 401 Unauthorized")
        abort(401)
    print(f"    ✅ Token validation successful!")

    kind = request.headers.get("X-Gitlab-Event", "unknown")
    print(f"\n📡 Event processing...")
    print(f"    📋 Event type: '{kind}'")
    
    try:
        payload = request.get_json(force=True, silent=False)
        print(f"    ✅ JSON payload parsed successfully")
        print(f"    📊 Payload keys: {list(payload.keys()) if payload else 'None'}")
    except Exception as e:
        print(f"    ❌ JSON parsing failed: {e}")
        print(f"    🚫 Aborting with 400 Bad Request")
        abort(400)

    # Print *exactly* what came in, nicely formatted
    print("\n=== Incoming GitLab Webhook ===")
    print(f"Event: {kind}")
    print(json.dumps(payload, indent=2, sort_keys=True))
    print("=== End ===\n", flush=True)

    print(f"\n🤖 Starting LiveReviewBot mention detection...")
    # Check for @livereviewbot mentions first
    mention_info = process_livereviewbot_mention(payload, kind)
    if mention_info:
        print("\n🎉 === LiveReviewBot Mentioned! ===")
        print(f"Author: {mention_info['author']['name']} (@{mention_info['author']['username']})")
        print(f"Note: {mention_info['note_text']}")
        if mention_info.get('merge_request'):
            mr = mention_info['merge_request']
            print(f"MR: #{mr['iid']} - {mr['title']}")
            print(f"MR URL: {mr['url']}")
            print(f"Source Branch: {mr['source_branch']} -> {mr['target_branch']}")
        print(f"Note URL: {mention_info['note_url']}")
        print(f"Project: {mention_info['project']['path_with_namespace']}")
        print("=== End LiveReviewBot Mention ===\n", flush=True)
        
        # You can add your logic here to process the mention
        # For example: trigger a code review, respond to the comment, etc.
    else:
        print(f"    ℹ️  No @livereviewbot mention detected in this event")

    print(f"\n✅ Webhook processing completed successfully")
    return {"ok": True}

def test_mention_processing():
    """Test the mention processing with a sample payload"""
    sample_payload = {
        "event_type": "note",
        "object_attributes": {
            "id": 23126,
            "note": "@LiveReviewBot - hey livereview please review this",
            "description": "@LiveReviewBot - hey livereview please review this",
            "created_at": "2025-07-27 15:17:38 UTC",
            "url": "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/408#note_23126",
            "noteable_type": "MergeRequest"
        },
        "user": {
            "id": 3,
            "username": "shrijith",
            "name": "Shrijith",
            "email": "test@example.com"
        },
        "project": {
            "id": 170,
            "name": "LiveAPI",
            "path_with_namespace": "hexmos/liveapi",
            "web_url": "https://git.apps.hexmos.com/hexmos/liveapi"
        },
        "merge_request": {
            "id": 1798,
            "iid": 408,
            "title": "fixing the ui alligment issue",
            "description": "Fixing the Ui alligment issue in webview in copilot",
            "state": "merged",
            "source_branch": "swagath/meile-key-change",
            "target_branch": "main",
            "url": "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/408",
            "author_id": 68,
            "assignee_ids": [68],
            "reviewer_ids": [5]
        }
    }
    
    mention_info = process_livereviewbot_mention(sample_payload, "Note Hook")
    if mention_info:
        print("✅ Test passed! Mention detected:")
        print(json.dumps(mention_info, indent=2))
    else:
        print("❌ Test failed! Mention not detected")

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
        print("Usage: python livereviewbot.py [install-webhook|listen|check-permissions|test-mention]")
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "install-webhook":
        install_webhook()
    elif cmd == "listen":
        run_server()
    elif cmd == "check-permissions":
        check_permissions()
    elif cmd == "test-mention":
        test_mention_processing()
    else:
        print(f"Unknown command: {cmd}")
        sys.exit(1)
