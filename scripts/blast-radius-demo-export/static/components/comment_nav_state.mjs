export function normalizeIndex(idx, length) {
    if (length <= 0) return -1;
    const mod = idx % length;
    const normalized = mod < 0 ? mod + length : mod;
    return Object.is(normalized, -0) ? 0 : normalized;
}

function toInt(value, fallback) {
    return Number.isInteger(value) ? value : fallback;
}

export function sanitizeCommentNavState(state, allCommentsLength) {
    const length = allCommentsLength > 0 ? allCommentsLength : 0;
    const rawCurrentIdx = toInt(state?.currentIdx, -1);
    const currentIdx = rawCurrentIdx >= 0 && rawCurrentIdx < length ? rawCurrentIdx : -1;

    const rawAnchorIdx = toInt(state?.anchorIdx, 0);
    // Anchor is an insertion position, so [0..length] is valid by design.
    const anchorIdx = Math.min(Math.max(rawAnchorIdx, 0), length);

    const activeCommentId = typeof state?.activeCommentId === 'string' ? state.activeCommentId : null;
    return { currentIdx, activeCommentId, anchorIdx };
}

export function reconcileCommentNavState(allComments, prevIdx, activeCommentId, anchorIdx) {
    const length = allComments.length;
    if (length === 0) {
        return { currentIdx: -1, activeCommentId: null, anchorIdx: 0 };
    }

    if (activeCommentId) {
        const activeIdx = allComments.findIndex((entry) => entry.commentId === activeCommentId);
        if (activeIdx >= 0) {
            return { currentIdx: activeIdx, activeCommentId, anchorIdx: activeIdx };
        }
    }

    if (prevIdx >= 0) {
        return {
            currentIdx: -1,
            activeCommentId: null,
            // Keep insertion anchor semantics to preserve wrap behavior at end-of-list.
            anchorIdx: Math.min(prevIdx, length)
        };
    }

    return {
        currentIdx: -1,
        activeCommentId: null,
        anchorIdx: Math.min(Math.max(anchorIdx, 0), length)
    };
}

export function resolveNextIndex(currentIdx, anchorIdx, length) {
    if (length <= 0) return -1;
    if (currentIdx >= 0) return normalizeIndex(currentIdx + 1, length);
    return normalizeIndex(anchorIdx, length);
}

export function resolvePrevIndex(currentIdx, anchorIdx, length) {
    if (length <= 0) return -1;
    if (currentIdx >= 0) return normalizeIndex(currentIdx - 1, length);
    return normalizeIndex(anchorIdx - 1, length);
}
