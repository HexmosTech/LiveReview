package scheduledreview

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
)

// gocron has no standalone "next run time" function - a throwaway Job/Scheduler is needed to read NextRuns() back.

// ParseCronExpression validates a standard 5-field cron expression (minute hour dom month dow), UTC.
func ParseCronExpression(expr string) error {
	_, err := nextRuns(expr, 1)
	return err
}

// nextRuns spins up a throwaway scheduler+job just long enough to read back `count` upcoming run times.
func nextRuns(expr string, count int) ([]time.Time, error) {
	// cron_expression is stored as UTC; gocron defaults to time.Local, so this must be explicit.
	s, err := gocron.NewScheduler(gocron.WithLocation(time.UTC))
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}
	defer func() { _ = s.Shutdown() }()

	j, err := s.NewJob(gocron.CronJob(expr, false), gocron.NewTask(func() {}))
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}

	s.Start()
	runs, err := j.NextRuns(count)
	if err != nil {
		return nil, fmt.Errorf("failed to compute next run(s) for %q: %w", expr, err)
	}
	return runs, nil
}

// NextRunAfter computes the next occurrence of a cron expression; `after` must be (approximately) time.Now(), since gocron always schedules relative to its own clock.
func NextRunAfter(expr string, after time.Time) (time.Time, error) {
	runs, err := nextRuns(expr, 1)
	if err != nil {
		return time.Time{}, err
	}
	if len(runs) == 0 {
		return time.Time{}, fmt.Errorf("no upcoming runs for cron expression %q", expr)
	}
	return runs[0], nil
}

// ApproxInterval estimates the gap between a cron's next two runs; used as the lookback window for a repo's first scheduled run.
func ApproxInterval(expr string, from time.Time) (time.Duration, error) {
	runs, err := nextRuns(expr, 2)
	if err != nil {
		return 0, err
	}
	if len(runs) < 2 {
		return 0, fmt.Errorf("not enough upcoming runs to estimate an interval for %q", expr)
	}
	return runs[1].Sub(runs[0]), nil
}
