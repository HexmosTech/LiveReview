package batch

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// TaskQueue is a queue for processing tasks with retry capabilities
type TaskQueue struct {
	tasks      []Task
	results    map[string]*TaskResult
	maxWorkers int
	maxRetries int
	retryDelay time.Duration
	mu         sync.Mutex
}

// NewTaskQueue creates a new task queue
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

// ProcessAll processes all tasks in the queue and returns the results
func (q *TaskQueue) ProcessAll(ctx context.Context) map[string]*TaskResult {
	q.mu.Lock()
	tasksCopy := make([]Task, len(q.tasks))
	copy(tasksCopy, q.tasks)
	q.mu.Unlock()

	// Create a channel for tasks
	taskCh := make(chan Task, len(tasksCopy))

	// Create a channel for results
	resultCh := make(chan *TaskResult, len(tasksCopy))

	// Create worker pool
	var wg sync.WaitGroup
	workerCount := q.maxWorkers
	if workerCount > len(tasksCopy) {
		workerCount = len(tasksCopy)
	}

	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				// Get task-specific max retries or use queue default
				maxRetries := task.MaxRetries()
				if maxRetries <= 0 {
					maxRetries = q.maxRetries
				}

				var result interface{}
				var err error
				retries := 0

				// Execute the task with retries
				for retries <= maxRetries {
					result, err = task.Execute(ctx)
					if err == nil {
						break
					}

					retries++
					if retries <= maxRetries {
						// Wait before retry
						select {
						case <-time.After(q.retryDelay):
							// Continue with retry
						case <-ctx.Done():
							// Context cancelled, stop retrying
							err = fmt.Errorf("task cancelled: %w", ctx.Err())
							break
						}
					}
				}

				// Send result
				resultCh <- &TaskResult{
					TaskID:  task.ID(),
					Result:  result,
					Error:   err,
					Retries: retries,
				}
			}
		}()
	}

	// Send tasks to workers
	for _, task := range tasksCopy {
		taskCh <- task
	}
	close(taskCh)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	results := make(map[string]*TaskResult)
	for result := range resultCh {
		results[result.TaskID] = result
	}

	// Store results
	q.mu.Lock()
	q.results = results
	q.mu.Unlock()

	return results
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
	id          string
	batch       []models.CodeDiff
	processor   func(context.Context, []models.CodeDiff) (*BatchResult, error)
	maxRetries  int
	batchNumber int
	logger      Logger
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

// Execute processes the batch
func (t *BatchTask) Execute(ctx context.Context) (interface{}, error) {
	if t.logger != nil {
		t.logger.Info("Processing batch task %s with %d files", t.id, len(t.batch))
	}

	result, err := t.processor(ctx, t.batch)

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
