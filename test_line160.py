#!/usr/bin/env python3
"""
Simple test script specifically focused on debugging line 160 in gk_input_handler.go.
This is a minimal script to test posting a comment on a deleted line.
"""

import requests
import json
import re
import sys

# GitLab API configuration
BASE_URL = "https://git.apps.hexmos.com"
TOKEN = "REDACTED_GITLAB_PAT_6"
API_URL = f"{BASE_URL}/api/v4"

# Merge Request details
MR_URL = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/403"

# File details
FILE_PATH = "liveapi-backend/gatekeeper/gk_input_handler.go"
LINE_NUM = 160
IS_DELETED_LINE = True  # Line 160 is a deleted line: "defer client.Close()"

def extract_project_id_from_url(mr_url):
    """Extract project ID and MR IID from the URL"""
    pattern = r"(.+)/-/merge_requests/(\d+)$"
    match = re.match(pattern, mr_url)
    if not match:
        raise ValueError(f"Could not extract project and MR ID from URL: {mr_url}")
    
    full_path = match.group(1)
    # Extract the project path after the base URL
    if BASE_URL in full_path:
        project_path = full_path.replace(BASE_URL + "/", "")
    else:
        project_path = full_path
    
    # For GitLab API, we need to URL encode the project path
    # But first, let's try using the project path as is with URL encoding
    project_id = project_path
    mr_iid = int(match.group(2))
    
    print(f"Extracted project path: {project_path}")
    
    return project_id, mr_iid

def get_mr_version(project_id, mr_iid):
    """Get the latest merge request version"""
    # Try multiple approaches for project ID encoding
    approaches = [
        project_id,  # As is
        requests.utils.quote(project_id, safe=''),  # URL encoded
        project_id.replace('/', '%2F')  # Manual slash encoding
    ]
    
    for i, encoded_id in enumerate(approaches):
        url = f"{API_URL}/projects/{encoded_id}/merge_requests/{mr_iid}/versions"
        print(f"Attempt {i+1}: Fetching MR versions from: {url}")
        
        headers = {"PRIVATE-TOKEN": TOKEN}
        response = requests.get(url, headers=headers)
        
        if response.status_code == 200:
            print(f"Success with approach {i+1}")
            versions = response.json()
            if not versions:
                print("No versions found for this merge request")
                continue
            return versions[0]
        else:
            print(f"Error getting MR version (attempt {i+1}): {response.status_code}")
            print(response.text)
    
    return None

def post_comment_on_line_160():
    """Post a test comment specifically on line 160"""
    try:
        # Get project ID and MR IID
        project_id, mr_iid = extract_project_id_from_url(MR_URL)
        print(f"Project ID: {project_id}")
        print(f"MR IID: {mr_iid}")
        
        # URL encode the project ID properly for API calls
        encoded_project_id = requests.utils.quote(project_id, safe='')
        
        # Get MR version information
        version = get_mr_version(project_id, mr_iid)
        if not version:
            return False
        
        print("MR Version Info:")
        print(f"  Base SHA: {version['base_commit_sha']}")
        print(f"  Head SHA: {version['head_commit_sha']}")
        print(f"  Start SHA: {version['start_commit_sha']}")
        
        # Create the request URL
        url = f"{API_URL}/projects/{encoded_project_id}/merge_requests/{mr_iid}/discussions"
        
        # Test Comment
        comment = "TEST: This is a comment on line 160 (deleted line: 'defer client.Close()')"
    except Exception as e:
        print(f"Error preparing request: {str(e)}")
        return False
    
    # Try multiple approaches
    approaches = [
        {
            "name": "JSON with old_line parameter",
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
                    "old_line": LINE_NUM  # Using old_line for deleted line
                }
            },
            "type": "json"
        },
        {
            "name": "Form with old_line parameter",
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
                "position[old_line]": str(LINE_NUM)  # Using old_line for deleted line
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
                    "new_line": LINE_NUM  # Adding new_line too
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
                "position[new_line]": str(LINE_NUM)  # Adding new_line too
            },
            "type": "form"
        }
    ]
    
    # Try each approach
    success = False
    for i, approach in enumerate(approaches):
        print(f"\n===== APPROACH {i+1}: {approach['name']} =====")
        
        try:
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
        except Exception as e:
            print(f"Error with approach {i+1}: {str(e)}")
            continue
    
    return success

def main():
    print("===== TESTING LINE COMMENT ON LINE 160 (DELETED LINE) =====")
    print(f"MR URL: {MR_URL}")
    print(f"File Path: {FILE_PATH}")
    print(f"Line Number: {LINE_NUM}")
    print(f"Is Deleted Line: {IS_DELETED_LINE}")
    print("\n")
    
    try:
        result = post_comment_on_line_160()
        
        if result:
            print("\n✅ TEST PASSED: Successfully posted comment on line 160")
        else:
            print("\n❌ TEST FAILED: Could not post comment on line 160")
    except Exception as e:
        print(f"\n❌ TEST FAILED with error: {str(e)}")
        import traceback
        traceback.print_exc()

if __name__ == "__main__":
    main()
