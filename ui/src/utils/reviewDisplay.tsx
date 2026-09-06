import React from 'react';
import { LuTerminal, LuClock } from 'react-icons/lu';
import { SiGitlab } from 'react-icons/si';
import { Icons } from '../components/UIPrimitives';
import { Review } from '../types/reviews';

// Shared "how do we display a review" helpers, extracted from
// ui/src/pages/Reviews/Reviews.tsx so every other place that lists reviews
// (currently: the CI/CD Gates "Add review" picker) renders titles, source
// icons, and status colors identically instead of re-deriving its own
// (inevitably slightly different) version of the same logic.

const toTitleCase = (value: string): string =>
    value.replace(/\b\w/g, (char) => char.toUpperCase());

// review.provider is literally "cli" for CLI-triggered reviews
// (internal/jobqueue/review_worker.go), not a real git provider - treated
// as its own "source" alongside github/gitlab/etc. Scheduled reviews carry
// a real git provider (whatever the connector uses), so they're identified
// by triggerType instead - checked first so it takes priority over provider.
export const normalizeSource = (provider?: string, triggerType?: string): string => {
    if (triggerType === 'scheduled') return 'scheduled';
    const normalized = (provider || '').toLowerCase();
    if (normalized === 'cli') return 'cli';
    if (normalized.startsWith('github')) return 'github';
    if (normalized.startsWith('gitlab')) return 'gitlab';
    if (normalized.startsWith('bitbucket')) return 'bitbucket';
    if (normalized.startsWith('gitea')) return 'gitea';
    if (normalized.startsWith('azuredevops')) return 'azuredevops';
    return normalized;
};

export const sourceLabel = (provider?: string, triggerType?: string): string => {
    switch (normalizeSource(provider, triggerType)) {
        case 'scheduled':
            return 'Scheduled';
        case 'cli':
            return 'CLI';
        case 'github':
            return 'GitHub';
        case 'gitlab':
            return 'GitLab';
        case 'bitbucket':
            return 'Bitbucket';
        case 'gitea':
            return 'Gitea';
        case 'azuredevops':
            return 'Azure DevOps';
        default:
            return provider ? toTitleCase(provider) : '—';
    }
};

export const SourceIcon: React.FC<{ provider?: string; triggerType?: string }> = ({ provider, triggerType }) => {
    switch (normalizeSource(provider, triggerType)) {
        case 'scheduled':
            return <LuClock size={18} />;
        case 'cli':
            return <LuTerminal size={18} />;
        case 'github':
            return <Icons.GitHub />;
        case 'gitlab':
            return (
                <SiGitlab className="w-5 h-5" style={{ color: '#FC6D26' }} />
            );
        case 'bitbucket':
            return <Icons.Bitbucket />;
        case 'gitea':
            return <Icons.Gitea />;
        case 'azuredevops':
            return <Icons.AzureDevOps />;
        default:
            return null;
    }
};

// GitHub: /owner/repo/pull/123 -> owner ; GitLab: /owner/repo/-/merge_requests/123 -> owner
// Bitbucket: /workspace/repo/pull-requests/123 -> workspace
export const extractAuthorFromUrl = (url: string): string | null => {
    try {
        const pathParts = new URL(url).pathname.split('/').filter(Boolean);
        if (pathParts.length >= 2) return pathParts[0];
    } catch {
        // ignore malformed URLs
    }
    return null;
};

export const extractMRInfo = (url: string): string => {
    try {
        const pathParts = new URL(url).pathname.split('/').filter(Boolean);
        if (pathParts.includes('pull') && pathParts.length >= 4) {
            return `PR #${pathParts[pathParts.indexOf('pull') + 1]}`;
        }
        if (pathParts.includes('merge_requests') && pathParts.length >= 4) {
            return `MR !${pathParts[pathParts.indexOf('merge_requests') + 1]}`;
        }
        if (pathParts.includes('pull-requests') && pathParts.length >= 4) {
            return `PR #${pathParts[pathParts.indexOf('pull-requests') + 1]}`;
        }
        return 'MR/PR';
    } catch {
        return 'MR/PR';
    }
};

export const getExecutionBadge = (
    review: Review
): { label: string; color: string; borderColor: string } | null => {
    const rawMode =
        typeof review.metadata?.ai_execution_mode === 'string'
            ? review.metadata.ai_execution_mode.toLowerCase()
            : '';
    const rawPlanCode =
        typeof review.metadata?.plan_code === 'string'
            ? review.metadata.plan_code.toLowerCase()
            : '';

    if (
        rawMode === 'hosted_auto' ||
        rawPlanCode === 'team_32usd' ||
        rawPlanCode === 'team'
    ) {
        return {
            label: 'Auto',
            color: '#6ee7b7',
            borderColor: 'rgba(16,185,129,0.35)',
        };
    }
    if (
        rawMode === 'byok_required' ||
        rawMode === 'byok_override' ||
        rawMode === 'byok_optional' ||
        rawPlanCode === 'free_30k' ||
        rawPlanCode === 'free'
    ) {
        return {
            label: 'BYOK',
            color: '#fcd34d',
            borderColor: 'rgba(245,158,11,0.35)',
        };
    }
    return null;
};

// Transparent-outline + colored text, same visual language as the Sync
// Status / PR State badges on the other Explore tables.
export const reviewStatusBadge = (
    status: string
): { color: string; borderColor: string } => {
    switch (status) {
        case 'completed':
            return { color: '#4ade80', borderColor: 'rgba(34,197,94,0.35)' };
        case 'failed':
            return { color: '#f87171', borderColor: 'rgba(239,68,68,0.35)' };
        case 'in_progress':
            return { color: '#60a5fa', borderColor: 'rgba(59,130,246,0.35)' };
        default:
            return { color: '#fcd34d', borderColor: 'rgba(245,158,11,0.35)' };
    }
};

export const getCleanRepository = (review: Pick<Review, 'repository'>): string => {
    if (!review.repository) return '';
    const withoutProtocol = review.repository
        .replace(/^https?:\/\//, '')
        .replace(/\/$/, '');
    const segments = withoutProtocol.split('/').filter(Boolean);
    // Drop a leading host segment (e.g. "gitlab.com", "github.com",
    // "gitlab.internal.corp") so the column shows just the org/repo path.
    if (segments.length > 1 && segments[0].includes('.')) {
        return segments.slice(1).join('/');
    }
    return withoutProtocol;
};

export const getRepoShortName = (cleanedRepository: string): string => {
    const repoSegments = cleanedRepository.split('/').filter(Boolean);
    return repoSegments.length ? repoSegments[repoSegments.length - 1] : '';
};

/** Same "what should the primary title read as" logic for a review, used
 * everywhere a review needs a single human-readable title (the Reviews
 * list's sort accessor + cell renderer, and the CI/CD Gates "Add review"
 * picker). */
export const getPrimaryTitle = (review: Review): string => {
    const rawMrDescriptor = review.prMrUrl ? extractMRInfo(review.prMrUrl) : '';
    const mrDescriptor =
        rawMrDescriptor && rawMrDescriptor !== 'MR/PR' ? rawMrDescriptor : '';
    const repoShort = getRepoShortName(getCleanRepository(review));

    if (review.triggerType === 'cli_diff') {
        return (
            review.aiSummaryTitle?.trim() ||
            review.friendlyName?.trim() ||
            repoShort ||
            'CLI Review'
        );
    }
    if (review.triggerType === 'scheduled') {
        return review.friendlyName?.trim() || repoShort || 'Scheduled Review';
    }
    const mrTitleCandidate = review.mrTitle?.trim();
    return mrTitleCandidate && mrTitleCandidate.length > 0
        ? mrTitleCandidate
        : mrDescriptor || repoShort || 'Code Review';
};
