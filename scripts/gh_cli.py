#!/usr/bin/env python3
"""
GitHub CLI tool for managing review comments using the GitHub HTTP API.
"""

import argparse
import json
import sys
import time
from typing import List, Dict, Optional
from urllib.parse import urlparse
import requests


class GitHubAPI:
    """GitHub API client for managing review comments."""
    
    def __init__(self, token: str):
        self.token = token
        self.session = requests.Session()
        self.session.headers.update({
            'Authorization': f'token {token}',
            'Accept': 'application/vnd.github.v3+json',
            'User-Agent': 'LiveReview-CLI/1.0'
        })
        self.base_url = 'https://api.github.com'
    
    def parse_github_url(self, url: str) -> tuple[str, str, int]:
        """Parse GitHub PR URL to extract owner, repo, and PR number."""
        parsed = urlparse(url)
        path_parts = parsed.path.strip('/').split('/')
        
        if len(path_parts) < 4 or path_parts[2] != 'pull':
            raise ValueError(f"Invalid GitHub PR URL: {url}")
        
        owner = path_parts[0]
        repo = path_parts[1]
        pr_number = int(path_parts[3])
        
        return owner, repo, pr_number
    
    def get_review_comments(self, owner: str, repo: str, pr_number: int) -> List[Dict]:
        """Get all review comments (line comments) for a pull request."""
        url = f"{self.base_url}/repos/{owner}/{repo}/pulls/{pr_number}/comments"
        all_comments = []
        page = 1
        
        while True:
            response = self.session.get(url, params={'page': page, 'per_page': 100})
            response.raise_for_status()
            
            comments = response.json()
            if not comments:
                break
                
            all_comments.extend(comments)
            page += 1
            
            # Rate limiting
            time.sleep(0.1)
        
        return all_comments
    
    def get_issue_comments(self, owner: str, repo: str, pr_number: int) -> List[Dict]:
        """Get all issue comments (general PR comments) for a pull request."""
        url = f"{self.base_url}/repos/{owner}/{repo}/issues/{pr_number}/comments"
        all_comments = []
        page = 1
        
        while True:
            response = self.session.get(url, params={'page': page, 'per_page': 100})
            response.raise_for_status()
            
            comments = response.json()
            if not comments:
                break
                
            all_comments.extend(comments)
            page += 1
            
            # Rate limiting
            time.sleep(0.1)
        
        return all_comments
    
    def delete_review_comment(self, owner: str, repo: str, comment_id: int) -> bool:
        """Delete a single review comment."""
        url = f"{self.base_url}/repos/{owner}/{repo}/pulls/comments/{comment_id}"
        
        try:
            response = self.session.delete(url)
            response.raise_for_status()
            return True
        except requests.exceptions.RequestException as e:
            print(f"Failed to delete review comment {comment_id}: {e}")
            return False
    
    def delete_issue_comment(self, owner: str, repo: str, comment_id: int) -> bool:
        """Delete a single issue comment (general PR comment)."""
        url = f"{self.base_url}/repos/{owner}/{repo}/issues/comments/{comment_id}"
        
        try:
            response = self.session.delete(url)
            response.raise_for_status()
            return True
        except requests.exceptions.RequestException as e:
            print(f"Failed to delete issue comment {comment_id}: {e}")
            return False
    
    def get_pending_review(self, owner: str, repo: str, pr_number: int) -> Optional[Dict]:
        """Get the current user's pending review if it exists."""
        try:
            url = f"{self.base_url}/repos/{owner}/{repo}/pulls/{pr_number}/reviews"
            response = self.session.get(url)
            response.raise_for_status()
            
            reviews = response.json()
            for review in reviews:
                if review.get('state') == 'PENDING':
                    return review
            return None
            
        except requests.exceptions.RequestException:
            return None
    
    def submit_pending_review(self, owner: str, repo: str, pr_number: int, review_id: int) -> bool:
        """Submit a pending review."""
        try:
            url = f"{self.base_url}/repos/{owner}/{repo}/pulls/{pr_number}/reviews/{review_id}/events"
            data = {
                'event': 'COMMENT',
                'body': 'Submitting pending review to allow new comments.'
            }
            
            response = self.session.post(url, json=data)
            response.raise_for_status()
            return True
            
        except requests.exceptions.RequestException as e:
            print(f"Failed to submit pending review: {e}")
            return False
    
    def delete_pending_review(self, owner: str, repo: str, pr_number: int, review_id: int) -> bool:
        """Delete a pending review."""
        try:
            url = f"{self.base_url}/repos/{owner}/{repo}/pulls/{pr_number}/reviews/{review_id}"
            response = self.session.delete(url)
            response.raise_for_status()
            return True
            
        except requests.exceptions.RequestException as e:
            print(f"Failed to delete pending review: {e}")
            return False

    def create_review_comment(self, pr_url: str, filepath: str, comment_text: str, 
                             old_line: Optional[int] = None, 
                             new_line: Optional[int] = None) -> bool:
        """Create a review comment on a specific line of code."""
        try:
            # Parse PR URL to get owner, repo, and PR number
            owner, repo, pr_number = self.parse_github_url(pr_url)
            
            # Check for pending reviews first
            pending_review = self.get_pending_review(owner, repo, pr_number)
            if pending_review:
                print(f"Found pending review (ID: {pending_review['id']})")
                choice = input("What would you like to do?\n1. Submit pending review and create comment\n2. Delete pending review and create comment\n3. Cancel\nEnter choice (1/2/3): ")
                
                if choice == '1':
                    print("Submitting pending review...")
                    if not self.submit_pending_review(owner, repo, pr_number, pending_review['id']):
                        return False
                elif choice == '2':
                    print("Deleting pending review...")
                    if not self.delete_pending_review(owner, repo, pr_number, pending_review['id']):
                        return False
                else:
                    print("Operation cancelled.")
                    return False
            
            # Get the latest commit SHA for the PR
            pr_url_api = f"{self.base_url}/repos/{owner}/{repo}/pulls/{pr_number}"
            pr_response = self.session.get(pr_url_api)
            pr_response.raise_for_status()
            pr_data = pr_response.json()
            commit_sha = pr_data['head']['sha']
            
            # Use the single review comment API endpoint
            url = f"{self.base_url}/repos/{owner}/{repo}/pulls/{pr_number}/comments"
            
            # Prepare the comment data
            comment_data = {
                'body': comment_text,
                'commit_id': commit_sha,
                'path': filepath
            }
            
            # Set the line position based on old_line or new_line
            if new_line is not None:
                comment_data['line'] = new_line
                comment_data['side'] = 'RIGHT'  # Comment on the new version
            elif old_line is not None:
                comment_data['line'] = old_line
                comment_data['side'] = 'LEFT'   # Comment on the old version
            else:
                raise ValueError("Either old_line or new_line must be specified")
            
            print("Creating review comment...")
            response = self.session.post(url, json=comment_data)
            response.raise_for_status()
            
            comment = response.json()
            print(f"Successfully created review comment (ID: {comment['id']})")
            print(f"Comment URL: {comment['html_url']}")
            return True
            
        except requests.exceptions.RequestException as e:
            print(f"Failed to create review comment: {e}")
            if hasattr(e, 'response') and e.response is not None:
                try:
                    error_data = e.response.json()
                    print(f"Error details: {error_data}")
                except:
                    print(f"Response text: {e.response.text}")
            return False
        except Exception as e:
            print(f"Error: {e}")
            return False
    
    def create_comment_from_args(self, pr_url: str, filepath: str, comment_text: str,
                                old_line: Optional[int] = None, 
                                new_line: Optional[int] = None) -> None:
        """Create a review comment from command line arguments."""
        try:
            owner, repo, pr_number = self.parse_github_url(pr_url)
            print(f"Creating comment on PR: {owner}/{repo}#{pr_number}")
            print(f"File: {filepath}")
            
            if old_line is not None:
                print(f"Line: {old_line} (deleted line in original)")
            elif new_line is not None:
                print(f"Line: {new_line} (added line in new version)")
            
            print(f"Comment: {comment_text[:100]}{'...' if len(comment_text) > 100 else ''}")
            
            # Confirm creation
            confirm = input(f"\nCreate this review comment? (y/N): ")
            if confirm.lower() != 'y':
                print("Comment creation cancelled.")
                return
            
            success = self.create_review_comment(
                pr_url, filepath, comment_text, old_line, new_line
            )
            
            if not success:
                sys.exit(1)
                
        except Exception as e:
            print(f"Error: {e}")
            sys.exit(1)

    def delete_all_review_comments(self, pr_url: str) -> None:
        """Delete all review comments from a pull request."""
        try:
            owner, repo, pr_number = self.parse_github_url(pr_url)
            print(f"Processing PR: {owner}/{repo}#{pr_number}")
            
            # Get all review comments (line comments)
            print("Fetching review comments (line comments)...")
            review_comments = self.get_review_comments(owner, repo, pr_number)
            
            # Get all issue comments (general PR comments)
            print("Fetching issue comments (general PR comments)...")
            issue_comments = self.get_issue_comments(owner, repo, pr_number)
            
            total_comments = len(review_comments) + len(issue_comments)
            
            if total_comments == 0:
                print("No comments found.")
                return
            
            print(f"Found {len(review_comments)} review comments and {len(issue_comments)} issue comments.")
            print(f"Total: {total_comments} comments")
            
            # Show some sample comments for confirmation
            if issue_comments:
                print("\nSample issue comments:")
                for i, comment in enumerate(issue_comments[:3]):
                    author = comment.get('user', {}).get('login', 'unknown')
                    body = comment.get('body', '')[:100] + '...' if len(comment.get('body', '')) > 100 else comment.get('body', '')
                    print(f"  [{i+1}] By {author}: {body}")
                if len(issue_comments) > 3:
                    print(f"  ... and {len(issue_comments) - 3} more issue comments")
            
            if review_comments:
                print(f"\nSample review comments:")
                for i, comment in enumerate(review_comments[:3]):
                    author = comment.get('user', {}).get('login', 'unknown')
                    body = comment.get('body', '')[:100] + '...' if len(comment.get('body', '')) > 100 else comment.get('body', '')
                    print(f"  [{i+1}] By {author}: {body}")
                if len(review_comments) > 3:
                    print(f"  ... and {len(review_comments) - 3} more review comments")
            
            # Confirm deletion
            confirm = input(f"\nAre you sure you want to delete all {total_comments} comments? (y/N): ")
            if confirm.lower() != 'y':
                print("Deletion cancelled.")
                return
            
            # Delete comments
            deleted_count = 0
            failed_count = 0
            
            # Delete issue comments first
            for i, comment in enumerate(issue_comments, 1):
                comment_id = comment['id']
                author = comment.get('user', {}).get('login', 'unknown')
                print(f"Deleting issue comment {i}/{len(issue_comments)} (ID: {comment_id}, by {author})...")
                
                if self.delete_issue_comment(owner, repo, comment_id):
                    deleted_count += 1
                else:
                    failed_count += 1
                
                # Rate limiting
                time.sleep(0.5)
            
            # Delete review comments
            for i, comment in enumerate(review_comments, 1):
                comment_id = comment['id']
                author = comment.get('user', {}).get('login', 'unknown')
                print(f"Deleting review comment {i}/{len(review_comments)} (ID: {comment_id}, by {author})...")
                
                if self.delete_review_comment(owner, repo, comment_id):
                    deleted_count += 1
                else:
                    failed_count += 1
                
                # Rate limiting
                time.sleep(0.5)
            
            print(f"\nDeletion complete:")
            print(f"  Successfully deleted: {deleted_count}")
            print(f"  Failed to delete: {failed_count}")
            
        except Exception as e:
            print(f"Error: {e}")
            sys.exit(1)


def main():
    """Main CLI entry point."""
    parser = argparse.ArgumentParser(
        description='GitHub CLI tool for managing review comments',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Delete all comments (both review and general PR comments)
  %(prog)s delete https://github.com/owner/repo/pull/123
  %(prog)s delete --token YOUR_TOKEN https://github.com/owner/repo/pull/123
  
  # Create line comments on specific files
  %(prog)s create https://github.com/owner/repo/pull/123 src/main.go "This needs refactoring" --new-line 42
  %(prog)s create https://github.com/owner/repo/pull/123 README.md "Why was this removed?" --old-line 15
        """
    )
    
    subparsers = parser.add_subparsers(dest='command', help='Available commands')
    
    # Delete subcommand
    delete_parser = subparsers.add_parser(
        'delete',
        help='Delete all comments from a pull request (both review comments and general PR comments)'
    )
    delete_parser.add_argument(
        'pr_url',
        help='GitHub pull request URL (e.g., https://github.com/owner/repo/pull/123)'
    )
    delete_parser.add_argument(
        '--token',
        help='GitHub personal access token (or set GITHUB_TOKEN env var)',
        default='REDACTED_GITHUB_PAT_2'  # Default token from original file
    )
    
    # Create subcommand
    create_parser = subparsers.add_parser(
        'create',
        help='Create a review comment on a specific line of code'
    )
    create_parser.add_argument(
        'pr_url',
        help='GitHub pull request URL (e.g., https://github.com/owner/repo/pull/123)'
    )
    create_parser.add_argument(
        'filepath',
        help='Path to the file in the repository (e.g., src/main.go)'
    )
    create_parser.add_argument(
        'comment_text',
        help='The comment text to post'
    )
    
    # Line specification - either old_line OR new_line
    line_group = create_parser.add_mutually_exclusive_group(required=True)
    line_group.add_argument(
        '--old-line',
        type=int,
        help='Line number in the original file (for deleted lines)'
    )
    line_group.add_argument(
        '--new-line', 
        type=int,
        help='Line number in the new file (for added lines)'
    )
    
    create_parser.add_argument(
        '--token',
        help='GitHub personal access token (or set GITHUB_TOKEN env var)',
        default='REDACTED_GITHUB_PAT_2'  # Default token from original file
    )
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        sys.exit(1)
    
    if args.command == 'delete':
        if not args.token:
            print("Error: GitHub token is required. Use --token or set GITHUB_TOKEN environment variable.")
            sys.exit(1)
        
        api = GitHubAPI(args.token)
        api.delete_all_review_comments(args.pr_url)
        
    elif args.command == 'create':
        if not args.token:
            print("Error: GitHub token is required. Use --token or set GITHUB_TOKEN environment variable.")
            sys.exit(1)
        
        api = GitHubAPI(args.token)
        api.create_comment_from_args(
            args.pr_url, 
            args.filepath, 
            args.comment_text,
            args.old_line,
            args.new_line
        )


if __name__ == '__main__':
    main()
