package streamz

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestProcessor implements Processor interface for testing retry logic.
type TestProcessor struct {
	name         string
	failureCount int32 // How many times to fail before succeeding
	callCount    int32 // Track number of calls
	processFunc  func(context.Context, int) (int, error)
}

func NewTestProcessor(name string) *TestProcessor {
	return &TestProcessor{
		name: name,
		processFunc: func(_ context.Context, item int) (int, error) {
			return item * 2, nil
		},
	}
}

func (tp *TestProcessor) WithFailures(count int) *TestProcessor {
	tp.failureCount = int32(count) // #nosec G115 - test code with small values
	return tp
}

func (tp *TestProcessor) WithFunc(fn func(context.Context, int) (int, error)) *TestProcessor {
	tp.processFunc = fn
	return tp
}

func (tp *TestProcessor) Process(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for {
			select {
			case item, ok := <-in:
				if !ok {
					return
				}

				atomic.AddInt32(&tp.callCount, 1)

				// Check if we should fail this time.
				if atomic.LoadInt32(&tp.failureCount) > 0 {
					atomic.AddInt32(&tp.failureCount, -1)
					// Simulate failure by not sending anything and returning.
					continue
				}

				result, err := tp.processFunc(ctx, item)
				if err != nil {
					// Simulate error by not sending result.
					continue
				}

				select {
				case out <- result:
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (tp *TestProcessor) Name() string {
	return tp.name
}

func (tp *TestProcessor) GetCallCount() int32 {
	return atomic.LoadInt32(&tp.callCount)
}

func TestRetryBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	// Create a processor that fails twice then succeeds
	processor := NewTestProcessor("test").WithFailures(2)
	retry := NewRetry(processor, RealClock).
		MaxAttempts(3).
		BaseDelay(1 * time.Millisecond) // Fast for testing

	// Test single item
	input := make(chan int, 1)
	input <- 5
	close(input)

	output := retry.Process(ctx, input)

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Should succeed on 3rd attempt and return 5 * 2 = 10
	if len(results) != 1 || results[0] != 10 {
		t.Errorf("expected [10], got %v", results)
	}
	if processor.GetCallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", processor.GetCallCount())
	}
}

func TestRetryMaxAttemptsExceeded(t *testing.T) {
	ctx := context.Background()

	// Create a processor that always fails
	processor := NewTestProcessor("test").WithFailures(10)
	retry := NewRetry(processor, RealClock).
		MaxAttempts(3).
		BaseDelay(1 * time.Millisecond)

	input := make(chan int, 1)
	input <- 5
	close(input)

	output := retry.Process(ctx, input)

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Should fail all attempts and return no results
	if len(results) != 0 {
		t.Errorf("expected no results, got %v", results)
	}
	if processor.GetCallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", processor.GetCallCount())
	}
}

func TestRetrySuccessOnFirstAttempt(t *testing.T) {
	ctx := context.Background()

	// Create a processor that never fails
	processor := NewTestProcessor("test").WithFailures(0)
	retry := NewRetry(processor, RealClock)

	input := make(chan int, 1)
	input <- 5
	close(input)

	output := retry.Process(ctx, input)

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Should succeed immediately
	if len(results) != 1 || results[0] != 10 {
		t.Errorf("expected [10], got %v", results)
	}
	if processor.GetCallCount() != 1 {
		t.Errorf("expected 1 call, got %d", processor.GetCallCount())
	}
}

func TestRetryMultipleItems(t *testing.T) {
	ctx := context.Background()

	// Create a processor that succeeds on all items (no failures)
	processor := NewTestProcessor("test").WithFailures(0)
	retry := NewRetry(processor, RealClock).
		MaxAttempts(2).
		BaseDelay(1 * time.Millisecond)

	input := make(chan int)
	go func() {
		defer close(input)
		for i := 1; i <= 3; i++ {
			input <- i
		}
	}()

	output := retry.Process(ctx, input)

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Should get all items processed successfully on first attempt
	expected := []int{2, 4, 6} // 1*2, 2*2, 3*2
	if len(results) != len(expected) {
		t.Errorf("expected %d results, got %d", len(expected), len(results))
	}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
	if processor.GetCallCount() != 3 { // 1 attempt * 3 items
		t.Errorf("expected 3 calls, got %d", processor.GetCallCount())
	}
}

func TestRetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create a processor that always fails
	processor := NewTestProcessor("test").WithFailures(10)
	retry := NewRetry(processor, RealClock).
		MaxAttempts(5).
		BaseDelay(100 * time.Millisecond) // Longer delay to test cancellation

	input := make(chan int, 1)
	input <- 5
	close(input)

	output := retry.Process(ctx, input)

	// Cancel context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Should return no results due to cancellation
	if len(results) != 0 {
		t.Errorf("expected no results due to cancellation, got %v", results)
	}
	// Should have made at least 1 call but not all 5
	callCount := processor.GetCallCount()
	if callCount < 1 {
		t.Errorf("expected at least 1 call, got %d", callCount)
	}
	if callCount >= 5 {
		t.Errorf("expected less than 5 calls due to cancellation, got %d", callCount)
	}
}

func TestRetryExponentialBackoff(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test").WithFailures(3)
	clk := NewFakeClock(time.Now())
	retry := NewRetry(processor, clk).
		MaxAttempts(4).
		BaseDelay(10 * time.Millisecond).
		WithJitter(false) // Disable jitter for predictable timing

	input := make(chan int, 1)
	input <- 5
	close(input)

	output := retry.Process(ctx, input)

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	done := make(chan bool)
	go func() {
		for result := range output {
			results = append(results, result)
		}
		done <- true
	}()

	// Advance clock for retries: 0ms (initial), 10ms, 20ms, 40ms
	// We need to advance past the initial processing timeout too
	for i := 0; i < 5; i++ {
		clk.Step(100 * time.Millisecond)
		clk.BlockUntilReady()
		time.Sleep(10 * time.Millisecond) // Allow goroutine scheduling
	}

	<-done

	// Should succeed on 4th attempt
	if len(results) != 1 || results[0] != 10 {
		t.Errorf("expected [10], got %v", results)
	}
	if processor.GetCallCount() != 4 {
		t.Errorf("expected 4 calls, got %d", processor.GetCallCount())
	}

	// Should have had delays: 0ms (initial), 10ms, 20ms, 40ms with fake clock
}

func TestRetryMaxDelay(t *testing.T) {
	retry := NewRetry(NewTestProcessor("test"), RealClock).
		BaseDelay(100 * time.Millisecond).
		MaxDelay(150 * time.Millisecond).
		WithJitter(false)

	// Test delay calculation for various attempts
	delay1 := retry.calculateDelay(1) // Should be 200ms but capped at 150ms
	delay2 := retry.calculateDelay(2) // Should be 400ms but capped at 150ms

	if delay1 != 150*time.Millisecond {
		t.Errorf("expected 150ms delay, got %v", delay1)
	}
	if delay2 != 150*time.Millisecond {
		t.Errorf("expected 150ms delay, got %v", delay2)
	}
}

func TestRetryJitter(t *testing.T) {
	retry := NewRetry(NewTestProcessor("test"), RealClock).
		BaseDelay(100 * time.Millisecond).
		WithJitter(true)

	// Test that jitter produces different delays
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = retry.calculateDelay(1)
	}

	// With jitter, delays should vary between 100ms and 200ms (50% to 100% of 200ms)
	// calculateDelay(1) with 100ms base = 100ms * 2^1 = 200ms, then jittered to 100-200ms
	for i, delay := range delays {
		if delay < 100*time.Millisecond {
			t.Errorf("delay %d too small: expected >= 100ms, got %v", i, delay)
		}
		if delay > 200*time.Millisecond {
			t.Errorf("delay %d too large: expected <= 200ms, got %v", i, delay)
		}
	}

	// Should have some variation (not all the same)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("jitter should produce varying delays, but all delays were the same")
	}
}

func TestRetryCustomErrorHandler(t *testing.T) {
	ctx := context.Background()

	var errorCallCount int
	processor := NewTestProcessor("test").WithFailures(5)
	retry := NewRetry(processor, RealClock).
		MaxAttempts(5).
		BaseDelay(1 * time.Millisecond).
		OnError(func(_ error, attempt int) bool {
			errorCallCount++
			// Only retry on first attempt (so 2 total attempts)
			return attempt == 1
		})

	input := make(chan int, 1)
	input <- 5
	close(input)

	output := retry.Process(ctx, input)

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Should stop retrying after 2 attempts due to custom error handler
	if len(results) != 0 {
		t.Errorf("expected no results, got %v", results)
	}
	if processor.GetCallCount() != 2 {
		t.Errorf("expected 2 calls, got %d", processor.GetCallCount())
	}
}

func TestRetryFluentAPI(t *testing.T) {
	processor := NewTestProcessor("test")

	// Test that fluent API returns the same instance
	retry := NewRetry(processor, RealClock).
		MaxAttempts(5).
		BaseDelay(200 * time.Millisecond).
		MaxDelay(10 * time.Second).
		WithJitter(false).
		WithName("custom-retry")

	if retry.maxAttempts != 5 {
		t.Errorf("expected 5 max attempts, got %d", retry.maxAttempts)
	}
	if retry.baseDelay != 200*time.Millisecond {
		t.Errorf("expected 200ms base delay, got %v", retry.baseDelay)
	}
	if retry.maxDelay != 10*time.Second {
		t.Errorf("expected 10s max delay, got %v", retry.maxDelay)
	}
	if retry.withJitter != false {
		t.Errorf("expected jitter disabled, got %t", retry.withJitter)
	}
	if retry.name != "custom-retry" {
		t.Errorf("expected name 'custom-retry', got %s", retry.name)
	}
}

func TestRetryName(t *testing.T) {
	processor := NewTestProcessor("test")
	retry := NewRetry(processor, RealClock)

	// Default name
	if retry.Name() != "retry" {
		t.Errorf("expected default name 'retry', got %s", retry.Name())
	}

	// Custom name
	retry.WithName("custom-retry")
	if retry.Name() != "custom-retry" {
		t.Errorf("expected custom name 'custom-retry', got %s", retry.Name())
	}
}

func TestRetryParameterValidation(t *testing.T) {
	processor := NewTestProcessor("test")
	retry := NewRetry(processor, RealClock)

	// Test parameter bounds
	retry.MaxAttempts(-1)
	if retry.maxAttempts != 1 {
		t.Errorf("expected max attempts clamped to 1, got %d", retry.maxAttempts)
	}

	retry.MaxAttempts(0)
	if retry.maxAttempts != 1 {
		t.Errorf("expected max attempts clamped to 1, got %d", retry.maxAttempts)
	}

	retry.BaseDelay(-100 * time.Millisecond)
	if retry.baseDelay != 0 {
		t.Errorf("expected base delay clamped to 0, got %v", retry.baseDelay)
	}

	retry.MaxDelay(-100 * time.Millisecond)
	if retry.maxDelay != 0 {
		t.Errorf("expected max delay clamped to 0, got %v", retry.maxDelay)
	}
}

func TestRetryErrorClassification(t *testing.T) {
	tests := []struct {
		name     string
		error    string
		expected bool
	}{
		{"timeout error", "request timeout", true},
		{"connection refused", "connection refused", true},
		{"network error", "network unreachable", true},
		{"rate limit", "rate limit exceeded", true},
		{"unauthorized", "authentication failed", false},
		{"not found", "resource not found", false},
		{"invalid input", "invalid request format", false},
		{"unknown error", "something went wrong", true}, // Default to retryable
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.error)
			result := isRetryableError(err)
			if result != tt.expected {
				t.Errorf("expected %t for error '%s', got %t", tt.expected, tt.error, result)
			}
		})
	}

	// Test nil error
	if isRetryableError(nil) {
		t.Error("expected nil error to be non-retryable")
	}
}

// Benchmark the retry processor performance.
func BenchmarkRetrySuccessfulProcessing(b *testing.B) {
	ctx := context.Background()
	processor := NewTestProcessor("bench")
	retry := NewRetry(processor, RealClock)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		output := retry.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Consume output
		}
	}
}

func BenchmarkRetryWithFailures(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		processor := NewTestProcessor("bench").WithFailures(2) // Fail twice
		retry := NewRetry(processor, RealClock).
			MaxAttempts(3).
			BaseDelay(1 * time.Microsecond) // Minimal delay for benchmarking

		input := make(chan int, 1)
		input <- i
		close(input)

		output := retry.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Consume output
		}
	}
}

// Example demonstrates basic retry usage.
func ExampleRetry() {
	ctx := context.Background()

	// Create a processor that might fail
	processor := NewTestProcessor("example").WithFailures(1)

	// Wrap with retry logic
	retry := NewRetry(processor, RealClock).
		MaxAttempts(3).
		BaseDelay(100 * time.Millisecond).
		WithName("example-retry")

	// Process data
	input := make(chan int, 1)
	input <- 42
	close(input)

	output := retry.Process(ctx, input)
	for result := range output {
		fmt.Printf("Result: %d\n", result)
	}

	// Output: Result: 84
}

// Example demonstrates custom error handling.
func ExampleRetry_customErrorHandling() {
	ctx := context.Background()

	processor := NewTestProcessor("example")
	retry := NewRetry(processor, RealClock).
		MaxAttempts(3).
		OnError(func(err error, attempt int) bool {
			// Custom retry logic
			fmt.Printf("Error on attempt %d: %v\n", attempt, err)
			return attempt < 2 // Only retry once
		})

	input := make(chan int, 1)
	input <- 42
	close(input)

	output := retry.Process(ctx, input)
	for result := range output {
		fmt.Printf("Result: %d\n", result)
	}
}
