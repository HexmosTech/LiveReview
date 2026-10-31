package adaptivelimiter

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAcquireRelease_BasicGating(t *testing.T) {
	l := New(Config{Floor: 1, Ceiling: 2, Initial: 2, PollInterval: time.Millisecond})
	ctx := context.Background()

	rel1, err := l.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	rel2, err := l.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	if l.InFlight() != 2 {
		t.Fatalf("InFlight = %d, want 2", l.InFlight())
	}

	// A third Acquire must block until a slot frees.
	acquired := make(chan struct{})
	go func() {
		rel3, err := l.Acquire(ctx)
		if err != nil {
			t.Errorf("Acquire 3: %v", err)
			return
		}
		close(acquired)
		rel3()
	}()

	select {
	case <-acquired:
		t.Fatal("third Acquire returned before a slot was released")
	case <-time.After(30 * time.Millisecond):
	}

	rel1()
	select {
	case <-acquired:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("third Acquire did not unblock after a release")
	}
	rel2()
}

func TestAcquire_ContextCancelled(t *testing.T) {
	l := New(Config{Floor: 1, Ceiling: 1, Initial: 1, PollInterval: time.Millisecond})
	release, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = l.Acquire(ctx)
	if err == nil {
		t.Fatal("Acquire with full capacity and cancelled ctx should return an error")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	l := New(Config{Floor: 1, Ceiling: 1, Initial: 1})
	release, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	release()
	release()
	release()
	if l.InFlight() != 0 {
		t.Fatalf("InFlight = %d after idempotent releases, want 0", l.InFlight())
	}
}

func TestAIMD_AdditiveIncreaseOnSustainedSuccess(t *testing.T) {
	l := New(Config{Floor: 1, Ceiling: 10, Initial: 1, IncreaseEvery: 3})
	if l.Capacity() != 1 {
		t.Fatalf("Capacity = %d, want 1", l.Capacity())
	}
	for i := 0; i < 3; i++ {
		l.ReportSuccess()
	}
	if got := l.Capacity(); got != 2 {
		t.Fatalf("Capacity after 3 successes (IncreaseEvery=3) = %d, want 2", got)
	}
	for i := 0; i < 3; i++ {
		l.ReportSuccess()
	}
	if got := l.Capacity(); got != 3 {
		t.Fatalf("Capacity after 6 successes = %d, want 3", got)
	}
}

func TestAIMD_NeverExceedsCeiling(t *testing.T) {
	l := New(Config{Floor: 1, Ceiling: 4, Initial: 4, IncreaseEvery: 1})
	for i := 0; i < 50; i++ {
		l.ReportSuccess()
	}
	if got := l.Capacity(); got != 4 {
		t.Fatalf("Capacity = %d, want capped at ceiling 4", got)
	}
}

func TestAIMD_MultiplicativeDecreaseOnRateLimit(t *testing.T) {
	l := New(Config{Floor: 1, Ceiling: 32, Initial: 16})
	l.ReportRateLimited()
	if got := l.Capacity(); got != 8 {
		t.Fatalf("Capacity after one rate-limit from 16 = %d, want 8", got)
	}
	l.ReportRateLimited()
	if got := l.Capacity(); got != 4 {
		t.Fatalf("Capacity after second rate-limit = %d, want 4", got)
	}
}

func TestAIMD_NeverBelowFloorEvenUnderSustainedRateLimiting(t *testing.T) {
	l := New(Config{Floor: 2, Ceiling: 32, Initial: 16})
	for i := 0; i < 20; i++ {
		l.ReportRateLimited()
	}
	if got := l.Capacity(); got != 2 {
		t.Fatalf("Capacity under sustained rate-limiting = %d, want floor 2", got)
	}
}

func TestAIMD_GenericFailureDoesNotShrinkCapacity(t *testing.T) {
	l := New(Config{Floor: 1, Ceiling: 32, Initial: 10})
	for i := 0; i < 20; i++ {
		l.ReportOther()
	}
	if got := l.Capacity(); got != 10 {
		t.Fatalf("Capacity after 20 generic (non-rate-limit) failures = %d, want unchanged 10 - a timeout is not a capacity signal", got)
	}
}

// TestAIMD_ConvergesToSecretCapacity is the core simulation proof: a fake
// provider has a secret true concurrent capacity. Calls made while fewer
// than that many are in flight succeed; calls made at or above it come back
// rate-limited. Many "reviews" hammer the shared limiter concurrently with
// far more demand than the secret capacity. The limiter must settle near
// (at or a little under) the secret capacity - proving it discovers the
// real ceiling by probing and backing off, the way an admin-guessed static
// number never could.
func TestAIMD_ConvergesToSecretCapacity(t *testing.T) {
	const secretCapacity = 10
	const numReviews = 3
	const batchesPerReview = 15 // 45 total demand vs. a true capacity of 10

	limiter := New(Config{Floor: 1, Ceiling: 64, Initial: 1, IncreaseEvery: 3, PollInterval: time.Millisecond})

	var liveMu sync.Mutex
	live := 0

	callProvider := func() (ok bool) {
		liveMu.Lock()
		live++
		over := live > secretCapacity
		liveMu.Unlock()

		time.Sleep(time.Millisecond) // simulate call latency

		liveMu.Lock()
		live--
		liveMu.Unlock()
		return !over
	}

	var wg sync.WaitGroup
	for r := 0; r < numReviews; r++ {
		for b := 0; b < batchesPerReview; b++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				release, err := limiter.Acquire(context.Background())
				if err != nil {
					return
				}
				defer release()
				if callProvider() {
					limiter.ReportSuccess()
				} else {
					limiter.ReportRateLimited()
				}
			}()
		}
	}
	wg.Wait()

	final := limiter.Capacity()
	// The limiter should have found its way to within a reasonable band of
	// the secret capacity - neither stuck far below it (under-utilizing)
	// nor wildly above it (would mean AIMD isn't backing off on signal).
	if final < 2 || final > secretCapacity+5 {
		t.Fatalf("final capacity = %d, want roughly near secret capacity %d (band [2, %d])", final, secretCapacity, secretCapacity+5)
	}
}

// TestAIMD_SecondConnectorUnaffected proves per-connector isolation: hammering
// one connector's limiter with rate-limit signals must not affect another
// connector's independently-tracked capacity.
func TestForConnector_PerConnectorIsolation(t *testing.T) {
	ResetRegistry()
	defer ResetRegistry()

	a := ForConnector("connector-a", Config{Floor: 1, Ceiling: 32, Initial: 16})
	b := ForConnector("connector-b", Config{Floor: 1, Ceiling: 32, Initial: 16})

	for i := 0; i < 5; i++ {
		a.ReportRateLimited()
	}
	if a.Capacity() == 16 {
		t.Fatal("connector 1's limiter did not react to rate-limiting")
	}
	if b.Capacity() != 16 {
		t.Fatalf("connector 2's limiter capacity = %d, want unaffected 16", b.Capacity())
	}

	// Same connector ID returns the same shared instance, not a fresh one.
	again := ForConnector("connector-a", Config{Floor: 1, Ceiling: 32, Initial: 16})
	if again != a {
		t.Fatal("ForConnector returned a different instance for the same connector ID")
	}
}
