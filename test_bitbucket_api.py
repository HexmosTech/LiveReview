#!/usr/bin/env python3
"""
Simple test script to validate Bitbucket API token authentication
This helps debug the authentication issue independently of the Go backend
"""

import requests
import json
import sys
from base64 import b64encode

def test_bitbucket_auth(email, api_token):
    print(f"Testing Bitbucket API authentication...")
    print(f"Email: {email}")
    print(f"API Token: {api_token[:4]}...{api_token[-4:]}")
    print("-" * 50)
    
    # Test 1: Basic user profile endpoint
    print("Test 1: /2.0/user endpoint")
    try:
        response = requests.get(
            "https://api.bitbucket.org/2.0/user",
            auth=(email, api_token),
            headers={
                "Accept": "application/json",
                "User-Agent": "LiveReview-Test/1.0"
            }
        )
        print(f"Status Code: {response.status_code}")
        if response.status_code == 200:
            data = response.json()
            print(f"Success! User: {data.get('display_name', 'N/A')} ({data.get('username', 'N/A')})")
        else:
            print(f"Error: {response.text}")
    except Exception as e:
        print(f"Exception: {e}")
    
    print()
    
    # Test 2: Repositories endpoint (minimal permissions)
    print("Test 2: /2.0/repositories endpoint")
    try:
        response = requests.get(
            "https://api.bitbucket.org/2.0/repositories?role=member&pagelen=1",
            auth=(email, api_token),
            headers={
                "Accept": "application/json",
                "User-Agent": "LiveReview-Test/1.0"
            }
        )
        print(f"Status Code: {response.status_code}")
        if response.status_code == 200:
            print("Repository access: OK")
        else:
            print(f"Error: {response.text}")
    except Exception as e:
        print(f"Exception: {e}")
    
    print()
    
    # Test 3: Raw Basic Auth header test
    print("Test 3: Manual Basic Auth header")
    try:
        auth_string = f"{email}:{api_token}"
        encoded_auth = b64encode(auth_string.encode()).decode()
        
        response = requests.get(
            "https://api.bitbucket.org/2.0/user",
            headers={
                "Authorization": f"Basic {encoded_auth}",
                "Accept": "application/json",
                "User-Agent": "LiveReview-Test/1.0"
            }
        )
        print(f"Status Code: {response.status_code}")
        print(f"Auth Header: Basic {encoded_auth[:20]}...")
        if response.status_code == 200:
            print("Manual Basic Auth: Success!")
        else:
            print(f"Error: {response.text}")
    except Exception as e:
        print(f"Exception: {e}")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python3 test_bitbucket_api.py <email> <api_token>")
        print("Example: python3 test_bitbucket_api.py user@example.com ATBB...")
        sys.exit(1)
    
    email = sys.argv[1]
    api_token = sys.argv[2]
    
    test_bitbucket_auth(email, api_token)
