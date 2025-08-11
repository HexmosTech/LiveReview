# Bitbucket Pull Request Comment Deletion Script

This script deletes all visible comments in a specified Bitbucket pull request using the Bitbucket REST API.

## Features

- Fetches all comments from a specified pull request
- Deletes each comment individually
- Handles pagination for pull requests with many comments
- Provides detailed progress and summary information
- Error handling for failed deletions

## Prerequisites

- Python 3.6 or higher
- `requests` library
- Valid Bitbucket API key with appropriate permissions

## Installation

1. Install the required dependencies:
   ```bash
   pip install -r requirements.txt
   ```

## Configuration

The script is pre-configured with:
- **API Key**: Your Bitbucket API key (hardcoded in the script)
- **Pull Request URL**: The target pull request URL (hardcoded in the script)

## Usage

Run the script:
```bash
python scripts/delete_pr_comments.py
```

Or make it executable and run directly:
```bash
chmod +x scripts/delete_pr_comments.py
./scripts/delete_pr_comments.py
```

## Output

The script will display:
- Repository information (workspace, repository name, PR ID)
- Number of comments found
- Progress for each comment deletion
- Summary of successful and failed deletions

## API Permissions Required

Your Bitbucket API key needs the following permissions:
- Read access to the repository
- Write access to delete comments on pull requests

## Security Note

⚠️ **Warning**: This script contains your API key hardcoded in the source code. For production use, consider:
- Using environment variables for the API key
- Storing the API key in a secure configuration file
- Using Bitbucket App passwords instead of personal access tokens

## Error Handling

The script handles various error scenarios:
- Invalid pull request URL format
- API authentication failures
- Network connectivity issues
- Individual comment deletion failures

## Example Output

```
Bitbucket Pull Request Comment Deletion Script
==================================================
Workspace: contorted
Repository: fb_backends
Pull Request ID: 1

Fetching comments...
Found 5 comments to delete.

Deleting comment 12345: This is a test comment...
  ✓ Successfully deleted comment 12345
Deleting comment 12346: Another comment here...
  ✓ Successfully deleted comment 12346

Deletion Summary:
  Total comments: 5
  Successfully deleted: 5
  Failed to delete: 0
All comments have been successfully deleted!
``` 