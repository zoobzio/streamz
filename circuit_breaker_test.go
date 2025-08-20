package streamz

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCircuitBreakerBasicStates tests basic state transitions.
func TestCircuitBreakerBasicStates(t *testing.T) {
	ctx := context.Background()

	// Create a processor that we can control
	processor := NewTestProcessor("test")
	cb := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.5).
		MinRequests(2).
		RecoveryTimeout(100 * time.Millisecond)

	// Should start in closed state
	if cb.GetState() != StateClosed {
		t.Errorf("expected initial state to be closed, got %s", cb.GetState())
	}

	// Test successful requests don't open circuit
	processor.WithFailures(0)
	input := make(chan int, 3)
	input <- 1
	input <- 2
	input <- 3
	close(input)

	output := cb.Process(ctx, input)
	results := collectAll(output)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if cb.GetState() != StateClosed {
		t.Errorf("expected state to remain closed, got %s", cb.GetState())
	}
}

// TestCircuitBreakerOpensOnFailures tests circuit opening on high failure rate.
func TestCircuitBreakerOpensOnFailures(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test")

	var stateChanges []string
	cb := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.5).
		MinRequests(2). // Lower for easier testing
		OnStateChange(func(from, to State) {
			stateChanges = append(stateChanges, fmt.Sprintf("%s->%s", from, to))
		})

	// Send requests that will fail
	processor.WithFailures(10) // All will fail

	input := make(chan int)
	go func() {
		defer close(input)
		for i := 1; i <= 5; i++ {
			input <- i
			time.Sleep(10 * time.Millisecond) // Space out requests
		}
	}()

	output := cb.Process(ctx, input)
	results := collectAll(output)

	// After 2+ requests with 100% failure rate, circuit should open
	stats := cb.GetStats()
	if cb.GetState() != StateOpen {
		t.Errorf("expected circuit to be open, got %s", cb.GetState())
	}

	// Should have very few results (all fail)
	if len(results) > 0 {
		t.Errorf("expected no results with all failures, got %d", len(results))
	}

	// Verify state change was recorded
	if len(stateChanges) == 0 {
		t.Errorf("expected state change callback, got none")
	}

	t.Logf("State changes: %v", stateChanges)
	t.Logf("Stats: requests=%d, failures=%d, successes=%d",
		stats.Requests, stats.Failures, stats.Successes)
}

// TestCircuitBreakerRecovery tests half-open and recovery behavior.
func TestCircuitBreakerRecovery(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test")
	clk := NewFakeClock(time.Now())
	cb := NewCircuitBreaker(processor, clk).
		FailureThreshold(0.5).
		MinRequests(2).
		RecoveryTimeout(100 * time.Millisecond).
		HalfOpenRequests(2)

	// Force circuit to open
	processor.WithFailures(3)
	input1 := make(chan int, 3)
	for i := 1; i <= 3; i++ {
		input1 <- i
	}
	close(input1)

	output1 := cb.Process(ctx, input1)
	_ = collectAll(output1)

	if cb.GetState() != StateOpen {
		t.Errorf("expected circuit to be open, got %s", cb.GetState())
	}

	// Advance time past recovery timeout
	clk.Step(150 * time.Millisecond)

	// Now processor succeeds
	processor.WithFailures(0)

	// Send new requests - should transition to half-open
	input2 := make(chan int, 4)
	for i := 4; i <= 7; i++ {
		input2 <- i
	}
	close(input2)

	output2 := cb.Process(ctx, input2)
	results2 := collectAll(output2)

	// Circuit should have allowed some half-open requests
	// and then closed after successes
	if cb.GetState() != StateClosed {
		t.Errorf("expected circuit to close after successful half-open, got %s", cb.GetState())
	}

	if len(results2) < 2 {
		t.Errorf("expected at least 2 results in recovery, got %d", len(results2))
	}
}

// TestCircuitBreakerHalfOpenFailure tests returning to open from half-open on failures.
func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test")

	var stateChanges []string
	cb := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.5).
		MinRequests(2).
		RecoveryTimeout(50 * time.Millisecond).
		HalfOpenRequests(2).
		OnStateChange(func(from, to State) {
			stateChanges = append(stateChanges, fmt.Sprintf("%s->%s", from, to))
		})

	// Force circuit to open
	processor.WithFailures(3)
	input1 := make(chan int, 3)
	for i := 1; i <= 3; i++ {
		input1 <- i
	}
	close(input1)

	_ = collectAll(cb.Process(ctx, input1))

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)

	// Processor still fails
	processor.WithFailures(3)

	// Send requests during half-open
	input2 := make(chan int, 3)
	for i := 4; i <= 6; i++ {
		input2 <- i
	}
	close(input2)

	_ = collectAll(cb.Process(ctx, input2))

	// Circuit should return to open after half-open failures
	if cb.GetState() != StateOpen {
		t.Errorf("expected circuit to reopen after half-open failures, got %s", cb.GetState())
	}

	// Should see transitions: closed->open->half-open->open
	if len(stateChanges) < 3 {
		t.Errorf("expected at least 3 state changes, got %d: %v",
			len(stateChanges), stateChanges)
	}
}

// TestCircuitBreakerConcurrency tests thread safety.
func TestCircuitBreakerConcurrency(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test")
	cb := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.5).
		MinRequests(10)

	// Run concurrent producers with mixed success/failure
	var wg sync.WaitGroup
	var resultCount int64

	// Start with some successes to prevent immediate circuit opening
	processor.WithFailures(0)

	for i := 0; i < 5; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			input := make(chan int)
			go func() {
				defer close(input)
				for j := 0; j < 10; j++ {
					input <- idx*10 + j
					time.Sleep(time.Millisecond)
				}
			}()

			output := cb.Process(ctx, input)
			for range output {
				atomic.AddInt64(&resultCount, 1)
			}
		}(i)
	}

	wg.Wait()

	stats := cb.GetStats()
	t.Logf("Concurrent test stats: requests=%d, failures=%d, successes=%d, state=%s",
		stats.Requests, stats.Failures, stats.Successes, stats.State)

	// Should have processed some requests (at least some before any potential circuit opening)
	results := atomic.LoadInt64(&resultCount)
	if results == 0 {
		t.Error("expected some successful results from concurrent processing")
	}

	// Should have recorded all attempts
	if stats.Requests == 0 {
		t.Error("expected requests to be recorded")
	}
}

// TestCircuitBreakerMinRequests tests minimum request threshold.
func TestCircuitBreakerMinRequests(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test").WithFailures(100) // Always fail
	cb := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.5).
		MinRequests(5) // Need 5 requests before opening

	// Process requests one by one to see state changes
	for i := 1; i <= 10; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		_ = collectAll(cb.Process(ctx, input))

		stats := cb.GetStats()
		state := cb.GetState()

		t.Logf("After request %d: state=%s, requests=%d, failures=%d",
			i, state, stats.Requests, stats.Failures)

		// Should remain closed until we have MinRequests
		if i < 5 && state != StateClosed {
			t.Errorf("request %d: expected closed state with < MinRequests, got %s", i, state)
		}

		// Should open after MinRequests with 100% failure rate
		if i >= 5 && state != StateOpen {
			t.Errorf("request %d: expected open state after MinRequests with failures, got %s", i, state)
			break // No point continuing if it's not opening
		}
	}
}

// TestCircuitBreakerCallbacks tests callback functionality.
func TestCircuitBreakerCallbacks(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test")

	var openCallbackInvoked bool
	var openStats CircuitStats
	var stateChanges []string

	cb := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.5).
		MinRequests(2).
		OnStateChange(func(from, to State) {
			stateChanges = append(stateChanges, fmt.Sprintf("%s->%s", from, to))
		}).
		OnOpen(func(stats CircuitStats) {
			openCallbackInvoked = true
			openStats = stats
		})

	// Force circuit to open
	processor.WithFailures(3)
	input := make(chan int, 3)
	for i := 1; i <= 3; i++ {
		input <- i
	}
	close(input)

	_ = collectAll(cb.Process(ctx, input))

	// Verify callbacks were invoked
	if !openCallbackInvoked {
		t.Error("expected OnOpen callback to be invoked")
	}

	if openStats.State != StateOpen {
		t.Errorf("expected stats to show open state, got %s", openStats.State)
	}

	if openStats.Failures < 2 {
		t.Errorf("expected at least 2 failures in stats, got %d", openStats.Failures)
	}

	if len(stateChanges) == 0 {
		t.Error("expected state change callback to be invoked")
	}
}

// TestCircuitBreakerFluentAPI tests fluent configuration.
func TestCircuitBreakerFluentAPI(t *testing.T) {
	processor := NewTestProcessor("test")

	cb := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.7).
		MinRequests(20).
		RecoveryTimeout(60 * time.Second).
		HalfOpenRequests(5).
		WithName("test-circuit")

	if cb.failureThreshold != 0.7 {
		t.Errorf("expected failure threshold 0.7, got %f", cb.failureThreshold)
	}
	if cb.minRequests != 20 {
		t.Errorf("expected min requests 20, got %d", cb.minRequests)
	}
	if cb.recoveryTimeout != 60*time.Second {
		t.Errorf("expected recovery timeout 60s, got %v", cb.recoveryTimeout)
	}
	if cb.halfOpenRequests != 5 {
		t.Errorf("expected half-open requests 5, got %d", cb.halfOpenRequests)
	}
	if cb.Name() != "test-circuit" {
		t.Errorf("expected name 'test-circuit', got %s", cb.Name())
	}
}

// TestCircuitBreakerParameterValidation tests parameter bounds.
func TestCircuitBreakerParameterValidation(t *testing.T) {
	processor := NewTestProcessor("test")
	cb := NewCircuitBreaker(processor, RealClock)

	// Test threshold bounds
	cb.FailureThreshold(-0.1)
	if cb.failureThreshold != 0 {
		t.Errorf("expected threshold clamped to 0, got %f", cb.failureThreshold)
	}

	cb.FailureThreshold(1.5)
	if cb.failureThreshold != 1 {
		t.Errorf("expected threshold clamped to 1, got %f", cb.failureThreshold)
	}

	// Test min requests
	cb.MinRequests(0)
	if cb.minRequests != 1 {
		t.Errorf("expected min requests clamped to 1, got %d", cb.minRequests)
	}

	// Test recovery timeout
	cb.RecoveryTimeout(-10 * time.Second)
	if cb.recoveryTimeout != 0 {
		t.Errorf("expected recovery timeout clamped to 0, got %v", cb.recoveryTimeout)
	}

	// Test half-open requests
	cb.HalfOpenRequests(0)
	if cb.halfOpenRequests != 1 {
		t.Errorf("expected half-open requests clamped to 1, got %d", cb.halfOpenRequests)
	}
}

// TestCircuitBreakerGetStats tests statistics retrieval.
func TestCircuitBreakerGetStats(t *testing.T) {
	ctx := context.Background()

	processor := NewTestProcessor("test")
	cb := NewCircuitBreaker(processor, RealClock)

	// Process some successful requests
	processor.WithFailures(0)
	input1 := make(chan int, 3)
	for i := 1; i <= 3; i++ {
		input1 <- i
	}
	close(input1)
	_ = collectAll(cb.Process(ctx, input1))

	// Process some failed requests
	processor.WithFailures(2)
	input2 := make(chan int, 2)
	for i := 4; i <= 5; i++ {
		input2 <- i
	}
	close(input2)
	_ = collectAll(cb.Process(ctx, input2))

	stats := cb.GetStats()

	if stats.Requests != 5 {
		t.Errorf("expected 5 total requests, got %d", stats.Requests)
	}
	if stats.Successes != 3 {
		t.Errorf("expected 3 successes, got %d", stats.Successes)
	}
	if stats.Failures != 2 {
		t.Errorf("expected 2 failures, got %d", stats.Failures)
	}
	if stats.State != StateClosed {
		t.Errorf("expected closed state, got %s", stats.State)
	}
}

// TestCircuitBreakerStateStrings tests State string representation.
func TestCircuitBreakerStateStrings(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

// BenchmarkCircuitBreakerClosed benchmarks closed state performance.
func BenchmarkCircuitBreakerClosed(b *testing.B) {
	ctx := context.Background()
	processor := NewTestProcessor("bench")
	cb := NewCircuitBreaker(processor, RealClock)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		output := cb.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Consume
		}
	}
}

// BenchmarkCircuitBreakerOpen benchmarks open state performance.
func BenchmarkCircuitBreakerOpen(b *testing.B) {
	ctx := context.Background()
	processor := NewTestProcessor("bench").WithFailures(1000)
	cb := NewCircuitBreaker(processor, RealClock).MinRequests(1)

	// Force circuit open
	input := make(chan int, 10)
	for i := 0; i < 10; i++ {
		input <- i
	}
	close(input)
	_ = collectAll(cb.Process(ctx, input))

	if cb.GetState() != StateOpen {
		b.Fatalf("circuit should be open for benchmark")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		output := cb.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Should be empty when open
		}
	}
}

// Example demonstrates basic circuit breaker usage.
func ExampleCircuitBreaker() {
	ctx := context.Background()

	// Create a processor that might fail
	processor := NewTestProcessor("api")

	// Wrap with circuit breaker
	protected := NewCircuitBreaker(processor, RealClock).
		FailureThreshold(0.5).
		MinRequests(10).
		WithName("api-protection")

	// Process requests
	input := make(chan int, 1)
	input <- 42
	close(input)

	output := protected.Process(ctx, input)
	for result := range output {
		fmt.Printf("Result: %d\n", result)
	}

	// Output: Result: 84
}

// Example demonstrates state change monitoring.
func ExampleCircuitBreaker_monitoring() {
	ctx := context.Background()

	processor := NewTestProcessor("service")
	cb := NewCircuitBreaker(processor, RealClock).
		OnStateChange(func(from, to State) {
			fmt.Printf("State changed: %s -> %s\n", from, to)
		}).
		OnOpen(func(stats CircuitStats) {
			fmt.Printf("Circuit opened! Failures: %d/%d\n",
				stats.Failures, stats.Requests)
		})

	// Simulate failures to open circuit
	processor.WithFailures(5)
	input := make(chan int, 5)
	for i := 0; i < 5; i++ {
		input <- i
	}
	close(input)

	_ = cb.Process(ctx, input)
}

// Helper function to collect all items from a channel.
func collectAll[T any](ch <-chan T) []T {
	var results []T //nolint:prealloc // dynamic growth acceptable in test code
	for item := range ch {
		results = append(results, item)
	}
	return results
}
