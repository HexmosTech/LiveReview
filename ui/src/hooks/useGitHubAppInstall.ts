import { useEffect, useState } from 'react';
import { GitHubAppReturn, startGitHubAppConnect, GitHubAppConnectMode } from '../api/githubApp';
import { showRedirectOverlay } from '../utils/redirectOverlay';
import { notify } from '../utils/notify';

// Sends the user to GitHub for the LiveReview GitHub App: the install page ("Add repositories or organization",
// first install, etc.) or GitHub's sign-in ("Find my GitHub accounts"). The round trip is bound to the current
// session and org on the server, so whatever email the GitHub account uses, the user comes back to `returnTo` still
// signed in as themselves, with the accounts connected to this org.
export const useGitHubAppInstall = (returnTo: GitHubAppReturn, src: string, useNewTab = true) => {
    // Which round trip is starting (for the spinner on the button that was clicked); null when idle.
    const [pending, setPending] = useState<GitHubAppConnectMode | null>(null);
    // Whether GitHub was opened in a new tab (for showing the refresh banner).
    const [tabOpened, setTabOpened] = useState(false);

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

        if (useNewTab) {
            // Open GitHub in a new tab synchronously (bypasses popup blockers).
            const tab = window.open('', '_blank');
            // Cut the link back to this page for security.
            if (tab) tab.opener = null;

            try {
                const url = await startGitHubAppConnect(mode, src, returnTo);
                if (tab) {
                    tab.location.href = url;
                    setTabOpened(true);
                }
            } catch (err: any) {
                tab?.close();
                notify.error(err?.message || 'Could not start connecting to GitHub. Please try again.');
            } finally {
                setPending(null);
            }
        } else {
            // Original behavior: navigate away in the same tab.
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
        }
    };

    // Wrapped so they can be used directly as onClick handlers (which pass an event).
    const startInstall = (): void => {
        void run('install');
    };
    const startDiscover = (): void => {
        void run('discover');
    };

    return {
        startInstall,
        startDiscover,
        busy: pending !== null,
        installing: pending === 'install',
        discovering: pending === 'discover',
        tabOpened,
        dismissBanner: () => setTabOpened(false),
    };
};
