package streamz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"streamz/clock"
	clocktesting "streamz/clock/testing"
)

// failingProcessor for test - fails a configurable number of times.
type failingProcessor struct {
	name         string
	failureCount int32
	callCount    int32
	failureError error
}

func newFailingProcessor(name string, failures int) *failingProcessor {
	return &failingProcessor{
		name:         name,
		failureCount: int32(failures), // #nosec G115 - test code with small values
		failureError: errors.New("simulated failure"),
	}
}

func (fp *failingProcessor) Process(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for item := range in {
			atomic.AddInt32(&fp.callCount, 1)

			if atomic.LoadInt32(&fp.failureCount) > 0 {
				atomic.AddInt32(&fp.failureCount, -1)
				// Simulate failure by not sending output
				continue
			}

			// Success - send doubled value
			select {
			case out <- item * 2:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (fp *failingProcessor) Name() string {
	return fp.name
}

func (fp *failingProcessor) GetCallCount() int32 {
	return atomic.LoadInt32(&fp.callCount)
}

// TestDLQBasicFunctionality tests basic DLQ operations.
func TestDLQBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	// Processor that fails first 2 items
	processor := newFailingProcessor("test", 2)

	var failedItems []DLQItem[int]
	dlq := NewDeadLetterQueue(processor, clock.Real).
		OnFailure(func(_ context.Context, item DLQItem[int]) {
			failedItems = append(failedItems, item)
		})

	// Process 5 items
	input := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		input <- i
	}
	close(input)

	output := dlq.Process(ctx, input)

	// Collect successful results
	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Should have 3 successful results (3, 4, 5 doubled)
	if len(results) != 3 {
		t.Errorf("expected 3 successful results, got %d", len(results))
	}

	// Should have 2 failed items
	if len(failedItems) != 2 {
		t.Errorf("expected 2 failed items, got %d", len(failedItems))
	}

	// Verify failed items
	if len(failedItems) >= 2 {
		if failedItems[0].Item != 1 {
			t.Errorf("expected first failed item to be 1, got %d", failedItems[0].Item)
		}
		if failedItems[1].Item != 2 {
			t.Errorf("expected second failed item to be 2, got %d", failedItems[1].Item)
		}
	}

	// Check stats
	stats := dlq.GetStats()
	if stats.Processed != 5 {
		t.Errorf("expected 5 processed, got %d", stats.Processed)
	}
	if stats.Succeeded != 3 {
		t.Errorf("expected 3 succeeded, got %d", stats.Succeeded)
	}
	if stats.Failed != 2 {
		t.Errorf("expected 2 failed, got %d", stats.Failed)
	}
}

// TestDLQWithRetries tests retry functionality.
func TestDLQWithRetries(t *testing.T) {
	ctx := context.Background()

	// Processor that fails first 2 attempts for each item
	processor := newFailingProcessor("test", 2)

	clk := clocktesting.NewFakeClock(time.Now())
	var retryLog []string
	dlq := NewDeadLetterQueue(processor, clk).
		MaxRetries(3).
		RetryDelay(10 * time.Millisecond).
		OnRetry(func(item int, attempt int, _ error) {
			retryLog = append(retryLog, fmt.Sprintf("retry-%d-attempt-%d", item, attempt))
		})

	// Process 1 item
	input := make(chan int, 1)
	input <- 42
	close(input)

	output := dlq.Process(ctx, input)

	// Collect results asynchronously
	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	done := make(chan bool)
	go func() {
		for result := range output {
			results = append(results, result)
		}
		done <- true
	}()

	// Advance clock for retries
	for i := 0; i < 3; i++ {
		clk.Step(35 * time.Second) // Timeout + retry delay
		clk.BlockUntilReady()
		time.Sleep(10 * time.Millisecond) // Allow goroutine scheduling
	}

	<-done

	if len(results) != 1 || results[0] != 84 {
		t.Errorf("expected [84], got %v", results)
	}

	// Should have 2 retry attempts
	if len(retryLog) != 2 {
		t.Errorf("expected 2 retry attempts, got %d: %v", len(retryLog), retryLog)
	}

	// Verify call count (1 initial + 2 retries = 3)
	if processor.GetCallCount() != 3 {
		t.Errorf("expected 3 calls to processor, got %d", processor.GetCallCount())
	}

	stats := dlq.GetStats()
	if stats.Retried != 2 {
		t.Errorf("expected 2 retries, got %d", stats.Retried)
	}
}

// TestDLQMaxRetriesExceeded tests behavior when max retries are exceeded.
func TestDLQMaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()

	// Processor that always fails
	processor := newFailingProcessor("test", 100)

	var failedItems []DLQItem[int]
	dlq := NewDeadLetterQueue(processor, clock.Real).
		MaxRetries(2).
		RetryDelay(1 * time.Millisecond).
		OnFailure(func(_ context.Context, item DLQItem[int]) {
			failedItems = append(failedItems, item)
		})

	input := make(chan int, 1)
	input <- 42
	close(input)

	output := dlq.Process(ctx, input)

	// Should have no successful results
	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 0 {
		t.Errorf("expected no results, got %v", results)
	}

	// Should have 1 failed item
	if len(failedItems) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(failedItems))
	}

	// Verify attempts (1 initial + 2 retries = 3)
	if failedItems[0].Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", failedItems[0].Attempts)
	}

	// Verify processor was called 3 times
	if processor.GetCallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", processor.GetCallCount())
	}
}

// TestDLQFailedItemsChannel tests accessing failed items via channel.
func TestDLQFailedItemsChannel(t *testing.T) {
	ctx := context.Background()

	processor := newFailingProcessor("test", 3)
	dlq := NewDeadLetterQueue(processor, clock.Real).
		WithFailedBufferSize(10)

	input := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		input <- i
	}
	close(input)

	// Start processing
	output := dlq.Process(ctx, input)

	// Collect failed items from channel
	var failedItems []DLQItem[int]
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for failed := range dlq.FailedItems() {
			failedItems = append(failedItems, failed)
		}
	}()

	// Drain successful output
	for range output { //nolint:revive // Intentionally draining channel
		// Just drain
	}

	wg.Wait()

	// Should have 3 failed items
	if len(failedItems) != 3 {
		t.Errorf("expected 3 failed items, got %d", len(failedItems))
	}

	// Verify items
	for i, failed := range failedItems {
		if failed.Item != i+1 {
			t.Errorf("failed item %d: expected %d, got %d", i, i+1, failed.Item)
		}
	}
}

// TestDLQContinueOnError tests continue vs stop behavior.
func TestDLQContinueOnError(t *testing.T) {
	ctx := context.Background()

	t.Run("ContinueOnError=true", func(t *testing.T) {
		processor := newFailingProcessor("test", 2)
		dlq := NewDeadLetterQueue(processor, clock.Real).
			ContinueOnError(true) // Default

		input := make(chan int, 5)
		for i := 1; i <= 5; i++ {
			input <- i
		}
		close(input)

		output := dlq.Process(ctx, input)

		var results []int //nolint:prealloc // dynamic growth acceptable in test code
		for result := range output {
			results = append(results, result)
		}

		// Should process all items despite failures
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("ContinueOnError=false", func(t *testing.T) {
		processor := newFailingProcessor("test", 1)
		dlq := NewDeadLetterQueue(processor, clock.Real).
			ContinueOnError(false)

		input := make(chan int)
		go func() {
			defer close(input)
			for i := 1; i <= 5; i++ {
				input <- i
				time.Sleep(10 * time.Millisecond)
			}
		}()

		output := dlq.Process(ctx, input)

		var results []int //nolint:prealloc // dynamic growth acceptable in test code
		for result := range output {
			results = append(results, result)
		}

		// Should stop after first failure
		if len(results) != 0 {
			t.Errorf("expected 0 results (stop on first failure), got %d", len(results))
		}

		stats := dlq.GetStats()
		if stats.Failed != 1 {
			t.Errorf("expected 1 failure, got %d", stats.Failed)
		}
	})
}

// TestDLQShouldRetry tests custom retry logic.
func TestDLQShouldRetry(t *testing.T) {
	ctx := context.Background()

	// Custom processor that returns different errors
	customProcessor := &customFailingProcessor{
		errors: []error{
			errors.New("network timeout"),
			errors.New("invalid input"),
			errors.New("connection refused"),
		},
	}

	var retryLog []string
	dlq := NewDeadLetterQueue(customProcessor, clock.Real).
		MaxRetries(5).
		ShouldRetry(func(err error) bool {
			// Only retry network errors
			errStr := strings.ToLower(err.Error())
			return strings.Contains(errStr, "network") ||
				strings.Contains(errStr, "connection")
		}).
		OnRetry(func(_ int, _ int, err error) {
			retryLog = append(retryLog, err.Error())
		})

	input := make(chan int, 3)
	input <- 1 // network timeout - will retry
	input <- 2 // invalid input - won't retry
	input <- 3 // connection refused - will retry
	close(input)

	output := dlq.Process(ctx, input)
	// Drain output channel
	for range output { //nolint:revive // Intentionally draining channel
		// Items are processed, focusing on retry behavior
	}

	// Item 2 should fail immediately, others might succeed after retry
	stats := dlq.GetStats()
	if stats.Failed < 1 {
		t.Error("expected at least 1 permanent failure")
	}

	// Should only retry network-related errors
	hasInvalidInput := false
	for _, log := range retryLog {
		if strings.Contains(log, "invalid input") {
			hasInvalidInput = true
			break
		}
	}
	if hasInvalidInput {
		t.Error("should not retry 'invalid input' error")
	}
}

// TestDLQContextCancellation tests graceful shutdown.
func TestDLQContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Slow processor
	slowProcessor := ProcessorFunc[int, int](func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for item := range in {
				select {
				case <-time.After(100 * time.Millisecond):
					out <- item * 2
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	})

	dlq := NewDeadLetterQueue(slowProcessor, clock.Real)

	input := make(chan int)
	go func() {
		defer close(input)
		for i := 1; i <= 10; i++ {
			select {
			case input <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	output := dlq.Process(ctx, input)

	// Process some items then cancel
	var count atomic.Int32
	done := make(chan struct{})

	go func() {
		defer close(done)
		for range output {
			count.Add(1)
		}
	}()

	// Let it process a few items
	time.Sleep(250 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for completion
	<-done

	// Should have processed some but not all
	finalCount := count.Load()
	if finalCount == 0 {
		t.Error("expected some items to be processed")
	}
	if finalCount >= 10 {
		t.Errorf("expected cancellation to stop processing, but processed %d items", finalCount)
	}
}

// TestDLQFluentAPI tests fluent configuration.
func TestDLQFluentAPI(t *testing.T) {
	processor := NewTestProcessor("test")

	dlq := NewDeadLetterQueue(processor, clock.Real).
		MaxRetries(5).
		RetryDelay(2 * time.Second).
		ContinueOnError(false).
		WithFailedBufferSize(200).
		WithName("test-dlq")

	if dlq.maxRetries != 5 {
		t.Errorf("expected max retries 5, got %d", dlq.maxRetries)
	}
	if dlq.retryDelay != 2*time.Second {
		t.Errorf("expected retry delay 2s, got %v", dlq.retryDelay)
	}
	if dlq.continueOnError != false {
		t.Error("expected continueOnError to be false")
	}
	if dlq.failedBufferSize != 200 {
		t.Errorf("expected buffer size 200, got %d", dlq.failedBufferSize)
	}
	if dlq.Name() != "test-dlq" {
		t.Errorf("expected name 'test-dlq', got %s", dlq.Name())
	}
}

// TestDLQStats tests statistics tracking.
func TestDLQStats(t *testing.T) {
	ctx := context.Background()

	// Processor that fails specific items
	specificFailProcessor := ProcessorFunc[int, int](func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for item := range in {
				// Fail items 1, 2, 3 even with retries
				if item <= 3 {
					continue // Simulate failure
				}
				// Success for others
				select {
				case out <- item * 2:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	})

	dlq := NewDeadLetterQueue(specificFailProcessor, clock.Real).
		MaxRetries(1)

	input := make(chan int, 10)
	for i := 1; i <= 10; i++ {
		input <- i
	}
	close(input)

	output := dlq.Process(ctx, input)
	for range output { //nolint:revive // Intentionally draining channel
		// Drain
	}

	stats := dlq.GetStats()

	if stats.Processed != 10 {
		t.Errorf("expected 10 processed, got %d", stats.Processed)
	}
	if stats.Succeeded != 7 {
		t.Errorf("expected 7 succeeded, got %d", stats.Succeeded)
	}
	if stats.Failed != 3 {
		t.Errorf("expected 3 failed, got %d", stats.Failed)
	}

	// Check rates
	successRate := stats.SuccessRate()
	if successRate != 70.0 {
		t.Errorf("expected 70%% success rate, got %.2f%%", successRate)
	}

	failureRate := stats.FailureRate()
	if failureRate != 30.0 {
		t.Errorf("expected 30%% failure rate, got %.2f%%", failureRate)
	}
}

// TestDLQConcurrency tests concurrent processing.
func TestDLQConcurrency(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var failedItems []int
	var totalProcessed atomic.Int64

	// Process with single DLQ instance
	randomProcessor := ProcessorFunc[int, int](func(_ context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for item := range in {
				if item%3 == 0 { // Fail multiples of 3
					continue
				}
				out <- item * 2
			}
		}()
		return out
	})

	dlq := NewDeadLetterQueue(randomProcessor, clock.Real).
		OnFailure(func(_ context.Context, item DLQItem[int]) {
			mu.Lock()
			failedItems = append(failedItems, item.Item)
			mu.Unlock()
		})

	// Single input channel
	input := make(chan int)
	output := dlq.Process(ctx, input)

	// Producer goroutines
	var wg sync.WaitGroup
	numGoroutines := 5
	itemsPerGoroutine := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineIdx int) {
			defer wg.Done()

			for j := 0; j < itemsPerGoroutine; j++ {
				item := goroutineIdx*100 + j
				select {
				case input <- item:
					totalProcessed.Add(1)
				case <-ctx.Done():
					return
				}
			}
		}(i)
	}

	// Collect results
	var totalSuccesses atomic.Int64
	go func() {
		for range output {
			totalSuccesses.Add(1)
		}
	}()

	// Wait for producers
	wg.Wait()
	close(input)

	// Wait a bit for processing to complete
	time.Sleep(100 * time.Millisecond)

	stats := dlq.GetStats()
	processed := totalProcessed.Load()
	if stats.Processed != processed {
		t.Errorf("expected %d processed, got %d", processed, stats.Processed)
	}
}

// BenchmarkDLQNoFailures benchmarks DLQ with no failures.
func BenchmarkDLQNoFailures(b *testing.B) {
	ctx := context.Background()
	processor := NewTestProcessor("bench")
	dlq := NewDeadLetterQueue(processor, clock.Real)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		output := dlq.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Consume
		}
	}
}

// BenchmarkDLQWithRetries benchmarks DLQ with retries.
func BenchmarkDLQWithRetries(b *testing.B) {
	ctx := context.Background()
	processor := newFailingProcessor("bench", 1) // Fail once
	dlq := NewDeadLetterQueue(processor, clock.Real).
		MaxRetries(2).
		RetryDelay(0) // No delay for benchmark

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset failure count
		atomic.StoreInt32(&processor.failureCount, 1)

		input := make(chan int, 1)
		input <- i
		close(input)

		output := dlq.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Consume
		}
	}
}

// customFailingProcessor returns specific errors for testing.
type customFailingProcessor struct {
	errors []error
	index  int
	mu     sync.Mutex
}

func (p *customFailingProcessor) Process(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for item := range in {
			p.mu.Lock()
			if p.index < len(p.errors) {
				// Return next error by not sending output
				p.index++
				p.mu.Unlock()
				continue
			}
			p.mu.Unlock()

			// Success
			select {
			case out <- item * 2:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (*customFailingProcessor) Name() string {
	return "custom-failing"
}

// ProcessorFunc is a function type that implements Processor.
type ProcessorFunc[In, Out any] func(context.Context, <-chan In) <-chan Out

func (f ProcessorFunc[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
	return f(ctx, in)
}

func (ProcessorFunc[In, Out]) Name() string {
	return "processor-func"
}

// Example demonstrates basic DLQ usage.
func ExampleDeadLetterQueue() {
	ctx := context.Background()

	// Processor that might fail
	processor := NewTestProcessor("api-calls")

	// Wrap with DLQ
	dlq := NewDeadLetterQueue(processor, clock.Real).
		MaxRetries(3).
		OnFailure(func(_ context.Context, item DLQItem[int]) {
			fmt.Printf("Failed to process: %d after %d attempts\n",
				item.Item, item.Attempts)
		})

	// Process items
	input := make(chan int, 3)
	input <- 1
	input <- 2
	input <- 3
	close(input)

	output := dlq.Process(ctx, input)
	for result := range output {
		fmt.Printf("Processed: %d\n", result)
	}

	// Output:
	// Processed: 2
	// Processed: 4
	// Processed: 6
}
