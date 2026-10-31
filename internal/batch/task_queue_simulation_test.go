package batch

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// simTask is a fake Task whose Execute just sleeps for a configured duration
// instead of calling a real LLM - deterministic, no network, fast in CI.
// Durations below are review 13536's real shape scaled down 1000x (seconds
// -> milliseconds) so the test suite stays fast while preserving the ratio
// that actually matters: 3 batches that were each individually slow (~5min
// real) vs. 4 batches that were fast (~1s real) once given a worker.
type simTask struct {
	id       string
	duration time.Duration
	started  *atomic.Int64 // number of times Execute began, for starvation checks
}

func (s *simTask) Execute(ctx context.Context) (interface{}, error) {
	if s.started != nil {
		s.started.Add(1)
	}
	select {
	case <-time.After(s.duration):
		return "ok", nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *simTask) ID() string      { return s.id }
func (s *simTask) MaxRetries() int { return 0 } // no retries needed for this simulation

// review13528Shape returns review 13536's real batch durations, scaled
// 1000x down from seconds to milliseconds (batches 1-3's ~5min real stalls
// become 300ms; batches 4-7's near-instant real latency becomes 1ms).
func review13536Shape() []time.Duration {
	return []time.Duration{
		300 * time.Millisecond, // batch-1 (25 files, real ~5min)
		300 * time.Millisecond, // batch-2 (9 files, real ~5min)
		300 * time.Millisecond, // batch-3 (7 files, real ~5min)
		time.Millisecond,       // batch-4 (10 files, real ~1s once it got a worker)
		time.Millisecond,       // batch-5 (1 file, real ~1s)
		time.Millisecond,       // batch-6 (6 files, real ~1s)
		time.Millisecond,       // batch-7 (5 files, real ~1s)
	}
}

func buildTasks(durations []time.Duration) []Task {
	tasks := make([]Task, len(durations))
	for i, d := range durations {
		tasks[i] = &simTask{id: fmt.Sprintf("batch-%d", i+1), duration: d}
	}
	return tasks
}

// TestSimulation_OldTwoWorkerModel_IsSlow reproduces the OLD behavior -
// NewTaskQueue(2), matching runtime.NumCPU() on the production box this bug
// was found on - against review 13536's real shape, and shows it takes
// roughly as long as the 3 slow batches serialized two-at-a-time (~450ms
// scaled, i.e. ~7.5 real minutes), not the ~300ms a fully concurrent run
// would take.
func TestSimulation_OldTwoWorkerModel_IsSlow(t *testing.T) {
	q := NewTaskQueue(2)
	for _, task := range buildTasks(review13536Shape()) {
		q.AddTask(task)
	}

	start := time.Now()
	results := q.ProcessAll(context.Background())
	elapsed := time.Since(start)

	if len(results) != 7 {
		t.Fatalf("got %d results, want 7", len(results))
	}
	for id, r := range results {
		if r.Error != nil {
			t.Fatalf("task %s failed: %v", id, r.Error)
		}
	}

	// With 2 workers and 3 tasks at 300ms each plus 4 at 1ms, the 3 slow
	// tasks alone can't all run concurrently - they serialize in pairs,
	// taking roughly 2x a single slow task's duration (~600ms) at minimum.
	// This asserts the old model is measurably slower than one slow batch's
	// duration, proving the starvation is real, not theoretical.
	if elapsed < 450*time.Millisecond {
		t.Fatalf("old 2-worker model finished in %v, expected it to be slowed by serializing the 3 slow batches (>= 450ms)", elapsed)
	}
}

// TestSimulation_NewCeilingModel_RunsAllConcurrently is the direct comparison:
// the same review13536Shape, but with a ceiling of 16 (this plan's new
// default) instead of 2. All 7 batches fit under the ceiling, so they all
// run at once - total wall-clock should be bounded by the single SLOWEST
// task (~300ms), not by the sum/serialization of the slow ones.
func TestSimulation_NewCeilingModel_RunsAllConcurrently(t *testing.T) {
	q := NewTaskQueue(16)
	for _, task := range buildTasks(review13536Shape()) {
		q.AddTask(task)
	}

	start := time.Now()
	results := q.ProcessAll(context.Background())
	elapsed := time.Since(start)

	if len(results) != 7 {
		t.Fatalf("got %d results, want 7", len(results))
	}
	for id, r := range results {
		if r.Error != nil {
			t.Fatalf("task %s failed: %v", id, r.Error)
		}
	}

	// Bounded by the single slowest task (300ms) plus scheduling slop, not
	// by any serialization - this is the ~2x+ measured improvement over the
	// old model's >= 450ms floor.
	if elapsed > 400*time.Millisecond {
		t.Fatalf("new ceiling-16 model finished in %v, expected ~300ms (all batches run concurrently, bounded by the single slowest one)", elapsed)
	}
}

// TestSimulation_StarvationUnderDeadline reproduces the exact review-13536
// bug directly: 3 "hog" tasks that each run for nearly the entire review
// budget, and 4 "fast" tasks that would succeed almost instantly *if given a
// worker in time*. Under the OLD model (2 workers), the fast tasks never get
// a worker before the outer deadline fires and are cancelled uncompleted -
// exactly what happened to batches 4-7. Under the NEW model (ceiling >= 7),
// every fast task gets to run immediately alongside the hogs and completes
// well before the deadline.
func TestSimulation_StarvationUnderDeadline(t *testing.T) {
	const fastDuration = time.Millisecond
	const deadline = 200 * time.Millisecond // the "review timeout" for this test

	// Matches review 13536's real shape, not three equal-duration hogs:
	// batch-1 occupied ONE worker slot for virtually the entire 10-minute
	// review (it needed two back-to-back ~5min attempts); batch-2 (one
	// ~5min attempt) and batch-3 (dispatched right after, also ~5min)
	// together kept occupying the OTHER slot for the full duration too.
	// Net effect: both slots stay continuously busy for the whole deadline
	// window, so batch-4..7 never get a slot at all - not "mostly don't."
	buildShape := func() []Task {
		return []Task{
			&simTask{id: "hog-1", duration: 250 * time.Millisecond}, // spans the whole deadline alone
			&simTask{id: "hog-2", duration: 90 * time.Millisecond},  // frees early...
			&simTask{id: "hog-3", duration: 150 * time.Millisecond}, // ...but hog-3 immediately re-occupies that slot past the deadline
			&simTask{id: "fast-1", duration: fastDuration},
			&simTask{id: "fast-2", duration: fastDuration},
			&simTask{id: "fast-3", duration: fastDuration},
			&simTask{id: "fast-4", duration: fastDuration},
		}
	}

	countFastSuccesses := func(results map[string]*TaskResult) int {
		n := 0
		for id, r := range results {
			if len(id) >= 4 && id[:4] == "fast" && r.Error == nil {
				n++
			}
		}
		return n
	}

	t.Run("old model: 2 workers lets fast tasks die uncompleted", func(t *testing.T) {
		q := NewTaskQueue(2)
		for _, task := range buildShape() {
			q.AddTask(task)
		}
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()

		results := q.ProcessAll(ctx)
		fastOK := countFastSuccesses(results)
		if fastOK != 0 {
			t.Fatalf("old 2-worker model let %d/4 fast tasks complete, want 0 - both slots stay continuously busy with hogs for the entire deadline window, so no fast task should ever get a slot (this is exactly what happened to review 13536's batch-4..7)", fastOK)
		}
	})

	t.Run("new model: ceiling >= 7 lets all fast tasks complete despite the hogs", func(t *testing.T) {
		q := NewTaskQueue(16)
		for _, task := range buildShape() {
			q.AddTask(task)
		}
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()

		results := q.ProcessAll(ctx)
		fastOK := countFastSuccesses(results)
		if fastOK != 4 {
			t.Fatalf("new ceiling model: %d/4 fast tasks completed before the deadline, want all 4 (every batch should get a worker immediately, regardless of the 3 hogs)", fastOK)
		}
	})
}

// TestSimulation_RetryLoopBailsImmediatelyOnCancellation proves the
// break-only-exits-select bug fix: a task that always errors, under a
// context that's cancelled mid-retry, must not keep calling Execute after
// cancellation - Retries should reflect bailing out promptly, and Execute
// must not be invoked after ctx is done.
func TestSimulation_RetryLoopBailsImmediatelyOnCancellation(t *testing.T) {
	var started atomic.Int64
	task := &alwaysFailsTask{id: "batch-x", started: &started}

	q := NewTaskQueue(1)
	q.SetRetryDelay(5 * time.Millisecond)
	q.AddTask(task)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	results := q.ProcessAll(ctx)
	r, ok := results["batch-x"]
	if !ok {
		t.Fatal("missing result for batch-x")
	}
	if r.Error == nil {
		t.Fatal("expected an error after cancellation, got nil")
	}

	// Each attempt takes ~1ms and retryDelay is 5ms; cancellation fires at
	// ~10ms. The old (buggy) loop would keep calling Execute every retry
	// iteration near-instantly once ctx was done, racking up well beyond
	// what a properly-bailing loop would attempt. A generous upper bound
	// catches the regression without being timing-flaky.
	if got := started.Load(); got > 6 {
		t.Fatalf("task.Execute was called %d times after a 10ms cancellation with instant-failing attempts - the retry loop did not bail promptly on ctx cancellation (break-only-exits-select regression)", got)
	}
}

type alwaysFailsTask struct {
	id      string
	started *atomic.Int64
}

func (a *alwaysFailsTask) Execute(ctx context.Context) (interface{}, error) {
	a.started.Add(1)
	return nil, fmt.Errorf("simulated failure")
}
func (a *alwaysFailsTask) ID() string      { return a.id }
func (a *alwaysFailsTask) MaxRetries() int { return 50 }
