/**
 * Utility functions for checking connector duplicates
 */

/**
 * Normalizes a URL by removing protocol, trailing slashes, and normalizing case
 * @param url URL to normalize
 * @returns Normalized URL for comparison
 */
export function normalizeUrl(url: string): string {
  if (!url) return '';
  return url.replace(/^https?:\/\//, '')  // Remove protocol
            .replace(/\/+$/, '')          // Remove trailing slashes
            .trim().toLowerCase();        // Normalize case and whitespace
}

/**
 * Checks if a provider URL already exists in the connectors list
 * @param connectors List of existing connectors
 * @param newProviderUrl New provider URL to check
 * @returns Boolean indicating if the URL is a duplicate
 */
export function isDuplicateConnector(
  connectors: Array<{url?: string, metadata?: {provider_url?: string}}>, 
  newProviderUrl: string
): boolean {
  if (!newProviderUrl) return false;
  
  const normalizedNewUrl = normalizeUrl(newProviderUrl);
  
  return connectors.some(connector => {
    const connectorUrl = normalizeUrl(connector.url || '');
    const metadataProviderUrl = connector.metadata?.provider_url ? 
      normalizeUrl(connector.metadata.provider_url) : '';
    
    return connectorUrl === normalizedNewUrl || metadataProviderUrl === normalizedNewUrl;
  });
}
