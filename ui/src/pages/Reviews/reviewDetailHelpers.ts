import { ReviewAccounting, ReviewAccountingStage, ReviewEvent, ToolSummary } from '../../types/reviews';
import { ToolAccountingData, ToolBreakdownItem } from '../../components/reviews/ToolAnalysisCard';

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
