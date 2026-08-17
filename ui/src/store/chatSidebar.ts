import { useSyncExternalStore } from 'react';

// Livi chat conversation sidebar state. Lives outside Redux/component state
// (same pattern as uiLayout.ts) because the Navbar reads it too - it insets
// its own content by the sidebar's reserved width on the chat route - and
// the Navbar is a sibling of the routed chat page, not an ancestor/
// descendant of it.
//
// Two independent things:
//  - `pinned`: whether the sidebar permanently reserves layout space (the
//    Navbar/main content shift over for it). Unpinned = a compact rail by
//    default.
//  - `hoverExpanded`: while unpinned, hovering the compact rail shows the
//    full sidebar as a temporary OVERLAY on top of the content underneath -
//    it does NOT change reserved layout width, so nothing else on the page
//    shifts just from hovering. Irrelevant once pinned (already full width).

export const SIDEBAR_DEFAULT_WIDTH = 220;
export const SIDEBAR_MIN_WIDTH = 200;
export const SIDEBAR_MAX_WIDTH = 480;
// The permanently-reserved width while unpinned - just enough for the pin
// and "new chat" affordances.
export const SIDEBAR_COLLAPSED_WIDTH = 52;

const WIDTH_STORAGE_KEY = 'chatSidebarWidth';
const PINNED_STORAGE_KEY = 'chatSidebarPinned';

function loadStoredWidth(): number {
    const raw = localStorage.getItem(WIDTH_STORAGE_KEY);
    const parsed = raw ? Number(raw) : NaN;
    if (!Number.isFinite(parsed)) return SIDEBAR_DEFAULT_WIDTH;
    return clampWidth(parsed);
}

function loadStoredPinned(): boolean {
    return localStorage.getItem(PINNED_STORAGE_KEY) === '1';
}

function clampWidth(px: number): number {
    return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(px)));
}

let pinned = loadStoredPinned();
let width = loadStoredWidth();
// True only while a resize drag is in progress, so consumers can suspend
// their width transition - animating toward every intermediate mousemove
// value would make the drag feel laggy/rubber-banded.
let resizing = false;
// True while hovering the compact (unpinned) rail - drives the sidebar's
// own peek-overlay rendering only, not the reserved layout width.
let hoverExpanded = false;
let snapshot = { pinned, width, resizing, hoverExpanded };

const listeners = new Set<() => void>();

function subscribe(cb: () => void) {
    listeners.add(cb);
    return () => listeners.delete(cb);
}

function getSnapshot() {
    return snapshot;
}

function publish() {
    snapshot = { pinned, width, resizing, hoverExpanded };
    listeners.forEach((l) => l());
}

export function setChatSidebarResizing(value: boolean) {
    if (value === resizing) return;
    resizing = value;
    publish();
}

export function setChatSidebarHoverExpanded(value: boolean) {
    if (value === hoverExpanded) return;
    hoverExpanded = value;
    publish();
}

function setPinned(value: boolean) {
    if (value === pinned) return;
    pinned = value;
    localStorage.setItem(PINNED_STORAGE_KEY, value ? '1' : '0');
    publish();
}

export const toggleChatSidebarPinned = () => setPinned(!pinned);

// Persisted immediately on every call rather than only on drag-end - a
// resize is a handful of calls total, not a hot path, so there's no reason
// to add debounce complexity for it.
export function setChatSidebarWidth(px: number) {
    const next = clampWidth(px);
    if (next === width) return;
    width = next;
    localStorage.setItem(WIDTH_STORAGE_KEY, String(next));
    publish();
}

export function useChatSidebarPinned() {
    return useSyncExternalStore(subscribe, getSnapshot).pinned;
}

export function useChatSidebarWidth() {
    return useSyncExternalStore(subscribe, getSnapshot).width;
}

export function useChatSidebarResizing() {
    return useSyncExternalStore(subscribe, getSnapshot).resizing;
}

export function useChatSidebarHoverExpanded() {
    return useSyncExternalStore(subscribe, getSnapshot).hoverExpanded;
}

// The width the rest of the page (Navbar, main content) should permanently
// reserve for the sidebar. Deliberately ignores hover - a peek is an
// overlay, not a layout change, so nothing else on the page shifts just
// because the mouse is over the rail.
export function useChatSidebarReservedWidth() {
    const { pinned: isPinned, width: w } = useSyncExternalStore(subscribe, getSnapshot);
    return isPinned ? w : SIDEBAR_COLLAPSED_WIDTH;
}
