package batch

import (
	"runtime"
	"time"
)

// Config holds configuration for batch processing
type Config struct {
	MaxWorkers   int           // Maximum number of concurrent workers
	MaxRetries   int           // Maximum number of retries for failed batches
	RetryDelay   time.Duration // Delay between retries
	MaxBatchSize int           // Maximum number of tokens per batch
}

// DefaultConfig returns a default configuration for batch processing
func DefaultConfig() Config {
	return Config{
		MaxWorkers:   runtime.NumCPU(),
		MaxRetries:   3,
		RetryDelay:   2 * time.Second,
		MaxBatchSize: 10000,
	}
}

// ConfigFromMap creates a Config from a map (typically from TOML/JSON config)
func ConfigFromMap(configMap map[string]interface{}) Config {
	config := DefaultConfig()

	// Extract max workers
	if maxWorkers, ok := configMap["max_workers"].(int); ok && maxWorkers > 0 {
		config.MaxWorkers = maxWorkers
	} else if maxWorkers, ok := configMap["max_workers"].(float64); ok && maxWorkers > 0 {
		config.MaxWorkers = int(maxWorkers)
	}

	// Extract max retries
	if maxRetries, ok := configMap["max_retries"].(int); ok && maxRetries > 0 {
		config.MaxRetries = maxRetries
	} else if maxRetries, ok := configMap["max_retries"].(float64); ok && maxRetries > 0 {
		config.MaxRetries = int(maxRetries)
	}

	// Extract retry delay
	if retryDelayMs, ok := configMap["retry_delay_ms"].(int); ok && retryDelayMs > 0 {
		config.RetryDelay = time.Duration(retryDelayMs) * time.Millisecond
	} else if retryDelayMs, ok := configMap["retry_delay_ms"].(float64); ok && retryDelayMs > 0 {
		config.RetryDelay = time.Duration(retryDelayMs) * time.Millisecond
	}

	// Extract max batch size
	if maxBatchSize, ok := configMap["max_batch_size"].(int); ok && maxBatchSize > 0 {
		config.MaxBatchSize = maxBatchSize
	} else if maxBatchSize, ok := configMap["max_batch_size"].(float64); ok && maxBatchSize > 0 {
		config.MaxBatchSize = int(maxBatchSize)
	}

	return config
}

// ConfigureTaskQueue configures a TaskQueue based on Config
func ConfigureTaskQueue(config Config) *TaskQueue {
	queue := NewTaskQueue(config.MaxWorkers)
	queue.SetMaxRetries(config.MaxRetries)
	queue.SetRetryDelay(config.RetryDelay)
	return queue
}

// ConfigureBatchProcessor configures a BatchProcessor based on Config
func ConfigureBatchProcessor(config Config) *BatchProcessor {
	processor := DefaultBatchProcessor()
	processor.MaxBatchTokens = config.MaxBatchSize
	return processor
}
