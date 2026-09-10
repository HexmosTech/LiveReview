# Settings → User Management

**Route(s):** `/settings#users`, `/settings/users/add`,
`/settings/users/add/bulk`, `/settings/users/edit/:userId`
**Component:** tab in `Settings.tsx`; forms in
`ui/src/components/UserManagement/UserForm.tsx`
**Who sees it:** any org member (read-only for non-owners)

## Purpose

Manage the org's user roster — invite new users, assign roles
(`super_admin`/`owner`/`member`), deactivate users, force password resets.

**This is the one and only place a new team member is added.** There is no
seat to buy or assign first: per-seat licensing is deprecated and an owner can
add unlimited users here. Nothing in Settings -> License, Team Checkout, or
License Assignment is part of onboarding someone.

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

## Learn more (public docs)

- [Collaboration — team rollout patterns that keep adoption smooth](https://hexmos.com/livereview/docs/git-lrc/concepts/collaboration)
- [Roles — suggested responsibilities when adopting LiveReview](https://hexmos.com/livereview/docs/git-lrc/concepts/roles)
