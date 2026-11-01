import { formatSyncError } from './Repositories';

describe('formatSyncError', () => {
  it('returns default fallback when errorMsg is empty or undefined', () => {
    expect(formatSyncError(undefined)).toBe('Sync failed: an error occurred during repository sync');
    expect(formatSyncError('')).toBe('Sync failed: an error occurred during repository sync');
  });

  it('sanitizes upstream HTML error page with status code', () => {
    const rawHtmlErr = 'API request failed with status 404: <!DOCTYPE html><html><head><title>404</title></head><body>Not Found</body></html>';
    expect(formatSyncError(rawHtmlErr)).toBe('API request failed with status 404 (upstream returned HTML page)');
  });

  it('sanitizes upstream HTML error page without status code', () => {
    const rawHtmlErr = '<html><body>Bad Gateway</body></html>';
    expect(formatSyncError(rawHtmlErr)).toBe('Sync failed: upstream provider returned an unexpected HTML error page');
  });

  it('truncates excessively long error messages to 250 characters', () => {
    const longErr = 'A'.repeat(300);
    const result = formatSyncError(longErr);
    expect(result.length).toBe(253); // 250 + '...'
    expect(result.endsWith('...')).toBe(true);
  });

  it('keeps clean standard error messages intact', () => {
    const cleanErr = 'Authentication failed: access token expired';
    expect(formatSyncError(cleanErr)).toBe(cleanErr);
  });
});

