import { defineNotification } from './registry';

/**
 * Git provider connector authentication failure (e.g. expired access token).
 * Rendered as a banner on /dashboard, /reviews, /explore/repositories, and /git.
 */
export const ConnectorAuthAlert = defineNotification<{
    connectorId: number;
    provider: string;
    connectionName: string;
    repoCount: number;
    wikiUrl?: string;
}>({
    source: 'connector',
    severity: 'error',
    id: ({ connectorId }) => `connector-auth-${connectorId}`,
    title: ({ provider }) => `Your ${provider} provider credential expired, please renew it.`,
    message: ({ connectionName, repoCount }) =>
        `Connector "${connectionName}": ${repoCount} ${repoCount === 1 ? 'repository' : 'repositories'} could not sync due to expired or invalid credentials.`,
    bannerRoutes: ['/dashboard', '/reviews', '/explore/repositories', '/git'],
    persistDismiss: true,
    actions: ({ connectorId, provider, wikiUrl }) => [
        { label: 'Renew Credentials', actionId: `/git/connector/${connectorId}` },
        ...(wikiUrl ? [{ label: `${provider} Setup Guide`, actionId: wikiUrl }] : []),
    ],
});

/**
 * Provider mismatch on connector repositories.
 */
export const ConnectorMismatchAlert = defineNotification<{
    connectorId: number;
    provider: string;
    connectionName: string;
    repoCount: number;
    wikiUrl?: string;
}>({
    source: 'connector',
    severity: 'error',
    id: ({ connectorId }) => `connector-mismatch-${connectorId}`,
    title: ({ provider, connectionName }) => `Provider Mismatch: ${provider} (${connectionName})`,
    message: ({ repoCount }) =>
        `${repoCount} ${repoCount === 1 ? 'repository' : 'repositories'} have a mismatched provider configured.`,
    bannerRoutes: ['/dashboard', '/reviews', '/explore/repositories', '/git'],
    persistDismiss: true,
    actions: ({ connectorId, provider, wikiUrl }) => [
        { label: 'Review Connector', actionId: `/git/connector/${connectorId}` },
        ...(wikiUrl ? [{ label: `${provider} Setup Guide`, actionId: wikiUrl }] : []),
    ],
});

/**
 * Connector setup progress banner (strictly visible on /dashboard).
 */
export const ConnectorSetupProgressAlert = defineNotification<{
    connectorId: number;
    message: string;
    severity?: 'info' | 'warning' | 'success';
}>({
    source: 'connector',
    severity: 'info',
    id: ({ connectorId }) => `connector-${connectorId}`,
    message: ({ message }) => message,
    bannerRoutes: ['/dashboard'],
    persistDismiss: true,
});

/**
 * Production URL mismatch warning banner.
 */
export const UrlMismatchAlert = defineNotification<{
    storedHostname: string;
    currentHostname: string;
    isProduction: boolean;
}>({
    source: 'url-mismatch',
    severity: 'warning',
    id: () => 'system-url-mismatch',
    title: 'URL Mismatch Warning',
    message: ({ storedHostname, currentHostname, isProduction }) =>
        `Your production URL (${storedHostname}) doesn't match your current domain (${currentHostname}). ${
            isProduction
                ? 'This may cause OAuth redirects to fail.'
                : 'You should update this when switching to production mode.'
        }`,
    toast: true,
    persistDismiss: true,
    actions: [
        { label: 'Fix URL', actionId: 'system-url-mismatch:fix' },
        { label: 'Settings', actionId: 'system-url-mismatch:settings' },
    ],
});

/**
 * LOC quota usage alert (blocked, exhausted, or warning).
 */
export const QuotaAlert = defineNotification<{
    level: 'blocked' | 'exhausted' | 'warning';
    locUsed: number;
    locLimit: number;
    usagePct: number;
}>({
    source: 'quota',
    severity: 'warning',
    id: ({ level }) => `quota-${level}`,
    title: ({ level }) =>
        level === 'blocked'
            ? 'Quota Exceeded'
            : level === 'exhausted'
            ? 'Monthly Limit Reached'
            : 'LOC Usage Nearing Limit',
    message: ({ level, locUsed, locLimit, usagePct }) => {
        const locLimitText = locLimit > 0 ? locLimit.toLocaleString() : 'N/A';
        if (level === 'blocked') {
            return `Monthly LOC quota exceeded (${locUsed.toLocaleString()} / ${locLimitText} LOC). Reviews are blocked until quota resets or you upgrade.`;
        }
        if (level === 'exhausted') {
            return `You've used all ${locLimitText} LOC this month. Upgrade to continue reviewing without interruption.`;
        }
        return `You've used ${locUsed.toLocaleString()} of ${locLimitText} LOC (${usagePct}%) this month.`;
    },
    toast: true,
    persistDismiss: true,
});

/**
 * Self-hosted or Enterprise license expiry alert.
 */
export const LicenseExpiryAlert = defineNotification<{
    daysRemaining: number;
    renewalUrl: string;
    expiresAtTimestamp: number;
}>({
    source: 'license',
    severity: 'warning',
    id: () => 'license-expiry-warning',
    title: 'License Expiring Soon',
    message: ({ daysRemaining }) =>
        daysRemaining <= 0
            ? 'Your license has expired. Please renew your license to continue using LiveReview.'
            : `Your license expires in ${daysRemaining} day${daysRemaining === 1 ? '' : 's'}. Renew now to avoid interruption.`,
    toast: true,
    persistDismiss: true,
    actions: ({ renewalUrl }) => [{ label: 'Renew License', actionId: renewalUrl }],
    expiresAt: ({ expiresAtTimestamp }) => expiresAtTimestamp,
});

/**
 * Onboarding checklist nudge alert.
 */
export const OnboardingStepperAlert = defineNotification({
    source: 'onboarding',
    severity: 'info',
    id: () => 'onboarding-stepper-nudge',
    message: 'Complete your onboarding checklist to get the most out of LiveReview.',
    toast: false,
    persistDismiss: true,
});
