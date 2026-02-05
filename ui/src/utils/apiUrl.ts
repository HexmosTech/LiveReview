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
    // Trim whitespace to handle malformed config values
    const configUrl = (window as any).LIVEREVIEW_CONFIG?.apiUrl;
    if (configUrl && typeof configUrl === 'string' && configUrl.trim() !== '') {
        return configUrl.trim();
    }
    
    // Fall back to current host (production reverse proxy mode)
    const currentUrl = new URL(window.location.href);
    return `${currentUrl.protocol}//${currentUrl.host}`;
}
