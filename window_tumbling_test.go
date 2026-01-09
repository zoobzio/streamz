package streamz

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

func TestTumblingWindow_BasicOperation(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	window := NewTumblingWindow[int](100*time.Millisecond, clock)

	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	output := window.Process(ctx, input)

	// Advance clock to trigger window emission
	clock.Advance(150 * time.Millisecond)

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Verify window metadata is attached
	for i, r := range results {
		if !r.IsSuccess() {
			t.Errorf("result %d: expected success", i)
			continue
		}
		meta, err := GetWindowMetadata(r)
		if err != nil {
			t.Errorf("result %d: expected window metadata: %v", i, err)
			continue
		}
		if meta.Type != "tumbling" {
			t.Errorf("result %d: expected type 'tumbling', got %q", i, meta.Type)
		}
	}
}

func TestTumblingWindow_ErrorPassthrough(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	window := NewTumblingWindow[int](100*time.Millisecond, clock)

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

func TestTumblingWindow_MultipleWindows(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	window := NewTumblingWindow[int](50*time.Millisecond, clock)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	go func() {
		// First window
		input <- NewSuccess(1)
		input <- NewSuccess(2)

		clock.Advance(60 * time.Millisecond)
		time.Sleep(10 * time.Millisecond) // Allow processing

		// Second window
		input <- NewSuccess(3)
		input <- NewSuccess(4)

		clock.Advance(60 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)

		close(input)
	}()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	if len(results) != 4 {
		t.Errorf("expected 4 results across windows, got %d", len(results))
	}
}

func TestTumblingWindow_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clock := clockz.NewFakeClock()

	window := NewTumblingWindow[int](100*time.Millisecond, clock)

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

func TestTumblingWindow_EmptyWindow(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	window := NewTumblingWindow[int](50*time.Millisecond, clock)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	go func() {
		// Advance clock without sending any items
		clock.Advance(60 * time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		close(input)
	}()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	// Empty windows produce no output
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty window, got %d", len(results))
	}
}

func TestTumblingWindow_WithName(t *testing.T) {
	clock := clockz.NewFakeClock()
	window := NewTumblingWindow[int](100*time.Millisecond, clock).WithName("custom-tumbling")

	if window.Name() != "custom-tumbling" {
		t.Errorf("expected name 'custom-tumbling', got %q", window.Name())
	}
}

func TestTumblingWindow_WindowMetadataFields(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()
	windowSize := 100 * time.Millisecond

	window := NewTumblingWindow[int](windowSize, clock)

	input := make(chan Result[int], 1)
	input <- NewSuccess(42)
	close(input)

	output := window.Process(ctx, input)
	clock.Advance(150 * time.Millisecond)

	result := <-output
	meta, err := GetWindowMetadata(result)
	if err != nil {
		t.Fatalf("expected window metadata: %v", err)
	}

	if meta.Type != "tumbling" {
		t.Errorf("expected type 'tumbling', got %q", meta.Type)
	}
	if meta.Size != windowSize {
		t.Errorf("expected size %v, got %v", windowSize, meta.Size)
	}
	if meta.End.Sub(meta.Start) != windowSize {
		t.Errorf("expected window duration %v, got %v", windowSize, meta.End.Sub(meta.Start))
	}
}
