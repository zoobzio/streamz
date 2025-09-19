package streamz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

func TestBatcher_Name(t *testing.T) {
	batcher := NewBatcher[string](BatchConfig{MaxSize: 10}, RealClock)
	if batcher.Name() != "batcher" {
		t.Errorf("expected name 'batcher', got %q", batcher.Name())
	}
}

func TestBatcher_SingleItem_SizeOnly(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{MaxSize: 1}, clock)
	ctx := context.Background()

	in := make(chan Result[int], 1)
	in <- NewSuccess(42)
	close(in)

	out := batcher.Process(ctx, in)

	// Should receive batch immediately when size limit reached
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	batch := result.Value()
	if len(batch) != 1 {
		t.Errorf("expected batch size 1, got %d", len(batch))
	}
	if batch[0] != 42 {
		t.Errorf("expected batch[0] = 42, got %d", batch[0])
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_SingleItem_FlushOnClose(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx := context.Background()

	in := make(chan Result[int], 1)
	in <- NewSuccess(42)
	close(in)

	out := batcher.Process(ctx, in)

	// Should receive batch immediately on channel close (flush pending)
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	batch := result.Value()
	if len(batch) != 1 {
		t.Errorf("expected batch size 1, got %d", len(batch))
	}
	if batch[0] != 42 {
		t.Errorf("expected batch[0] = 42, got %d", batch[0])
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_MultipleItems_SizeLimit(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[string](BatchConfig{
		MaxSize:    3,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx := context.Background()

	in := make(chan Result[string], 5)
	in <- NewSuccess("first")
	in <- NewSuccess("second")
	in <- NewSuccess("third")
	in <- NewSuccess("fourth")
	in <- NewSuccess("fifth")
	close(in)

	out := batcher.Process(ctx, in)

	// Should receive first batch of 3 items
	result1 := <-out
	if result1.IsError() {
		t.Errorf("unexpected error: %v", result1.Error())
	}
	batch1 := result1.Value()
	if len(batch1) != 3 {
		t.Errorf("expected first batch size 3, got %d", len(batch1))
	}
	expected1 := []string{"first", "second", "third"}
	for i, expected := range expected1 {
		if batch1[i] != expected {
			t.Errorf("expected batch1[%d] = %q, got %q", i, expected, batch1[i])
		}
	}

	// Should receive second batch with remaining 2 items (flushed on close)
	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	batch2 := result2.Value()
	if len(batch2) != 2 {
		t.Errorf("expected second batch size 2, got %d", len(batch2))
	}
	expected2 := []string{"fourth", "fifth"}
	for i, expected := range expected2 {
		if batch2[i] != expected {
			t.Errorf("expected batch2[%d] = %q, got %q", i, expected, batch2[i])
		}
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_TimeBasedBatching(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := batcher.Process(ctx, in)

	// Send items without reaching size limit
	in <- NewSuccess(1)
	in <- NewSuccess(2)

	// Should not receive batch immediately
	select {
	case result := <-out:
		t.Errorf("unexpected immediate result: %v", result)
	default:
		// Good, nothing received yet
	}

	// Advance time to trigger timer
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady() // Ensure timer processed before continuing

	// Now should receive the time-triggered batch
	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	batch := result.Value()
	if len(batch) != 2 {
		t.Errorf("expected batch size 2, got %d", len(batch))
	}
	if batch[0] != 1 || batch[1] != 2 {
		t.Errorf("expected batch [1, 2], got %v", batch)
	}

	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_ErrorsPassThrough(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[string](BatchConfig{
		MaxSize:    3,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx := context.Background()

	in := make(chan Result[string], 5)
	in <- NewSuccess("item1")
	in <- NewError("", errors.New("test error"), "test-processor")
	in <- NewSuccess("item2")
	in <- NewSuccess("item3")
	close(in)

	out := batcher.Process(ctx, in)

	// Should immediately receive the error
	result1 := <-out
	if !result1.IsError() {
		t.Error("expected error result")
	}
	if result1.Error().Err.Error() != "test error" {
		t.Errorf("expected error 'test error', got %q", result1.Error().Err.Error())
	}

	// Should receive batch with successful items (flushed on close)
	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	batch := result2.Value()
	if len(batch) != 3 {
		t.Errorf("expected batch size 3, got %d", len(batch))
	}
	expected := []string{"item1", "item2", "item3"}
	for i, exp := range expected {
		if batch[i] != exp {
			t.Errorf("expected batch[%d] = %q, got %q", i, exp, batch[i])
		}
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_MultipleErrors(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{MaxSize: 10}, clock)
	ctx := context.Background()

	in := make(chan Result[int], 3)
	in <- NewError(0, errors.New("error 1"), "processor-1")
	in <- NewError(0, errors.New("error 2"), "processor-2")
	in <- NewError(0, errors.New("error 3"), "processor-3")
	close(in)

	out := batcher.Process(ctx, in)

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

func TestBatcher_ErrorsWithTimeBasedBatch(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := batcher.Process(ctx, in)

	// Send successful items
	in <- NewSuccess(1)
	in <- NewSuccess(2)

	// Send error - should pass through immediately
	in <- NewError(0, errors.New("test error"), "test-processor")

	// Should immediately receive the error
	result1 := <-out
	if !result1.IsError() {
		t.Error("expected error result")
	}

	// Should not receive batch yet (timer still running)
	select {
	case result := <-out:
		t.Errorf("unexpected result before timer: %v", result)
	default:
		// Good, timer still running
	}

	// Advance time to trigger batch
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady() // Ensure timer processed before input

	// Now should receive the time-triggered batch
	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	batch := result2.Value()
	if len(batch) != 2 {
		t.Errorf("expected batch size 2, got %d", len(batch))
	}
	if batch[0] != 1 || batch[1] != 2 {
		t.Errorf("expected batch [1, 2], got %v", batch)
	}

	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_ContextCancellation(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[string](BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan Result[string])
	out := batcher.Process(ctx, in)

	// Send items
	in <- NewSuccess("test1")
	in <- NewSuccess("test2")

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

func TestBatcher_ContextCancellationDuringOutput(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[string](BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan Result[string])
	out := batcher.Process(ctx, in)

	// Send items
	in <- NewSuccess("test1")
	in <- NewSuccess("test2")

	// Cancel context immediately
	cancel()

	// Channel should eventually be closed without emitting batch
	select {
	case result, ok := <-out:
		if !ok {
			// Channel closed without emitting - this is acceptable
			return
		}
		// If we get a result, it might be a race between timer and cancellation
		// This is actually acceptable behavior - the timer fired just before cancellation
		t.Logf("Got result after cancellation (race condition): %v", result)
	case <-time.After(50 * time.Millisecond):
		t.Error("timeout waiting for channel action")
	}

	// Consume any remaining items and verify channel closes
	//nolint:revive // empty-block: intentional channel draining
	for range out {
		// Drain channel until closed
	}

	close(in)
}

func TestBatcher_EmptyInput(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[string](BatchConfig{MaxSize: 10}, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	close(in)

	out := batcher.Process(ctx, in)

	// Should not receive anything and channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed immediately")
	}
}

func TestBatcher_NoLatencyConfig(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{
		MaxSize: 3, // No MaxLatency configured
	}, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := batcher.Process(ctx, in)

	// Send items without reaching size limit
	in <- NewSuccess(1)
	in <- NewSuccess(2)

	// Should not receive batch (no timer started)
	select {
	case result := <-out:
		t.Errorf("unexpected result without timer: %v", result)
	default:
		// Good, no timer = no time-based batching
	}

	// Advance time (should have no effect)
	clock.Advance(time.Hour)

	select {
	case result := <-out:
		t.Errorf("unexpected result after time advance: %v", result)
	default:
		// Good, no timer was created
	}

	// Close channel should flush pending batch
	close(in)

	result := <-out
	if result.IsError() {
		t.Errorf("unexpected error: %v", result.Error())
	}
	batch := result.Value()
	if len(batch) != 2 {
		t.Errorf("expected batch size 2, got %d", len(batch))
	}

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_TimerResetBehavior(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := batcher.Process(ctx, in)

	// Send first batch
	in <- NewSuccess(1)
	in <- NewSuccess(2)

	// Advance time to trigger first batch
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady() // First batch timer

	// Should receive first batch
	result1 := <-out
	if result1.IsError() {
		t.Errorf("unexpected error: %v", result1.Error())
	}
	batch1 := result1.Value()
	if len(batch1) != 2 {
		t.Errorf("expected first batch size 2, got %d", len(batch1))
	}

	// Send second batch
	in <- NewSuccess(3)
	in <- NewSuccess(4)

	// Timer should be reset for new batch
	// Advance less than timeout duration
	clock.Advance(50 * time.Millisecond)

	// Should not receive second batch yet
	select {
	case result := <-out:
		t.Errorf("unexpected early result: %v", result)
	default:
		// Good, timer still running
	}

	// Advance remaining time to trigger second batch
	clock.Advance(50 * time.Millisecond)
	clock.BlockUntilReady() // Second batch timer

	// Should receive second batch
	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	batch2 := result2.Value()
	if len(batch2) != 2 {
		t.Errorf("expected second batch size 2, got %d", len(batch2))
	}

	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestBatcher_SizeAndTimeInteraction(t *testing.T) {
	clock := clockz.NewFakeClock()
	batcher := NewBatcher[int](BatchConfig{
		MaxSize:    3,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := batcher.Process(ctx, in)

	// Send items that reach size limit before timeout
	in <- NewSuccess(1)
	in <- NewSuccess(2)
	in <- NewSuccess(3) // Should trigger size-based batch

	// Should receive batch immediately due to size limit
	result1 := <-out
	if result1.IsError() {
		t.Errorf("unexpected error: %v", result1.Error())
	}
	batch1 := result1.Value()
	if len(batch1) != 3 {
		t.Errorf("expected batch size 3, got %d", len(batch1))
	}

	// Send partial batch
	in <- NewSuccess(4)
	in <- NewSuccess(5)

	// Should not receive anything immediately
	select {
	case result := <-out:
		t.Errorf("unexpected immediate result: %v", result)
	default:
		// Good, waiting for timer or size limit
	}

	// Advance time to trigger time-based batch
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady() // Ensure timer processed

	// Should receive second batch via timeout
	result2 := <-out
	if result2.IsError() {
		t.Errorf("unexpected error: %v", result2.Error())
	}
	batch2 := result2.Value()
	if len(batch2) != 2 {
		t.Errorf("expected batch size 2, got %d", len(batch2))
	}

	close(in)

	// Channel should be closed
	_, ok := <-out
	if ok {
		t.Error("expected channel to be closed")
	}
}

// Benchmark tests for performance validation

// BenchmarkBatcher_SizeBasedBatching tests size-based batching performance.
func BenchmarkBatcher_SizeBasedBatching(b *testing.B) {
	batcher := NewBatcher[int](BatchConfig{MaxSize: 100}, RealClock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 100)

		// Fill channel with 100 items
		for j := 0; j < 100; j++ {
			in <- NewSuccess(j)
		}
		close(in)

		out := batcher.Process(ctx, in)
		<-out // Wait for batch
	}
}

// BenchmarkBatcher_TimeBasedBatching tests time-based batching performance.
func BenchmarkBatcher_TimeBasedBatching(b *testing.B) {
	batcher := NewBatcher[int](BatchConfig{
		MaxSize:    1000,
		MaxLatency: time.Nanosecond, // Very short duration for benchmark
	}, RealClock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 1)
		in <- NewSuccess(i)
		close(in)

		out := batcher.Process(ctx, in)
		<-out // Wait for batch
	}
}

// BenchmarkBatcher_ErrorPassthrough tests error handling performance.
func BenchmarkBatcher_ErrorPassthrough(b *testing.B) {
	batcher := NewBatcher[int](BatchConfig{MaxSize: 100}, RealClock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 1)
		in <- NewError(i, errors.New("test error"), "test-processor")
		close(in)

		out := batcher.Process(ctx, in)
		<-out // Wait for error result
	}
}

// BenchmarkBatcher_MixedItems tests performance with mixed success/error items.
func BenchmarkBatcher_MixedItems(b *testing.B) {
	batcher := NewBatcher[int](BatchConfig{MaxSize: 10}, RealClock)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in := make(chan Result[int], 20)

		// Mix of success and error items
		for j := 0; j < 20; j++ {
			if j%3 == 0 {
				in <- NewError(j, errors.New("test error"), "test-processor")
			} else {
				in <- NewSuccess(j)
			}
		}
		close(in)

		out := batcher.Process(ctx, in)

		// Consume all results
		//nolint:revive // empty-block: intentional channel draining
		for range out {
		}
	}
}
