// Deterministic "Google-style" avatar identity: the same user always gets the
// same initials and the same background color, derived from stable fields
// (user id, falling back to email) rather than anything that can change
// per-render or per-session.

const AVATAR_PALETTE = [
    '#1d4ed8', // blue-700
    '#0f766e', // teal-700
    '#7e22ce', // purple-700
    '#b91c1c', // red-700
    '#a16207', // amber-700
    '#15803d', // green-700
    '#be185d', // pink-700
    '#4338ca', // indigo-700
    '#c2410c', // orange-700
    '#0e7490', // cyan-700
    '#6d28d9', // violet-700
    '#4d7c0f', // lime-700
];

// 32-bit DJB2 hash - small, stable, good-enough distribution for picking a
// palette index (not used for anything security-sensitive).
const hashString = (value: string): number => {
    let hash = 5381;
    for (let i = 0; i < value.length; i++) {
        hash = (hash * 33) ^ value.charCodeAt(i);
    }
    return Math.abs(hash >>> 0);
};

/**
 * Picks a stable background color for a user identity. Prefers the numeric
 * user id (never changes); falls back to email for callers that only have
 * that (e.g. before `user.id` is loaded).
 */
export const avatarColorFor = (identity: { id?: number | string; email?: string }): string => {
    const key = identity.id !== undefined && identity.id !== null && identity.id !== ''
        ? `id:${identity.id}`
        : `email:${(identity.email || '').trim().toLowerCase()}`;
    const index = hashString(key) % AVATAR_PALETTE.length;
    return AVATAR_PALETTE[index];
};

/**
 * Derives initials from a display name if present, otherwise from the local
 * part of an email address. Always 1-2 uppercase characters.
 */
export const avatarInitialsFor = (identity: { name?: string; email?: string }): string => {
    const name = (identity.name || '').trim();
    if (name) {
        const words = name.split(/\s+/).filter(Boolean);
        if (words.length >= 2) {
            return (words[0][0] + words[1][0]).toUpperCase();
        }
        return words[0].slice(0, 2).toUpperCase();
    }

    const email = (identity.email || '').trim();
    const localPart = email.split('@')[0] || '';
    const alnum = localPart.replace(/[^a-zA-Z0-9]/g, '');
    if (alnum.length >= 2) {
        return alnum.slice(0, 2).toUpperCase();
    }
    if (alnum.length === 1) {
        return alnum.toUpperCase();
    }
    return '?';
};
