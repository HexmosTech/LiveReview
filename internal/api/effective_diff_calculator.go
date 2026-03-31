package api

import "github.com/livereview/cmd/mrmodel/lib"

// CalculateEffectiveDiffLOCFromLocalDiffs returns billable LOC for an operation.
// Billable LOC is defined as added + deleted lines across all hunks.
func CalculateEffectiveDiffLOCFromLocalDiffs(localDiffs []lib.LocalCodeDiff) int64 {
	var total int64
	for _, diff := range localDiffs {
		for _, hunk := range diff.Hunks {
			for _, line := range hunk.Lines {
				switch line.LineType {
				case "added", "deleted":
					total++
				}
			}
		}
	}
	return total
}
