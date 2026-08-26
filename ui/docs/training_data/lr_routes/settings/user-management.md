# Settings → User Management

**Route(s):** `/settings#users`, `/settings/users/add`,
`/settings/users/add/bulk`, `/settings/users/edit/:userId`
**Component:** tab in `Settings.tsx`; forms in
`ui/src/components/UserManagement/UserForm.tsx`
**Who sees it:** any org member (read-only for non-owners)

## Purpose

Manage the org's user roster — invite new users, assign roles
(`super_admin`/`owner`/`member`), deactivate users, force password resets.

## Key actions

- View org members and their roles.
- Add a single user (`/settings/users/add`) or bulk-import users
  (`/settings/users/add/bulk`) — owner action.
- Edit a user's role or details (`/settings/users/edit/:userId`) — owner
  action.
- Deactivate a user or force a password reset — owner action.
- Non-owners see the roster read-only.

## Related pages

[Settings overview](settings-overview.md)
