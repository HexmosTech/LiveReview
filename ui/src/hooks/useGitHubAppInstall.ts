import { useEffect, useRef, useState } from 'react';
import { GitHubAppReturn, startGitHubAppConnect, GitHubAppConnectMode } from '../api/githubApp';
import { showRedirectOverlay } from '../utils/redirectOverlay';
import { notify } from '../utils/notify';

// How long the banner shows "Opening GitHub in a new tab…" after a click. Then it switches to the "close that tab and
// click Refresh" message and, READ_DELAY_MS later, the GitHub tab opens (like the install button on New Review). The tab is
// opened from a timer: browsers still allow that shortly after a click.
const OPEN_DELAY_MS = 1000;
// How long the instructions are on screen before the GitHub tab opens and takes focus, so they can be read first.
const READ_DELAY_MS = 3000;

export type GitHubTabState = 'idle' | 'opening' | 'opened' | 'blocked';

// What the "GitHub is open in another tab" banner needs.
export interface GitHubTabControls {
    state: GitHubTabState;
    // Which button started it: the install page ("Add repositories or organization") or a "Manage on GitHub" link.
    kind: 'install' | 'manage' | null;
    // The page that was to open, offered as a plain link when the browser blocked the tab.
    url: string | null;
    // True from the click until the GitHub tab has opened (or failed to): the clicked button keeps its spinner.
    launching: boolean;
    // Opens GitHub's install page in a new tab (this page stays put), after a short "Opening GitHub…" banner.
    openInstall: () => void;
    // Opens a GitHub page (an installation's settings) the same way.
    openManage: (url: string) => void;
    dismiss: () => void;
}

// Sends the user to GitHub for the LiveReview GitHub App.
//  - startInstall / startDiscover leave this page for GitHub and come back to `returnTo` (first install, "Find my
//    GitHub accounts"). The round trip is bound to the current session and org on the server, so whatever email the
//    GitHub account uses, the user comes back still signed in as themselves, with the accounts connected to this org.
//  - githubTab opens GitHub in a new tab for accounts that are already connected, so this page stays put and the
//    user clicks Refresh when done.
export const useGitHubAppInstall = (returnTo: GitHubAppReturn, src: string) => {
    // Which round trip is starting (for the spinner on the button that was clicked); null when idle.
    const [pending, setPending] = useState<GitHubAppConnectMode | null>(null);
    const [tab, setTab] = useState<{ state: GitHubTabState; kind: 'install' | 'manage' | null; url: string | null; launching: boolean }>({
        state: 'idle',
        kind: null,
        url: null,
        launching: false,
    });
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
    // Bumped by every click and dismissal, so a slow start call from an earlier click can't touch a newer state.
    const attempt = useRef(0);
    const clearTimer = () => {
        if (timer.current) clearTimeout(timer.current);
        timer.current = null;
    };
    useEffect(() => clearTimer, []);

    // Coming back with the browser's Back button can restore this page as it was (spinner and all): reset it.
    useEffect(() => {
        const onPageShow = (e: PageTransitionEvent) => {
            if (e.persisted) setPending(null);
        };
        window.addEventListener('pageshow', onPageShow);
        return () => window.removeEventListener('pageshow', onPageShow);
    }, []);

    const run = async (mode: GitHubAppConnectMode) => {
        setPending(mode);
        let url: string;
        try {
            url = await startGitHubAppConnect(mode, src, returnTo);
        } catch (err: any) {
            setPending(null);
            notify.error(err?.message || 'Could not start connecting to GitHub. Please try again.');
            return;
        }
        // Covers the page while the browser starts navigating, so nothing flashes.
        showRedirectOverlay('Taking you to GitHub…');
        // assign, not replace: Back from GitHub returns to this page.
        window.location.assign(url);
    };

    // Shows the banner now and opens `getUrl()` in a new tab after OPEN_DELAY_MS.
    const openInTab = (kind: 'install' | 'manage', getUrl: () => Promise<string>, knownUrl: string | null = null) => {
        clearTimer();
        const mine = ++attempt.current;
        setTab({ state: 'opening', kind, url: knownUrl, launching: true });
        const urlPromise = getUrl();
        let failed = false;
        urlPromise.catch((err: any) => {
            failed = true;
            if (mine !== attempt.current) return;
            clearTimer();
            setTab({ state: 'idle', kind: null, url: null, launching: false });
            notify.error(err?.message || 'Could not start connecting to GitHub. Please try again.');
        });
        timer.current = setTimeout(async () => {
            timer.current = null;
            if (failed) return;
            let url: string;
            try {
                url = await urlPromise;
            } catch {
                return; // reported above
            }
            if (mine !== attempt.current) return; // dismissed or replaced while the start call ran
            // The instructions show first; the tab opens READ_DELAY_MS later.
            setTab({ state: 'opened', kind, url, launching: true });
            timer.current = setTimeout(() => {
                timer.current = null;
                const opened = window.open(url, '_blank');
                // A tab opened this way can reach back to this page through window.opener; cut that link.
                if (opened) opened.opener = null;
                setTab({ state: opened ? 'opened' : 'blocked', kind, url, launching: false });
            }, READ_DELAY_MS);
        }, OPEN_DELAY_MS);
    };

    // Wrapped so they can be used directly as onClick handlers (which pass an event).
    const startInstall = (): void => {
        void run('install');
    };
    const startDiscover = (): void => {
        void run('discover');
    };

    const githubTab: GitHubTabControls = {
        state: tab.state,
        kind: tab.kind,
        url: tab.url,
        launching: tab.launching,
        openInstall: () => openInTab('install', () => startGitHubAppConnect('install', src, returnTo, undefined, true)),
        openManage: (url: string) => openInTab('manage', () => Promise.resolve(url), url),
        dismiss: () => {
            attempt.current++;
            clearTimer();
            setTab({ state: 'idle', kind: null, url: null, launching: false });
        },
    };

    return {
        startInstall,
        startDiscover,
        busy: pending !== null,
        installing: pending === 'install',
        discovering: pending === 'discover',
        githubTab,
    };
};
