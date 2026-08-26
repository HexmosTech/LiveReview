package blastradius

// Tier buckets a 0-100 Combined score into the same four tiers
// ui/src/lib/blastRadius.ts's blastRadiusTier uses, so a hunk's stored Tier
// and the frontend's own (fallback) computation never disagree.
func Tier(combined float64) string {
	switch {
	case combined >= 66:
		return "blast-radius-high"
	case combined >= 33:
		return "blast-radius-medium"
	case combined > 0:
		return "blast-radius-low"
	default:
		return "blast-radius-none"
	}
}
