package streamz

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

func TestSlidingWindow_BasicOperation(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	// Tumbling mode (slide == size)
	window := NewSlidingWindow[int](100*time.Millisecond, clock)

	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	output := window.Process(ctx, input)
	clock.Advance(150 * time.Millisecond)

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestSlidingWindow_WithSlide(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	// Overlapping windows: 100ms window, 50ms slide
	window := NewSlidingWindow[int](100*time.Millisecond, clock).
		WithSlide(50 * time.Millisecond)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	go func() {
		input <- NewSuccess(1)
		clock.Advance(30 * time.Millisecond)
		time.Sleep(5 * time.Millisecond)

		input <- NewSuccess(2)
		clock.Advance(30 * time.Millisecond)
		time.Sleep(5 * time.Millisecond)

		input <- NewSuccess(3)
		clock.Advance(100 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)

		close(input)
	}()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	// With overlapping windows, items can appear in multiple windows
	if len(results) < 3 {
		t.Errorf("expected at least 3 results with overlapping windows, got %d", len(results))
	}
}

func TestSlidingWindow_ErrorPassthrough(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	window := NewSlidingWindow[int](100*time.Millisecond, clock)

	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewError(0, nil, "test")
	input <- NewSuccess(2)
	close(input)

	output := window.Process(ctx, input)
	clock.Advance(150 * time.Millisecond)

	var successCount, errorCount int
	for r := range output {
		if r.IsSuccess() {
			successCount++
		} else {
			errorCount++
		}
	}

	if successCount != 2 {
		t.Errorf("expected 2 successes, got %d", successCount)
	}
	if errorCount != 1 {
		t.Errorf("expected 1 error, got %d", errorCount)
	}
}

func TestSlidingWindow_TumblingModeOptimization(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	// When slide == size, uses optimized tumbling mode
	window := NewSlidingWindow[int](100*time.Millisecond, clock)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	go func() {
		// First window
		input <- NewSuccess(1)
		input <- NewSuccess(2)

		clock.Advance(110 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)

		// Second window
		input <- NewSuccess(3)

		clock.Advance(110 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)

		close(input)
	}()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results in tumbling mode, got %d", len(results))
	}
}

func TestSlidingWindow_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clock := clockz.NewFakeClock()

	window := NewSlidingWindow[int](100*time.Millisecond, clock)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	// Unbuffered sends block until processor reads - guarantees synchronization
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	cancel()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	// Should emit partial window on cancellation
	if len(results) != 2 {
		t.Errorf("expected 2 results on cancellation, got %d", len(results))
	}
}

func TestSlidingWindow_WithName(t *testing.T) {
	clock := clockz.NewFakeClock()
	window := NewSlidingWindow[int](100*time.Millisecond, clock).WithName("custom-sliding")

	if window.Name() != "custom-sliding" {
		t.Errorf("expected name 'custom-sliding', got %q", window.Name())
	}
}

func TestSlidingWindow_WindowMetadataFields(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()
	windowSize := 100 * time.Millisecond
	slideInterval := 50 * time.Millisecond

	window := NewSlidingWindow[int](windowSize, clock).WithSlide(slideInterval)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	// Unbuffered send synchronizes with goroutine
	input <- NewSuccess(42)
	close(input)

	result := <-output
	meta, err := GetWindowMetadata(result)
	if err != nil {
		t.Fatalf("expected window metadata: %v", err)
	}

	if meta.Type != "sliding" {
		t.Errorf("expected type 'sliding', got %q", meta.Type)
	}
	if meta.Size != windowSize {
		t.Errorf("expected size %v, got %v", windowSize, meta.Size)
	}
	if meta.Slide == nil || *meta.Slide != slideInterval {
		t.Errorf("expected slide %v, got %v", slideInterval, meta.Slide)
	}
}

func TestSlidingWindow_OverlappingItems(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	// 100ms window, 25ms slide = 4 overlapping windows at any time
	window := NewSlidingWindow[int](100*time.Millisecond, clock).
		WithSlide(25 * time.Millisecond)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	go func() {
		input <- NewSuccess(1)
		clock.Advance(50 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)

		input <- NewSuccess(2)
		clock.Advance(150 * time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		close(input)
	}()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	// Items should appear in multiple overlapping windows
	if len(results) < 2 {
		t.Errorf("expected at least 2 results with overlapping windows, got %d", len(results))
	}
}
