import './styles/index.css';
import './styles/custom.css';
import './styles/darkmode.css';

import React from 'react';
import { createRoot } from 'react-dom/client';
import { Provider as ReduxProvider } from 'react-redux';
// import { PostHogProvider } from 'posthog-js/react';
// import posthog from 'posthog-js';
// import 'posthog-js/dist/posthog-recorder';

import configureAppStore, { getPreloadedState } from './store/configureStore';
import AppContextProvider from './contexts/AppContextProvider';
import App from './App';
import { injectStore } from './api/apiClient';

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
            <AppContextProvider>
                <App />
            </AppContextProvider>
        </ReduxProvider>
    </React.StrictMode>
);

root.render(appTree);

if (typeof window !== 'undefined' && typeof (window as any).__lr_hide_boot === 'function') {
    (window as any).__lr_hide_boot();
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