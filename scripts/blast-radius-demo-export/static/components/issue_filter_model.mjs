export const DEFAULT_SEVERITIES = ['critical', 'warning', 'info'];
export const CONFIDENCE_ORDER = ['high', 'medium', 'low'];

export const SELECTION_FIELDS = Object.freeze({
    severity: 'severities',
    confidence: 'confidences',
    type: 'types',
    category: 'categories',
    subcategory: 'subcategories',
});

export const FACET_FIELDS = Object.freeze(Object.keys(SELECTION_FIELDS));

function normalizeDisabledSet(values, normalizer = normalizeFacetValue) {
    const normalized = new Set();
    if (!(values instanceof Set) && !Array.isArray(values)) {
        return normalized;
    }
    values.forEach((value) => {
        const next = normalizer(value);
        if (next) {
            normalized.add(next);
        }
    });
    return normalized;
}

function cloneDisabledSelections(disabled) {
    return {
        severities: new Set(disabled?.severities || []),
        confidences: new Set(disabled?.confidences || []),
        types: new Set(disabled?.types || []),
        categories: new Set(disabled?.categories || []),
        subcategories: new Set(disabled?.subcategories || []),
    };
}

export function normalizeText(value) {
    return String(value || '').trim();
}

export function normalizeFacetValue(value) {
    return normalizeText(value).toLowerCase();
}

export function normalizeSeverity(value) {
    const severity = normalizeFacetValue(value);
    if (DEFAULT_SEVERITIES.includes(severity)) {
        return severity;
    }
    return 'info';
}

export function normalizeCommentShape(comment) {
    return {
        severity: normalizeSeverity(comment?.Severity ?? comment?.severity),
        confidence: normalizeText(comment?.Confidence ?? comment?.confidence),
        type: normalizeText(comment?.Type ?? comment?.type),
        category: normalizeText(comment?.Category ?? comment?.category),
        subcategory: normalizeText(comment?.Subcategory ?? comment?.subcategory),
        content: normalizeText(comment?.Content ?? comment?.content),
        line: comment?.Line ?? comment?.line ?? '',
    };
}

export function formatSeverityLabel(value) {
    return value.charAt(0).toUpperCase() + value.slice(1);
}

export function sortValues(values, preferredOrder = []) {
    return [...values].sort((left, right) => {
        const leftIndex = preferredOrder.indexOf(left);
        const rightIndex = preferredOrder.indexOf(right);
        if (leftIndex !== -1 || rightIndex !== -1) {
            if (leftIndex === -1) return 1;
            if (rightIndex === -1) return -1;
            return leftIndex - rightIndex;
        }
        return left.localeCompare(right);
    });
}

export function getSelectionFieldName(field) {
    return SELECTION_FIELDS[field] || '';
}

export function getNormalizedFacetValue(shape, field) {
    if (field === 'severity') {
        return shape.severity;
    }
    return normalizeFacetValue(shape[field]);
}

export function createDefaultIssueFilters() {
    return {
        disabled: cloneDisabledSelections(),
    };
}

export function cloneIssueFilters(filters) {
    const source = normalizeIssueFilters(filters);
    return {
        disabled: cloneDisabledSelections(source.disabled),
    };
}

export function normalizeIssueFilters(filters) {
    const source = filters?.disabled || {};
    return {
        disabled: {
            severities: normalizeDisabledSet(source.severities, normalizeSeverity),
            confidences: normalizeDisabledSet(source.confidences),
            types: normalizeDisabledSet(source.types),
            categories: normalizeDisabledSet(source.categories),
            subcategories: normalizeDisabledSet(source.subcategories),
        },
    };
}

export function hasActiveIssueFilters(filters) {
    const normalized = normalizeIssueFilters(filters);
    return Object.values(normalized.disabled).some((selection) => selection.size > 0);
}

export function toggleIssueFilterValue(filters, field, rawValue, options = {}) {
    const selectionField = getSelectionFieldName(field);
    if (!selectionField) {
        return normalizeIssueFilters(filters);
    }

    const next = cloneIssueFilters(filters);
    const disabledSet = next.disabled[selectionField];
    const value = field === 'severity' ? normalizeSeverity(rawValue) : normalizeFacetValue(rawValue);
    const childValues = Array.isArray(options.childValues)
        ? options.childValues.map((entry) => normalizeFacetValue(entry)).filter(Boolean)
        : [];

    if (!value) {
        return next;
    }

    if (field === 'category') {
        if (disabledSet.has(value)) {
            disabledSet.delete(value);
            childValues.forEach((childValue) => next.disabled.subcategories.delete(childValue));
        } else {
            disabledSet.add(value);
            childValues.forEach((childValue) => next.disabled.subcategories.add(childValue));
        }
        return next;
    }

    if (disabledSet.has(value)) {
        disabledSet.delete(value);
    } else {
        disabledSet.add(value);
    }

    return next;
}

export function buildCommentVisibilityKey(filePath, comment) {
    const path = filePath || comment?.FilePath || comment?.file_path || comment?.filePath || '';
    const shape = normalizeCommentShape(comment);
    const content = shape.content.replace(/\s+/g, ' ');
    return `${path}::${shape.line}::${shape.severity}::${normalizeFacetValue(shape.confidence)}::${normalizeFacetValue(shape.type)}::${normalizeFacetValue(shape.category)}::${normalizeFacetValue(shape.subcategory)}::${content}`;
}
