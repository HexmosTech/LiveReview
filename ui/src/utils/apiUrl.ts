/**
 * Get the correct API URL for the LiveReview backend.
 * 
 * Priority:
 * 1. Runtime config injected by Go server (window.LIVEREVIEW_CONFIG.apiUrl)
 * 2. Fall back to current host (production reverse proxy mode)
 */
export function getApiUrl(): string {
    if (typeof window === 'undefined') {
        return 'http://localhost:8888';
    }
    
    // Primary: runtime config injected by Go server
    if ((window as any).LIVEREVIEW_CONFIG?.apiUrl) {
        return (window as any).LIVEREVIEW_CONFIG.apiUrl;
    }
    
    // Fall back to current host (production reverse proxy mode)
    const currentUrl = new URL(window.location.href);
    return `${currentUrl.protocol}//${currentUrl.host}`;
}
