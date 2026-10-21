import { useEffect, useState } from 'react';
import { githubAppStartUrl } from '../api/githubApp';
import { showRedirectOverlay } from '../utils/redirectOverlay';

// Sends the user to GitHub's install page for the LiveReview GitHub App ("Add repositories or organization",
// first install, etc.). The user stays signed in while away; the callback ends in the normal Cloud login
// (utils/dropSessionOnLoginReturn lets it replace the session) and that returns the user to `returnHash`
// (for example '#/git/github-app').
export const useGitHubAppInstall = (returnHash: string, src: string) => {
    const [busy, setBusy] = useState(false);

    // Coming back with the browser's Back button can restore this page as it was (spinner and all): reset it.
    useEffect(() => {
        const onPageShow = (e: PageTransitionEvent) => {
            if (e.persisted) setBusy(false);
        };
        window.addEventListener('pageshow', onPageShow);
        return () => window.removeEventListener('pageshow', onPageShow);
    }, []);

    // `targetId` opens GitHub's install page already on that account or organization.
    const run = (targetId?: number) => {
        setBusy(true);
        // Covers the page while the browser starts navigating, so nothing flashes.
        showRedirectOverlay('Taking you to GitHub…');
        try {
            sessionStorage.setItem('redirectAfterLogin', returnHash);
        } catch {
            // Falls back to the dashboard after login.
        }
        // assign, not replace: Back from GitHub returns to this page.
        window.location.assign(githubAppStartUrl('install', src, targetId));
    };

    // Separate from startInstallOn so it can be used directly as an onClick handler (which passes an event).
    const startInstall = () => run();
    const startInstallOn = (targetId?: number) => run(targetId);

    return { startInstall, startInstallOn, busy };
};
