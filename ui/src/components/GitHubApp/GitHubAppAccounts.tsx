import React, { useState } from 'react';
import { Avatar, Button, Icons } from '../UIPrimitives';
import GitHubAppRepositories from './GitHubAppRepositories';
import { ConfirmModal } from '../ConfirmModal';
import { notify } from '../../utils/notify';
import { disconnectGitHubInstallation, GitHubAppInstallation, GitHubAppRepository } from '../../api/githubApp';

interface GitHubAppAccountsProps {
    installations: GitHubAppInstallation[];
    repos: GitHubAppRepository[];
    // Only organization owners can uninstall; the server enforces it too.
    canDisconnect: boolean;
    onInstallAnother: () => void;
    installBusy?: boolean;
    // Called after an account was uninstalled; the parent refreshes its data.
    onDisconnected: () => void | Promise<void>;
}

// The org's connected GitHub accounts: each one is a heading with its repositories indented below it, a
// "Manage on GitHub" button and (for owners) "Disconnect"; plus "Add repositories or organization".
// Shared by the Git Providers page and the get-started page.
const GitHubAppAccounts: React.FC<GitHubAppAccountsProps> = ({
    installations,
    repos,
    canDisconnect,
    onInstallAnother,
    installBusy = false,
    onDisconnected,
}) => {
    const [disconnecting, setDisconnecting] = useState<GitHubAppInstallation | null>(null);
    const [busy, setBusy] = useState(false);

    const reposByOwner = (owner: string) =>
        repos.filter((r) => r.full_name.split('/')[0].toLowerCase() === owner.toLowerCase());

    const confirmDisconnect = async () => {
        if (!disconnecting) return;
        setBusy(true);
        try {
            await disconnectGitHubInstallation(disconnecting.connector_id);
            notify.success(`Uninstalled the app from ${disconnecting.account_login} successfully`);
            setDisconnecting(null);
            await onDisconnected();
        } catch (err: any) {
            notify.error(err?.message || 'Failed to disconnect GitHub installation');
        } finally {
            setBusy(false);
        }
    };

    return (
        <>
            <ul className="space-y-4">
                {installations.map((inst) => {
                    const orgRepos = reposByOwner(inst.account_login);
                    return (
                        <li key={inst.connector_id} className="border border-slate-600 rounded-lg bg-slate-700 p-4">
                            <div className="flex flex-wrap items-center justify-between gap-3">
                                <div className="flex items-center min-w-0">
                                    <Avatar size="md" initials={inst.account_login.charAt(0).toUpperCase()} className="flex-shrink-0 mr-3" />
                                    <div className="min-w-0">
                                        <div className="flex flex-wrap items-center gap-2">
                                            <h3 className="font-medium text-white break-all">{inst.account_login}</h3>
                                            <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-900/20 text-blue-300">
                                                {orgRepos.length} {orgRepos.length === 1 ? 'repository' : 'repositories'}
                                            </span>
                                        </div>
                                        <p className="text-sm text-slate-300">Connected through the GitHub App</p>
                                    </div>
                                </div>
                                <div className="flex items-center gap-2">
                                    {inst.settings_url && (
                                        <Button
                                            size="sm"
                                            variant="outline"
                                            icon={<Icons.GitHub />}
                                            onClick={() => window.open(inst.settings_url, '_blank', 'noopener,noreferrer')}
                                        >
                                            Manage on GitHub
                                        </Button>
                                    )}
                                    {canDisconnect && (
                                        <Button
                                            size="sm"
                                            variant="outline"
                                            onClick={() => setDisconnecting(inst)}
                                            className="!text-red-400 hover:!text-red-300 hover:!border-red-400"
                                        >
                                            Disconnect
                                        </Button>
                                    )}
                                </div>
                            </div>
                            <GitHubAppRepositories repos={orgRepos} />
                        </li>
                    );
                })}
            </ul>
            <Button
                className="mt-4"
                size="sm"
                variant="outline"
                icon={<Icons.Add />}
                onClick={onInstallAnother}
                isLoading={installBusy}
            >
                Add repositories or organization
            </Button>
            <ConfirmModal
                show={disconnecting !== null}
                loading={busy}
                type="danger"
                title={`Uninstall from ${disconnecting?.account_login ?? 'this account'}?`}
                message="This uninstalls the LiveReview GitHub App from this GitHub account and stops LiveReview reviewing its pull requests. To use it again, install the app on GitHub again."
                confirmText="Uninstall and disconnect"
                onConfirm={confirmDisconnect}
                onCancel={() => setDisconnecting(null)}
            />
        </>
    );
};

export default GitHubAppAccounts;
