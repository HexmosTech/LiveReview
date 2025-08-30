import './styles/index.css';
import './styles/custom.css';
import './styles/darkmode.css';

import React from 'react';
import { createRoot } from 'react-dom/client';
import { Provider as ReduxProvider } from 'react-redux';

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

    root.render(
        <React.StrictMode>
            <ReduxProvider store={store}>
                <AppContextProvider>
                    <App />
                </AppContextProvider>
            </ReduxProvider>
        </React.StrictMode>
    );
})();
