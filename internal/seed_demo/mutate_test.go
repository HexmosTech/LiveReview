package seed_demo

import (
	"testing"
	"time"
)

func TestRandomTimeInDayNeverFuture(t *testing.T) {
	cases := []struct {
		name      string
		day, now  time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "prod daily run at 13:00 UTC",
			day:       time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC),
			now:       time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 9, 26, 7, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 9, 26, 12, 58, 0, 0, time.UTC),
		},
		{
			name:      "backfill a past day uses the full 7am-11pm window",
			day:       time.Date(2026, 9, 24, 13, 0, 0, 0, time.UTC),
			now:       time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 9, 24, 23, 0, 0, 0, time.UTC),
		},
		{
			name:      "run before 7am falls back to a midnight start",
			day:       time.Date(2026, 9, 26, 5, 0, 0, 0, time.UTC),
			now:       time.Date(2026, 9, 26, 5, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 9, 26, 4, 58, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 2000; i++ {
				got, err := randomTimeInDay(tc.day, tc.now)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Before(tc.wantStart) || got.After(tc.wantEnd) {
					t.Fatalf("got %s, want within [%s, %s]", got, tc.wantStart, tc.wantEnd)
				}
			}
		})
	}
}

func TestRandomTimeInDayNoTimeLeft(t *testing.T) {
	midnight := time.Date(2026, 9, 26, 0, 1, 0, 0, time.UTC)
	if _, err := randomTimeInDay(midnight, midnight); err == nil {
		t.Fatal("expected an error when no past time is left on the day")
	}
}
