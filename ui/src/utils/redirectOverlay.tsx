import React from 'react';
import { createRoot } from 'react-dom/client';
import { FullScreenLoader } from '../components/FullScreenLoader';

// Covers the whole page with the shared full-screen loader until the browser navigates away. Used when a flow signs
// the user out and then leaves for GitHub: without it the logged-out login screen flashes while the navigation starts.
export const showRedirectOverlay = (text: string) => {
    if (document.getElementById('lr-redirect-overlay')) return;
    const host = document.createElement('div');
    host.id = 'lr-redirect-overlay';
    host.style.cssText = 'position:fixed;inset:0;z-index:2147483647;';
    document.body.appendChild(host);
    const root = createRoot(host);
    root.render(<FullScreenLoader text={text} />);
    // Back from the destination can restore this page from the browser's cache with the overlay still on it.
    window.addEventListener(
        'pageshow',
        (e) => {
            if (!e.persisted) return;
            root.unmount();
            host.remove();
        },
        { once: true }
    );
};
