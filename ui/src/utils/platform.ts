export const isMacPlatform = (): boolean => {
    if (typeof navigator === 'undefined') return false;
    return /Mac|iPod|iPhone|iPad/.test(navigator.platform || navigator.userAgent || '');
};

export const shortcutKeyLabel = (): string => (isMacPlatform() ? '⌘K' : 'Ctrl K');
