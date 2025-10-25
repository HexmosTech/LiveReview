import requests

# Replace these
GITHUB_TOKEN = "REDACTED_GITHUB_PAT_3"
OWNER = "livereviewbot"
REPO = "glabmig"
PR_NUMBER = 2

HEADERS = {
    "Authorization": f"token {GITHUB_TOKEN}",
    "Accept": "application/vnd.github.v3+json",
}

def fetch_all(url):
    results = []
    while url:
        resp = requests.get(url, headers=HEADERS)
        resp.raise_for_status()
        results.extend(resp.json())
        # GitHub paginates using 'Link' header
        link = resp.headers.get('Link')
        if link and 'rel="next"' in link:
            url = [l.split(";")[0].strip("<> ") for l in link.split(",") if 'rel="next"' in l][0]
        else:
            url = None
    return results

# 1. Issue comments (general top-level comments on the PR)
issue_comments_url = f"https://api.github.com/repos/{OWNER}/{REPO}/issues/{PR_NUMBER}/comments"
issue_comments = fetch_all(issue_comments_url)

# 2. Review comments (comments on diffs/lines in the PR)
review_comments_url = f"https://api.github.com/repos/{OWNER}/{REPO}/pulls/{PR_NUMBER}/comments"
review_comments = fetch_all(review_comments_url)

# 3. Pull request reviews (with body and submitted_at info)
reviews_url = f"https://api.github.com/repos/{OWNER}/{REPO}/pulls/{PR_NUMBER}/reviews"
reviews = fetch_all(reviews_url)

# Combine all
all_comments = {
    "issue_comments": issue_comments,
    "review_comments": review_comments,
    "reviews": reviews,
}

print(f"Total issue comments: {len(issue_comments)}")
print(f"Total review comments: {len(review_comments)}")
print(f"Total reviews: {len(reviews)}")

# Optionally save to JSON
import json
with open(f"pr_{PR_NUMBER}_all_comments.json", "w") as f:
    json.dump(all_comments, f, indent=2)

