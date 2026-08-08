# LiveReview UI Design Conventions

This file exists so any agent (or human) touching `ui/src` can match the existing look
instead of reinventing styling per-page. It was written after a redesign of the bulk
invite flow (`ui/src/components/UserManagement/UserForm.tsx`) surfaced repeated
"doesn't look like the rest of the app" feedback — read this **before** styling
anything, not after.

Rule of thumb: **grep for an existing example before inventing a new pattern.**
Every rule below links to a real file:line you can open and copy from.

## Component library

`ui/src/components/UIPrimitives.tsx` is the shared component file. Before writing raw
`<div>`/`<button>`/`<span>` markup for something common, check whether it already
exports what you need: `Button`, `Card`, `Input`, `Select`, `Badge`, `EmptyState`,
`Avatar`, `StatCard`, `Alert`, `Icons`, `PageHeader`, `Section`, `Container`,
`Popover`, `Spinner`, `Tooltip`, `Divider`.

There is **no shared `Modal` or `Tabs` component** — every modal
(`ui/src/components/CreateOrganizationModal/`, `ui/src/components/License/LicenseModal.tsx`,
`ui/src/components/Subscriptions/*Modal.tsx`) and every tab bar hand-rolls its own
markup. If you need one, follow an existing modal's structure rather than adding a
new abstraction.

## Icons

`Icons` in `ui/src/components/UIPrimitives.tsx` is the shared icon object — check it
first for the concept you need (`Add`, `Edit`, `Delete`, `Download`, `Search`,
`Settings`, `User`, `Info`, `Success`, `Warning`, `Error`, etc.) before writing a new
inline `<svg>`.

**`react-icons` (`^5.7.0`) is already a dependency** and bundles dozens of icon sets
under one package — no new install needed to reach for a better icon. Sub-packages
already used in this codebase (`ui/src/components/UIPrimitives.tsx`): `react-icons/si`
(brand/simple icons — GitHub, GitLab, Slack logo, etc.), `react-icons/fa6` (Font
Awesome 6), `react-icons/md` (Material Design). Also available in
`node_modules/react-icons/` (import from any, same zero-cost pattern): `fa`, `fc`,
`bs` (Bootstrap Icons), `hi` / `hi2` (Heroicons v1/v2), `io` / `io5` (Ionicons),
`ai` (Ant Design), `bi`, `ri` (Remix Icon), `tb` (Tabler), `pi` (Phosphor), `lu`
(Lucide), `gi`, `gr`, `go`, `im`, `cg`, `ci`, `di`, `sl`, `tfi`, `ti`, `vsc`, `wi`,
`lia`.

**Pick the icon that actually matches the concept, don't default to a generic one.**
Example: the "Invite a user" / "Send bulk invitation" nav links used to both point at
the generic `+` (`Icons.Add`) — swapped for real user-add/group-add icons instead of
two identical `+` signs.

**When two icons represent related concepts (singular/plural, on/off, add/remove),
pull both from the *same* icon family so they're visually consistent** — different
families draw the "same" concept differently (badge position, stroke weight,
corner radius), so mixing families for a pair looks subtly mismatched even when each
icon is individually correct. This bit us once: `MdPersonAdd` + `MdGroupAdd`
(Material Design) individually looked right, but Material draws the "+" badge on a
different side/position for the person-icon vs. the group-icon, so side-by-side they
looked inconsistent. Fixed by using Tabler's `TbUserPlus` + `TbUsersPlus`
(`react-icons/tb`) instead — a pair actually designed together, so the badge lines
up. If you need a matched singular/plural or on/off pair, check whether one family
(Tabler and Phosphor tend to have the most complete matched sets) has both before
picking icons from two different families.

Add reusable icons to the shared `Icons` object in `UIPrimitives.tsx` rather than
importing `react-icons` ad hoc in a single page file, so the next person finds it in
the same place.

## Buttons

`<Button>` from UIPrimitives supports `variant`: `primary | secondary | outline |
ghost | danger`, and `size`: `sm | md | lg` (default `md`).

- **Primary action** (Save, Submit, Invite, Create Key): `variant="primary"`
  (the default — you usually don't need to pass it).
- **Cancel / dismiss when it is NOT the primary action**: `variant="ghost"`, **not**
  `secondary`. `ghost` is transparent at rest with `hover:bg-slate-700` — see
  `ui/src/pages/Settings/APIKeysTab.tsx:301-308` ("Cancel" next to "Create Key").
  Using `secondary` here (solid gray box) is a common mistake — it reads as a
  second primary action, not a dismiss.
- **`secondary`** is for a real second action that still needs visual weight (e.g. an
  "Export" button next to "Save"), not for Cancel.
- Footer button pairs use `space-x-4` (not `gap-*`) inside
  `flex justify-end ... pt-4` — see `ui/src/components/UserManagement/UserForm.tsx`'s
  form footer.
- **Loading state**: use the `isLoading` prop, not manual disabled+text-swap. It
  renders a spinner and auto-disables. Example:
  `ui/src/pages/Settings/StorageSettingsTab.tsx:269,277` — `isLoading={isSaving}`.
  (Some older forms use `disabled={x} + {x ? 'Saving...' : 'Save'}` instead —
  `isLoading` is the pattern to follow for new code.)
- **Disabled styling**: don't invent anything extra. `Button`'s built-in
  `disabled:opacity-70`/`disabled:cursor-not-allowed` (or the raw
  `disabled:opacity-50 disabled:cursor-not-allowed` used on plain `<button>`s) is the
  house style. No icons, tooltips, or color inversion on disabled controls anywhere
  in the app.

## Badges / status chips

Two conventions coexist — prefer the second one for anything on the app's dark
background:

1. `<Badge variant="...">` from UIPrimitives — fixed light-mode colors
   (`bg-green-100 text-green-800` etc.). Used in
   `ui/src/components/reviews/BatchSummary.tsx`, `ui/src/pages/Settings/APIKeysTab.tsx`,
   `ui/src/pages/AIProviders/components/ConnectorCard.tsx`. Fine for badges sitting on
   lighter surfaces, but the light background can look out of place on
   `bg-slate-800`/`900` cards.
2. **Dark-theme inline chip** —
   `ui/src/components/UserManagement/UserList.tsx:345-352`'s pattern, used for
   Active/Inactive style status pills directly on dark backgrounds:
   ```
   inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium
   bg-{color}-900/20 text-{color}-300
   ```
   This is what `ui/src/components/UserManagement/UserForm.tsx`'s bulk-invite table
   chips ("New invite", "Existing member", "Invited"/"Updated"/"Failed") were
   switched to. Use this form whenever the chip sits directly on a dark card/table
   background.

**Semantic color mapping** (from `ui/src/components/reviews/BatchSummary.tsx:62-75`,
reused everywhere): `green` = success/completed/active · `amber/yellow` =
pending/warning/needs attention · `red` = failed/inactive/error · `blue` =
informational/in-progress/type tag.

## Toasts (`react-hot-toast`)

Root `<Toaster />` in `ui/src/App.tsx` uses defaults — no custom position/styling, so
don't add per-call style overrides.

House phrasing, from real call sites (`ui/src/pages/Licenses/LicenseAssignment.tsx`,
`ui/src/pages/Settings/StorageSettingsTab.tsx`,
`ui/src/pages/Settings/LicenseSeatAssignment.tsx`, `ui/src/pages/Settings/LicenseTab.tsx`):
- Success: a short declarative sentence ending in **"successfully"** —
  `"License assigned successfully"`, `"3 license(s) assigned successfully"`. Use the
  literal `(s)` suffix for pluralization (matches existing calls), not a ternary
  `user${n!==1?'s':''}`.
- Errors: `toast.error(err.message || 'Failed to <verb> <noun>')` — always have a
  fallback string, and prefer surfacing the real error message when you have one.
- No emoji, no exclamation marks (one legacy call has `!`, don't copy it), Title-case
  first word.

## Layout / page width

- Full-page views (Dashboard, table-heavy list pages like
  `ui/src/components/UserManagement/UserList.tsx`, `ui/src/pages/Settings/Settings.tsx`)
  use Tailwind's `container mx-auto px-4 py-8` — **not** a fixed `max-w-*`.
  `container` is responsive (grows with breakpoints) and is what makes a page "the
  same width as Dashboard." If a request says "match the Dashboard width," this is
  the exact class to copy — see `ui/src/components/Dashboard/Dashboard.tsx:366`.
- Simple centered forms (no table): `max-w-2xl mx-auto` —
  `ui/src/components/UserManagement/UserOnboardingDetails.tsx:50`.
- Modals scale with content weight: `max-w-sm` (confirm dialogs) → `max-w-md`
  (`ui/src/pages/Settings/LicenseTab.tsx:245`) → `max-w-2xl`
  (`ui/src/pages/Settings/SubscriptionTab.tsx:2413`) → `max-w-3xl`/`max-w-4xl`
  (`ui/src/pages/Settings/LearningsTab.tsx:437`) → `max-w-5xl` for the heaviest data
  modals (`ui/src/pages/.../TaxonomyReports.tsx:1980`).
- A data table needing more horizontal room than its container gives it should
  either drop the width cap (like `UserList.tsx`, which doesn't constrain width at
  all) or temporarily hide adjacent non-essential panels rather than letting the
  table overflow/truncate — see `ui/src/components/UserManagement/UserForm.tsx`'s
  `showSteps` pattern, which hides a side "how it works" panel once the bulk-review
  table needs the full card width.

## Tab bars

Active tab underline is **`border-blue-500 text-white`**, not `border-white`. This
is the same primary blue used for the top nav's active item (Dashboard/Reviews/
Settings) — see `ui/src/pages/Explore/ExploreTabs.tsx:21-25`:

```tsx
className={`py-3 px-1 border-b-2 text-sm font-medium transition-colors ${
  active === tab.key
    ? 'border-blue-500 text-white'
    : 'border-transparent text-slate-400 hover:text-slate-200 hover:border-slate-600'
}`}
```

A plain white underline (`border-white`) reads as an unstyled default, not an
intentional accent — always tie the active indicator to the app's primary blue.

## Color / dark theme background hierarchy

Two competing page-background tokens exist in the codebase (`bg-gray-900` and
`bg-slate-900`) — the app shell (`ui/src/App.tsx` footer/loading screen) uses
**`bg-slate-900`**, so prefer that for new top-level page wrappers. Card/panel
layering, in order from page background inward:

```
page background   → bg-gray-900 or bg-slate-900 (pick slate-900 for new pages)
card               → bg-slate-800, border border-slate-700, rounded-lg
nested panel       → bg-slate-900/40 (e.g. a sidebar-like section inside a card)
input fields       → bg-slate-700 or bg-slate-900, border-slate-600
text hierarchy     → text-white (headings) → text-slate-300 (body) → text-slate-400 (secondary/help)
```

## Numbered steps / stepper UI

If you need a vertical "how it works" step list, use **flexbox**, not absolute
positioning with manually-computed negative margins — the latter is fragile and
drifts out of alignment as soon as any step's text wraps to multiple lines (this bit
us once in `ui/src/components/UserManagement/UserForm.tsx`). Pattern:

```tsx
<li className="flex gap-4">
  <div className="flex flex-col items-center">
    <span className="flex-shrink-0 flex items-center justify-center w-6 h-6 rounded-full bg-blue-600 text-xs font-semibold text-white">
      {i + 1}
    </span>
    {i < steps.length - 1 && <div className="w-px flex-1 my-1 bg-slate-700/70" />}
  </div>
  <p className="text-sm text-slate-300 leading-relaxed pb-5">{step}</p>
</li>
```

The badge and text are true flex siblings, so the badge top always aligns with the
text's first line — no pixel math to get wrong.

## Success / completion screens

`ui/src/components/UserManagement/UserOnboardingDetails.tsx` is the reference
pattern for "action succeeded, here's what to do next" full-page screens (and
`ui/src/components/UserManagement/BulkOnboardingDetails.tsx` is its multi-row
sibling): centered card (`max-w-2xl mx-auto bg-gray-800 p-8 rounded-lg border
border-emerald-500/30 shadow-xl shadow-emerald-950/20`), a green check icon in a
circle (`bg-emerald-500/10 text-emerald-400 rounded-full`), a bold emerald headline,
then detail sections in `bg-gray-900/50 p-5 rounded-md border border-gray-700`
boxes, and a final action row (`flex justify-between gap-4 pt-4 border-t
border-gray-700`) with a secondary "download/export" action on the left and the
primary "continue" action on the right.

## When in doubt

Grep first: `grep -rn "toast.success(" ui/src/pages` (or whatever pattern you're
about to invent) almost always turns up 3-4 real examples faster than guessing.
