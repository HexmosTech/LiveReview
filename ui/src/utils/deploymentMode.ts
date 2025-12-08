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

/**
 * Runtime validation: verify frontend/backend deployment mode match
 * Call this on app initialization to detect configuration mismatches
 * 
 * @param backendIsCloud - the isCloud value returned from backend /api/v1/ui-config
 * @returns true if modes match, false if mismatch detected
 */
export const validateDeploymentModeMatch = (backendIsCloud: boolean): boolean => {
	const frontendIsCloud = isCloudMode();
	
	if (frontendIsCloud !== backendIsCloud) {
		console.error('[CRITICAL] Frontend/Backend cloud mode mismatch!');
		console.error(`  Frontend LIVEREVIEW_IS_CLOUD: ${frontendIsCloud}`);
		console.error(`  Backend LIVEREVIEW_IS_CLOUD: ${backendIsCloud}`);
		console.error('  This indicates a configuration error. Please contact support.');
		return false;
	}
	
	console.info(`[LiveReview] Deployment mode validated: ${frontendIsCloud ? 'Cloud' : 'Self-Hosted'}`);
	return true;
};
