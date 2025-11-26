import './styles/index.css';
import './styles/custom.css';
import './styles/darkmode.css';

import React from 'react';
import { createRoot } from 'react-dom/client';
import { Provider as ReduxProvider } from 'react-redux';
import { PostHogProvider } from 'posthog-js/react';
import "posthog-js/dist/recorder"

import configureAppStore, { getPreloadedState } from './store/configureStore';
import AppContextProvider from './contexts/AppContextProvider';
import App from './App';
import { injectStore } from './api/apiClient';

(async () => {
    const preloadedState = getPreloadedState();
    const store = configureAppStore(preloadedState);
    injectStore(store);

    // Make store available globally for token refresh
    window.__REDUX_STORE__ = store;
    
    const root = createRoot(document.getElementById('root'));

    const isCloud = (process.env.LIVEREVIEW_IS_CLOUD || '').toString().toLowerCase() === 'true';
    if (isCloud) {
        console.info("[LiveReview] Running in Cloud mode");
        root.render(
            <React.StrictMode>
                <PostHogProvider
                    apiKey="REDACTED_POSTHOG_KEY"
                    options={{
                        api_host: 'https://us.i.posthog.com',
                        person_profiles: 'always',
                        capture_pageview: true,
                        capture_pageleave: true,
                        capture_exceptions: true
                    }}
                >
                    <ReduxProvider store={store}>
                        <AppContextProvider>
                            <App />
                        </AppContextProvider>
                    </ReduxProvider>
                </PostHogProvider>
            </React.StrictMode>
            );
    } else {
        console.info("[LiveReview] Running in Self-Hosted mode");
        root.render(
            <React.StrictMode>
                    <ReduxProvider store={store}>
                        <AppContextProvider>
                            <App />
                        </AppContextProvider>
                    </ReduxProvider>
            </React.StrictMode>
            );
    }
})();