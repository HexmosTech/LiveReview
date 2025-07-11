from pprint import pprint

url = "https://git.apps.hexmos.com"
token = "REDACTED_GITLAB_PAT_6"

mr_url = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/335"

import requests
import re
import json

# Extract project and MR ID from the URL
def extract_mr_info(mr_url):
    pattern = r"https://[^/]+/([^/]+/[^/]+)/-/merge_requests/(\d+)"
    match = re.match(pattern, mr_url)
    if match:
        project_path = match.group(1)
        mr_id = match.group(2)
        return project_path, mr_id
    return None, None

# Encode project path for API URL
def encode_project_path(project_path):
    return project_path.replace('/', '%2F')

# Fetch merge request versions to get required SHA values
def get_mr_versions(base_url, project_path, mr_id, headers):
    versions_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/versions"
    response = requests.get(versions_url, headers=headers)
    
    if response.status_code == 200:
        versions = response.json()
        if versions:
            # Return the latest version (first in the array)
            return versions[0]
    
    print(f"Failed to fetch MR versions. Status code: {response.status_code}")
    print(f"Error: {response.text}")
    return None

# Create a new thread in a merge request
def create_mr_thread(base_url, project_path, mr_id, headers, version_info, comment_data):
    threads_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/discussions"
    
    # Prepare the data for the request
    data = {
        'body': comment_data['body']
    }
    
    # Add position data if this is a diff comment
    if 'position' in comment_data:
        position = comment_data['position']
        data['position'] = {
            'position_type': 'text',
            'base_sha': version_info['base_commit_sha'],
            'head_sha': version_info['head_commit_sha'],
            'start_sha': version_info['start_commit_sha'],
            'new_path': position['new_path'],
            'old_path': position['old_path']
        }
        
        # Add line information
        if 'new_line' in position:
            data['position']['new_line'] = position['new_line']
        if 'old_line' in position:
            data['position']['old_line'] = position['old_line']
        
        # Add line range for multiline comments if provided
        if 'line_range' in position:
            data['position']['line_range'] = position['line_range']
    
    # Make the request
    response = requests.post(threads_url, json=data, headers=headers)
    
    if response.status_code in [200, 201]:
        return response.json()
    else:
        print(f"Failed to create thread. Status code: {response.status_code}")
        print(f"Error: {response.text}")
        return None

# Main script execution
project_path, mr_id = extract_mr_info(mr_url)
if not project_path or not mr_id:
    print(f"Failed to extract project path and MR ID from URL: {mr_url}")
    exit(1)

encoded_project_path = encode_project_path(project_path)

# Set up headers for API request
headers = {
    "PRIVATE-TOKEN": token,
    "Content-Type": "application/json"
}

# Fetch merge request details
api_url = f"{url}/api/v4/projects/{encoded_project_path}/merge_requests/{mr_id}"
response = requests.get(api_url, headers=headers)

if response.status_code == 200:
    mr_details = response.json()
    print(f"Successfully fetched merge request information:")
    print(f"Title: {mr_details['title']}")
    print(f"State: {mr_details['state']}")
    print(f"Author: {mr_details['author']['name']}")
    print(f"Description: {mr_details['description'][:100]}..." if len(mr_details['description']) > 100 else mr_details['description'])
    
    # You can also fetch additional information like comments, changes, approvals, etc.
    
    # Example: Fetch the changes (diff)
    changes_url = f"{api_url}/changes"
    changes_response = requests.get(changes_url, headers=headers)
    
    if changes_response.status_code == 200:
        changes = changes_response.json()
        print(f"\nChanges in merge request:")
        print(f"Number of changed files: {len(changes['changes'])}")
        
        # Print changed files
        for change in changes['changes']:
            print(f"File: {change['new_path']} (Added: {change['new_file']}, Deleted: {change['deleted_file']})")
    else:
        print(f"Failed to fetch changes. Status code: {changes_response.status_code}")
        print(f"Error: {changes_response.text}")
    
    # Example: Fetch comments
    discussions_url = f"{api_url}/discussions"
    discussions_response = requests.get(discussions_url, headers=headers)
    
    if discussions_response.status_code == 200:
        discussions = discussions_response.json()
        print("=========================")
        print(f"\nDiscussions in merge request:")
        
        # More detailed analysis of discussion notes
        visible_comments = []
        system_notes = []
        
        for discussion in discussions:
            for note in discussion['notes']:
                # Filter out system notes which aren't visible as "comments" in the UI
                # System notes are usually automatic messages about state changes
                if note.get('system', False):
                    system_notes.append(note)
                elif note.get('type') == 'DiscussionNote' or note.get('resolvable', False):
                    visible_comments.append(note)
        
        # Display counts with better categorization
        print(f"\nVisible user comments: {len(visible_comments)}")
        print(f"System/automatic notes: {len(system_notes)}")
        print(f"Total notes: {len(visible_comments) + len(system_notes)}")
        
        # Show sample of user comments if any exist
        if visible_comments:
            print("\nSample user comments:")
            for i, comment in enumerate(visible_comments[:3]):  # Show up to 3 samples
                print(f"Comment {i+1} by {comment['author']['name']}:")
                print(f"  {comment['body'][:100]}..." if len(comment['body']) > 100 else f"  {comment['body']}")
        else:
            print("\nNo visible user comments found in this merge request")
            
        print("=========================")
    else:
        print(f"Failed to fetch discussions. Status code: {discussions_response.status_code}")
        print(f"Error: {discussions_response.text}")
    
    # Save full response to a file for reference
    with open('mr_details.json', 'w') as f:
        json.dump(mr_details, f, indent=2)
    print("\nFull details saved to 'mr_details.json'")
    
    # Function to add a general comment to the MR
def add_general_comment(base_url, project_path, mr_id, headers, comment_text):
    comment_data = {
        'body': comment_text
    }
    
    # Get version info first
    version_info = get_mr_versions(base_url, project_path, mr_id, headers)
    if not version_info:
        print("Failed to get version information, cannot add comment")
        return None
    
    # Create the thread
    result = create_mr_thread(base_url, project_path, mr_id, headers, version_info, comment_data)
    if result:
        print(f"\nSuccessfully added general comment: '{comment_text}'")
        print(f"Thread ID: {result['id']}")
        return result
    return None

# Function to add a line comment to a specific file and line
def add_line_comment(base_url, project_path, mr_id, headers, file_path, line_number, comment_text, changes_data=None):
    # Get version info first
    version_info = get_mr_versions(base_url, project_path, mr_id, headers)
    if not version_info:
        print("Failed to get version information, cannot add comment")
        return None
    
    # If we don't have changes data, fetch it
    if not changes_data:
        changes_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/changes"
        changes_response = requests.get(changes_url, headers=headers)
        
        if changes_response.status_code != 200:
            print(f"Failed to fetch changes. Status code: {changes_response.status_code}")
            return None
        
        changes_data = changes_response.json()
    
    # Find the file in the changes
    # Check the structure of changes_data and access it correctly
    target_change = None
    changes_list = changes_data
    
    # Handle both list and dict with 'changes' key
    if isinstance(changes_data, dict) and 'changes' in changes_data:
        changes_list = changes_data['changes']
    
    for change in changes_list:
        if change['new_path'] == file_path:
            target_change = change
            break
    
    if not target_change:
        print(f"File '{file_path}' not found in the merge request changes")
        return None
    
    # Prepare comment data
    comment_data = {
        'body': comment_text,
        'position': {
            'new_path': file_path,
            'old_path': target_change['old_path'],
            'new_line': line_number
            # Use old_line instead if commenting on a removed line
        }
    }
    
    # Create the thread
    result = create_mr_thread(base_url, project_path, mr_id, headers, version_info, comment_data)
    if result:
        print(f"\nSuccessfully added line comment to {file_path}:{line_number}")
        print(f"Comment: '{comment_text}'")
        print(f"Thread ID: {result['id']}")
        return result
    return None

# Get merge request versions to retrieve necessary SHAs
version_info = get_mr_versions(url, encoded_project_path, mr_id, headers)
    
if version_info:
    print("\n=========================")
    print("Merge Request Version Information:")
    print(f"Base SHA: {version_info['base_commit_sha']}")
    print(f"Head SHA: {version_info['head_commit_sha']}")
    print(f"Start SHA: {version_info['start_commit_sha']}")
    
    # Example 1: Add a general comment
    general_comment = "This merge request looks good overall. Nice work!"
    general_result = add_general_comment(url, encoded_project_path, mr_id, headers, general_comment)
    
    if general_result:
        # Save the created thread details
        with open('general_comment.json', 'w') as f:
            json.dump(general_result, f, indent=2)
        print("General comment details saved to 'general_comment.json'")
    
    # Example 2: Add a line comment to a file in the MR
    line_comment = "This line could use some additional error handling."
    
    # Pass the changes data correctly
    changes_data_to_pass = None
    if 'changes' in locals() and changes is not None:
        changes_data_to_pass = changes
        
        # Get the list of changed files
        changed_files = []
        for i, change in enumerate(changes['changes']):
            changed_files.append(change['new_path'])
            
        if changed_files:
            print("\nAvailable files to comment on:")
            for i, file_path in enumerate(changed_files):
                print(f"{i+1}: {file_path}")
            
            # Choose the first file for the demo (you can modify this to select any file)
            selected_file = changed_files[0]
            line_number = 19  # Just use line 5 as an example
            
            print(f"\nAdding comment to first available file: {selected_file} at line {line_number}")
            
            line_result = add_line_comment(url, encoded_project_path, mr_id, headers, 
                                          selected_file, line_number, line_comment, 
                                          changes_data_to_pass)
            
            if line_result:
                # Save the created thread details
                with open('line_comment.json', 'w') as f:
                    json.dump(line_result, f, indent=2)
                print("Line comment details saved to 'line_comment.json'")
        else:
            print("No changed files found in this merge request to comment on")
    else:
        print("No changes data available to add a line comment")
    
else:
    print(f"Failed to fetch merge request. Status code: {response.status_code}")
    print(f"Error: {response.text}")