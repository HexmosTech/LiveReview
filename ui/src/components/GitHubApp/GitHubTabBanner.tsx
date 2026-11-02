import React, { useState } from 'react';
import { Button, Icons } from '../UIPrimitives';
import { notify } from '../../utils/notify';
import { GitHubTabControls } from '../../hooks/useGitHubAppInstall';

interface GitHubTabBannerProps {
    githubTab: GitHubTabControls;
    // Refetches the accounts; the banner goes away once it has run.
    onRefresh: () => void | Promise<void>;
}

// Shown under the account list while GitHub is (about to be) open in another tab: the changes made there are
// picked up with Refresh, since this page isn't told when they are done.
const GitHubTabBanner: React.FC<GitHubTabBannerProps> = ({ githubTab, onRefresh }) => {
    const [refreshing, setRefreshing] = useState(false);
    if (githubTab.state === 'idle') return null;

    const refresh = async () => {
        setRefreshing(true);
        try {
            await onRefresh();
            githubTab.dismiss();
        } catch (err: any) {
            notify.error(err?.message || 'Failed to refresh accounts');
        } finally {
            setRefreshing(false);
        }
    };

    return (
        <div className="border-l-4 border-l-blue-500 border-t border-r border-b border-slate-700/50 bg-slate-800/90 rounded-lg p-4 mt-4">
            <div className="flex items-start gap-3">
                <div className="flex-shrink-0 w-8 h-8 rounded-full bg-blue-500/10 flex items-center justify-center border border-blue-500/20 mt-0.5">
                    <div className="text-blue-400">
                        <Icons.GitHub />
                    </div>
                </div>
                <div className="flex-1 min-w-0">
                    {githubTab.state === 'opening' && <p className="text-sm text-slate-200">Opening GitHub in a new tab…</p>}
                    {githubTab.state === 'opened' && (
                        <>
                            <p className="text-sm text-slate-200">
                                GitHub opened in a new tab. When you&apos;re done managing repositories, close that tab and click Refresh.
                            </p>
                            <p className="text-xs text-slate-400 mt-1">
                                If something still doesn&apos;t show, try &quot;Find my GitHub accounts&quot;.
                            </p>
                        </>
                    )}
                    {githubTab.state === 'blocked' && (
                        <p className="text-sm text-slate-200">
                            Your browser blocked the new tab.{' '}
                            {githubTab.url && (
                                <a
                                    href={githubTab.url}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="text-blue-300 underline hover:text-blue-200"
                                >
                                    Open GitHub
                                </a>
                            )}
                            , then close it and click Refresh when you&apos;re done.
                        </p>
                    )}
                </div>
                {githubTab.state !== 'opening' && (
                    <div className="flex-shrink-0 flex gap-2">
                        <Button size="sm" onClick={refresh} isLoading={refreshing}>
                            Refresh
                        </Button>
                    </div>
                )}
            </div>
        </div>
    );
};

export default GitHubTabBanner;
