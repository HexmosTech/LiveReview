import React, { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Avatar, Button, Icons } from '../UIPrimitives';
import GitHubAppRepositories from './GitHubAppRepositories';
import { ConfirmModal } from '../ConfirmModal';
import { notify } from '../../utils/notify';
import { disconnectGitHubInstallation, GitHubAppInstallation, GitHubAppRepository } from '../../api/githubApp';

interface GitHubAppAccountsProps {
    installations: GitHubAppInstallation[];
    repos: GitHubAppRepository[];
    // Only organization owners can disconnect or uninstall; the server enforces it too.
    canDisconnect: boolean;
    onInstallAnother: () => void;
    installBusy?: boolean;
    // Called after an account was disconnected or uninstalled; the parent refreshes its data.
    onDisconnected: () => void | Promise<void>;
}

// What the confirm dialog is for: disconnect only this LiveReview account, or uninstall the App from GitHub.
interface PendingAction {
    inst: GitHubAppInstallation;
    uninstall: boolean;
}

const accountsLabel = (n: number) => `${n} other LiveReview ${n === 1 ? 'account' : 'accounts'}`;

// The org's connected GitHub accounts: each one is a heading with its repositories indented below it, a
// "Manage on GitHub" button and (for owners) "Disconnect" and "Uninstall"; plus "Add repositories or organization".
// Several LiveReview accounts can share one installation, so Disconnect only affects this account while Uninstall
// removes the App from GitHub for all of them.
// Shared by the Git Providers page and the get-started page.
const GitHubAppAccounts: React.FC<GitHubAppAccountsProps> = ({
    installations,
    repos,
    canDisconnect,
    onInstallAnother,
    installBusy = false,
    onDisconnected,
}) => {
    const [pending, setPending] = useState<PendingAction | null>(null);
    const [showingOthers, setShowingOthers] = useState<GitHubAppInstallation | null>(null);
    const [busy, setBusy] = useState(false);

    const reposByOwner = (owner: string) =>
        repos.filter((r) => r.full_name.split('/')[0].toLowerCase() === owner.toLowerCase());

    const confirmAction = async () => {
        if (!pending) return;
        const { inst, uninstall } = pending;
        setBusy(true);
        try {
            await disconnectGitHubInstallation(inst.connector_id, uninstall);
            notify.success(uninstall
                ? `Uninstalled the app from ${inst.account_login} successfully`
                : `Disconnected ${inst.account_login} successfully`);
            setPending(null);
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
                                        <p className="text-sm text-slate-300">
                                            {inst.repo_scope === 'user'
                                                ? 'The GitHub App was already installed here. Showing the repositories your GitHub account can access.'
                                                : 'Connected through the GitHub App'}
                                            {inst.other_orgs > 0 && (
                                                <>
                                                    {' '}Also used by{' '}
                                                    <button
                                                        type="button"
                                                        onClick={() => setShowingOthers(inst)}
                                                        className="text-blue-400 hover:text-blue-300 underline underline-offset-2"
                                                    >
                                                        {accountsLabel(inst.other_orgs)}
                                                    </button>
                                                    .
                                                </>
                                            )}
                                        </p>
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
                                        <>
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                onClick={() => setPending({ inst, uninstall: false })}
                                                className="!text-red-400 hover:!text-red-300 hover:!border-red-400"
                                            >
                                                Disconnect
                                            </Button>
                                            <Button
                                                size="sm"
                                                variant="outline"
                                                onClick={() => setPending({ inst, uninstall: true })}
                                                className="!text-red-400 hover:!text-red-300 hover:!border-red-400"
                                            >
                                                Uninstall
                                            </Button>
                                        </>
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
                show={pending !== null}
                loading={busy}
                type="danger"
                title={pending?.uninstall
                    ? `Uninstall from ${pending.inst.account_login}?`
                    : `Disconnect ${pending?.inst.account_login ?? 'this account'}?`}
                message={confirmMessage(pending)}
                confirmText={pending?.uninstall ? 'Uninstall and disconnect' : 'Disconnect'}
                onConfirm={confirmAction}
                onCancel={() => setPending(null)}
            />
            {showingOthers && (
                <OtherAccountsModal inst={showingOthers} onClose={() => setShowingOthers(null)} />
            )}
        </>
    );
};

// Lists the other LiveReview accounts using an installation, by their owners' masked emails.
const OtherAccountsModal: React.FC<{ inst: GitHubAppInstallation; onClose: () => void }> = ({ inst, onClose }) => {
    const unnamed = inst.other_orgs - inst.other_accounts.length;
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        document.addEventListener('keydown', onKey);
        return () => document.removeEventListener('keydown', onKey);
    }, [onClose]);
    return createPortal(
        <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
            <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} />
            <div
                role="dialog"
                aria-modal="true"
                aria-labelledby="gh-other-accounts-title"
                className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-sm w-full"
            >
                <div className="p-6">
                    <h3 id="gh-other-accounts-title" className="text-lg font-medium text-white">Also using LiveReview on {inst.account_login}</h3>
                    <p className="mt-1 text-sm text-slate-400">
                        Each of these LiveReview accounts has its own connection to this installation.
                    </p>
                    <ul className="mt-4 space-y-2">
                        {inst.other_accounts.map((email, i) => (
                            // Masked emails can repeat (same first letters and domain), so they aren't keys.
                            <li key={i} className="flex items-center gap-2 text-sm text-slate-200">
                                <Icons.User />
                                <span className="break-all">{email}</span>
                            </li>
                        ))}
                        {unnamed > 0 && (
                            <li className="text-sm text-slate-400">
                                {unnamed} more {unnamed === 1 ? 'account' : 'accounts'} without an active owner
                            </li>
                        )}
                    </ul>
                    <div className="flex justify-end pt-6">
                        <Button onClick={onClose} autoFocus>Close</Button>
                    </div>
                </div>
            </div>
        </div>,
        document.body
    );
};

function confirmMessage(pending: PendingAction | null): string {
    if (!pending) return '';
    const { inst, uninstall } = pending;
    if (!uninstall) {
        return `LiveReview stops reviewing ${inst.account_login}'s pull requests for your account and removes its repositories here. ` +
            'The app stays installed on GitHub' +
            (inst.other_orgs > 0
                ? `, so the ${accountsLabel(inst.other_orgs)} using it ${inst.other_orgs === 1 ? 'keeps' : 'keep'} working`
                : '') +
            '. To connect it again, use "Add repositories or organization".';
    }
    return `This uninstalls the LiveReview GitHub App from ${inst.account_login} on GitHub and stops LiveReview reviewing its pull requests. ` +
        (inst.other_orgs > 0
            ? `${accountsLabel(inst.other_orgs)} ${inst.other_orgs === 1 ? 'also uses' : 'also use'} LiveReview on ${inst.account_login}; uninstalling removes it for all of them. `
            : '') +
        'To use it again, install the app on GitHub again.';
}

export default GitHubAppAccounts;
