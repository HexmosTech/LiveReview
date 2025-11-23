# Create Organization Feature

## Overview
This feature allows super administrators to create new organizations through the UI.

## Components

### CreateOrganizationModal
Location: `/ui/src/components/CreateOrganizationModal/CreateOrganizationModal.tsx`

A modal dialog that allows super admins to create new organizations with:
- **Name** (required, max 255 characters)
- **Description** (optional, max 1000 characters)

### Integration with OrganizationSelector
The "Create Organization" option appears in the organization selector dropdown for super admins.

## API Endpoint

**POST** `/api/v1/admin/organizations`

### Request Body
```json
{
  "name": "Organization Name",
  "description": "Optional description"
}
```

### Response
```json
{
  "message": "Organization created successfully",
  "organization": {
    "id": 1,
    "name": "Organization Name",
    "description": "Optional description",
    "is_active": true,
    "created_at": "2025-11-23T...",
    "updated_at": "2025-11-23T...",
    "created_by_user_id": 1
  }
}
```

## Backend Behavior

When a new organization is created:
1. Organization record is created in the `orgs` table
2. Creator is automatically assigned as **owner** via `user_roles` table
3. Default prompt context is created in `prompt_application_context` table

## User Experience

1. Super admin clicks on the organization selector in the navbar
2. At the bottom of the dropdown, they see "Create Organization" option (green text with + icon)
3. Clicking it opens a modal with a form
4. They fill in the organization name (required) and optional description
5. Clicking "Create Organization" button:
   - Shows loading state
   - Sends request to backend
   - On success: modal closes, organization list refreshes automatically
   - On error: shows error message in the modal
6. The new organization appears in the organization selector list

## Validation

### Frontend
- Name: required, max 255 characters
- Description: optional, max 1000 characters
- Character count shown for description

### Backend
- Name: required, 1-255 characters
- Description: optional, max 1000 characters
- Super admin authentication required

## Redux State Management

The feature integrates with the Organizations Redux slice:
- `createOrganization` async thunk handles the API call
- `loading.creating` tracks submission state
- On success, new org is added to `userOrganizations` array
- `loadUserOrganizations` is called to refresh the full list

## Permissions

Only users with `super_admin` role can:
- See the "Create Organization" option
- Access the `/api/v1/admin/organizations` endpoint
