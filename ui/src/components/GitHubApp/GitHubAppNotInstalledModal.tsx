import React, { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { LuChevronDown, LuChevronUp, LuExternalLink } from 'react-icons/lu';
import { Button, Icons } from '../UIPrimitives';

// How long the button shows "Opening GitHub…" after a click before the GitHub tab actually opens. The "Finished
// installing?" section appears right away, so it's clear something happened. The tab is opened from a timer:
// browsers still allow that shortly after a click.
const OPEN_DELAY_MS = 2000;

interface GitHubAppNotInstalledModalProps {
    owner: string;
    repository: string;
    // Opens GitHub's install page for `owner` in a new tab (this page stays put). Returns false when the browser
    // blocked the tab; `installUrl` is then offered as a plain link.
    onInstall: () => boolean;
    installUrl: string;
    // After installing: resolves true when LiveReview can now see the installation (the parent then starts the
    // review), false when it can't yet.
    onInstalled: () => Promise<boolean>;
    // Run the review anyway; its findings are shown in LiveReview but nothing is posted to the pull request.
    onContinue: () => void;
    onCancel: () => void;
    continueBusy?: boolean;
}

// Shown before a review starts from a pasted GitHub pull request URL when the LiveReview GitHub App can't post to
// that repository. Styled like ConfirmModal. Installing happens in another tab (the GitHub round trip ends in a
// sign-in there), so once it is opened a collapsible "Finished installing?" section appears with the highlighted
// button that continues the review here.
const GitHubAppNotInstalledModal: React.FC<GitHubAppNotInstalledModalProps> = ({
    owner,
    repository,
    onInstall,
    installUrl,
    onInstalled,
    onContinue,
    onCancel,
    continueBusy = false,
}) => {
    const [opening, setOpening] = useState(false);
    const [opened, setOpened] = useState(false);
    const [popupBlocked, setPopupBlocked] = useState(false);
    const openTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
    useEffect(() => () => { if (openTimer.current) clearTimeout(openTimer.current); }, []);
    const [sectionOpen, setSectionOpen] = useState(true);
    const [checking, setChecking] = useState(false);
    const [notSeenYet, setNotSeenYet] = useState(false);
    const [checkFailed, setCheckFailed] = useState(false);
    const busy = checking || continueBusy || opening;

    const install = () => {
        setOpening(true);
        setOpened(true);
        setSectionOpen(true);
        setPopupBlocked(false);
        setNotSeenYet(false);
        setCheckFailed(false);
        if (openTimer.current) clearTimeout(openTimer.current);
        openTimer.current = setTimeout(() => {
            setPopupBlocked(!onInstall());
            setOpening(false);
        }, OPEN_DELAY_MS);
    };

    const installed = async () => {
        setChecking(true);
        setNotSeenYet(false);
        setCheckFailed(false);
        try {
            if (!(await onInstalled())) setNotSeenYet(true);
        } catch (err) {
            console.error('GitHub App install check failed:', err);
            setCheckFailed(true);
        } finally {
            setChecking(false);
        }
    };

    return createPortal(
        <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
            <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={busy ? undefined : onCancel} />
            <div
                role="dialog"
                aria-modal="true"
                className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-md w-full"
            >
                <div className="p-6">
                    <div className="flex items-center mb-4">
                        <div className="flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center mr-3 bg-yellow-500/20 text-yellow-400">
                            <Icons.Warning />
                        </div>
                        <h3 className="text-lg font-medium text-white">GitHub App not installed on {owner}</h3>
                    </div>
                    <p className="text-sm text-slate-300">
                        The LiveReview GitHub App isn&apos;t installed on <span className="font-medium text-white">{owner}</span> or
                        on <span className="font-medium text-white">{repository}</span>, so LiveReview can&apos;t post comments to this
                        pull request. You can still review it and read the findings in LiveReview.
                    </p>
                    <div className="mt-6 flex flex-col gap-3">
                        <div>
                            <Button
                                variant={opened ? 'outline' : 'primary'}
                                icon={<Icons.GitHub />}
                                onClick={install}
                                isLoading={opening}
                                disabled={busy}
                                className="w-full"
                            >
                                {opening ? 'Opening GitHub…' : opened ? `Open the GitHub install page for ${owner}` : `Install GitHub App on ${owner}`}
                                <LuExternalLink className="ml-2 h-4 w-4" aria-hidden="true" />
                            </Button>

                            {opened && (
                                <div className="mt-3 rounded-md border border-blue-700 bg-blue-900/30">
                                    <button
                                        type="button"
                                        onClick={() => setSectionOpen((v) => !v)}
                                        aria-expanded={sectionOpen}
                                        className="flex w-full items-center justify-between px-3 py-2 text-sm font-medium text-blue-200"
                                    >
                                        Finished installing on GitHub?
                                        {sectionOpen ? <LuChevronUp className="h-4 w-4" /> : <LuChevronDown className="h-4 w-4" />}
                                    </button>
                                    {sectionOpen && (
                                        <div className="space-y-3 px-3 pb-3">
                                            {popupBlocked ? (
                                                <p className="text-xs text-yellow-300">
                                                    Your browser blocked the new tab.{' '}
                                                    <a className="underline" href={installUrl} target="_blank" rel="noopener noreferrer">
                                                        Open the GitHub install page
                                                    </a>
                                                    , finish installing, then come back to this tab.
                                                </p>
                                            ) : opening ? (
                                                <p className="text-xs text-blue-300">Opening GitHub in a new tab…</p>
                                            ) : (
                                                <p className="text-xs text-blue-300">
                                                    GitHub opened in a new tab. Finish installing there, then come back to this tab. You can close
                                                    the other tab afterwards.
                                                </p>
                                            )}
                                            <Button onClick={installed} isLoading={checking} disabled={busy} className="w-full">
                                                I&apos;ve installed it, continue review
                                            </Button>
                                            {checkFailed && (
                                                <p className="text-xs text-red-300">Couldn&apos;t check the installation right now. Please try again.</p>
                                            )}
                                            {notSeenYet && (
                                                <p className="text-xs text-yellow-300">
                                                    LiveReview doesn&apos;t see the installation on {repository} yet. Make sure you clicked Install on
                                                    GitHub, wait a few seconds, then try again.
                                                </p>
                                            )}
                                        </div>
                                    )}
                                </div>
                            )}
                        </div>
                        <Button variant="outline" onClick={onContinue} isLoading={continueBusy} disabled={busy} className="w-full">
                            Continue review without posting comments
                        </Button>
                    </div>
                </div>
            </div>
        </div>,
        document.body
    );
};

export default GitHubAppNotInstalledModal;
