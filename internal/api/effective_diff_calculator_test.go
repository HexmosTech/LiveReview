package api

import (
	"testing"

	"github.com/livereview/cmd/mrmodel/lib"
)

func TestCalculateEffectiveDiffLOCFromLocalDiffs(t *testing.T) {
	input := []lib.LocalCodeDiff{
		{
			OldPath: "a.go",
			NewPath: "a.go",
			Hunks: []lib.LocalDiffHunk{
				{
					Lines: []lib.LocalDiffLine{
						{LineType: "context"},
						{LineType: "added"},
						{LineType: "added"},
						{LineType: "deleted"},
					},
				},
			},
		},
		{
			OldPath: "b.go",
			NewPath: "b.go",
			Hunks: []lib.LocalDiffHunk{
				{
					Lines: []lib.LocalDiffLine{
						{LineType: "context"},
						{LineType: "deleted"},
					},
				},
			},
		},
	}

	got := CalculateEffectiveDiffLOCFromLocalDiffs(input)
	if got != 4 {
		t.Fatalf("expected billable loc=4, got=%d", got)
	}
}

func TestCalculateEffectiveDiffLOCFromLocalDiffs_Empty(t *testing.T) {
	if got := CalculateEffectiveDiffLOCFromLocalDiffs(nil); got != 0 {
		t.Fatalf("expected 0 for nil input, got=%d", got)
	}

	if got := CalculateEffectiveDiffLOCFromLocalDiffs([]lib.LocalCodeDiff{}); got != 0 {
		t.Fatalf("expected 0 for empty input, got=%d", got)
	}
}

func TestCalculateEffectiveDiffLOCFromLocalDiffs_IgnoresUnknownLineTypes(t *testing.T) {
	input := []lib.LocalCodeDiff{
		{
			Hunks: []lib.LocalDiffHunk{
				{
					Lines: []lib.LocalDiffLine{
						{LineType: "added"},
						{LineType: "deleted"},
						{LineType: "context"},
						{LineType: "other"},
					},
				},
			},
		},
	}

	got := CalculateEffectiveDiffLOCFromLocalDiffs(input)
	if got != 2 {
		t.Fatalf("expected billable loc=2, got=%d", got)
	}
}
