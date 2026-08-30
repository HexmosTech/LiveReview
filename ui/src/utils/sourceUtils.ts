// Shared Git Provider / Source Normalization Utilities
// Shared across Reviews list and ReviewDetail pages

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
            return provider ? provider.replace(/\b\w/g, (char) => char.toUpperCase()) : '—';
    }
};
