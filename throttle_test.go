package streamz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

func TestThrottle_Name(t *testing.T) {
	throttle := NewThrottle[string](100*time.Millisecond, RealClock)
	if throttle.Name() != "throttle" {
		t.Errorf("expected name 'throttle', got %q", throttle.Name())
	}
}

// TestThrottle_TimestampBasic tests the fundamental timestamp-based throttling behavior.
func TestThrottle_TimestampBasic(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[string](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	out := throttle.Process(ctx, in)

	// First item passes immediately (leading edge)
	in <- NewSuccess("first")
	result1 := <-out
	if result1.IsError() || result1.Value() != "first" {
		t.Errorf("expected success('first'), got %v", result1)
	}

	// Second item during cooling should be dropped
	in <- NewSuccess("dropped")

	// Verify nothing received (non-blocking check)
	select {
	case unexpected := <-out:
		t.Errorf("unexpected result during cooling: %v", unexpected)
	case <-time.After(10 * time.Millisecond):
		// Good - nothing during cooling
	}

	// Advance time past cooling period
	clock.Advance(100 * time.Millisecond)
	// No BlockUntilReady() needed - no timers to synchronize!

	// Next item should pass
	in <- NewSuccess("second")
	result2 := <-out
	if result2.Value() != "second" {
		t.Errorf("expected 'second', got %v", result2)
	}

	close(in)
	_, ok := <-out
	if ok {
		t.Error("expected output channel to be closed")
	}
}

// TestThrottle_TimestampErrorPassthrough verifies errors bypass throttling entirely.
func TestThrottle_TimestampErrorPassthrough(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := throttle.Process(ctx, in)

	// Start cooling with success
	in <- NewSuccess(1)
	<-out

	// Error should pass through immediately, even during cooling
	in <- NewError(0, errors.New("test error"), "test-proc")
	errorResult := <-out

	if !errorResult.IsError() {
		t.Error("expected error to pass through during cooling")
	}
	if errorResult.Error().Err.Error() != "test error" {
		t.Errorf("expected 'test error', got %q", errorResult.Error().Err.Error())
	}

	// Success during cooling should still be dropped
	in <- NewSuccess(2)
	select {
	case unexpected := <-out:
		t.Errorf("success leaked during cooling: %v", unexpected)
	case <-time.After(10 * time.Millisecond):
		// Good
	}

	close(in)
}

// TestThrottle_TimestampContextCancellation verifies graceful shutdown.
func TestThrottle_TimestampContextCancellation(t *testing.T) {
	throttle := NewThrottle[string](100*time.Millisecond, RealClock)
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan Result[string])
	out := throttle.Process(ctx, in)

	// Cancel context
	cancel()

	// Output should close promptly
	select {
	case _, ok := <-out:
		if ok {
			t.Error("expected output to be closed after cancellation")
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("output didn't close promptly after cancellation")
	}

	close(in)
}

// TestThrottle_TimestampZeroDuration verifies zero duration behavior.
func TestThrottle_TimestampZeroDuration(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[int](0, clock) // Zero duration - no throttling
	ctx := context.Background()

	in := make(chan Result[int])
	out := throttle.Process(ctx, in)

	// All items should pass through immediately
	for i := 1; i <= 5; i++ {
		in <- NewSuccess(i)
		result := <-out
		if result.Value() != i {
			t.Errorf("expected %d, got %d", i, result.Value())
		}
	}

	close(in)
	_, ok := <-out
	if ok {
		t.Error("expected output to be closed")
	}
}

// TestThrottle_TimestampMultipleCycles verifies throttling works across multiple cycles.
func TestThrottle_TimestampMultipleCycles(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[int](50*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := throttle.Process(ctx, in)

	for cycle := 1; cycle <= 3; cycle++ {
		// Send item
		in <- NewSuccess(cycle)
		result := <-out
		if result.Value() != cycle {
			t.Errorf("cycle %d: expected %d, got %d", cycle, cycle, result.Value())
		}

		// Wait for cooling to end
		clock.Advance(50 * time.Millisecond)
		// No synchronization needed - timestamp immediately available
	}

	close(in)
}

// TestThrottle_TimestampEmptyInput verifies behavior with immediately closed input.
func TestThrottle_TimestampEmptyInput(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[string](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	close(in) // Close immediately

	out := throttle.Process(ctx, in)

	// Should close immediately with no processing goroutines
	select {
	case _, ok := <-out:
		if ok {
			t.Error("expected output to be closed immediately")
		}
	case <-time.After(10 * time.Millisecond):
		t.Error("output didn't close promptly")
	}
}

// BenchmarkThrottle_TimestampSuccess benchmarks timestamp-based performance.
func BenchmarkThrottle_TimestampSuccess(b *testing.B) {
	throttle := NewThrottle[int](time.Microsecond, RealClock) // Very short for benchmark
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 1)
		in <- NewSuccess(i)
		close(in)

		out := throttle.Process(ctx, in)
		<-out // Wait for result
	}
}

// BenchmarkThrottle_TimestampErrors benchmarks error passthrough performance.
func BenchmarkThrottle_TimestampErrors(b *testing.B) {
	throttle := NewThrottle[int](100*time.Millisecond, RealClock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 1)
		in <- NewError(i, errors.New("test error"), "test-processor")
		close(in)

		out := throttle.Process(ctx, in)
		<-out // Wait for result
	}
}
