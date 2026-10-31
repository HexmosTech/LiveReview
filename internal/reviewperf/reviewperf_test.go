package reviewperf

import "testing"

func intPtr(v int) *int { return &v }

func TestResolveMaxConcurrentBatches(t *testing.T) {
	settings := Settings{MaxConcurrentBatchesCeiling: 16}

	if got := settings.ResolveMaxConcurrentBatches(nil); got != 16 {
		t.Errorf("nil preference: got %d, want 16 (system ceiling)", got)
	}
	if got := settings.ResolveMaxConcurrentBatches(intPtr(4)); got != 4 {
		t.Errorf("preference below ceiling: got %d, want 4", got)
	}
	if got := settings.ResolveMaxConcurrentBatches(intPtr(64)); got != 16 {
		t.Errorf("preference above ceiling: got %d, want clamped to 16", got)
	}
	if got := settings.ResolveMaxConcurrentBatches(intPtr(-5)); got != 1 {
		t.Errorf("negative preference: got %d, want clamped to floor 1", got)
	}
}

func TestResolveReviewTimeoutSeconds(t *testing.T) {
	settings := Settings{MinReviewTimeoutSeconds: 60, MaxReviewTimeoutSeconds: 240}

	if got := settings.ResolveReviewTimeoutSeconds(nil); got != 240 {
		t.Errorf("nil preference: got %d, want 240 (system ceiling)", got)
	}
	if got := settings.ResolveReviewTimeoutSeconds(intPtr(90)); got != 90 {
		t.Errorf("preference within bounds: got %d, want 90", got)
	}
	if got := settings.ResolveReviewTimeoutSeconds(intPtr(600)); got != 240 {
		t.Errorf("preference above ceiling: got %d, want clamped to 240", got)
	}
	if got := settings.ResolveReviewTimeoutSeconds(intPtr(10)); got != 60 {
		t.Errorf("preference below floor: got %d, want clamped to 60", got)
	}
}

func TestResolve_ZeroValueSettingsFallsBackToDefaults(t *testing.T) {
	// A Settings{} (e.g. corrupt/legacy row) must not resolve to a ceiling
	// of 0, which would make every review's concurrency clamp to 0.
	var settings Settings
	if got := settings.ResolveMaxConcurrentBatches(nil); got != DefaultMaxConcurrentBatchesCeiling {
		t.Errorf("zero-value settings: got %d, want default %d", got, DefaultMaxConcurrentBatchesCeiling)
	}
	if got := settings.ResolveReviewTimeoutSeconds(nil); got != DefaultMaxReviewTimeoutSeconds {
		t.Errorf("zero-value settings: got %d, want default %d", got, DefaultMaxReviewTimeoutSeconds)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		s       Settings
		wantErr bool
	}{
		{"valid", Settings{MaxConcurrentBatchesCeiling: 16, MinReviewTimeoutSeconds: 60, MaxReviewTimeoutSeconds: 240}, false},
		{"zero ceiling", Settings{MaxConcurrentBatchesCeiling: 0, MinReviewTimeoutSeconds: 60, MaxReviewTimeoutSeconds: 240}, true},
		{"zero floor", Settings{MaxConcurrentBatchesCeiling: 16, MinReviewTimeoutSeconds: 0, MaxReviewTimeoutSeconds: 240}, true},
		{"ceiling below floor", Settings{MaxConcurrentBatchesCeiling: 16, MinReviewTimeoutSeconds: 100, MaxReviewTimeoutSeconds: 60}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.s.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
