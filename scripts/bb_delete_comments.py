#!/usr/bin/env python3
"""
Bitbucket Pull Request Comment Deletion Script

This script deletes all visible comments in a specified Bitbucket pull request.
"""

import requests
import json
import sys
from urllib.parse import urlparse

# Configuration - hardcoded values as requested
# For Bitbucket account API tokens, Basic Auth requires email (preferred) or username + token
EMAIL = "contortedexpression@gmail.com"  # e.g. "you@example.com". If empty, USERNAME will be used
USERNAME = "contorted"  # Fallback if EMAIL is empty
API_TOKEN = "REDACTED_BITBUCKET_TOKEN_2"
PULL_REQUEST_URL = "https://bitbucket.org/contorted/fb_backends/pull-requests/1"

# Bitbucket API base URL
BITBUCKET_API_BASE = "https://api.bitbucket.org/2.0"


def get_basic_auth_credentials():
    """Return (username_or_email, token) for Basic Auth matching Go client behavior."""
    username_or_email = EMAIL if EMAIL else USERNAME
    return username_or_email, API_TOKEN

def extract_repo_and_pr_id(pr_url):
    """
    Extract repository workspace, repository name, and PR ID from the pull request URL.
    
    Args:
        pr_url (str): The pull request URL
        
    Returns:
        tuple: (workspace, repo_name, pr_id)
    """
    try:
        # Parse URL like: https://bitbucket.org/contorted/fb_backends/pull-requests/1
        parsed = urlparse(pr_url)
        path_parts = parsed.path.strip('/').split('/')
        
        if len(path_parts) >= 4 and path_parts[2] == 'pull-requests':
            workspace = path_parts[0]
            repo_name = path_parts[1]
            pr_id = path_parts[3]
            return workspace, repo_name, pr_id
        else:
            raise ValueError("Invalid pull request URL format")
    except Exception as e:
        print(f"Error parsing pull request URL: {e}")
        sys.exit(1)

def get_pull_request_comments(workspace, repo_name, pr_id):
    """
    Get all comments for a pull request.
    
    Args:
        workspace (str): Bitbucket workspace name
        repo_name (str): Repository name
        pr_id (str): Pull request ID
        
    Returns:
        list: List of comment objects
    """
    url = f"{BITBUCKET_API_BASE}/repositories/{workspace}/{repo_name}/pullrequests/{pr_id}/comments"

    # Match Go client: Basic Auth, Accept JSON, UA header
    auth = get_basic_auth_credentials()
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "User-Agent": "LiveReview/1.0",
    }

    all_comments = []
    next_url = url

    while next_url:
        response = requests.get(next_url, auth=auth, headers=headers, params={"pagelen": 50})
        
        if response.status_code != 200:
            print(f"Error fetching comments: {response.status_code} - {response.text}")
            return []
        
        data = response.json()
        comments = data.get("values", [])
        
        if comments:
            all_comments.extend(comments)

        # Follow server-provided pagination URL
        next_url = data.get("next")
    
    return all_comments

def delete_comment(workspace, repo_name, pr_id, comment_id):
    """
    Delete a specific comment from a pull request.
    
    Args:
        workspace (str): Bitbucket workspace name
        repo_name (str): Repository name
        pr_id (str): Pull request ID
        comment_id (str): Comment ID to delete
        
    Returns:
        bool: True if successful, False otherwise
    """
    url = f"{BITBUCKET_API_BASE}/repositories/{workspace}/{repo_name}/pullrequests/{pr_id}/comments/{comment_id}"

    # Match Go client headers and auth
    auth = get_basic_auth_credentials()
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "User-Agent": "LiveReview/1.0",
    }

    response = requests.delete(url, auth=auth, headers=headers)
    
    if response.status_code == 204:
        return True
    else:
        print(f"Error deleting comment {comment_id}: {response.status_code} - {response.text}")
        return False

def main():
    """
    Main function to delete all comments in the specified pull request.
    """
    print("Bitbucket Pull Request Comment Deletion Script")
    print("=" * 50)
    
    # Extract repository information from the pull request URL
    workspace, repo_name, pr_id = extract_repo_and_pr_id(PULL_REQUEST_URL)
    
    print(f"Workspace: {workspace}")
    print(f"Repository: {repo_name}")
    print(f"Pull Request ID: {pr_id}")
    print()
    
    # Get all comments for the pull request
    print("Fetching comments...")
    comments = get_pull_request_comments(workspace, repo_name, pr_id)
    
    if not comments:
        print("No comments found in the pull request.")
        return
    
    print(f"Found {len(comments)} comments to delete.")
    print()
    
    # Delete each comment
    deleted_count = 0
    failed_count = 0
    
    for comment in comments:
        comment_id = comment.get("id")
        comment_text = comment.get("content", {}).get("raw", "")[:50] + "..." if len(comment.get("content", {}).get("raw", "")) > 50 else comment.get("content", {}).get("raw", "")
        
        print(f"Deleting comment {comment_id}: {comment_text}")
        
        if delete_comment(workspace, repo_name, pr_id, comment_id):
            deleted_count += 1
            print(f"  ✓ Successfully deleted comment {comment_id}")
        else:
            failed_count += 1
            print(f"  ✗ Failed to delete comment {comment_id}")
    
    print()
    print("Deletion Summary:")
    print(f"  Total comments: {len(comments)}")
    print(f"  Successfully deleted: {deleted_count}")
    print(f"  Failed to delete: {failed_count}")
    
    if failed_count == 0:
        print("All comments have been successfully deleted!")
    else:
        print(f"Warning: {failed_count} comments could not be deleted.")

if __name__ == "__main__":
    main() 