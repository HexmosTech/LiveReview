#!/usr/bin/env python3
"""
GitHub CLI tool for managing review comments and webhooks using the GitHub HTTP API.
"""

import argparse
import json
import sys
import time
import hashlib
import hmac
import threading
from typing import List, Dict, Optional
from urllib.parse import urlparse
from flask import Flask, request, jsonify
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
    
    def parse_github_url(self, url: str) -> tuple[str, str, Optional[int]]:
        """Parse GitHub URL to extract owner, repo, and optionally PR number."""
        parsed = urlparse(url)
        path_parts = parsed.path.strip('/').split('/')
        
        if len(path_parts) < 2:
            raise ValueError(f"Invalid GitHub URL: {url}")
        
        owner = path_parts[0]
        repo = path_parts[1]
        
        # Check if it's a PR URL
        if len(path_parts) >= 4 and path_parts[2] == 'pull':
            pr_number = int(path_parts[3])
            return owner, repo, pr_number
        
        # Regular repository URL
        return owner, repo, None
    
    def parse_repo_url(self, url: str) -> tuple[str, str]:
        """Parse GitHub repository URL to extract owner and repo."""
        owner, repo, _ = self.parse_github_url(url)
        return owner, repo
    
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
            if pr_number is None:
                raise ValueError("Invalid PR URL - PR number is required for creating comments")
            
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
            if pr_number is None:
                raise ValueError("Invalid PR URL - PR number is required for creating comments")
            
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
            if pr_number is None:
                raise ValueError("Invalid PR URL - PR number is required for deleting comments")
            
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

    def install_webhook(self, repo_url: str, webhook_url: str, secret: Optional[str] = None, 
                       events: Optional[List[str]] = None) -> bool:
        """Install a webhook on a GitHub repository."""
        try:
            owner, repo = self.parse_repo_url(repo_url)
            
            if events is None:
                events = [
                    'push',
                    'pull_request',
                    'pull_request_review',
                    'pull_request_review_comment',
                    'issue_comment',
                    'issues'
                ]
            
            webhook_config = {
                'url': webhook_url,
                'content_type': 'json',
                'insecure_ssl': '0'
            }
            
            if secret:
                webhook_config['secret'] = secret
            
            webhook_data = {
                'name': 'web',
                'active': True,
                'events': events,
                'config': webhook_config
            }
            
            url = f"{self.base_url}/repos/{owner}/{repo}/hooks"
            
            print(f"Installing webhook on {owner}/{repo}...")
            print(f"Webhook URL: {webhook_url}")
            print(f"Events: {', '.join(events)}")
            
            response = self.session.post(url, json=webhook_data)
            response.raise_for_status()
            
            webhook = response.json()
            print(f"Successfully installed webhook (ID: {webhook['id']})")
            print(f"Webhook URL: {webhook['config']['url']}")
            print(f"Events: {', '.join(webhook['events'])}")
            
            return True
            
        except requests.exceptions.RequestException as e:
            print(f"Failed to install webhook: {e}")
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
    
    def list_webhooks(self, repo_url: str) -> List[Dict]:
        """List all webhooks for a repository."""
        try:
            owner, repo = self.parse_repo_url(repo_url)
            
            url = f"{self.base_url}/repos/{owner}/{repo}/hooks"
            response = self.session.get(url)
            response.raise_for_status()
            
            webhooks = response.json()
            return webhooks
            
        except requests.exceptions.RequestException as e:
            print(f"Failed to list webhooks: {e}")
            return []
    
    def delete_webhook(self, repo_url: str, webhook_id: int) -> bool:
        """Delete a webhook from a repository."""
        try:
            owner, repo = self.parse_repo_url(repo_url)
            
            url = f"{self.base_url}/repos/{owner}/{repo}/hooks/{webhook_id}"
            response = self.session.delete(url)
            response.raise_for_status()
            
            print(f"Successfully deleted webhook (ID: {webhook_id})")
            return True
            
        except requests.exceptions.RequestException as e:
            print(f"Failed to delete webhook: {e}")
            return False


class WebhookListener:
    """Simple webhook listener using Flask."""
    
    def __init__(self, port: int = 8080, secret: Optional[str] = None):
        self.port = port
        self.secret = secret
        self.app = Flask(__name__)
        self.setup_routes()
    
    def setup_routes(self):
        """Setup Flask routes for webhook handling."""
        @self.app.route('/webhook', methods=['POST'])
        def handle_webhook():
            return self.process_webhook()
        
        @self.app.route('/health', methods=['GET'])
        def health_check():
            return jsonify({'status': 'healthy'}), 200
    
    def verify_signature(self, payload_body: bytes, signature_header: str) -> bool:
        """Verify GitHub webhook signature."""
        if not self.secret:
            return True  # Skip verification if no secret is set
        
        if not signature_header:
            return False
        
        hash_object = hmac.new(
            self.secret.encode('utf-8'),
            msg=payload_body,
            digestmod=hashlib.sha256
        )
        expected_signature = "sha256=" + hash_object.hexdigest()
        
        return hmac.compare_digest(expected_signature, signature_header)
    
    def process_webhook(self):
        """Process incoming webhook payload."""
        try:
            # Get the raw payload
            payload_body = request.get_data()
            signature_header = request.headers.get('X-Hub-Signature-256')
            
            # Verify signature if secret is configured
            if not self.verify_signature(payload_body, signature_header):
                print("ERROR: Invalid webhook signature")
                return jsonify({'error': 'Invalid signature'}), 401
            
            # Parse JSON payload
            payload = request.get_json()
            if not payload:
                print("ERROR: No JSON payload received")
                return jsonify({'error': 'No JSON payload'}), 400
            
            # Extract event information
            event_type = request.headers.get('X-GitHub-Event', 'unknown')
            delivery_id = request.headers.get('X-GitHub-Delivery', 'unknown')
            
            print(f"\n{'='*60}")
            print(f"WEBHOOK RECEIVED - {time.strftime('%Y-%m-%d %H:%M:%S')}")
            print(f"{'='*60}")
            print(f"Event Type: {event_type}")
            print(f"Delivery ID: {delivery_id}")
            print(f"Signature Verified: {signature_header is not None and self.secret is not None}")
            
            # Print repository information
            if 'repository' in payload:
                repo = payload['repository']
                print(f"Repository: {repo.get('full_name', 'unknown')}")
                print(f"Repository URL: {repo.get('html_url', 'unknown')}")
            
            # Print action if available
            if 'action' in payload:
                print(f"Action: {payload['action']}")
            
            # Event-specific information
            if event_type == 'push':
                self.handle_push_event(payload)
            elif event_type == 'pull_request':
                self.handle_pull_request_event(payload)
            elif event_type == 'pull_request_review':
                self.handle_pull_request_review_event(payload)
            elif event_type == 'pull_request_review_comment':
                self.handle_pull_request_review_comment_event(payload)
            elif event_type == 'issue_comment':
                self.handle_issue_comment_event(payload)
            elif event_type == 'issues':
                self.handle_issues_event(payload)
            else:
                print(f"Event type '{event_type}' - displaying full payload:")
                self.print_payload(payload)
            
            print(f"{'='*60}")
            
            return jsonify({'status': 'received'}), 200
            
        except Exception as e:
            print(f"ERROR processing webhook: {e}")
            return jsonify({'error': str(e)}), 500
    
    def handle_push_event(self, payload: Dict):
        """Handle push event."""
        print(f"Branch: {payload.get('ref', 'unknown')}")
        print(f"Commits: {len(payload.get('commits', []))}")
        
        if 'pusher' in payload:
            print(f"Pushed by: {payload['pusher'].get('name', 'unknown')}")
        
        if 'head_commit' in payload and payload['head_commit']:
            commit = payload['head_commit']
            print(f"Head commit: {commit.get('id', 'unknown')[:8]}")
            print(f"Commit message: {commit.get('message', 'No message')[:100]}")
    
    def handle_pull_request_event(self, payload: Dict):
        """Handle pull request event."""
        if 'pull_request' in payload:
            pr = payload['pull_request']
            print(f"PR #{pr.get('number', 'unknown')}: {pr.get('title', 'No title')}")
            print(f"State: {pr.get('state', 'unknown')}")
            print(f"Author: {pr.get('user', {}).get('login', 'unknown')}")
            print(f"PR URL: {pr.get('html_url', 'unknown')}")
    
    def handle_pull_request_review_event(self, payload: Dict):
        """Handle pull request review event."""
        if 'review' in payload:
            review = payload['review']
            print(f"Review ID: {review.get('id', 'unknown')}")
            print(f"Review state: {review.get('state', 'unknown')}")
            print(f"Reviewer: {review.get('user', {}).get('login', 'unknown')}")
        
        if 'pull_request' in payload:
            pr = payload['pull_request']
            print(f"PR #{pr.get('number', 'unknown')}: {pr.get('title', 'No title')}")
    
    def handle_pull_request_review_comment_event(self, payload: Dict):
        """Handle pull request review comment event."""
        if 'comment' in payload:
            comment = payload['comment']
            print(f"Comment ID: {comment.get('id', 'unknown')}")
            print(f"Author: {comment.get('user', {}).get('login', 'unknown')}")
            print(f"File: {comment.get('path', 'unknown')}")
            print(f"Line: {comment.get('line', 'unknown')}")
            print(f"Comment: {comment.get('body', 'No body')[:100]}")
        
        if 'pull_request' in payload:
            pr = payload['pull_request']
            print(f"PR #{pr.get('number', 'unknown')}: {pr.get('title', 'No title')}")
    
    def handle_issue_comment_event(self, payload: Dict):
        """Handle issue comment event."""
        if 'comment' in payload:
            comment = payload['comment']
            print(f"Comment ID: {comment.get('id', 'unknown')}")
            print(f"Author: {comment.get('user', {}).get('login', 'unknown')}")
            print(f"Comment: {comment.get('body', 'No body')[:100]}")
        
        if 'issue' in payload:
            issue = payload['issue']
            print(f"Issue #{issue.get('number', 'unknown')}: {issue.get('title', 'No title')}")
    
    def handle_issues_event(self, payload: Dict):
        """Handle issues event."""
        if 'issue' in payload:
            issue = payload['issue']
            print(f"Issue #{issue.get('number', 'unknown')}: {issue.get('title', 'No title')}")
            print(f"State: {issue.get('state', 'unknown')}")
            print(f"Author: {issue.get('user', {}).get('login', 'unknown')}")
    
    def print_payload(self, payload: Dict, max_depth: int = 3, current_depth: int = 0):
        """Print payload with limited depth to avoid overwhelming output."""
        if current_depth >= max_depth:
            print("... (truncated)")
            return
        
        for key, value in payload.items():
            if isinstance(value, dict):
                print(f"{'  ' * current_depth}{key}:")
                self.print_payload(value, max_depth, current_depth + 1)
            elif isinstance(value, list) and value:
                print(f"{'  ' * current_depth}{key}: [{len(value)} items]")
                if isinstance(value[0], dict):
                    print(f"{'  ' * (current_depth + 1)}First item:")
                    self.print_payload(value[0], max_depth, current_depth + 2)
                else:
                    print(f"{'  ' * (current_depth + 1)}{value[0]}")
            else:
                # Truncate long strings
                if isinstance(value, str) and len(value) > 100:
                    value = value[:97] + "..."
                print(f"{'  ' * current_depth}{key}: {value}")
    
    def start(self):
        """Start the webhook listener."""
        print(f"Starting webhook listener on port {self.port}")
        print(f"Webhook endpoint: http://localhost:{self.port}/webhook")
        print(f"Health check: http://localhost:{self.port}/health")
        print(f"Secret configured: {'Yes' if self.secret else 'No'}")
        print("Press Ctrl+C to stop\n")
        
        try:
            self.app.run(host='0.0.0.0', port=self.port, debug=False)
        except KeyboardInterrupt:
            print("\nWebhook listener stopped.")


def start_webhook_listener(port: int, secret: Optional[str] = None):
    """Start the webhook listener."""
    listener = WebhookListener(port, secret)
    listener.start()


def main():
    """Main CLI entry point."""
    parser = argparse.ArgumentParser(
        description='GitHub CLI tool for managing review comments and webhooks',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Delete all comments (both review and general PR comments)
  %(prog)s delete https://github.com/owner/repo/pull/123
  %(prog)s delete --token YOUR_TOKEN https://github.com/owner/repo/pull/123
  
  # Create line comments on specific files
  %(prog)s create https://github.com/owner/repo/pull/123 src/main.go "This needs refactoring" --new-line 42
  %(prog)s create https://github.com/owner/repo/pull/123 README.md "Why was this removed?" --old-line 15
  
  # Install webhook on a repository
  %(prog)s install-webhook https://github.com/owner/repo http://yourserver.com/webhook --secret mysecret
  
  # List existing webhooks
  %(prog)s list-webhooks https://github.com/owner/repo
  
  # Delete a webhook
  %(prog)s delete-webhook https://github.com/owner/repo 12345
  
  # Start webhook listener
  %(prog)s listen --port 8080 --secret mysecret
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
        default='REDACTED_GITHUB_PAT_3'
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
    
    # Install webhook subcommand
    install_webhook_parser = subparsers.add_parser(
        'install-webhook',
        help='Install a webhook on a GitHub repository'
    )
    install_webhook_parser.add_argument(
        'repo_url',
        help='GitHub repository URL (e.g., https://github.com/owner/repo)'
    )
    install_webhook_parser.add_argument(
        'webhook_url',
        help='URL where GitHub should send webhook payloads'
    )
    install_webhook_parser.add_argument(
        '--secret',
        help='Secret for webhook signature verification'
    )
    install_webhook_parser.add_argument(
        '--events',
        nargs='+',
        help='List of events to subscribe to (default: push, pull_request, etc.)'
    )
    install_webhook_parser.add_argument(
        '--token',
        help='GitHub personal access token (or set GITHUB_TOKEN env var)',
        default='REDACTED_GITHUB_PAT_2'
    )
    
    # List webhooks subcommand
    list_webhooks_parser = subparsers.add_parser(
        'list-webhooks',
        help='List all webhooks for a repository'
    )
    list_webhooks_parser.add_argument(
        'repo_url',
        help='GitHub repository URL (e.g., https://github.com/owner/repo)'
    )
    list_webhooks_parser.add_argument(
        '--token',
        help='GitHub personal access token (or set GITHUB_TOKEN env var)',
        default='REDACTED_GITHUB_PAT_2'
    )
    
    # Delete webhook subcommand
    delete_webhook_parser = subparsers.add_parser(
        'delete-webhook',
        help='Delete a webhook from a repository'
    )
    delete_webhook_parser.add_argument(
        'repo_url',
        help='GitHub repository URL (e.g., https://github.com/owner/repo)'
    )
    delete_webhook_parser.add_argument(
        'webhook_id',
        type=int,
        help='ID of the webhook to delete'
    )
    delete_webhook_parser.add_argument(
        '--token',
        help='GitHub personal access token (or set GITHUB_TOKEN env var)',
        default='REDACTED_GITHUB_PAT_2'
    )
    
    # Webhook listener subcommand
    listen_parser = subparsers.add_parser(
        'listen',
        help='Start a webhook listener server'
    )
    listen_parser.add_argument(
        '--port',
        type=int,
        default=8080,
        help='Port to listen on (default: 8080)'
    )
    listen_parser.add_argument(
        '--secret',
        help='Secret for webhook signature verification'
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
    
    elif args.command == 'install-webhook':
        if not args.token:
            print("Error: GitHub token is required. Use --token or set GITHUB_TOKEN environment variable.")
            sys.exit(1)
        
        api = GitHubAPI(args.token)
        success = api.install_webhook(
            args.repo_url,
            args.webhook_url,
            args.secret,
            args.events
        )
        if not success:
            sys.exit(1)
    
    elif args.command == 'list-webhooks':
        if not args.token:
            print("Error: GitHub token is required. Use --token or set GITHUB_TOKEN environment variable.")
            sys.exit(1)
        
        api = GitHubAPI(args.token)
        webhooks = api.list_webhooks(args.repo_url)
        
        if not webhooks:
            print("No webhooks found.")
        else:
            print(f"Found {len(webhooks)} webhook(s):")
            for webhook in webhooks:
                print(f"\nWebhook ID: {webhook['id']}")
                print(f"URL: {webhook['config']['url']}")
                print(f"Events: {', '.join(webhook['events'])}")
                print(f"Active: {webhook['active']}")
                print(f"Created: {webhook['created_at']}")
                print(f"Updated: {webhook['updated_at']}")
    
    elif args.command == 'delete-webhook':
        if not args.token:
            print("Error: GitHub token is required. Use --token or set GITHUB_TOKEN environment variable.")
            sys.exit(1)
        
        api = GitHubAPI(args.token)
        success = api.delete_webhook(args.repo_url, args.webhook_id)
        if not success:
            sys.exit(1)
    
    elif args.command == 'listen':
        start_webhook_listener(args.port, args.secret)


if __name__ == '__main__':
    main()
