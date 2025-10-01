#!/usr/bin/env python3
"""
Bitbucket API Test Script

This script tests the Bitbucket API key to verify it's working correctly.
"""

import requests
import json

# Configuration - same as the main script
# For account API tokens, we need username and token
USERNAME = "contorted"  # Your Bitbucket username
API_TOKEN = "REDACTED_BITBUCKET_TOKEN_2"
PULL_REQUEST_URL = "https://bitbucket.org/contorted/fb_backends/pull-requests/1"

# Bitbucket API base URL
BITBUCKET_API_BASE = "https://api.bitbucket.org/2.0"

def test_api_key():
    """
    Test the API key by making a simple request to get user information.
    """
    print("Testing Bitbucket API Key...")
    print("=" * 40)
    
    # Test 1: Get current user info
    print("Test 1: Getting current user information...")
    url = f"{BITBUCKET_API_BASE}/user"
    
    # Use Basic Authentication for account API tokens
    auth = (USERNAME, API_TOKEN)
    headers = {
        "Content-Type": "application/json"
    }
    
    try:
        response = requests.get(url, auth=auth, headers=headers)
        print(f"Status Code: {response.status_code}")
        
        if response.status_code == 200:
            user_data = response.json()
            print(f"✓ API Key is valid!")
            print(f"  Username: {user_data.get('username', 'N/A')}")
            print(f"  Display Name: {user_data.get('display_name', 'N/A')}")
            print(f"  Account ID: {user_data.get('account_id', 'N/A')}")
        else:
            print(f"✗ API Key test failed: {response.status_code}")
            print(f"  Response: {response.text}")
            
    except Exception as e:
        print(f"✗ Error testing API key: {e}")
    
    print()

def test_repository_access():
    """
    Test access to the specific repository.
    """
    print("Test 2: Testing repository access...")
    
    url = f"{BITBUCKET_API_BASE}/repositories/contorted/fb_backends"
    
    # Use Basic Authentication for account API tokens
    auth = (USERNAME, API_TOKEN)
    headers = {
        "Content-Type": "application/json"
    }
    
    try:
        response = requests.get(url, auth=auth, headers=headers)
        print(f"Status Code: {response.status_code}")
        
        if response.status_code == 200:
            repo_data = response.json()
            print(f"✓ Repository access successful!")
            print(f"  Repository: {repo_data.get('name', 'N/A')}")
            print(f"  Workspace: {repo_data.get('workspace', {}).get('name', 'N/A')}")
            print(f"  Private: {repo_data.get('is_private', 'N/A')}")
        else:
            print(f"✗ Repository access failed: {response.status_code}")
            print(f"  Response: {response.text}")
            
    except Exception as e:
        print(f"✗ Error testing repository access: {e}")
    
    print()

def test_pull_request_access():
    """
    Test access to the specific pull request.
    """
    print("Test 3: Testing pull request access...")
    
    url = f"{BITBUCKET_API_BASE}/repositories/contorted/fb_backends/pullrequests/1"
    
    # Use Basic Authentication for account API tokens
    auth = (USERNAME, API_TOKEN)
    headers = {
        "Content-Type": "application/json"
    }
    
    try:
        response = requests.get(url, auth=auth, headers=headers)
        print(f"Status Code: {response.status_code}")
        
        if response.status_code == 200:
            pr_data = response.json()
            print(f"✓ Pull request access successful!")
            print(f"  PR ID: {pr_data.get('id', 'N/A')}")
            print(f"  Title: {pr_data.get('title', 'N/A')}")
            print(f"  State: {pr_data.get('state', 'N/A')}")
        else:
            print(f"✗ Pull request access failed: {response.status_code}")
            print(f"  Response: {response.text}")
            
    except Exception as e:
        print(f"✗ Error testing pull request access: {e}")
    
    print()

def test_comments_endpoint():
    """
    Test the comments endpoint specifically.
    """
    print("Test 4: Testing comments endpoint...")
    
    url = f"{BITBUCKET_API_BASE}/repositories/contorted/fb_backends/pullrequests/1/comments"
    
    # Use Basic Authentication for account API tokens
    auth = (USERNAME, API_TOKEN)
    headers = {
        "Content-Type": "application/json"
    }
    
    try:
        response = requests.get(url, auth=auth, headers=headers)
        print(f"Status Code: {response.status_code}")
        
        if response.status_code == 200:
            comments_data = response.json()
            comment_count = len(comments_data.get("values", []))
            print(f"✓ Comments endpoint access successful!")
            print(f"  Number of comments: {comment_count}")
            
            if comment_count > 0:
                print("  Sample comments:")
                for i, comment in enumerate(comments_data.get("values", [])[:3]):
                    print(f"    {i+1}. ID: {comment.get('id')} - {comment.get('content', {}).get('raw', '')[:50]}...")
        else:
            print(f"✗ Comments endpoint access failed: {response.status_code}")
            print(f"  Response: {response.text}")
            
    except Exception as e:
        print(f"✗ Error testing comments endpoint: {e}")
    
    print()

def main():
    """
    Run all tests to diagnose the API key issue.
    """
    print("Bitbucket API Key Diagnostic Tool")
    print("=" * 50)
    print()
    
    test_api_key()
    test_repository_access()
    test_pull_request_access()
    test_comments_endpoint()
    
    print("Diagnostic Summary:")
    print("=" * 50)
    print("If any test failed with 401 errors, your API key may be:")
    print("1. Expired - Generate a new API key in Bitbucket")
    print("2. Invalid format - Make sure you're using the correct token type")
    print("3. Missing permissions - Ensure the key has repository read/write access")
    print("4. Wrong token type - Use App Password or OAuth token, not username/password")
    print()
    print("To generate a new API key:")
    print("1. Go to Bitbucket Settings > App passwords")
    print("2. Create a new app password with repository permissions")
    print("3. Copy the generated token and update the script")

if __name__ == "__main__":
    main()
