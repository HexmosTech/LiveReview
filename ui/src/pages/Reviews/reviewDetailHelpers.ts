import {
    ReviewAccounting,
    ReviewAccountingStage,
    ReviewEvent,
    ReviewSummary,
    SeverityCounts,
    ToolSummary,
} from '../../types/reviews';
import { ToolAccountingData, ToolBreakdownItem } from '../../components/reviews/ToolAnalysisCard';

const classifyCommentSeverity = (rawSeverity: unknown, counts: SeverityCounts): void => {
    const sev = typeof rawSeverity === 'string' ? rawSeverity.trim().toLowerCase() : '';
    switch (sev) {
        case 'critical':
        case 'high':
        case 'error':
            counts.high++;
            break;
        case 'warning':
        case 'medium':
        case 'warn':
        case '':
            counts.medium++;
            break;
        case 'info':
        case 'low':
            counts.low++;
            break;
    }
};

/**
 * Resolves severity counts from backend summary first when non-zero, then
 * falls back to persisted `review.metadata.review_result.comments`, completed
 * `batch` events' `comments`, and finally individual severity/warn/error events.
 */
export function deriveSeverityCounts(
    summary: Pick<ReviewSummary, 'severityCounts'> | null | undefined,
    review: { metadata?: Record<string, any> | null } | null | undefined,
    events: ReviewEvent[]
): SeverityCounts {
    const summaryCounts = summary?.severityCounts;
    if (summaryCounts && (summaryCounts.high + summaryCounts.medium + summaryCounts.low) > 0) {
        return summaryCounts;
    }

    const metaComments = review?.metadata?.review_result?.comments;
    if (Array.isArray(metaComments) && metaComments.length > 0) {
        const counts: SeverityCounts = { high: 0, medium: 0, low: 0 };
        for (const comment of metaComments) {
            if (comment && typeof comment === 'object') {
                classifyCommentSeverity(comment.severity ?? comment.Severity, counts);
            }
        }
        return counts;
    }

    const batchCounts: SeverityCounts = { high: 0, medium: 0, low: 0 };
    let hasBatchComments = false;
    for (const e of events) {
        if (String(e.type) !== 'batch') continue;
        const dataAny = e.data as Record<string, any> | undefined;
        if (dataAny?.status && dataAny.status !== 'completed') continue;
        if (Array.isArray(dataAny?.comments) && dataAny.comments.length > 0) {
            hasBatchComments = true;
            for (const comment of dataAny.comments) {
                if (comment && typeof comment === 'object') {
                    classifyCommentSeverity(comment.severity ?? comment.Severity, batchCounts);
                }
            }
        }
    }
    if (hasBatchComments) {
        return batchCounts;
    }

    const eventCounts: SeverityCounts = { high: 0, medium: 0, low: 0 };
    for (const e of events) {
        const dataAny = e.data as Record<string, any> | undefined;
        const sev = typeof dataAny?.severity === 'string' ? dataAny.severity.trim().toLowerCase() : '';
        if (e.level === 'error' || sev === 'critical' || sev === 'high' || sev === 'error') {
            eventCounts.high++;
        } else if (e.level === 'warn' || sev === 'warning' || sev === 'medium' || sev === 'warn') {
            eventCounts.medium++;
        } else if (sev === 'info' || sev === 'low') {
            eventCounts.low++;
        }
    }
    return eventCounts;
}

export const ACCOUNTING_REFRESH_INTERVAL_MS = 15000;
export const ACTIVE_POLL_INTERVAL_MS = 3000;
export const COMMITS_PREVIEW_LIMIT = 5;

export const isSafeExternalUrl = (url: string | null | undefined): boolean => {
    if (!url) return false;
    try {
        const parsed = new URL(url);
        return parsed.protocol === 'http:' || parsed.protocol === 'https:';
    } catch {
        return false;
    }
};

export const hasAccountingDetails = (value: ReviewAccounting | null): boolean => {
    if (!value) {
        return false;
    }

    return (
        value.accountedOperations > 0 ||
        value.totalBillableLoc > 0 ||
        value.tokenTrackedOperations > 0 ||
        !!value.stageBreakdown?.length ||
        !!value.latestOperation
    );
};

export function deriveToolAccounting(
    toolSummary: ToolSummary | undefined,
    dbMultipliers: Record<string, number>
): ToolAccountingData | null {
    if (!toolSummary || !toolSummary.toolBreakdown || toolSummary.toolBreakdown.length === 0) {
        return null;
    }
    const breakdown: ToolBreakdownItem[] = toolSummary.toolBreakdown.map((item) => ({
        ...item,
        creditsUsed: typeof item.creditsUsed === 'number'
            ? item.creditsUsed
            : (dbMultipliers[item.toolName.toLowerCase()] ?? 1.0),
    }));
    const totalCredits = breakdown.reduce((sum, i) => sum + (i.creditsUsed || 0), 0);
    const executedCount = typeof toolSummary.toolsExecuted === 'number'
        ? toolSummary.toolsExecuted
        : breakdown.filter(t => t.status === 'completed' || t.status === 'clean' || t.status === 'failed' || t.status === 'partial').length;

    return {
        totalToolCredits: totalCredits,
        toolsExecuted: executedCount,
        totalCommentsGenerated: toolSummary.totalCommentsGenerated || 0,
        toolBreakdown: breakdown,
    };
}

export const formatInt = (value?: number): string => {
    if (value === undefined || value === null) {
        return 'Not tracked yet';
    }
    return value.toLocaleString();
};

export const formatCurrency = (value?: number): string => {
    if (value === undefined || value === null) {
        return 'Not tracked yet';
    }
    return `$${value.toFixed(4)}`;
};

export const formatDuration = (startedAt?: string, completedAt?: string): string | null => {
    if (!startedAt || !completedAt) {
        return null;
    }
    const ms = new Date(completedAt).getTime() - new Date(startedAt).getTime();
    if (!isFinite(ms) || ms < 0) {
        return null;
    }
    const totalSeconds = Math.round(ms / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
};

export const formatStageLabel = (stage: string): string => {
    switch (stage) {
        case 'leader':
            return 'Leader model';
        case 'helper':
            return 'Helper model';
        default:
            return stage.charAt(0).toUpperCase() + stage.slice(1);
    }
};

export interface StageAIConfig {
    mode?: string;
    source?: string;
    provider?: string;
    connector?: string;
}

export const getStageRouteText = (
    stage: ReviewAccountingStage,
    leader?: StageAIConfig,
    helper?: StageAIConfig
): string => {
    if (stage.stage === 'helper') {
        return [helper?.provider || stage.provider, helper?.connector]
            .filter(Boolean)
            .join(' / ');
    }
    return [leader?.provider || stage.provider, leader?.connector]
        .filter(Boolean)
        .join(' / ');
};

export const getStageExecutionText = (
    stage: ReviewAccountingStage,
    leader?: StageAIConfig,
    helper?: StageAIConfig
): string => {
    if (stage.stage === 'helper') {
        return [helper?.mode, helper?.source].filter(Boolean).join(' via ');
    }
    return [leader?.mode, leader?.source].filter(Boolean).join(' via ');
};

export const formatEventData = (event: ReviewEvent): string => {
    const data = (event.data || {}) as Record<string, any>;
    switch (event.type) {
        case 'status':
            return data.status ? `Status: ${data.status}` : 'Status changed';
        case 'log':
            return data.message || 'Log entry';
        case 'batch':
            return event.batchId ? `Batch: ${event.batchId}` : `Batch processing`;
        case 'artifact':
            return data.url ? `Generated: ${data.kind || 'Artifact'}` : `Artifact: ${data.kind || 'Unknown'}`;
        case 'completion':
            return data.resultSummary ? `Completed: ${data.resultSummary}` : 'Process completed';
        default:
            return JSON.stringify(data, null, 2);
    }
};
