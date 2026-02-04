/**
 * Hook for determining the current license tier in self-hosted deployments.
 */
import { useAppSelector, RootState } from '../store/configureStore';
import { isSelfHostedMode } from '../utils/deploymentMode';
import { LicenseTier, hasTierAccess } from '../constants/licenseTiers';

/**
 * Returns the current license tier based on license state.
 * 
 * Tier detection logic:
 * - Checks appName for 'enterprise', 'team', 'community', 'free' keywords
 * - Falls back to 'team' for active licenses without explicit tier
 * - Returns 'community' for missing/invalid/expired licenses
 */
export function useLicenseTier(): LicenseTier {
  const license = useAppSelector((state: RootState) => state.License);
  
  // No license or invalid license = community tier
  if (!license.loadedOnce || license.status === 'missing' || license.status === 'invalid' || license.status === 'expired') {
    return 'community';
  }
  
  const appName = license.appName?.toLowerCase() || '';
  
  // Check appName for tier indicators
  if (appName.includes('enterprise')) {
    return 'enterprise';
  }
  
  if (appName.includes('team')) {
    return 'team';
  }
  
  // Check if explicitly community/free tier
  if (appName.includes('community') || appName.includes('free')) {
    return 'community';
  }
  
  // Active/warning/grace license without explicit tier indication
  // defaults to team (paid license without specific tier name)
  if (license.status === 'active' || license.status === 'warning' || license.status === 'grace') {
    return 'team';
  }
  
  return 'community';
}

/**
 * Check if the current license tier has access to a feature requiring a specific tier.
 * In cloud mode, always returns true (no tier-based restrictions).
 * In self-hosted mode, checks against the actual license tier.
 */
export function useHasLicenseFor(requiredTier: LicenseTier): boolean {
  const currentTier = useLicenseTier();
  
  // Cloud mode has no license restrictions for these features
  if (!isSelfHostedMode()) {
    return true;
  }
  
  return hasTierAccess(currentTier, requiredTier);
}

// Re-export for convenience
export { LicenseTier, hasTierAccess, COMMUNITY_TIER_LIMITS } from '../constants/licenseTiers';
