#!/usr/bin/env python3
"""
Test script for debugging GitLab line comments.
This script will post test comments to specific lines in a file to help debug 
the line comment functionality in LiveReview.
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

# File to comment on
FILE_PATH = "liveapi-backend/gatekeeper/gk_input_handler.go"

# Extract project ID and MR IID from the URL
def extract_mr_info(mr_url):
    pattern = r"(.+)/-/merge_requests/(\d+)$"
    match = re.match(pattern, mr_url)
    if not match:
        raise ValueError(f"Could not extract project and MR ID from URL: {mr_url}")
    
    project_path = match.group(1).replace(BASE_URL + "/", "")
    mr_iid = int(match.group(2))
    
    return project_path, mr_iid

# Get the latest MR version
def get_latest_mr_version(project_id, mr_iid):
    url = f"{API_URL}/projects/{project_id}/merge_requests/{mr_iid}/versions"
    headers = {"PRIVATE-TOKEN": TOKEN}
    
    response = requests.get(url, headers=headers)
    response.raise_for_status()
    
    versions = response.json()
    if not versions:
        raise ValueError("No versions found for this merge request")
    
    return versions[0]

# Generate a line code for GitLab comments
def generate_line_code(start_sha, head_sha, file_path, line_num):
    import hashlib
    
    # Take first 8 characters of each SHA
    short_start_sha = start_sha[:8] if len(start_sha) > 8 else start_sha
    short_head_sha = head_sha[:8] if len(head_sha) > 8 else head_sha
    
    # Normalize the file path
    normalized_path = file_path.replace("/", "_")
    
    # Generate a hash for the file path
    path_hash = hashlib.sha1(file_path.encode()).hexdigest()[:8]
    
    # Generate new style line code
    new_style_line_code = f"{path_hash}_{line_num}"
    
    # Generate old style line code
    old_style_line_code = f"{short_start_sha}_{short_head_sha}_{normalized_path}_right_{line_num}"
    
    return new_style_line_code, old_style_line_code

# Post a comment on a specific line
def post_line_comment(project_id, mr_iid, file_path, line_num, comment, is_deleted_line=False):
    print(f"\n----- POSTING COMMENT TO {'OLD' if is_deleted_line else 'NEW'} LINE {line_num} -----")
    print(f"File: {file_path}")
    print(f"Line Type: {'Deleted Line' if is_deleted_line else 'Added/Context Line'}")
    
    # Get the latest MR version
    version = get_latest_mr_version(project_id, mr_iid)
    print(f"MR Version: {json.dumps(version, indent=2)}")
    
    # Generate line codes
    new_style_code, old_style_code = generate_line_code(
        version["start_commit_sha"], 
        version["head_commit_sha"], 
        file_path, 
        line_num
    )
    print(f"New Style Line Code: {new_style_code}")
    print(f"Old Style Line Code: {old_style_code}")
    
    # Set up the request URL and headers
    url = f"{API_URL}/projects/{project_id}/merge_requests/{mr_iid}/discussions"
    headers = {
        "PRIVATE-TOKEN": TOKEN,
        "Content-Type": "application/json"
    }
    
    # Create position data based on whether it's a deleted line or not
    position = {
        "position_type": "text",
        "base_sha": version["base_commit_sha"],
        "head_sha": version["head_commit_sha"],
        "start_sha": version["start_commit_sha"],
        "new_path": file_path,
        "old_path": file_path,
        "line_code": new_style_code
    }
    
    # Set either old_line or new_line based on whether it's a deleted line
    if is_deleted_line:
        position["old_line"] = line_num
        print(f"Using old_line={line_num} (for deleted line)")
    else:
        position["new_line"] = line_num
        print(f"Using new_line={line_num} (for added/context line)")
    
    # Create the request payload
    payload = {
        "body": comment,
        "position": position
    }
    
    print("\nRequest Payload:")
    print(json.dumps(payload, indent=2))
    
    # Send the request
    response = requests.post(url, headers=headers, json=payload)
    print(f"\nResponse Status: {response.status_code}")
    
    # If the first attempt fails, try with the old style line code
    if response.status_code != 201 and response.status_code != 200:
        print(f"First attempt failed with status {response.status_code}")
        print(f"Response body: {response.text[:200]}...")
        print("\nTrying with old style line code...")
        
        # Update line_code to use old style
        payload["position"]["line_code"] = old_style_code
        
        # Send the request again
        response = requests.post(url, headers=headers, json=payload)
        print(f"Second attempt status: {response.status_code}")
    
    # Print response data
    try:
        response_data = response.json()
        print(f"Response Data (truncated):")
        print(json.dumps(response_data, indent=2)[:500] + "...")
    except:
        print(f"Response Body: {response.text[:500]}...")
    
    return response.status_code, response.text

# Try multiple approaches for comment creation
def try_multiple_approaches(project_id, mr_iid, file_path, line_num, comment, is_deleted_line=False):
    # First try JSON approach
    status, response = post_line_comment(project_id, mr_iid, file_path, line_num, comment, is_deleted_line)
    
    # If that fails, try form-based approach
    if status != 201 and status != 200:
        print("\n\n===== JSON APPROACH FAILED. TRYING FORM-BASED APPROACH =====")
        version = get_latest_mr_version(project_id, mr_iid)
        
        new_style_code, _ = generate_line_code(
            version["start_commit_sha"], 
            version["head_commit_sha"], 
            file_path, 
            line_num
        )
        
        url = f"{API_URL}/projects/{project_id}/merge_requests/{mr_iid}/discussions"
        headers = {
            "PRIVATE-TOKEN": TOKEN,
            "Content-Type": "application/x-www-form-urlencoded"
        }
        
        # Build form data
        form_data = {
            "body": comment,
            "position[position_type]": "text",
            "position[base_sha]": version["base_commit_sha"],
            "position[start_sha]": version["start_commit_sha"],
            "position[head_sha]": version["head_commit_sha"],
            "position[new_path]": file_path,
            "position[old_path]": file_path,
            "position[line_code]": new_style_code
        }
        
        # Set either old_line or new_line
        if is_deleted_line:
            form_data["position[old_line]"] = str(line_num)
            print(f"Using position[old_line]={line_num} (for deleted line)")
        else:
            form_data["position[new_line]"] = str(line_num)
            print(f"Using position[new_line]={line_num} (for added/context line)")
        
        print("\nForm Data:")
        print(json.dumps(form_data, indent=2))
        
        # Send the form-based request
        response = requests.post(url, headers=headers, data=form_data)
        print(f"\nForm-based response status: {response.status_code}")
        print(f"Response body: {response.text[:200]}...")
        
        status = response.status_code
        
    return status

# Test line comments on specific problematic lines
def test_specific_lines():
    # Extract project ID and MR IID
    project_id, mr_iid = extract_mr_info(MR_URL)
    print(f"Project ID: {project_id}")
    print(f"MR IID: {mr_iid}")
    
    # Test line 160 (deleted line: "defer client.Close()")
    test_line_160_as_deleted = try_multiple_approaches(
        project_id, 
        mr_iid, 
        FILE_PATH, 
        160, 
        "TEST: This is a comment on line 160 (deleted line: 'defer client.Close()') [DELETED]", 
        is_deleted_line=True
    )
    
    # Test line 44 (added line: "TopK *float64")
    test_line_44_as_added = try_multiple_approaches(
        project_id, 
        mr_iid, 
        FILE_PATH, 
        44, 
        "TEST: This is a comment on line 44 (added line: 'TopK *float64') [ADDED]", 
        is_deleted_line=False
    )
    
    print("\n\n===== TEST RESULTS =====")
    print(f"Line 160 (Deleted) Test: {'SUCCESS' if test_line_160_as_deleted in [200, 201] else 'FAILED'}")
    print(f"Line 44 (Added) Test: {'SUCCESS' if test_line_44_as_added in [200, 201] else 'FAILED'}")

# Direct testing of specific line
def test_specific_line(line_num, is_deleted=False):
    project_id, mr_iid = extract_mr_info(MR_URL)
    comment = f"TEST: Direct comment on line {line_num} ({'deleted' if is_deleted else 'added/context'} line)"
    
    result = try_multiple_approaches(
        project_id, 
        mr_iid, 
        FILE_PATH, 
        line_num, 
        comment,
        is_deleted_line=is_deleted
    )
    
    print(f"\nTest on line {line_num} ({'deleted' if is_deleted else 'added/context'}): {'SUCCESS' if result in [200, 201] else 'FAILED'}")
    return result in [200, 201]

# Main function
def main():
    print("===== GITLAB LINE COMMENT DEBUGGING SCRIPT =====")
    
    # If command line arguments provided, use them
    if len(sys.argv) > 1:
        if sys.argv[1] == "test_all":
            test_specific_lines()
        elif len(sys.argv) >= 3:
            line_num = int(sys.argv[1])
            is_deleted = sys.argv[2].lower() in ["deleted", "true", "1", "yes"]
            test_specific_line(line_num, is_deleted)
    else:
        # Default: Test both problematic lines
        test_specific_lines()

if __name__ == "__main__":
    main()
