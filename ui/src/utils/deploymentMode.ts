/**
 * Centralized deployment mode detection utilities
 * Use these helpers instead of inline environment variable checks
 */

/**
 * Check if LiveReview is running in cloud deployment mode
 * @returns true if LIVEREVIEW_IS_CLOUD=true, false otherwise
 */
export const isCloudMode = (): boolean => {
	return (process.env.LIVEREVIEW_IS_CLOUD || '').toString().toLowerCase() === 'true';
};

/**
 * Check if LiveReview is running in self-hosted deployment mode
 * @returns true if LIVEREVIEW_IS_CLOUD=false or not set, false otherwise
 */
export const isSelfHostedMode = (): boolean => {
	return !isCloudMode();
};
