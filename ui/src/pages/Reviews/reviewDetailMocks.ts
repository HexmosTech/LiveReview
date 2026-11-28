import { Review, ReviewEvent, ReviewSummary } from '../../types/reviews';
import { ToolAccountingData } from '../../components/reviews/ToolAnalysisCard';

export interface MockReviewData {
    review: Review;
    summary: ReviewSummary;
    toolAccounting: ToolAccountingData;
    events: ReviewEvent[];
}

export function getMockReviewData(id: string): MockReviewData | null {
    if (id !== 'test' && id !== 'test1' && id !== 'test2' && id !== 'test3') {
        return null;
    }

    const stage = id === 'test1' ? 1 : id === 'test2' ? 2 : 3;

    const testPrMrUrl = id === 'test1'
        ? 'https://github.com/HexmosTech/git-lrc/pull/131'
        : id === 'test3'
        ? 'https://git.apps.hexmos.com/hexmos/livereview/-/merge_requests/2'
        : undefined;

    const testProvider = id === 'test1' ? 'github'
        : id === 'test3' ? 'gitlab'
        : 'cli';

    const testRepo = id === 'test3' ? 'livereview' : id === 'test2' ? 'repo-b' : 'git-lrc';
    const testBranch = id === 'test3' ? 'feat/rag-query' : id === 'test2' ? 'main' : 'feat/tools-integration-beta';

    const review: Review = {
        id: 999,
        orgId: 1,
        repository: testRepo,
        branch: testBranch,
        prMrUrl: testPrMrUrl,
        triggerType: testProvider,
        userEmail: 'ganeshkumar6120@gmail.com',
        provider: testProvider,
        status: stage === 3 ? 'completed' : 'in_progress',
        createdAt: new Date().toISOString(),
        completedAt: stage === 3 ? new Date().toISOString() : undefined,
    };

    const summary: ReviewSummary = {
        reviewId: 999,
        currentStatus: stage === 3 ? 'completed' : 'in_progress',
        lastActivity: new Date().toISOString(),
        batchCount: 0,
        eventCounts: { tool_result: stage === 1 ? 0 : stage === 2 ? 5 : 15 },
    };

    let toolAccounting: ToolAccountingData;
    if (stage === 1) {
        toolAccounting = {
            totalToolCredits: 0,
            toolsExecuted: 0,
            totalCommentsGenerated: 0,
            toolBreakdown: [
                { toolName: 'ruff', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'bandit', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'gitleaks', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'eslint', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'hadolint', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'actionlint', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'shellcheck', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'semgrep', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'trufflehog', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'trivy', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'spectral', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'brakeman', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'kubescape', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'zizmor', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'openapi', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
            ],
        };
    } else if (stage === 2) {
        toolAccounting = {
            totalToolCredits: 4.0,
            toolsExecuted: 5,
            totalCommentsGenerated: 14,
            toolBreakdown: [
                { toolName: 'ruff', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                { toolName: 'bandit', creditsUsed: 1.0, commentsGenerated: 14, status: 'completed' },
                { toolName: 'gitleaks', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                { toolName: 'hadolint', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                { toolName: 'shellcheck', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                { toolName: 'eslint', creditsUsed: 2.0, commentsGenerated: 0, status: 'running' },
                { toolName: 'semgrep', creditsUsed: 3.0, commentsGenerated: 0, status: 'running' },
                { toolName: 'trufflehog', creditsUsed: 2.0, commentsGenerated: 0, status: 'running' },
                { toolName: 'trivy', creditsUsed: 2.5, commentsGenerated: 0, status: 'running' },
                { toolName: 'actionlint', creditsUsed: 0.5, commentsGenerated: 0, status: 'running' },
                { toolName: 'spectral', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'brakeman', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'kubescape', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'zizmor', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                { toolName: 'openapi', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
            ],
        };
    } else {
        toolAccounting = {
            totalToolCredits: 21.0,
            toolsExecuted: 15,
            totalCommentsGenerated: 35,
            toolBreakdown: [
                { toolName: 'ruff', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                { toolName: 'bandit', creditsUsed: 1.0, commentsGenerated: 14, status: 'completed' },
                { toolName: 'gitleaks', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                { toolName: 'eslint', creditsUsed: 2.0, commentsGenerated: 5, status: 'completed' },
                { toolName: 'hadolint', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                { toolName: 'actionlint', creditsUsed: 0.5, commentsGenerated: 2, status: 'completed' },
                { toolName: 'shellcheck', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                { toolName: 'semgrep', creditsUsed: 3.0, commentsGenerated: 8, status: 'completed' },
                { toolName: 'trufflehog', creditsUsed: 2.0, commentsGenerated: 0, status: 'clean' },
                { toolName: 'trivy', creditsUsed: 2.5, commentsGenerated: 3, status: 'completed' },
                { toolName: 'spectral', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                { toolName: 'brakeman', creditsUsed: 1.5, commentsGenerated: 1, status: 'completed' },
                { toolName: 'kubescape', creditsUsed: 2.0, commentsGenerated: 0, status: 'clean' },
                { toolName: 'zizmor', creditsUsed: 1.0, commentsGenerated: 2, status: 'completed' },
                { toolName: 'openapi', creditsUsed: 0.5, commentsGenerated: 0, status: 'failed' },
            ],
        };
    }

    const events: ReviewEvent[] = [
        {
            id: 1,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage started: preparation' },
        },
        {
            id: 2,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage completed: preparation' },
        },
        {
            id: 3,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage started: analysis' },
        },
        {
            id: 4,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage completed: analysis' },
        },
        {
            id: 5,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage started: review' },
        },
        {
            id: 6,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'log',
            level: 'info',
            data: { message: 'ruff: Clean (0 findings)' },
        },
        {
            id: 7,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'log',
            level: 'warn',
            data: { message: 'bandit: 14 comments generated' },
        },
        {
            id: 8,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage completed: review' },
        },
        {
            id: 9,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage started: artifact generation' },
        },
        {
            id: 10,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'artifact',
            level: 'info',
            data: { message: 'Posted 14 comments to merge request' },
        },
        {
            id: 11,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'status',
            level: 'info',
            data: { message: 'Stage completed: artifact generation' },
        },
        {
            id: 12,
            reviewId: 999,
            orgId: 1,
            time: new Date().toISOString(),
            type: 'completion',
            level: 'info',
            data: { resultSummary: 'Review process completed', message: 'finalization complete' },
        },
    ];

    return { review, summary, toolAccounting, events };
}

