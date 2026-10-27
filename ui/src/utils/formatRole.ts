// Formats a raw org role string (e.g. "super_admin") into a display label
// (e.g. "Super Admin"). CSS `capitalize` alone can't do this - it only
// capitalizes the first letter of whitespace-separated words, so
// "super_admin" renders as "Super_admin", not "Super Admin".
export const formatRoleLabel = (role?: string | null): string => {
    const raw = (role || '').trim();
    if (!raw) return '';
    return raw
        .split('_')
        .filter(Boolean)
        .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
        .join(' ');
};
