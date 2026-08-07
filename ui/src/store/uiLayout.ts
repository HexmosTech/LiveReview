import { useSyncExternalStore } from 'react';

// Whether the FloatingOnboardingNudge bar is currently occupying the bottom
// of the viewport. The global floating chat button reads this so it can
// position itself above the bar when it's up, and drop back to its default
// position when the bar is closed or scrolled away.
let nudgeOccupying = false;

const listeners = new Set<() => void>();

function emit() {
    listeners.forEach((l) => l());
}

function subscribe(cb: () => void) {
    listeners.add(cb);
    return () => listeners.delete(cb);
}

function getSnapshot() {
    return nudgeOccupying;
}

export function setNudgeOccupying(occupying: boolean) {
    if (nudgeOccupying === occupying) return;
    nudgeOccupying = occupying;
    emit();
}

export function useNudgeOccupying() {
    return useSyncExternalStore(subscribe, getSnapshot);
}
