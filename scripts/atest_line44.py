#!/usr/bin/env python3
"""
Simple test script specifically focused on debugging line 44 in gk_input_handler.go.
This is a minimal script to test posting a comment on an added line.
"""

import requests
import json
import re
import sys
import os

# GitLab API configuration
BASE_URL = "https://git.apps.hexmos.com"
# Test token for git.apps.hexmos.com - hardcoded for testing only
TOKEN = "REDACTED_GITLAB_PAT_6"
API_URL = f"{BASE_URL}/api/v4"

# Merge Request details
MR_URL = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/403"

# File details
FILE_PATH = "liveapi-backend/gatekeeper/gk_input_handler.go"
LINE_NUM = 44
IS_ADDED_LINE = True  # Line 44 is an added line: "TopK *float64"

def extract_project_id_from_url(mr_url):
    """Extract project ID and MR IID from the URL"""
    pattern = r"(.+)/-/merge_requests/(\d+)$"
    match = re.match(pattern, mr_url)
    if not match:
        raise ValueError(f"Could not extract project and MR ID from URL: {mr_url}")
    
    project_path = match.group(1).replace(BASE_URL + "/", "")
    mr_iid = int(match.group(2))
    
    return project_path, mr_iid

def get_mr_version(project_id, mr_iid):
    """Get the latest merge request version"""
    url = f"{API_URL}/projects/{project_id}/merge_requests/{mr_iid}/versions"
    headers = {"PRIVATE-TOKEN": TOKEN}
    
    response = requests.get(url, headers=headers)
    if response.status_code != 200:
        print(f"Error getting MR version: {response.status_code}")
        print(response.text)
        return None
    
    versions = response.json()
    if not versions:
        print("No versions found for this merge request")
        return None
    
    return versions[0]

def post_comment_on_line_44():
    """Post a test comment specifically on line 44"""
    # Get project ID and MR IID
    project_id, mr_iid = extract_project_id_from_url(MR_URL)
    print(f"Project ID: {project_id}")
    print(f"MR IID: {mr_iid}")
    
    # Get MR version information
    version = get_mr_version(project_id, mr_iid)
    if not version:
        return False
    
    print("MR Version Info:")
    print(f"  Base SHA: {version['base_commit_sha']}")
    print(f"  Head SHA: {version['head_commit_sha']}")
    print(f"  Start SHA: {version['start_commit_sha']}")
    
    # Create the request URL
    url = f"{API_URL}/projects/{project_id}/merge_requests/{mr_iid}/discussions"
    
    # Test Comment
    comment = "TEST: This is a comment on line 44 (added line: 'TopK *float64')"
    
    # Try multiple approaches
    approaches = [
        {
            "name": "JSON with new_line parameter",
            "headers": {
                "PRIVATE-TOKEN": TOKEN,
                "Content-Type": "application/json"
            },
            "data": {
                "body": comment,
                "position": {
                    "position_type": "text",
                    "base_sha": version["base_commit_sha"],
                    "head_sha": version["head_commit_sha"],
                    "start_sha": version["start_commit_sha"],
                    "old_path": FILE_PATH,
                    "new_path": FILE_PATH,
                    "new_line": LINE_NUM  # Using new_line for added line
                }
            },
            "type": "json"
        },
        {
            "name": "Form with new_line parameter",
            "headers": {
                "PRIVATE-TOKEN": TOKEN,
                "Content-Type": "application/x-www-form-urlencoded"
            },
            "data": {
                "body": comment,
                "position[position_type]": "text",
                "position[base_sha]": version["base_commit_sha"],
                "position[head_sha]": version["head_commit_sha"],
                "position[start_sha]": version["start_commit_sha"],
                "position[old_path]": FILE_PATH,
                "position[new_path]": FILE_PATH,
                "position[new_line]": str(LINE_NUM)  # Using new_line for added line
            },
            "type": "form"
        },
        # Additional approaches with both old_line and new_line
        {
            "name": "JSON with both old_line and new_line",
            "headers": {
                "PRIVATE-TOKEN": TOKEN,
                "Content-Type": "application/json"
            },
            "data": {
                "body": comment,
                "position": {
                    "position_type": "text",
                    "base_sha": version["base_commit_sha"],
                    "head_sha": version["head_commit_sha"],
                    "start_sha": version["start_commit_sha"],
                    "old_path": FILE_PATH,
                    "new_path": FILE_PATH,
                    "old_line": LINE_NUM,
                    "new_line": LINE_NUM  # Adding both
                }
            },
            "type": "json"
        },
        {
            "name": "Form with both old_line and new_line",
            "headers": {
                "PRIVATE-TOKEN": TOKEN,
                "Content-Type": "application/x-www-form-urlencoded"
            },
            "data": {
                "body": comment,
                "position[position_type]": "text",
                "position[base_sha]": version["base_commit_sha"],
                "position[head_sha]": version["head_commit_sha"],
                "position[start_sha]": version["start_commit_sha"],
                "position[old_path]": FILE_PATH,
                "position[new_path]": FILE_PATH,
                "position[old_line]": str(LINE_NUM),
                "position[new_line]": str(LINE_NUM)  # Adding both
            },
            "type": "form"
        }
    ]
    
    # Try each approach
    success = False
    for i, approach in enumerate(approaches):
        print(f"\n===== APPROACH {i+1}: {approach['name']} =====")
        
        if approach["type"] == "json":
            print("Request JSON:")
            print(json.dumps(approach["data"], indent=2))
            response = requests.post(url, headers=approach["headers"], json=approach["data"])
        else:  # form
            print("Form Data:")
            print(json.dumps(approach["data"], indent=2))
            response = requests.post(url, headers=approach["headers"], data=approach["data"])
        
        print(f"Response Status: {response.status_code}")
        print(f"Response Preview: {response.text[:200]}...")
        
        if response.status_code in [200, 201]:
            print(f"SUCCESS! Approach {i+1} worked.")
            success = True
            break
    
    return success

def main():
    print("===== TESTING LINE COMMENT ON LINE 44 (ADDED LINE) =====")
    result = post_comment_on_line_44()
    
    if result:
        print("\n✅ TEST PASSED: Successfully posted comment on line 44")
    else:
        print("\n❌ TEST FAILED: Could not post comment on line 44")

if __name__ == "__main__":
    main()
