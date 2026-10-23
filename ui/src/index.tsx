import './styles/index.css';
import './styles/custom.css';
import './styles/darkmode.css';

import React from 'react';
import { createRoot } from 'react-dom/client';
import { Provider as ReduxProvider } from 'react-redux';
import { QueryClientProvider } from '@tanstack/react-query';
// import { PostHogProvider } from 'posthog-js/react';
// import posthog from 'posthog-js';
// import 'posthog-js/dist/posthog-recorder';

import './utils/dropSessionOnLoginReturn';
import configureAppStore, { getPreloadedState } from './store/configureStore';
import AppContextProvider from './contexts/AppContextProvider';
import App from './App';
import { queryClient } from './api/queryClient';
import { injectStore } from './api/apiClient';
import { injectStore as injectNotifyStore } from './utils/notify';

// /gh-login is a shareable path ("Continue with GitHub"), but the app is a HashRouter: move it into the hash,
// keeping the query string (src). The Go UI server and the webpack dev server both already serve index.html
// for this path.
if (typeof window !== 'undefined' && /^\/gh-login\/?$/.test(window.location.pathname)) {
    window.location.replace('/#/gh-login' + window.location.search);
}

// Where a successful GitHub sign-in from /gh-login lands, from a short-lived cookie the callback sets: get-started
// for new users, the GitHub App card on Git Providers (where connected accounts are managed) for returning ones.
const GH_NEXT_PAGES: Record<string, string> = {
    'get-started': '#/get-started/github',
    git: '#/git/github-app',
};
if (typeof document !== 'undefined') {
    try {
        const match = document.cookie.match(/(?:^|; )lr_gh_next=([a-z-]+)/);
        if (match) {
            const page = GH_NEXT_PAGES[match[1]];
            if (page) sessionStorage.setItem('redirectAfterLogin', page);
            document.cookie = 'lr_gh_next=; Max-Age=0; Path=/';
        }
    } catch {
        // Cookies or sessionStorage unavailable: the user lands on the dashboard.
    }
}

// The GitHub App callback names the org it connected the installation(s) to. A user can belong to several orgs and
// the app opens the last one used, so switch to that org before the store loads (the server sets and we clear it).
if (typeof document !== 'undefined') {
    try {
        const match = document.cookie.match(/(?:^|; )lr_gh_org=(\d+)/);
        if (match) {
            localStorage.setItem('currentOrgId', match[1]);
            document.cookie = 'lr_gh_org=; Max-Age=0; Path=/';
        }
    } catch {
        // Cookies or localStorage unavailable: the user keeps their last-used org.
    }
}

type ClarityFunction = ((...args: unknown[]) => void) & {
    q?: IArguments[];
};

type ClarityWindow = typeof window & {
    __REDUX_STORE__?: ReturnType<typeof configureAppStore>;
    __CLARITY_INITIALIZED__?: boolean;
    clarity?: ClarityFunction;
};

const ensureMicrosoftClarity = (siteId: string) => {
    const scope = window as ClarityWindow;
    if (scope.__CLARITY_INITIALIZED__) {
        console.info('[LiveReview][Clarity] already initialized', { siteId });
        return;
    }

    if (!scope.clarity) {
        const clarityFn = function () {
            (clarityFn.q = clarityFn.q || []).push(arguments);
        } as ClarityFunction;
        scope.clarity = clarityFn;
    }

    const script = document.createElement('script');
    script.async = true;
    script.src = `https://www.clarity.ms/tag/${siteId}`;
    const firstScript = document.getElementsByTagName('script')[0];
    if (firstScript?.parentNode) {
        firstScript.parentNode.insertBefore(script, firstScript);
    } else {
        document.head?.appendChild(script);
    }

    scope.__CLARITY_INITIALIZED__ = true;
    console.info('[LiveReview][Clarity] initialized', { siteId });
};

const preloadedState = getPreloadedState();
const store = configureAppStore(preloadedState);
injectStore(store);
injectNotifyStore(store);

// Make store available globally for token refresh
(window as ClarityWindow).__REDUX_STORE__ = store;

const rootElement = document.getElementById('root');
if (!rootElement) {
    throw new Error('Root element not found');
}
const root = createRoot(rootElement);

const appTree = (
    <React.StrictMode>
        <ReduxProvider store={store}>
            <QueryClientProvider client={queryClient}>
                <AppContextProvider>
                    <App />
                </AppContextProvider>
            </QueryClientProvider>
        </ReduxProvider>
    </React.StrictMode>
);

root.render(appTree);

// Wait a frame so the initial paint has actually happened before hiding the static boot
// screen - calling this synchronously right after render() only means React has scheduled
// the commit, not that anything is on screen yet, which could show a blank flash instead of a
// smooth handoff. This is now the app's only boot overlay (the old duplicate React
// `BootScreen` in App.tsx was removed - it rendered the identical logo+spinner on top of this
// one for up to 1200ms more, one loading screen doing the job of two).
if (typeof window !== 'undefined' && typeof (window as any).__lr_hide_boot === 'function') {
    requestAnimationFrame(() => {
        (window as any).__lr_hide_boot();
    });
}

void (async () => {
    const { isCloudMode, validateDeploymentModeMatch } = await import('./utils/deploymentMode');
    // Validate frontend/backend deployment mode match (non-blocking for first paint)
    try {
        const response = await fetch('/api/v1/ui-config');
        if (response.ok) {
            const config = await response.json();
            validateDeploymentModeMatch(config.isCloud);
        }
    } catch (err) {
        console.warn('[LiveReview] Could not validate deployment mode:', err);
    }

    if (isCloudMode()) {
        console.info('[LiveReview] Running in Cloud mode');
        const clarityId = process.env.LR_CLARITY_ID;
        if (clarityId) {
            console.info('[LiveReview] Initializing Clarity');
            ensureMicrosoftClarity(clarityId);
        }
    } else {
        console.info('[LiveReview] Running in Self-Hosted mode');
    }
})();