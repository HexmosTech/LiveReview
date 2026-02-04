/**
 * License tier configuration for self-hosted deployments.
 * These values define feature limits for each tier.
 */

export type LicenseTier = 'community' | 'team' | 'enterprise';

/**
 * Community tier limits - features available without a paid license
 */
export const COMMUNITY_TIER_LIMITS = {
  /** Maximum number of users allowed in an organization */
  MAX_USERS: 3,
  /** Maximum number of API keys per organization */
  MAX_API_KEYS: 1,
} as const;

/**
 * Tier order for comparison (higher number = more features)
 */
export const TIER_ORDER: Record<LicenseTier, number> = {
  community: 0,
  team: 1,
  enterprise: 2,
};

/**
 * Display names for tiers
 */
export const TIER_DISPLAY_NAMES: Record<LicenseTier, string> = {
  community: 'Community',
  team: 'Team',
  enterprise: 'Enterprise',
};

/**
 * Check if current tier has access to features requiring a specific tier
 */
export function hasTierAccess(currentTier: LicenseTier, requiredTier: LicenseTier): boolean {
  return TIER_ORDER[currentTier] >= TIER_ORDER[requiredTier];
}
