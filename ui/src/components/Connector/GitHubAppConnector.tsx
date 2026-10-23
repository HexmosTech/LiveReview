import React from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Icons, Spinner } from '../UIPrimitives';
import GitHubAppAccounts from '../GitHubApp/GitHubAppAccounts';
import { getGitHubAppDemo } from '../../api/githubApp';
import { useGitHubAppEnabled } from '../../hooks/useGitHubAppEnabled';
import { useGitHubAppInstall } from '../../hooks/useGitHubAppInstall';
import { useOrgContext } from '../../hooks/useOrgContext';

// The "GitHub App" provider card: install the app, and see/manage every connected GitHub account with its
// repositories. Returning GitHub sign-ins land here (see the lr_gh_next cookie handling in index.tsx).
const GitHubAppConnector: React.FC = () => {
    const navigate = useNavigate();
    const enabled = useGitHubAppEnabled();
    const { isSuperAdmin, currentOrg } = useOrgContext();
    const canDisconnect = isSuperAdmin || currentOrg?.role === 'owner';
    const { startInstall, busy } = useGitHubAppInstall('#/git/github-app', 'git-providers');

    const { data: demo, isLoading, error, refetch } = useQuery({
        queryKey: ['github-app', 'demo', 'git-providers'],
        queryFn: () => getGitHubAppDemo(false),
        enabled: enabled === true,
        staleTime: 0,
        retry: false,
    });

    const connected = Boolean(demo?.connected);
    const available = demo?.available || [];

    // Self-hosted (or the App isn't configured): there is no GitHub App card, so don't sit on a spinner.
    if (enabled === false) return <Navigate to="/git" replace />;

    return (
        <div className="space-y-4">
            <div className="flex items-center">
                <Button variant="ghost" icon={<Icons.Back />} onClick={() => navigate('/git')} iconPosition="left" className="text-sm -ml-4">
                    Back to providers
                </Button>
            </div>
            <div className="rounded-md bg-blue-900 border border-blue-700 px-4 py-3">
                <h3 className="text-sm font-medium text-blue-200">How the GitHub App works</h3>
                <div className="mt-1 text-sm text-blue-300">
                    Install the LiveReview GitHub App on the accounts and organizations you want reviewed. You choose the
                    repositories on GitHub, and there are no access tokens or webhooks to set up.
                </div>
            </div>
            {!enabled || isLoading ? (
                <div className="flex items-center gap-3 py-2 text-slate-300">
                    <Spinner size="sm" color="text-blue-400" />
                    <span className="text-sm">Loading your GitHub connections…</span>
                </div>
            ) : error ? (
                <Alert variant="error" title="Could not load your GitHub connections">
                    Please refresh the page.
                </Alert>
            ) : connected || available.length > 0 ? (
                <GitHubAppAccounts
                    installations={demo?.installations || []}
                    available={available}
                    repos={demo?.repositories || []}
                    canDisconnect={canDisconnect}
                    onInstallAnother={startInstall}
                    installBusy={busy}
                    onChanged={async () => {
                        await refetch();
                    }}
                />
            ) : (
                <Button icon={<Icons.GitHub />} onClick={startInstall} isLoading={busy}>
                    Install the GitHub App
                </Button>
            )}
        </div>
    );
};

export default GitHubAppConnector;
