#!/usr/bin/env python3
"""
Comprehensive GitLab line comment testing and debugging toolkit.
This script can analyze diffs, test line comments, and help diagnose issues.
"""

import argparse
import os
import re
import json
import sys
import requests
from colorama import init, Fore, Style

# Initialize colorama for cross-platform colored output
init()

# GitLab API configuration
BASE_URL = "https://git.apps.hexmos.com"
API_URL = f"{BASE_URL}/api/v4"

# Default problematic lines to test
PROBLEM_LINES = {
    44: {"file_path": "liveapi-backend/gatekeeper/gk_input_handler.go", "is_deleted": False, "content": "TopK *float64"},
    160: {"file_path": "liveapi-backend/gatekeeper/gk_input_handler.go", "is_deleted": True, "content": "defer client.Close()"}
}

def extract_project_id_from_url(mr_url):
    """Extract project ID and MR IID from the URL"""
    pattern = r"(.+)/-/merge_requests/(\d+)$"
    match = re.match(pattern, mr_url)
    if not match:
        raise ValueError(f"Could not extract project and MR ID from URL: {mr_url}")
    
    project_path = match.group(1).replace(BASE_URL + "/", "")
    mr_iid = int(match.group(2))
    
    return project_path, mr_iid

def get_mr_version(project_id, mr_iid, token):
    """Get the latest merge request version"""
    url = f"{API_URL}/projects/{project_id}/merge_requests/{mr_iid}/versions"
    headers = {"PRIVATE-TOKEN": token}
    
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

def post_line_comment(mr_url, token, file_path, line_num, is_deleted, comment=None):
    """Post a line comment to GitLab"""
    # Get project ID and MR IID
    project_id, mr_iid = extract_project_id_from_url(mr_url)
    print(f"Project ID: {project_id}")
    print(f"MR IID: {mr_iid}")
    
    # Get MR version information
    version = get_mr_version(project_id, mr_iid, token)
    if not version:
        return False
    
    print("MR Version Info:")
    print(f"  Base SHA: {version['base_commit_sha']}")
    print(f"  Head SHA: {version['head_commit_sha']}")
    print(f"  Start SHA: {version['start_commit_sha']}")
    
    # Create the request URL
    url = f"{API_URL}/projects/{project_id}/merge_requests/{mr_iid}/discussions"
    
    # Test Comment
    if not comment:
        line_type = "deleted" if is_deleted else "added"
        comment = f"TEST: This is a comment on line {line_num} ({line_type} line)"
    
    # Create the position object based on line type
    position = {
        "position_type": "text",
        "base_sha": version["base_commit_sha"],
        "head_sha": version["head_commit_sha"],
        "start_sha": version["start_commit_sha"],
        "old_path": file_path,
        "new_path": file_path
    }
    
    # Set either old_line or new_line based on whether it's a deleted line
    if is_deleted:
        position["old_line"] = line_num
    else:
        position["new_line"] = line_num
    
    # Create the request data
    request_data = {
        "body": comment,
        "position": position
    }
    
    # Set headers
    headers = {
        "PRIVATE-TOKEN": token,
        "Content-Type": "application/json"
    }
    
    # Make the API call
    print(f"\n----- POSTING COMMENT TO {'OLD' if is_deleted else 'NEW'} LINE {line_num} -----")
    print("Request JSON:")
    print(json.dumps(request_data, indent=2))
    
    response = requests.post(url, headers=headers, json=request_data)
    
    print(f"Response Status: {response.status_code}")
    print(f"Response Preview: {response.text[:200]}...")
    
    return response.status_code in [200, 201]

def parse_hunk_log(log_file):
    """Parse a hunk log file and extract line information"""
    if not os.path.exists(log_file):
        print(f"Error: Hunk log file {log_file} not found")
        return None
    
    with open(log_file, 'r') as f:
        content = f.read()
    
    # Extract hunk headers and content
    hunks = []
    
    # Look for standard unified diff hunk headers
    hunk_pattern = r'@@ -(\d+),(\d+) \+(\d+),(\d+) @@([\s\S]+?)(?=@@ -\d+,\d+ \+\d+,\d+ @@|$)'
    matches = re.finditer(hunk_pattern, content)
    
    for match in matches:
        old_start = int(match.group(1))
        old_count = int(match.group(2))
        new_start = int(match.group(3))
        new_count = int(match.group(4))
        hunk_content = match.group(5).strip()
        
        hunks.append({
            'old_start': old_start,
            'old_count': old_count,
            'new_start': new_start,
            'new_count': new_count,
            'content': hunk_content,
            'lines': hunk_content.split('\n')
        })
    
    return hunks

def analyze_hunk_lines(hunks):
    """Analyze hunk lines to determine line types"""
    line_info = {}
    
    for hunk in hunks:
        old_line = hunk['old_start']
        new_line = hunk['new_start']
        
        for line in hunk['lines']:
            if not line:
                continue
                
            if line.startswith('-'):
                # Deleted line (exists in old file only)
                line_info[old_line] = {
                    'old_line': old_line,
                    'new_line': 0,
                    'content': line[1:],
                    'type': 'deleted'
                }
                old_line += 1
            elif line.startswith('+'):
                # Added line (exists in new file only)
                line_info[new_line] = {
                    'old_line': 0,
                    'new_line': new_line,
                    'content': line[1:],
                    'type': 'added'
                }
                new_line += 1
            else:
                # Context line (exists in both files)
                line_info[new_line] = {
                    'old_line': old_line,
                    'new_line': new_line,
                    'content': line,
                    'type': 'context'
                }
                old_line += 1
                new_line += 1
    
    return line_info

def analyze_hunk_file(log_file, target_lines=None):
    """Analyze a hunk log file and display results"""
    print(f"\n===== ANALYZING HUNK LOG: {log_file} =====")
    
    hunks = parse_hunk_log(log_file)
    if not hunks:
        return None
    
    print(f"Found {len(hunks)} hunks in {log_file}")
    
    line_info = analyze_hunk_lines(hunks)
    
    # Display line information
    print("\n=== LINE INFORMATION ===")
    print(f"{'TYPE':<10} {'OLD #':<6} {'NEW #':<6} CONTENT")
    print("-" * 80)
    
    # Sort by new line number, then old line number
    sorted_lines = sorted(line_info.values(), key=lambda x: (x['new_line'] if x['new_line'] > 0 else float('inf'), 
                                                         x['old_line'] if x['old_line'] > 0 else float('inf')))
    
    for info in sorted_lines:
        line_type = info['type']
        old_line = str(info['old_line']) if info['old_line'] > 0 else '-'
        new_line = str(info['new_line']) if info['new_line'] > 0 else '-'
        content = info['content']
        
        # Highlight if this is a target line
        is_target = False
        if target_lines:
            if info['old_line'] in target_lines or info['new_line'] in target_lines:
                is_target = True
        
        # Color based on line type
        if line_type == 'deleted':
            color = Fore.RED
        elif line_type == 'added':
            color = Fore.GREEN
        else:
            color = Fore.WHITE
        
        # Highlight target lines
        if is_target:
            print(f"{Style.BRIGHT}{color}{line_type:<10} {old_line:<6} {new_line:<6} {content[:60]}{Style.RESET_ALL}")
            print(f"  ↳ {Style.BRIGHT}TARGET LINE{Style.RESET_ALL} - GitLab requires {'old_line' if line_type == 'deleted' else 'new_line'} parameter")
        else:
            print(f"{color}{line_type:<10} {old_line:<6} {new_line:<6} {content[:60]}{Style.RESET_ALL}")
    
    return line_info

def find_line_by_content(line_info, content_snippet):
    """Find line by content snippet"""
    matching_lines = []
    
    for line_num, info in line_info.items():
        if content_snippet.lower() in info['content'].lower():
            matching_lines.append(info)
    
    return matching_lines

def test_problem_lines(mr_url, token):
    """Test the known problematic lines"""
    results = {}
    
    for line_num, info in PROBLEM_LINES.items():
        print(f"\n===== TESTING LINE {line_num} ({info['content']}) =====")
        result = post_line_comment(
            mr_url=mr_url,
            token=token,
            file_path=info['file_path'],
            line_num=line_num,
            is_deleted=info['is_deleted'],
            comment=f"TEST: Comment for line {line_num} ({info['content']})"
        )
        
        results[line_num] = result
        
        if result:
            print(f"✅ SUCCESS: Comment posted to line {line_num}")
        else:
            print(f"❌ FAILURE: Could not post comment to line {line_num}")
    
    return results

def main():
    parser = argparse.ArgumentParser(description='GitLab Line Comment Testing Toolkit')
    parser.add_argument('--mr-url', help='GitLab Merge Request URL')
    parser.add_argument('--token', help='GitLab Personal Access Token')
    parser.add_argument('--log-file', help='Hunk log file to analyze')
    parser.add_argument('--line', type=int, action='append', help='Target line number to test (can be used multiple times)')
    parser.add_argument('--file-path', help='File path for line comments')
    parser.add_argument('--deleted', action='store_true', help='Treat the line as a deleted line')
    parser.add_argument('--test-problem-lines', action='store_true', help='Test the known problematic lines')
    parser.add_argument('--analyze-only', action='store_true', help='Only analyze hunk logs, don\'t post comments')
    args = parser.parse_args()
    
    # If log file provided, analyze it
    line_info = None
    if args.log_file:
        line_info = analyze_hunk_file(args.log_file, args.line)
    
    # Skip the rest if analyze-only
    if args.analyze_only:
        return
    
    # Require token and MR URL for posting comments
    if not args.token:
        token = os.environ.get('GITLAB_TOKEN')
        if not token:
            print("Error: GitLab token is required. Provide it with --token or set GITLAB_TOKEN environment variable.")
            return
    else:
        token = args.token
    
    if not args.mr_url:
        print("Error: GitLab Merge Request URL is required for posting comments.")
        return
    
    # Test problem lines if requested
    if args.test_problem_lines:
        test_problem_lines(args.mr_url, token)
    
    # Test specific lines if provided
    if args.line and args.file_path:
        for line_num in args.line:
            post_line_comment(
                mr_url=args.mr_url,
                token=token,
                file_path=args.file_path,
                line_num=line_num,
                is_deleted=args.deleted
            )

if __name__ == "__main__":
    main()
