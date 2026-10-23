import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Card, Icons, PageHeader, Spinner } from '../../components/UIPrimitives';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import { getGitHubAppDemo } from '../../api/githubApp';
import GitHubAppAccounts from '../../components/GitHubApp/GitHubAppAccounts';
import GitHubAppNextSteps, { Step } from '../../components/GitHubApp/GitHubAppNextSteps';
import { useGitHubAppInstall } from '../../hooks/useGitHubAppInstall';
import { useOrgContext } from '../../hooks/useOrgContext';

// First-run page for orgs connected through the LiveReview GitHub App: repositories are connected, set up an AI
// provider, then open the merge requests list (or the review-by-URL page) to run the first review. Everything is
// read from the server; no GitHub URLs or ids live here.

const AUTO_CONNECT_KEY = 'lrGhAutoConnectTried';

const GetStartedGitHub: React.FC = () => {
    const navigate = useNavigate();

    const { startInstall: connectGitHub, busy: reconnecting } = useGitHubAppInstall('#/get-started/github', 'get-started');
    const { isSuperAdmin, currentOrg } = useOrgContext();
    const canDisconnect = isSuperAdmin || currentOrg?.role === 'owner';

    const { data: demo, isLoading, error: loadError, refetch } = useQuery({
        queryKey: ['github-app', 'demo'],
        queryFn: () => getGitHubAppDemo(false),
        staleTime: 0,
        retry: false,
    });
    // After a change (say, uninstalling the last account), show the fallback card instead of bouncing the user to GitHub.
    const afterChange = async () => {
        try {
            sessionStorage.setItem(AUTO_CONNECT_KEY, '1');
        } catch {
            // ignore
        }
        setAlreadyTried(true);
        await refetch();
    };

    const repos = demo?.repositories || [];
    const connected = Boolean(demo?.connected);
    const settingsUrl = demo?.installations?.[0]?.settings_url || '';
    const installations = demo?.installations || [];
    const available = demo?.available || [];
    // Nothing connected, but the app is already installed on an account (by someone in another LiveReview account,
    // or disconnected here): offer to connect it instead of sending the user to GitHub.
    const canConnectInstead = !isLoading && !loadError && !connected && available.length > 0;

    // Nothing to review yet (the app isn't installed, or it was given no repositories): send the user to GitHub
    // instead of showing this page, but only ONCE per browser session. Coming back without finishing shows the
    // fallback card below instead of redirecting again. location.replace keeps this page out of the history.
    const unconnected = !isLoading && !loadError && Boolean(demo) && !canConnectInstead &&
        (!connected || (repos.length === 0 && Boolean(settingsUrl)));
    const [alreadyTried, setAlreadyTried] = useState(() => {
        try {
            return sessionStorage.getItem(AUTO_CONNECT_KEY) === '1';
        } catch {
            return true; // storage unavailable: never redirect automatically (it could loop)
        }
    });
    const redirecting = unconnected && !alreadyTried;
    useEffect(() => {
        if (!redirecting) {
            if (!isLoading && connected && repos.length > 0) {
                try {
                    sessionStorage.removeItem(AUTO_CONNECT_KEY);
                } catch {
                    // ignore
                }
            }
            return;
        }
        try {
            sessionStorage.setItem(AUTO_CONNECT_KEY, '1');
        } catch {
            return;
        }
        if (!connected) {
            void connectGitHub();
        } else {
            window.location.replace(settingsUrl);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [redirecting, isLoading, connected, repos.length]);
    const needsGitHub = unconnected;

    return (
        <ErrorBoundary>
            <div className="max-w-2xl mx-auto px-4 py-8">
                <PageHeader title="Get started with GitHub" description="Connect your repositories and review your first pull request." />

                {isLoading || redirecting || reconnecting ? (
                    <div className="flex justify-center items-center py-12">
                        <Spinner size="lg" />
                    </div>
                ) : loadError ? (
                    <Alert variant="error" title="Could not load your GitHub connection" className="mt-6">
                        Please refresh the page.
                    </Alert>
                ) : null}

                {unconnected && alreadyTried && !reconnecting && (
                    <Card className="mt-6">
                        <div className="p-6 space-y-3">
                            <h2 className="text-lg font-semibold text-white">
                                {connected ? 'Give LiveReview access to a repository' : 'Connect your GitHub repositories'}
                            </h2>
                            <p className="text-sm text-slate-300">
                                {connected
                                    ? 'The LiveReview GitHub App is installed but has no repositories yet. Add one on GitHub, then refresh this page.'
                                    : 'Install the LiveReview GitHub App and choose which repositories it can review.'}
                            </p>
                            {connected ? (
                                <Button icon={<Icons.GitHub />} onClick={() => window.open(settingsUrl, '_blank', 'noopener,noreferrer')}>
                                    Add repositories on GitHub
                                </Button>
                            ) : (
                                <Button icon={<Icons.GitHub />} onClick={connectGitHub} isLoading={reconnecting}>
                                    Continue with GitHub
                                </Button>
                            )}
                        </div>
                    </Card>
                )}

                {canConnectInstead && (
                    <Card className="mt-6">
                        <div className="p-6 space-y-3">
                            <h2 className="text-lg font-semibold text-white">
                                LiveReview is already installed on {available.map((a) => a.account_login).join(', ')}
                            </h2>
                            <p className="text-sm text-slate-300">
                                {canDisconnect
                                    ? 'Connect it to your account to see its repositories and pull requests here and review them with your own AI provider.'
                                    : 'An owner of your LiveReview organization can connect it to see its repositories and pull requests here.'}
                            </p>
                            <GitHubAppAccounts
                                installations={[]}
                                available={available}
                                repos={[]}
                                canDisconnect={canDisconnect}
                                onInstallAnother={connectGitHub}
                                installBusy={reconnecting}
                                onChanged={async () => {
                                    await refetch();
                                }}
                            />
                        </div>
                    </Card>
                )}

                {!isLoading && !needsGitHub && connected && (
                    <Card className="mt-6">
                        <ol className="p-6">
                            <Step index={1} title="Repositories connected" done>
                                {repos.length === 0 ? (
                                    <p>No repositories yet. Add some on GitHub, then refresh this page.</p>
                                ) : null}
                                <div className="mt-2">
                                    <GitHubAppAccounts
                                        installations={installations}
                                        available={available}
                                        repos={repos}
                                        canDisconnect={canDisconnect}
                                        onInstallAnother={connectGitHub}
                                        installBusy={reconnecting}
                                        onChanged={afterChange}
                                    />
                                </div>
                            </Step>

                            <GitHubAppNextSteps startIndex={2} />
                        </ol>
                        <div className="px-6 pb-6 pt-4 border-t border-slate-700 flex justify-end">
                            <Button variant="ghost" onClick={() => navigate('/reviews/create-cli')}>
                                Next: catch issues before you commit with the CLI
                            </Button>
                        </div>
                    </Card>
                )}
            </div>
        </ErrorBoundary>
    );
};

export default GetStartedGitHub;
