package logging

import "testing"

func TestFindBatchIDCanonicalTokens(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "canonical hyphen token",
			message: "processing batch-42 now",
			want:    "batch-42",
		},
		{
			name:    "underscore token normalizes to hyphen",
			message: "processing batch_77 now",
			want:    "batch-77",
		},
		{
			name:    "batch token is extracted with punctuation",
			message: "completed (batch-9), retry_count=0",
			want:    "batch-9",
		},
		{
			name:    "non canonical batch then number is ignored",
			message: "processing batch 3 now",
			want:    "",
		},
		{
			name:    "alpha suffix is ignored",
			message: "processing batch-abc now",
			want:    "",
		},
		{
			name:    "mixed suffix is ignored",
			message: "processing batch-12x now",
			want:    "",
		},
		{
			name:    "uppercase token is accepted",
			message: "BATCH-15 started",
			want:    "batch-15",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findBatchID(tc.message)
			if got != tc.want {
				t.Fatalf("findBatchID(%q) = %q, want %q", tc.message, got, tc.want)
			}
		})
	}
}
