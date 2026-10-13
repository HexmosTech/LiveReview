# Explore → Repositories → Repository settings

**Route:** `/explore/repositories/:repoId/settings`
**Component:** `ui/src/pages/Explore/RepositorySettings.tsx`

## Purpose

Settings for one repository, laid out like the main Settings page: sections
on the left, the selected section on the right. Open it from the gear icon in
the Actions column of [Explore → Repositories](repositories.md).

## Who can access it

Any authenticated org member, for repositories in their own organization.
Changing settings (e.g. clearing the repo cache) needs an org owner or super
admin.

## Sections

- **Repo cache** (`#repo-cache`) — whether the server has a cached copy of the
  repo (used to score PR risk), its size and when a review last used it. Org
  owners and super admins can **Clear cache**; the repo is cloned again on its
  next PR review. If a review is using it right now, clearing is refused until
  it finishes. When the repo cache is turned off for the server, the section
  says so and admins get a link to Settings → Storage → Repo Cache.
- **Auto Approve** (`#auto-approve`) — override this repository's auto-approve
  policy (inherit the org default, or set a repo-specific mode and risk
  threshold percentile). See [Settings → Auto Approve](../settings/auto-approve.md).

## Related pages

- [Explore → Repositories](repositories.md)
- [Settings → Storage](../settings/storage.md)
