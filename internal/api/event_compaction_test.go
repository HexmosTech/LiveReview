package api

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultCompactionSettingsConfig(t *testing.T) {
	cfg := defaultCompactionSettingsConfig()

	if !cfg.Enabled {
		t.Errorf("expected default Enabled to be true, got false")
	}

	if cfg.RetentionDays != 30 {
		t.Errorf("expected default RetentionDays to be 30, got %d", cfg.RetentionDays)
	}

	if cfg.CronExpression != "0 2 * * *" {
		t.Errorf("expected default CronExpression to be '0 2 * * *', got %q", cfg.CronExpression)
	}
}

func TestDescribeCronSchedule(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
	}{
		{
			expr:     "0 2 * * *",
			expected: "Daily at 02:00 UTC",
		},
		{
			expr:     "0 */6 * * *",
			expected: "Every 6 hours",
		},
		{
			expr:     "0 0,12 * * *",
			expected: "Multiple times daily (0,12:00 UTC)",
		},
		{
			expr:     "custom_expr",
			expected: "custom_expr",
		},
	}

	for _, tt := range tests {
		result := describeCronSchedule(tt.expr)
		if !strings.Contains(result, strings.TrimSuffix(tt.expected, " (0,12:00 UTC)")) && result != tt.expected {
			t.Errorf("describeCronSchedule(%q) = %q, want %q", tt.expr, result, tt.expected)
		}
	}
}

func TestEventCompactionManager_ConfigUpdate(t *testing.T) {
	m := &EventCompactionManager{
		enabled:       true,
		cronExpr:      "0 2 * * *",
		retentionDays: 30,
		ctx:           context.Background(),
	}

	m.UpdateConfig(false, "0 0 * * *", 60)

	if m.enabled {
		t.Errorf("expected enabled to be false after UpdateConfig")
	}

	if m.retentionDays != 60 {
		t.Errorf("expected retentionDays to be 60, got %d", m.retentionDays)
	}

	if m.cronExpr != "0 0 * * *" {
		t.Errorf("expected cronExpr to be '0 0 * * *', got %q", m.cronExpr)
	}
}
