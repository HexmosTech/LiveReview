#!/usr/bin/env python3
"""
livereviewbot.py - Generic GitLab Bot Webhook Handler

A configurable bot that can monitor GitLab webhooks for:
1. Mentions in comments (configurable bot mention string)
2. Reviewer changes involving users with configurable name patterns

Sections:
1) Input (GitLab base URL, PAT, project URL/path, bot configuration)
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
# 1) INPUT  (configure here for your use case)
# ---------------------------------------------------------------------
GITLAB_BASE_URL   = "https://git.apps.hexmos.com"
# GITLAB_PAT        = "REDACTED_GITLAB_PAT_3"     # scope: api
GITLAB_PAT = "REDACTED_GITLAB_PAT_5"
GITLAB_PROJECT    = "hexmos/liveapi"        # namespace/path or numeric id
WEBHOOK_SECRET    = "super-secret-string"        # used to verify incoming webhooks
PUBLIC_ENDPOINT   = "https://talented-manually-turkey.ngrok-free.app/api/gitlab-hook"  # where GitLab can reach you

# Bot/Service name configuration - CUSTOMIZE THESE for your use case:
# - BOT_NAME: String to search for in usernames (case-insensitive)
# - BOT_MENTION: Exact mention string to detect in comments
BOT_NAME = "livereview"  # Used for detecting usernames containing this string
BOT_MENTION = "@livereviewbot"  # Used for detecting mentions in comments

# Examples for other bots:
# BOT_NAME = "codebot"
# BOT_MENTION = "@codebot"
#
# BOT_NAME = "reviewer"
# BOT_MENTION = "@reviewerbot"

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

def process_bot_mention(payload, kind):
    """
    Check if this is a note event mentioning the configured bot and extract relevant info
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
    
    print(f"\n🔍 [STEP 4] Checking for {BOT_MENTION} mention...")
    text_to_check = f"{note_text} {description}".lower()
    bot_mention_lower = BOT_MENTION.lower()
    print(f"    🔤 Combined text (lowercase): '{text_to_check}'")
    print(f"    🎯 Looking for: '{bot_mention_lower}'")
    
    if bot_mention_lower not in text_to_check:
        print(f"    ❌ No {BOT_MENTION} mention found")
        print(f"    🚫 Skipping mention processing")
        return None
    print(f"    ✅ {BOT_MENTION} mention found!")
    
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
        "event_type": f"{BOT_NAME}_mention",
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

def process_reviewer_change(payload, kind):
    """
    Check if this is a merge_request event with reviewer changes involving bot users
    """
    print(f"\n🔍 [REVIEWER CHANGE STEP 1] Processing reviewer change detection...")
    print(f"    📋 Event kind received: '{kind}'")
    print(f"    🎯 Expected kind: 'merge request hooks' (case-insensitive)")
    
    if kind.lower() != "merge request hook":
        print(f"    ❌ Event kind mismatch: '{kind.lower()}' != 'merge request hook'")
        print(f"    🚫 Skipping reviewer change processing")
        return None
    print(f"    ✅ Event kind matches!")
    
    print(f"\n🔍 [REVIEWER CHANGE STEP 2] Checking event_type in payload...")
    event_type = payload.get("event_type")
    print(f"    📋 Event type found: '{event_type}'")
    print(f"    🎯 Expected: 'merge_request'")
    
    if event_type != "merge_request":
        print(f"    ❌ Event type mismatch: '{event_type}' != 'merge_request'")
        print(f"    🚫 Skipping reviewer change processing")
        return None
    print(f"    ✅ Event type matches!")
    
    print(f"\n🔍 [REVIEWER CHANGE STEP 3] Checking for changes...")
    changes = payload.get("changes", {})
    reviewers_change = changes.get("reviewers")
    print(f"    📋 Changes found: {list(changes.keys()) if changes else 'None'}")
    print(f"    🎯 Looking for 'reviewers' change")
    print(f"    📦 Reviewers change exists: {bool(reviewers_change)}")
    
    if not reviewers_change:
        print(f"    ❌ No reviewers change found")
        print(f"    🚫 Skipping reviewer change processing")
        return None
    print(f"    ✅ Reviewers change found!")
    
    print(f"\n🔍 [REVIEWER CHANGE STEP 4] Analyzing reviewer changes...")
    current_reviewers = reviewers_change.get("current", [])
    previous_reviewers = reviewers_change.get("previous", [])
    print(f"    📋 Current reviewers count: {len(current_reviewers)}")
    print(f"    📋 Previous reviewers count: {len(previous_reviewers)}")
    
    # Check for bot users in both current and previous reviewers
    bot_found = False
    bot_users = []
    current_bot_reviewers = []
    previous_bot_reviewers = []
    
    bot_name_lower = BOT_NAME.lower()
    print(f"\n🔍 [REVIEWER CHANGE STEP 5] Checking for '{BOT_NAME}' usernames...")
    
    # Check previous reviewers
    print(f"    📋 Checking previous reviewers...")
    for i, reviewer in enumerate(previous_reviewers):
        username = reviewer.get("username", "")
        print(f"        👤 Previous reviewer {i+1}: '{username}'")
        if bot_name_lower in username.lower():
            print(f"        ✅ Found '{BOT_NAME}' in username: '{username}'")
            bot_found = True
            bot_users.append(("previous", reviewer))
            previous_bot_reviewers.append(reviewer)
        else:
            print(f"        ❌ No '{BOT_NAME}' in username: '{username}'")
    
    # Check current reviewers - THIS IS MOST IMPORTANT for triggering actions
    print(f"    📋 Checking current reviewers...")
    for i, reviewer in enumerate(current_reviewers):
        username = reviewer.get("username", "")
        print(f"        👤 Current reviewer {i+1}: '{username}'")
        if bot_name_lower in username.lower():
            print(f"        ✅ Found '{BOT_NAME}' in username: '{username}' - THIS WILL TRIGGER ACTIONS!")
            bot_found = True
            bot_users.append(("current", reviewer))
            current_bot_reviewers.append(reviewer)
        else:
            print(f"        ❌ No '{BOT_NAME}' in username: '{username}'")
    
    if not bot_found:
        print(f"    ❌ No '{BOT_NAME}' users found in reviewer changes")
        print(f"    🚫 Skipping reviewer change processing")
        return None
    
    # Log the important distinction
    print(f"    ✅ Found {len(bot_users)} '{BOT_NAME}' users in reviewer changes!")
    print(f"    🎯 CURRENT {BOT_NAME} reviewers (will trigger actions): {len(current_bot_reviewers)}")
    print(f"    📜 PREVIOUS {BOT_NAME} reviewers: {len(previous_bot_reviewers)}")
    
    # Determine if this is a bot assignment or removal
    is_bot_assigned = len(current_bot_reviewers) > 0
    is_bot_removed = len(previous_bot_reviewers) > 0 and len(current_bot_reviewers) == 0
    
    print(f"    🚀 {BOT_NAME.title()} assigned as reviewer: {is_bot_assigned}")
    print(f"    🗑️  {BOT_NAME.title()} removed as reviewer: {is_bot_removed}")
    
    print(f"\n🔍 [REVIEWER CHANGE STEP 6] Extracting user information...")
    user = payload.get("user", {})
    user_id = user.get("id")
    username = user.get("username")
    user_name = user.get("name")
    user_email = user.get("email")
    print(f"    👤 User ID: {user_id}")
    print(f"    👤 Username: {username}")
    print(f"    👤 Name: {user_name}")
    print(f"    👤 Email: {user_email}")
    
    print(f"\n🔍 [REVIEWER CHANGE STEP 7] Extracting project information...")
    project = payload.get("project", {})
    project_id = project.get("id")
    project_name = project.get("name")
    project_path = project.get("path_with_namespace")
    project_url = project.get("web_url")
    print(f"    🏗️  Project ID: {project_id}")
    print(f"    🏗️  Project name: {project_name}")
    print(f"    🏗️  Project path: {project_path}")
    print(f"    🏗️  Project URL: {project_url}")
    
    print(f"\n🔍 [REVIEWER CHANGE STEP 8] Extracting merge request information...")
    object_attributes = payload.get("object_attributes", {})
    mr_id = object_attributes.get("id")
    mr_iid = object_attributes.get("iid")
    mr_title = object_attributes.get("title")
    mr_state = object_attributes.get("state")
    mr_action = object_attributes.get("action")
    source_branch = object_attributes.get("source_branch")
    target_branch = object_attributes.get("target_branch")
    mr_url = object_attributes.get("url")
    updated_at = object_attributes.get("updated_at")
    
    print(f"    🔀 MR ID: {mr_id}")
    print(f"    🔀 MR IID: #{mr_iid}")
    print(f"    🔀 MR Title: '{mr_title}'")
    print(f"    🔀 MR State: {mr_state}")
    print(f"    🔀 MR Action: {mr_action}")
    print(f"    🔀 Source Branch: {source_branch}")
    print(f"    🔀 Target Branch: {target_branch}")
    print(f"    🔀 MR URL: {mr_url}")
    print(f"    🔀 Updated at: {updated_at}")
    
    print(f"\n🔍 [REVIEWER CHANGE STEP 9] Building reviewer_change_info object...")
    reviewer_change_info = {
        "event_type": "reviewer_change",
        "action": mr_action,
        "updated_at": updated_at,
        "bot_users": bot_users,
        "current_bot_reviewers": current_bot_reviewers,
        "previous_bot_reviewers": previous_bot_reviewers,
        "is_bot_assigned": is_bot_assigned,
        "is_bot_removed": is_bot_removed,
        "reviewer_changes": {
            "current": current_reviewers,
            "previous": previous_reviewers
        },
        "changed_by": {
            "id": user_id,
            "username": username,
            "name": user_name,
            "email": user_email
        },
        "merge_request": {
            "id": mr_id,
            "iid": mr_iid,
            "title": mr_title,
            "description": object_attributes.get("description"),
            "state": mr_state,
            "source_branch": source_branch,
            "target_branch": target_branch,
            "url": mr_url,
            "author_id": object_attributes.get("author_id"),
            "assignee_ids": object_attributes.get("assignee_ids", []),
            "reviewer_ids": object_attributes.get("reviewer_ids", []),
            "last_commit": object_attributes.get("last_commit", {})
        },
        "project": {
            "id": project_id,
            "name": project_name,
            "path_with_namespace": project_path,
            "web_url": project_url
        }
    }
    
    print(f"    📦 Reviewer change info created")
    print(f"    🆔 MR ID: {reviewer_change_info['merge_request']['id']}")
    print(f"    🔗 MR URL: {reviewer_change_info['merge_request']['url']}")
    print(f"    📅 Updated at: {reviewer_change_info['updated_at']}")
    print(f"    👥 {BOT_NAME.title()} users involved: {len(bot_users)}")
    print(f"    🚀 {BOT_NAME.title()} assigned: {is_bot_assigned}")
    print(f"    🗑️  {BOT_NAME.title()} removed: {is_bot_removed}")
    
    print(f"\n✅ [REVIEWER CHANGE STEP 10] Reviewer change processing completed successfully!")
    print(f"    📊 Final reviewer_change_info keys: {list(reviewer_change_info.keys())}")
    return reviewer_change_info

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

    print(f"\n🤖 Starting {BOT_NAME.title()} mention detection...")
    # Check for bot mentions first
    mention_info = process_bot_mention(payload, kind)
    if mention_info:
        print(f"\n🎉 === {BOT_NAME.title()} Bot Mentioned! ===")
        print(f"Author: {mention_info['author']['name']} (@{mention_info['author']['username']})")
        print(f"Note: {mention_info['note_text']}")
        if mention_info.get('merge_request'):
            mr = mention_info['merge_request']
            print(f"MR: #{mr['iid']} - {mr['title']}")
            print(f"MR URL: {mr['url']}")
            print(f"Source Branch: {mr['source_branch']} -> {mr['target_branch']}")
        print(f"Note URL: {mention_info['note_url']}")
        print(f"Project: {mention_info['project']['path_with_namespace']}")
        print(f"=== End {BOT_NAME.title()} Bot Mention ===\n", flush=True)
        
        # You can add your logic here to process the mention
        # For example: trigger a code review, respond to the comment, etc.
    else:
        print(f"    ℹ️  No {BOT_MENTION} mention detected in this event")

    print(f"\n🤖 Starting reviewer change detection...")
    # Check for reviewer changes involving bot users
    reviewer_change_info = process_reviewer_change(payload, kind)
    if reviewer_change_info:
        print(f"\n🎉 === {BOT_NAME.title()} User Reviewer Change Detected! ===")
        print(f"Changed by: {reviewer_change_info['changed_by']['name']} (@{reviewer_change_info['changed_by']['username']})")
        print(f"Action: {reviewer_change_info['action']}")
        print(f"Updated at: {reviewer_change_info['updated_at']}")
        
        mr = reviewer_change_info['merge_request']
        print(f"MR: #{mr['iid']} - {mr['title']}")
        print(f"MR URL: {mr['url']}")
        print(f"Source Branch: {mr['source_branch']} -> {mr['target_branch']}")
        print(f"Project: {reviewer_change_info['project']['path_with_namespace']}")
        
        # Show the reviewer changes
        changes = reviewer_change_info['reviewer_changes']
        print(f"\nReviewer Changes:")
        print(f"Previous reviewers ({len(changes['previous'])}):")
        for reviewer in changes['previous']:
            print(f"  - {reviewer['name']} (@{reviewer['username']})")
        print(f"Current reviewers ({len(changes['current'])}):")
        for reviewer in changes['current']:
            print(f"  - {reviewer['name']} (@{reviewer['username']})")
        
        # IMPORTANT: Highlight current bot reviewers that will trigger actions
        print(f"\n🚀 === TRIGGER CONDITION ANALYSIS ===")
        if reviewer_change_info['is_bot_assigned']:
            print(f"✅ {BOT_NAME.title()} user(s) ASSIGNED as current reviewer(s) - WILL TRIGGER ACTIONS!")
            print(f"Current {BOT_NAME.title()} reviewers ({len(reviewer_change_info['current_bot_reviewers'])}):")
            for reviewer in reviewer_change_info['current_bot_reviewers']:
                print(f"  🎯 {reviewer['name']} (@{reviewer['username']}) [ID: {reviewer['id']}]")
        else:
            print(f"❌ No {BOT_NAME.title()} users assigned as current reviewers - NO ACTIONS TRIGGERED")
        
        if reviewer_change_info['is_bot_removed']:
            print(f"📜 {BOT_NAME.title()} user(s) were REMOVED from reviewers")
            print(f"Previous {BOT_NAME.title()} reviewers ({len(reviewer_change_info['previous_bot_reviewers'])}):")
            for reviewer in reviewer_change_info['previous_bot_reviewers']:
                print(f"  📜 {reviewer['name']} (@{reviewer['username']}) [ID: {reviewer['id']}]")
        
        print(f"=== End {BOT_NAME.title()} Reviewer Change ===\n", flush=True)
        
        # IMPORTANT: This is where you would add your trigger logic
        if reviewer_change_info['is_bot_assigned']:
            print("🔥 === ACTION TRIGGER POINT ===")
            print(f"This is where you would trigger your {BOT_NAME.title()} actions:")
            print("- Start code review process")
            print("- Send notifications")
            print("- Update status")
            print("- etc.")
            print("=== End Action Trigger ===\n", flush=True)
    else:
        print(f"    ℹ️  No {BOT_NAME} user reviewer changes detected in this event")

    print(f"\n✅ Webhook processing completed successfully")
    return {"ok": True}

def test_mention_processing():
    """Test the mention processing with a sample payload"""
    sample_payload = {
        "event_type": "note",
        "object_attributes": {
            "id": 23126,
            "note": f"{BOT_MENTION} - hey {BOT_NAME} please review this",
            "description": f"{BOT_MENTION} - hey {BOT_NAME} please review this",
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
    
    mention_info = process_bot_mention(sample_payload, "Note Hook")
    if mention_info:
        print("✅ Test passed! Mention detected:")
        print(json.dumps(mention_info, indent=2))
    else:
        print("❌ Test failed! Mention not detected")

def test_reviewer_change_processing():
    """Test the reviewer change processing with a sample payload"""
    sample_payload = {
        "assignees": [
            {
                "avatar_url": "https://secure.gravatar.com/avatar/34e0b837169c9ebf7551bb8eb4df93c03e701491eab1874a08d7d5ae7c09dbd6?s=80&d=identicon",
                "email": "[REDACTED]",
                "id": 68,
                "name": "swagath",
                "username": "swagath"
            }
        ],
        "changes": {
            "reviewers": {
                "current": [
                    {
                        "avatar_url": "https://git.apps.hexmos.com/uploads/-/system/user/avatar/48/avatar.png",
                        "email": "[REDACTED]",
                        "id": 48,
                        "name": "Ganesh Kumar",
                        "username": "Ganesh"
                    }
                ],
                "previous": [
                    {
                        "avatar_url": "https://secure.gravatar.com/avatar/3839222089da191e4efe35a7bb35703b9e5c309b4d093410862678833b4d420f?s=80&d=identicon",
                        "email": "[REDACTED]",
                        "id": 83,
                        "name": f"{BOT_NAME.title()}Bot",
                        "username": f"{BOT_NAME.title()}Bot"
                    }
                ]
            },
            "updated_at": {
                "current": "2025-07-28 06:39:38 UTC",
                "previous": "2025-07-28 06:29:08 UTC"
            }
        },
        "event_type": "merge_request",
        "object_attributes": {
            "action": "update",
            "assignee_id": 68,
            "assignee_ids": [68],
            "author_id": 68,
            "id": 1798,
            "iid": 408,
            "title": "fixing the ui alligment issue",
            "description": "Fixing the Ui alligment issue in webview in copilot",
            "state": "merged",
            "source_branch": "swagath/meile-key-change",
            "target_branch": "main",
            "url": "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/408",
            "updated_at": "2025-07-28 06:39:38 UTC",
            "reviewer_ids": [48]
        },
        "object_kind": "merge_request",
        "project": {
            "id": 170,
            "name": "LiveAPI",
            "path_with_namespace": "hexmos/liveapi",
            "web_url": "https://git.apps.hexmos.com/hexmos/liveapi"
        },
        "user": {
            "avatar_url": "https://secure.gravatar.com/avatar/315382e7b250a15a9858e6e2735e316e1c32ee2807191d0596fc2e0a6f7d723f?s=80&d=identicon",
            "email": "[REDACTED]",
            "id": 3,
            "name": "Shrijith",
            "username": "shrijith"
        }
    }
    
    reviewer_change_info = process_reviewer_change(sample_payload, "Merge Request Hook")
    if reviewer_change_info:
        print("✅ Test passed! Reviewer change detected:")
        print(json.dumps(reviewer_change_info, indent=2))
    else:
        print("❌ Test failed! Reviewer change not detected")

def run_server():
    # Run the Flask dev server. In production, put this behind a real web server.
    app.run(host="0.0.0.0", port=8888, debug=False)

# ---------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------
if __name__ == "__main__":
    # Simple CLI for webhook management and testing
    #   python livereviewbot.py install-webhook
    #   python livereviewbot.py listen
    #   python livereviewbot.py install-webhook && python livereviewbot.py listen
    if len(sys.argv) < 2:
        print("Usage: python livereviewbot.py [install-webhook|listen|check-permissions|test-mention|test-reviewer-change]")
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
    elif cmd == "test-reviewer-change":
        test_reviewer_change_processing()
    else:
        print(f"Unknown command: {cmd}")
        sys.exit(1)
