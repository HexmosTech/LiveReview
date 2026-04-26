# Bitbucket API Migration: CHANGE-2770

## Replacing Deprecated API

1. Issue found in finding repository and webhook integration.
2. The error-returning APIs are deprecated, and here is the link to the deprecated API's: [Link](https://developer.atlassian.com/cloud/bitbucket/changelog/#CHANGE-2770)

## How this was fixed and what changes are made?

1. Updated these API with new API endpoints [Link](https://developer.atlassian.com/cloud/bitbucket/changelog/#CHANGE-3022:~:text=ANNOUNCEMENT,per%20app%20installation.)
2. The API endpoint should have new scope `read:workspace:bitbucket`  
3. Updated network doc and validated function pointing.

## Detailed API Flow Changes in LiveReview

To comply with this deprecation, our project discovery flow in `internal/providers/bitbucket/project_discovery.go` underwent a significant refactor from a single-call pattern to a multi-step iterative pattern.

### Old Deprecated Flow
Previously, LiveReview could retrieve data globally across all workspaces in a single step using cross-workspace endpoints:
- **`GET /2.0/workspaces`** (Removed)
- **`GET /2.0/repositories`** (Removed) 
- **`GET /2.0/user/permissions/workspaces`** (Removed)

*Why it failed:* Atlassian physically removed these endpoints, causing them to return `410 Gone` HTTP errors.

### New Compliant Flow (CHANGE-2770 & CHANGE-3022)
We have migrated to **workspace-scoped** API endpoints. The retrieval process is now broken down into sequential steps:

1. **Discover Accessible Workspaces**: 
   - **Endpoint:** `GET /2.0/user/workspaces`
   - **Action:** LiveReview first calls this new endpoint to fetch every workspace the authenticated user is a member of. This specifically relies on the `read:workspace:bitbucket` token scope.
   
2. **Iterative Repository Discovery**:
   - **Endpoint:** `GET /2.0/repositories/{workspace}?role=member`
   - **Action:** Instead of one massive global query, the code iterates through every `slug` returned in Step 1. For each workspace, it performs a separate scoped API call to list the repositories belonging to that specific workspace. 
   - **Handling Permissions:** Because the `GET /2.0/user/permissions/workspaces` endpoint was removed, we now enforce permissions at the repository query level. By appending the `?role=member` query parameter to the repositories endpoint, Bitbucket automatically filters the response to only return repositories where the user has explicit member permissions, completely replacing the need for a separate permissions check beforehand.

These changes are strictly required by Bitbucket Cloud to maintain security, performance, and per-app installation constraints.
