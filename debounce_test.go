package streamz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

func TestDebounce_Name(t *testing.T) {
	debounce := NewDebounce[string](100*time.Millisecond, RealClock)
	if debounce.Name() != "debounce" {
		t.Errorf("expected name 'debounce', got %q", debounce.Name())
	}
}

func TestDebounce_SingleItem(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int], 1)
	in <- NewSuccess(42)
	close(in)

	out := debounce.Process(ctx, in)

	// Should receive the item immediately on channel close (flush pending)
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	if result.Value() != 42 {
		t.Errorf("expected value 42, got %d", result.Value())
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_SingleItemWithDelay(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := debounce.Process(ctx, in)

	// Send item but don't close channel
	in <- NewSuccess(42)

	// Should not receive anything immediately
	select {
	case result := <-out:
		t.Errorf("unexpected immediate result: %v", result)
	default:
		// Good, nothing received yet
	}

	// Advance time to trigger debounce
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady()

	// Now should receive the debounced item
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	if result.Value() != 42 {
		t.Errorf("expected value 42, got %d", result.Value())
	}

	// Close channel
	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_MultipleItemsRapid(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](200*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string], 3)
	in <- NewSuccess("first")
	in <- NewSuccess("second")
	in <- NewSuccess("third")
	close(in)

	out := debounce.Process(ctx, in)

	// Should receive only the last item immediately (flush pending on close)
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	if result.Value() != "third" {
		t.Errorf("expected value 'third', got %q", result.Value())
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_MultipleItemsWithTimer(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](200*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	out := debounce.Process(ctx, in)

	// Send rapid items without closing
	in <- NewSuccess("first")
	in <- NewSuccess("second")
	in <- NewSuccess("third")

	// Should not receive anything immediately
	select {
	case result := <-out:
		t.Errorf("unexpected immediate result: %v", result)
	default:
		// Good, nothing received yet
	}

	// Advance time less than debounce duration
	clock.Advance(50 * time.Millisecond)
	select {
	case result := <-out:
		t.Errorf("unexpected early result: %v", result)
	default:
		// Good, still debouncing
	}

	// Advance time to complete debounce period
	clock.Advance(200 * time.Millisecond)
	clock.BlockUntilReady()

	// Should receive only the last item
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	if result.Value() != "third" {
		t.Errorf("expected value 'third', got %q", result.Value())
	}

	// Close channel
	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_ItemsWithGaps(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := debounce.Process(ctx, in)

	// Send first item
	in <- NewSuccess(1)

	// Give time for timer to be registered
	time.Sleep(time.Millisecond)

	// Advance time to trigger first debounce
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady()

	result1 := <-out
	if result1.IsError() {
		t.Errorf("unexpected error: %v", result1.Error())
	}
	if result1.Value() != 1 {
		t.Errorf("expected value 1, got %d", result1.Value())
	}

	// Send second item after a gap
	in <- NewSuccess(2)

	// Give time for timer to be registered
	time.Sleep(time.Millisecond)

	// Advance time to trigger second debounce
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady()

	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	if result2.Value() != 2 {
		t.Errorf("expected value 2, got %d", result2.Value())
	}

	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_ErrorsPassThrough(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string], 3)
	in <- NewSuccess("before")
	in <- NewError("", errors.New("test error"), "test-processor")
	in <- NewSuccess("after")
	close(in)

	out := debounce.Process(ctx, in)

	// Should immediately receive the error
	result1 := <-out
	if !result1.IsError() {
		t.Error("expected error result")
	}
	if result1.Error().Err.Error() != "test error" {
		t.Errorf("expected error 'test error', got %q", result1.Error().Err.Error())
	}

	// Should receive "after" immediately on channel close (flush pending)
	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	if result2.Value() != "after" {
		t.Errorf("expected value 'after', got %q", result2.Value())
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_ErrorsPassThroughWithTimer(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	out := debounce.Process(ctx, in)

	// Send success value first
	in <- NewSuccess("before")

	// Send error - should pass through immediately
	in <- NewError("", errors.New("test error"), "test-processor")

	// Should immediately receive the error
	result1 := <-out
	if !result1.IsError() {
		t.Error("expected error result")
	}
	if result1.Error().Err.Error() != "test error" {
		t.Errorf("expected error 'test error', got %q", result1.Error().Err.Error())
	}

	// Send another success value
	in <- NewSuccess("after")

	// Should not receive anything else immediately
	select {
	case result := <-out:
		t.Errorf("unexpected immediate result: %v", result)
	default:
		// Good, still debouncing "after"
	}

	// Give time for timer to be registered
	time.Sleep(time.Millisecond)

	// Advance time to trigger debounce for "after"
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady()

	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	if result2.Value() != "after" {
		t.Errorf("expected value 'after', got %q", result2.Value())
	}

	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_MultipleErrors(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int], 3)
	in <- NewError(0, errors.New("error 1"), "processor-1")
	in <- NewError(0, errors.New("error 2"), "processor-2")
	in <- NewError(0, errors.New("error 3"), "processor-3")
	close(in)

	out := debounce.Process(ctx, in)

	// Should receive all errors immediately
	for i := 1; i <= 3; i++ {
		result := <-out
		if !result.IsError() {
			t.Errorf("expected error result %d", i)
		}
		expectedMsg := "error " + string(rune('0'+i))
		if result.Error().Err.Error() != expectedMsg {
			t.Errorf("expected error %q, got %q", expectedMsg, result.Error().Err.Error())
		}
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_ContextCancellation(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](100*time.Millisecond, clock)
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan Result[string])
	out := debounce.Process(ctx, in)

	// Send item
	in <- NewSuccess("test")

	// Cancel context before timer fires
	cancel()

	// Should not receive anything due to cancellation
	select {
	case result := <-out:
		t.Errorf("unexpected result after cancellation: %v", result)
	default:
		// Good, nothing received
	}

	// Channel should eventually be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}

	close(in)
}

func TestDebounce_ContextCancellationDuringOutput(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](100*time.Millisecond, clock)
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan Result[string])
	out := debounce.Process(ctx, in)

	// Send item
	in <- NewSuccess("test")

	// Cancel context immediately before timer can fire
	cancel()

	// Advance time (timer should still fire but context is canceled)
	clock.Advance(100 * time.Millisecond)

	// Should not receive anything due to cancellation during output
	select {
	case result := <-out:
		t.Errorf("unexpected result after cancellation: %v", result)
	default:
		// Good, nothing received
	}

	// Channel should eventually be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}

	close(in)
}

func TestDebounce_FlushPendingOnClose(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int], 2)
	in <- NewSuccess(1)
	in <- NewSuccess(2)
	// Close without waiting for timer
	close(in)

	out := debounce.Process(ctx, in)

	// Should immediately receive the last pending item
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	if result.Value() != 2 {
		t.Errorf("expected value 2, got %d", result.Value())
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_EmptyInput(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	close(in)

	out := debounce.Process(ctx, in)

	// Should not receive anything and channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed immediately")
	}
}

func TestDebounce_OnlyErrors(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[string](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string], 2)
	in <- NewError("", errors.New("error 1"), "processor-1")
	in <- NewError("", errors.New("error 2"), "processor-2")
	close(in)

	out := debounce.Process(ctx, in)

	// Should receive both errors immediately
	result1 := <-out
	if !result1.IsError() {
		t.Error("expected first error result")
	}

	result2 := <-out
	if !result2.IsError() {
		t.Error("expected second error result")
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestDebounce_TimerResetBehavior(t *testing.T) {
	clock := clockz.NewFakeClock()
	debounce := NewDebounce[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := debounce.Process(ctx, in)

	// Send first item
	in <- NewSuccess(1)

	// Advance time partially
	clock.Advance(50 * time.Millisecond)

	// Send second item - should reset timer
	in <- NewSuccess(2)

	// Give time for timer to be reset
	time.Sleep(time.Millisecond)

	// Advance another 50ms - still within new debounce period
	clock.Advance(50 * time.Millisecond)

	// Should not receive anything yet
	select {
	case result := <-out:
		t.Errorf("unexpected result during reset period: %v", result)
	default:
		// Good
	}

	// Advance remaining time to complete new debounce period
	clock.Advance(50 * time.Millisecond)
	clock.BlockUntilReady()

	// Should receive the second item
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	if result.Value() != 2 {
		t.Errorf("expected value 2, got %d", result.Value())
	}

	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

// BenchmarkDebounce_Success tests the performance of successful debounce operations.
func BenchmarkDebounce_Success(b *testing.B) {
	debounce := NewDebounce[int](time.Nanosecond, RealClock) // Very short duration for benchmark
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 1)
		in <- NewSuccess(i)
		close(in)

		out := debounce.Process(ctx, in)
		<-out // Wait for result
	}
}

// BenchmarkDebounce_Errors tests the performance of error pass-through.
func BenchmarkDebounce_Errors(b *testing.B) {
	debounce := NewDebounce[int](100*time.Millisecond, RealClock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 1)
		in <- NewError(i, errors.New("test error"), "test-processor")
		close(in)

		out := debounce.Process(ctx, in)
		<-out // Wait for result
	}
}

// BenchmarkDebounce_RapidItems tests performance with multiple rapid items.
func BenchmarkDebounce_RapidItems(b *testing.B) {
	debounce := NewDebounce[int](time.Nanosecond, RealClock) // Very short duration
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 3)
		in <- NewSuccess(1)
		in <- NewSuccess(2)
		in <- NewSuccess(3)
		close(in)

		out := debounce.Process(ctx, in)
		<-out // Wait for result (should be 3)
	}
}
