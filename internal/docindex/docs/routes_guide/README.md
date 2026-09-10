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
- A small number of top-level files are **reference pages, not routes**
  (e.g. `contact.md`), for cross-cutting questions the chatbot must answer
  that no single page owns. Keep these rare and clearly marked as non-routes
  at the top of the file.
- Each file follows this shape:
  - **Route(s)** — path(s) and the component that renders them
  - **Purpose** — what the page is for, in plain language
  - **Who can access it** — role/permission gating, if any
  - **Key actions** — what a user can do on this page
  - **Related pages** — where this page links to / is linked from
  - **Learn more (public docs)** — optional; links to the matching page(s)
    on the public docs site, when one exists

## Linking to the public docs

Where a page is also covered by the public documentation site, end the file
with a **Learn more (public docs)** section linking to it. Base URL is
`https://hexmos.com/livereview/docs/<path>`, where `<path>` mirrors the file
layout under `../hexmos_docs/` (e.g. `livereview/integrations/slack.mdx` →
`https://hexmos.com/livereview/docs/livereview/integrations/slack`, and an
`index.mdx` drops the `/index`).

Only link to pages that actually exist in `../hexmos_docs/` — that folder is
synced from the live docs site, so it is the authority on what is publishable.
Never guess a URL: a 404 in the chatbot's answer is worse than no link.

## Keeping this in sync

See the root `AGENTS.md` rule: **any UI change must update the matching
file(s) here.** New route → new file. Route removed → delete its file.
Behavior/actions changed → update the file's "Key actions" section.
