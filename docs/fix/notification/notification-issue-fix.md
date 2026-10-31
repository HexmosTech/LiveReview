# Frontend Notification Architecture & Dismissal Fix

## Overview

This document details the frontend notification and alert architecture for LiveReview, the inventory of all notifications across the web application, how client-side persistence works, and the route-scoping rules for banners.

---

## 1. Notification & Banner Inventory (Counts & Colors)

The frontend web application contains **8 Total Notification Types** (7 persistent notifications + 1 dynamic CRUD action toast type) and **3 Layout Banners**.

### Visual Severity & Color Palette
In the Notification Bell tray and inline alerts, colors are mapped to semantic severities:
- 🔴 **Red (`bg-red-400` / `error`)**: Critical failures, expired credentials, blocked quotas, invalid licenses.
- 🟡 **Amber / Yellow (`bg-amber-400` / `warning`)**: Expiring licenses, 80% quota warnings, URL mismatches, connector warnings.
- 🔵 **Blue (`bg-blue-400` / `info`)**: Setup discovery, onboarding checklist nudges, informational steps.
- 🟢 **Green / Emerald (`bg-green-400` / `success`)**: Ready connector status, valid licenses, successful operations.

---

### A. Health & Credential Alerts (Banners & Bell Tray)
| Notification | Visual Color & Style | Dedupe Key | Where Displayed | Dismissal Persistence |
|---|---|---|---|---|
| **Git Provider Credential Expired** | 🔴 **Red Dot** (`bg-red-400`) in Tray<br>🟡 **Amber Banner** (`text-amber-100`, amber border) | `connector-auth-<connector_id>` | Banner (4 routes) + Bell Tray | Permanent (`persistDismiss: true`, `localStorage`) |
| **Provider Mismatch** | 🔴 **Red Dot** (`bg-red-400`) in Tray<br>🟡 **Amber Banner** (`text-amber-100`, amber border) | `connector-mismatch-<connector_id>` | Banner (4 routes) + Bell Tray | Permanent (`persistDismiss: true`, `localStorage`) |
| **URL Mismatch Warning** | 🟡 **Amber Dot** (`bg-amber-400`)<br>Toast pop-up | `system-url-mismatch` | Toast + Bell Tray | Permanent (`persistDismiss: true`, `localStorage`) |
| **License Expiry Warning** | 🟡 **Amber Dot** (`bg-amber-400`) in Tray<br>🟠 **Orange/Amber Bar** (`bg-yellow-900/40 text-yellow-300`) | `license-expiry-warning` | Top License Bar + Toast + Bell Tray | Daily Resurface (`expiresAt: nextCalendarDay`) |

### B. System & Operational Notices
| Notification | Visual Color & Style | Dedupe Key | Where Displayed | Dismissal Persistence |
|---|---|---|---|---|
| **Connector Setup Progress** | 🔵 **Blue Dot** (`bg-blue-400`) - discovering<br>🟡 **Amber Dot** (`bg-amber-400`) - installing<br>🟢 **Green Dot** (`bg-green-400`) - ready | `connector-<connector_id>` | Dashboard Banner + Bell Tray | Permanent (`persistDismiss: true`, `localStorage`) |
| **Monthly LOC Quota Exceeded / Warning** | 🔴 **Red Dot** (`bg-red-400`) for blocked/exhausted<br>🟡 **Amber Dot** (`bg-amber-400`) for warning (80% usage) | `quota-<blocked\|exhausted\|warning>` | Screen Toast + Bell Tray | Permanent (`persistDismiss: true`, `localStorage`) |
| **Onboarding Stepper Nudge** | 🔵 **Blue Dot** (`bg-blue-400`) | `onboarding-stepper-nudge` | Bell Tray | Permanent (`persistDismiss: true`, `localStorage`) |

### C. Transient Action Toasts
| Notification | Visual Color & Style | Dedupe Key | Where Displayed | Dismissal Persistence |
|---|---|---|---|---|
| **Operation Results** (Save, Delete, Sync, Test) | 🟢 **Green** for success (`notify.success`)<br>🔴 **Red** for error (`notify.error`)<br>🟡 **Amber** for warning (`notify.warning`) | Dynamic UUID | Hot-toast (4–6s) + Bell Tray History | Ephemeral toast; kept in tray history |

---

### D. Visual Layout Banners Summary (3 Distinct Banners)

| # | Banner Component | Visual Styling / Colors | Active Routes | Description |
|---|---|---|---|---|
| **1** | **`ConnectorAlertBanner`** | 🟡 **Amber Alert Card** (`bg-amber-950/30 border-amber-800/60`, `text-amber-100`) | `#/dashboard`, `#/reviews`, `#/explore/repositories`, `#/git` | Surfaced when git credentials expire or provider mismatch occurs. Has "Renew credentials" and guide links. |
| **2** | **`ConnectorSetupProgressBanner`** | 🔵/🟡/🟢 **Dynamic Card** (`getPhaseVariant`):<br>• `discovering` ➔ Blue<br>• `installing` ➔ Amber<br>• `ready` ➔ Green | **`#/dashboard` ONLY** | Shows live progress of repo discovery and webhook installation. Has "Don't show again" button. |
| **3** | **`LicenseStatusBar`** | 🟢/🟡/🟠/🔴 **Top Full-Width Bar**:<br>• `active` ➔ Emerald (`bg-emerald-900/40 text-emerald-300`)<br>• `warning` ➔ Yellow (`bg-yellow-900/40 text-yellow-300`)<br>• `grace` ➔ Orange (`bg-orange-900/40 text-orange-300`)<br>• `expired`/`missing` ➔ Red (`bg-red-900/40 text-red-300`) | Global Top of page (self-hosted / non-cloud mode) | Displays enterprise license status, days remaining, and "Renew now" upsell button. |

---

## 2. Banner Route-Scoping Rules

Banners rendered via `GlobalConnectorAlerts` in `ui/src/components/Connector/ConnectorAlerts.tsx` are filtered based on the active URL route:

```
┌──────────────────────────────────────┬───────────────────────────────┬─────────────────────────────┐
│ Page Route                           │ "Credential Expired" Banner   │ Remaining (Setup) Banners   │
├──────────────────────────────────────┼───────────────────────────────┼─────────────────────────────┤
│ #/dashboard                          │               YES             │             YES             │
│ #/reviews (and /reviews/*)           │               YES             │           HIDDEN            │
│ #/explore/repositories               │               YES             │           HIDDEN            │
│ #/git (and /git/*)                   │               YES             │           HIDDEN            │
│ Other pages (#/settings, etc.)       │             HIDDEN            │           HIDDEN            │
└──────────────────────────────────────┴───────────────────────────────┴─────────────────────────────┘
```

### Implementation Logic
```tsx
const isCredentialAlertRoute = (pathname: string): boolean => {
  return (
    pathname === '/' ||
    pathname.startsWith('/dashboard') ||
    pathname.startsWith('/reviews') ||
    pathname.startsWith('/explore/repositories') ||
    pathname.startsWith('/git')
  );
};

const isDashboardRoute = (pathname: string): boolean => {
  return pathname === '/' || pathname.startsWith('/dashboard');
};
```

---

## 3. Frontend Dismissal & Persistence Architecture

### The Problem Solved
Previously, clicking the close `X` on a notification did not survive a page refresh because:
1. Setup progress dismissals were stored in ephemeral component `useState`.
2. Initial Redux state started with empty `persisted: {}` before asynchronous `/auth/me` resolved.
3. Component effects dispatched `addNotification` before auth finished, inserting items with `dismissed: false`.
4. Subsequent save effects overwrote `localStorage` with un-dismissed data.

### The Solution (Client-Side)

```
┌────────────────────────────────────────────────────────────────────────┐
│                        Three-Tier Frontend Storage                     │
└────────────────────────────────────────────────────────────────────────┘

 1. Ephemeral UI State (React useState)
    └── Limited strictly to transient modal toggles, dropdown open/close.

 2. Central Store (Redux Toolkit - Notifications Slice)
    ├── Single source of truth for items[], toastQueue[], and persisted{}.
    └── Synchronous initial state loads persisted dismissals at t = 0.

 3. Local Storage (window.localStorage)
    ├── Key: lr_notifications_<userId> (fallback: lr_notifications_last_user).
    ├── Synchronous write on dismiss action (savePersistedMap).
    └── Protected merge: a dismissed item can never be flipped back to false.
```

### Core Code Components

#### 1. Synchronous Redux Initial State (`slice.ts`)
```typescript
const getInitialState = (): NotificationsState => ({
    ...initialNotificationsState,
    persisted: loadPersistedNotifications(), // Evaluated at module load
});
```

#### 2. Immediate Write-on-Dismiss (`slice.ts` & `storage.ts`)
```typescript
dismiss(state, action: PayloadAction<string>) {
    const item = state.items.find((i) => i.id === action.payload);
    if (item) {
        item.dismissed = true;
        item.persistDismiss = true;
    }
    state.persisted[action.payload] = { read: true, dismissed: true };
    savePersistedMap(state.hydratedUserId, state.persisted);
}
```

#### 3. Protected Merge on Save (`storage.ts`)
```typescript
items.forEach((n) => {
    if (n.dismissed) {
        persisted[n.id] = { read: n.read, dismissed: true };
    } else if (n.persistDismiss && !persisted[n.id]?.dismissed) {
        persisted[n.id] = { read: n.read, dismissed: false };
    }
});
```

---

## 4. Developer Guide: Integrating a New Notification

We created the declarative `defineNotification` module (`ui/src/store/Notifications/registry.ts`). To add any new notification, create a definition in `catalog.ts` (or your local feature module):

### Step 1: Define the Notification
```typescript
import { defineNotification } from '../store/Notifications';

export const ApiKeyExpiryAlert = defineNotification<{ keyName: string; daysLeft: number }>({
    source: 'system',
    severity: 'warning',
    id: ({ keyName }) => `api-key-expiry-${keyName}`,
    title: 'API Key Expiring Soon',
    message: ({ keyName, daysLeft }) => `Key "${keyName}" expires in ${daysLeft} days.`,
    bannerRoutes: ['/dashboard', '/settings'], // Where banners should render
    persistDismiss: true,                      // Automatically remembers dismissal on F5
    actions: [{ label: 'Rotate Key', actionId: '/settings#security' }],
});
```

### Step 2: Use in Any Component or Service
```typescript
// 1. Show / publish it:
ApiKeyExpiryAlert.publish({ keyName: 'CI Token', daysLeft: 3 });

// 2. Dismiss it:
ApiKeyExpiryAlert.dismiss({ keyName: 'CI Token', daysLeft: 3 });
// or by string ID:
ApiKeyExpiryAlert.dismiss('api-key-expiry-CI Token');

// 3. Check dismissal state inside React:
const isDismissed = ApiKeyExpiryAlert.useIsDismissed({ keyName: 'CI Token', daysLeft: 3 });

// 4. Check route matching for banners:
if (ApiKeyExpiryAlert.matchesRoute(location.pathname)) { ... }
```

---

## 5. Verification

- **Automated Tests**:
  - `src/store/Notifications/__tests__/notifications.test.ts` (6 tests: persistence, merges, empty states, and re-adds).
  - `src/store/Notifications/__tests__/registry.test.ts` (7 tests: definitions, publish, dismiss, and route matching).
- **TypeScript**: `npx tsc --noEmit` verifies type correctness with 0 errors.

