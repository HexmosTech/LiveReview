# Route Documentation

One Markdown file per UI route/page, mirroring `ui/src/pages/`. This is a
knowledge source for RAG / chatbot training and onboarding — it explains what
each page does, what actions a user can take there, and how pages relate,
not implementation detail.

## Structure

- Each file covers one route (or a tight group of sibling routes, e.g. a
  list + detail pair) and is named after the route's primary purpose.
- Subfolders mirror the top-level route groups in `ui/src/App.tsx`
  (`reviews/`, `explore/`, `git/`, `ai/`, `settings/`, `licenses/`,
  `reports/`, `chatbot/`, `auth/`). Top-level pages that don't belong to a
  group (dashboard, home) live directly in `ui/docs/training_data/lr_routes/`.
- Each file follows this shape:
  - **Route(s)** — path(s) and the component that renders them
  - **Purpose** — what the page is for, in plain language
  - **Who can access it** — role/permission gating, if any
  - **Key actions** — what a user can do on this page
  - **Related pages** — where this page links to / is linked from

## Keeping this in sync

See the root `AGENTS.md` rule: **any UI change must update the matching
file(s) here.** New route → new file. Route removed → delete its file.
Behavior/actions changed → update the file's "Key actions" section.
