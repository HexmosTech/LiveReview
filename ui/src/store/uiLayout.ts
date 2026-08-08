import { useSyncExternalStore } from 'react';

// Tracks whether any floating element is occupying the bottom-right region of
// the viewport, so the global floating chat button can adapt its position and
// label. Bitmask so multiple blockers can coexist if needed.
//
// bit 1: the FloatingOnboardingNudge bar (full-width bottom bar) — the chat
//        button moves above it.
// bit 2: the diff-viewer CommentNav prev/next pill — the chat label hides so
//        only the round button remains, clearing the pill (which sits at
//        right-24, just left of the button).
let blockers = 0;
let snapshot = blockers;

const listeners = new Set<() => void>();

function subscribe(cb: () => void) {
    listeners.add(cb);
    return () => listeners.delete(cb);
}

function getSnapshot() {
    return snapshot;
}

function setBit(bit: number, on: boolean) {
    const next = on ? blockers | bit : blockers & ~bit;
    if (next === blockers) return;
    blockers = next;
    snapshot = blockers;
    listeners.forEach((l) => l());
}

export const setNudgeOccupying = (on: boolean) => setBit(1, on);
export const setCommentNavOccupying = (on: boolean) => setBit(2, on);

export function useBottomRightBlockers() {
    return useSyncExternalStore(subscribe, getSnapshot);
}
