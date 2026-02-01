from pprint import pprint
import requests
import re
import json
import os

url = "https://git.apps.hexmos.com"
# Test token for git.apps.hexmos.com - hardcoded for testing only
token = "REDACTED_GITLAB_PAT_6"

mr_url = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/416"

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

# Function to delete a single note in GitLab
def delete_note(base_url, project_path, mr_id, note_id, headers):
    delete_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/notes/{note_id}"
    response = requests.delete(delete_url, headers=headers)
    
    if response.status_code in [200, 204]:
        print(f"Successfully deleted note with ID: {note_id}")
        return True
    else:
        print(f"Failed to delete note with ID: {note_id}. Status code: {response.status_code}")
        print(f"Error: {response.text}")
        return False

# Function to delete all visible comments in a merge request
def delete_all_visible_comments(base_url, project_path, mr_id, headers):
    # Fetch all discussions in the merge request
    discussions_url = f"{base_url}/api/v4/projects/{project_path}/merge_requests/{mr_id}/discussions"
    discussions_response = requests.get(discussions_url, headers=headers)
    
    if discussions_response.status_code != 200:
        print(f"Failed to fetch discussions. Status code: {discussions_response.status_code}")
        print(f"Error: {discussions_response.text}")
        return False
    
    discussions = discussions_response.json()
    visible_notes = []
    
    # Collect all visible notes (non-system notes)
    for discussion in discussions:
        for note in discussion['notes']:
            # Filter out system notes which aren't visible as "comments" in the UI
            if not note.get('system', False):
                visible_notes.append(note)
    
    if not visible_notes:
        print("No visible comments found to delete")
        return True
    
    print(f"Found {len(visible_notes)} visible comments to delete")
    
    # Delete each visible note
    success_count = 0
    failed_count = 0
    
    for note in visible_notes:
        note_id = note['id']
        if delete_note(base_url, project_path, mr_id, note_id, headers):
            success_count += 1
        else:
            failed_count += 1
    
    print(f"\nComments deletion summary:")
    print(f"Successfully deleted: {success_count}")
    print(f"Failed to delete: {failed_count}")
    
    return failed_count == 0

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

# Main script execution
def main():
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
        
        # Delete all visible comments automatically
        print("\n=========================")
        print("Starting deletion of all visible comments in the merge request...")
        print(f"Automatically deleting all visible comments from merge request: {mr_url}")
        
        # Call the function to delete all visible comments without asking for confirmation
        success = delete_all_visible_comments(url, encoded_project_path, mr_id, headers)
        
        if success:
            print("\nAll visible comments have been successfully deleted.")
        else:
            print("\nSome comments could not be deleted. See the errors above for details.")
    else:
        print(f"Failed to fetch merge request. Status code: {response.status_code}")
        print(f"Error: {response.text}")

if __name__ == "__main__":
    main()
