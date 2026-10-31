package batch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/livereview/internal/llm/adaptivelimiter"
	"github.com/livereview/internal/llm/errclass"
	"github.com/livereview/pkg/models"
)

// Task represents a unit of work to be processed
type Task interface {
	Execute(ctx context.Context) (interface{}, error)
	ID() string
	MaxRetries() int
}

// TaskResult represents the result of a task execution
type TaskResult struct {
	TaskID  string
	Result  interface{}
	Error   error
	Retries int
}

// TaskQueue runs tasks concurrently, capped at maxWorkers in flight at once,
// with per-task retries. maxWorkers is a per-review concurrency ceiling (tier
// 1 of the two-tier model - see internal/llm/adaptivelimiter for tier 2, the
// process-wide per-AI-connector limiter that individual BatchTasks gate
// against independently of this queue).
type TaskQueue struct {
	tasks      []Task
	results    map[string]*TaskResult
	maxWorkers int
	maxRetries int
	retryDelay time.Duration
	mu         sync.Mutex
}

// NewTaskQueue creates a new task queue with the given concurrency ceiling.
func NewTaskQueue(maxWorkers int) *TaskQueue {
	return &TaskQueue{
		tasks:      make([]Task, 0),
		results:    make(map[string]*TaskResult),
		maxWorkers: maxWorkers,
		maxRetries: 3,               // Default max retries
		retryDelay: 2 * time.Second, // Default retry delay
	}
}

// AddTask adds a task to the queue
func (q *TaskQueue) AddTask(task Task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, task)
}

// SetMaxRetries sets the maximum number of retries for tasks
func (q *TaskQueue) SetMaxRetries(maxRetries int) {
	q.maxRetries = maxRetries
}

// SetRetryDelay sets the delay between retries
func (q *TaskQueue) SetRetryDelay(delay time.Duration) {
	q.retryDelay = delay
}

// ProcessAll runs every queued task concurrently - one goroutine per task,
// gated by a semaphore sized to maxWorkers - and returns each task's result
// keyed by task ID once all of them finish.
//
// This is deliberately not a fixed pool of pre-spawned long-lived worker
// goroutines pulling from a channel: with goroutine-per-task, a review with
// fewer batches than the ceiling simply runs all of them at once (the
// semaphore never fills), and a review with more batches than the ceiling
// queues behind it - the concurrency self-scales to the review's actual
// shape instead of needing a pool pre-sized to guess it.
func (q *TaskQueue) ProcessAll(ctx context.Context) map[string]*TaskResult {
	q.mu.Lock()
	tasksCopy := make([]Task, len(q.tasks))
	copy(tasksCopy, q.tasks)
	q.mu.Unlock()

	ceiling := max(q.maxWorkers, 1)
	sem := make(chan struct{}, ceiling)

	resultCh := make(chan *TaskResult, len(tasksCopy))
	var wg sync.WaitGroup

	for _, task := range tasksCopy {
		// Dispatch is itself ctx-aware: a task still waiting for a free slot
		// when ctx expires is recorded as cancelled immediately instead of
		// blocking on the semaphore send indefinitely (a send on a full
		// channel does not observe context cancellation on its own).
		select {
		case sem <- struct{}{}: // blocks here once `ceiling` tasks are already in flight
		case <-ctx.Done():
			resultCh <- &TaskResult{
				TaskID: task.ID(),
				Error:  fmt.Errorf("task cancelled before start: %w", ctx.Err()),
			}
			continue
		}
		wg.Add(1)
		go func(task Task) {
			defer wg.Done()
			defer func() { <-sem }()
			resultCh <- q.runWithRetries(ctx, task)
		}(task)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make(map[string]*TaskResult)
	for result := range resultCh {
		results[result.TaskID] = result
	}

	q.mu.Lock()
	q.results = results
	q.mu.Unlock()

	return results
}

// runWithRetries executes task, retrying up to its configured MaxRetries (or
// the queue default) while the context remains live. A cancelled/expired
// context stops retrying immediately rather than burning through the
// remaining attempts - each of which would fail instantly anyway, since the
// prior implementation's `break` only exited the inner `select`, not this
// `for`, letting an already-dead context spin through every remaining retry
// as a near-instant guaranteed failure instead of bailing on the first one.
func (q *TaskQueue) runWithRetries(ctx context.Context, task Task) *TaskResult {
	maxRetries := task.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = q.maxRetries
	}

	var result interface{}
	var err error
	retries := 0

retryLoop:
	for retries <= maxRetries {
		if ctx.Err() != nil {
			err = fmt.Errorf("task cancelled: %w", ctx.Err())
			break
		}

		result, err = task.Execute(ctx)
		if err == nil {
			break
		}

		retries++
		if retries > maxRetries {
			break
		}

		select {
		case <-time.After(q.retryDelay):
			// continue to next retry
		case <-ctx.Done():
			err = fmt.Errorf("task cancelled: %w", ctx.Err())
			break retryLoop
		}
	}

	return &TaskResult{
		TaskID:  task.ID(),
		Result:  result,
		Error:   err,
		Retries: retries,
	}
}

// GetResults returns the current results
func (q *TaskQueue) GetResults() map[string]*TaskResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Create a copy of the results
	resultsCopy := make(map[string]*TaskResult, len(q.results))
	for k, v := range q.results {
		resultsCopy[k] = v
	}

	return resultsCopy
}

// BatchTask implements the Task interface for processing code review batches
type BatchTask struct {
	id           string
	batch        []models.CodeDiff
	processor    func(context.Context, []models.CodeDiff) (*BatchResult, error)
	maxRetries   int
	batchNumber  int
	logger       Logger
	limiter      *adaptivelimiter.Limiter
	providerType string
}

// NewBatchTask creates a new batch task
func NewBatchTask(id string, batch []models.CodeDiff, processor func(context.Context, []models.CodeDiff) (*BatchResult, error)) *BatchTask {
	return &BatchTask{
		id:         id,
		batch:      batch,
		processor:  processor,
		maxRetries: 3, // Default max retries
		logger:     &DefaultLogger{Verbose: false},
	}
}

// SetBatchNumber sets the batch number for this task
func (t *BatchTask) SetBatchNumber(batchNumber int) {
	t.batchNumber = batchNumber
}

// SetLogger sets the logger for this task
func (t *BatchTask) SetLogger(logger Logger) {
	t.logger = logger
}

// SetLimiter wires this task into the process-wide, self-tuning per-AI-connector
// concurrency limiter (tier 2 of the two-tier model - see
// internal/llm/adaptivelimiter). Optional: a nil limiter (the default) skips
// this gating entirely, which existing callers/tests rely on.
func (t *BatchTask) SetLimiter(limiter *adaptivelimiter.Limiter) {
	t.limiter = limiter
}

// SetProviderType records which AI provider this task's processor calls into
// (e.g. "openai", "anthropic", "googleai", "bedrock", "ollama"), used by
// internal/llm/errclass to classify a failure as rate-limited vs. not. Only
// meaningful alongside SetLimiter.
func (t *BatchTask) SetProviderType(providerType string) {
	t.providerType = providerType
}

// Execute processes the batch
func (t *BatchTask) Execute(ctx context.Context) (interface{}, error) {
	if t.logger != nil {
		t.logger.Info("Processing batch task %s with %d files", t.id, len(t.batch))
	}

	if t.limiter != nil {
		release, acquireErr := t.limiter.Acquire(ctx)
		if acquireErr != nil {
			if t.logger != nil {
				t.logger.Error("Batch %s: provider concurrency slot not acquired: %v", t.id, acquireErr)
			}
			return nil, fmt.Errorf("acquire provider concurrency slot: %w", acquireErr)
		}
		defer release()
	}

	result, err := t.processor(ctx, t.batch)

	if t.limiter != nil {
		switch {
		case err == nil:
			t.limiter.ReportSuccess()
		case errclass.Classify(t.providerType, err) == errclass.RateLimited:
			t.limiter.ReportRateLimited()
		default:
			t.limiter.ReportOther()
		}
	}

	if result != nil && t.batchNumber > 0 {
		// Set the batch ID in the result
		result.BatchID = fmt.Sprintf("Batch %d", t.batchNumber)

		if t.logger != nil {
			t.logger.Info("Completed batch %d: %d comments generated",
				t.batchNumber, len(result.Comments))
		}
	}

	if err != nil && t.logger != nil {
		t.logger.Error("Error processing batch %s: %v", t.id, err)
	}

	return result, err
}

// ID returns the task ID
func (t *BatchTask) ID() string {
	return t.id
}

// MaxRetries returns the maximum number of retries for this task
func (t *BatchTask) MaxRetries() int {
	return t.maxRetries
}

// SetMaxRetries sets the maximum number of retries for this task
func (t *BatchTask) SetMaxRetries(maxRetries int) {
	t.maxRetries = maxRetries
}
