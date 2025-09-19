package streamz

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewTap(t *testing.T) {
	var called bool
	tap := NewTap(func(_ Result[int]) {
		called = true
	})

	if tap.name != "tap" {
		t.Errorf("Expected default name 'tap', got %s", tap.name)
	}

	if tap.fn == nil {
		t.Fatal("Expected function to be set")
	}

	// Test function assignment
	tap.fn(NewSuccess(42))
	if !called {
		t.Error("Expected function to be called")
	}
}

func TestTap_WithName(t *testing.T) {
	tap := NewTap(func(_ Result[int]) {}).WithName("custom-tap")

	if tap.name != "custom-tap" {
		t.Errorf("Expected name 'custom-tap', got %s", tap.name)
	}

	if tap.Name() != "custom-tap" {
		t.Errorf("Expected Name() to return 'custom-tap', got %s", tap.Name())
	}
}

func TestTap_Name(t *testing.T) {
	tap := NewTap(func(_ Result[int]) {})

	if tap.Name() != "tap" {
		t.Errorf("Expected Name() to return 'tap', got %s", tap.Name())
	}
}

func TestTap_Process_Success(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 3)

	var observed []Result[int]
	tap := NewTap(func(result Result[int]) {
		observed = append(observed, result)
	})

	// Send test data
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	// Process through tap
	output := tap.Process(ctx, input)

	// Collect results
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	// Verify all items passed through unchanged
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	expectedValues := []int{1, 2, 3}
	for i, result := range results {
		if result.IsError() {
			t.Errorf("Expected success result at index %d, got error: %v", i, result.Error())
			continue
		}
		if result.Value() != expectedValues[i] {
			t.Errorf("Expected value %d at index %d, got %d", expectedValues[i], i, result.Value())
		}
	}

	// Verify side effect was called for all items
	if len(observed) != 3 {
		t.Fatalf("Expected side effect called 3 times, got %d", len(observed))
	}

	for i, obs := range observed {
		if obs.IsError() {
			t.Errorf("Expected success in observed at index %d, got error: %v", i, obs.Error())
			continue
		}
		if obs.Value() != expectedValues[i] {
			t.Errorf("Expected observed value %d at index %d, got %d", expectedValues[i], i, obs.Value())
		}
	}
}

func TestTap_Process_Errors(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 2)

	var observed []Result[int]
	tap := NewTap(func(result Result[int]) {
		observed = append(observed, result)
	})

	// Send error data
	testErr := errors.New("test error")
	input <- NewError(42, testErr, "test-processor")
	input <- NewError(99, testErr, "another-processor")
	close(input)

	// Process through tap
	output := tap.Process(ctx, input)

	// Collect results
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	// Verify all items passed through unchanged
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	for i, result := range results {
		if !result.IsError() {
			t.Errorf("Expected error result at index %d, got success: %v", i, result.Value())
			continue
		}
		if result.Error().Err.Error() != "test error" {
			t.Errorf("Expected error 'test error' at index %d, got %v", i, result.Error().Err)
		}
	}

	// Verify side effect was called for all items including errors
	if len(observed) != 2 {
		t.Fatalf("Expected side effect called 2 times, got %d", len(observed))
	}

	for i, obs := range observed {
		if !obs.IsError() {
			t.Errorf("Expected error in observed at index %d, got success: %v", i, obs.Value())
			continue
		}
		if obs.Error().Err.Error() != "test error" {
			t.Errorf("Expected observed error 'test error' at index %d, got %v", i, obs.Error().Err)
		}
	}
}

func TestTap_Process_Mixed(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[string], 4)

	var observed []Result[string]
	tap := NewTap(func(result Result[string]) {
		observed = append(observed, result)
	})

	// Send mixed data
	input <- NewSuccess("hello")
	input <- NewError("bad", errors.New("error1"), "proc1")
	input <- NewSuccess("world")
	input <- NewError("fail", errors.New("error2"), "proc2")
	close(input)

	// Process through tap
	output := tap.Process(ctx, input)

	// Collect results
	results := make([]Result[string], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	// Verify all 4 items passed through
	if len(results) != 4 {
		t.Fatalf("Expected 4 results, got %d", len(results))
	}

	// Verify first item (success)
	if results[0].IsError() || results[0].Value() != "hello" {
		t.Errorf("Expected first result to be success 'hello', got %+v", results[0])
	}

	// Verify second item (error)
	if !results[1].IsError() || results[1].Error().Err.Error() != "error1" {
		t.Errorf("Expected second result to be error 'error1', got %+v", results[1])
	}

	// Verify third item (success)
	if results[2].IsError() || results[2].Value() != "world" {
		t.Errorf("Expected third result to be success 'world', got %+v", results[2])
	}

	// Verify fourth item (error)
	if !results[3].IsError() || results[3].Error().Err.Error() != "error2" {
		t.Errorf("Expected fourth result to be error 'error2', got %+v", results[3])
	}

	// Verify side effects called for all items
	if len(observed) != 4 {
		t.Fatalf("Expected side effect called 4 times, got %d", len(observed))
	}
}

func TestTap_Process_EmptyStream(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int])

	var callCount int
	tap := NewTap(func(_ Result[int]) {
		callCount++
	})

	// Close empty input immediately
	close(input)

	// Process through tap
	output := tap.Process(ctx, input)

	// Should receive no items
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 0 {
		t.Errorf("Expected no results from empty stream, got %d", len(results))
	}

	if callCount != 0 {
		t.Errorf("Expected no side effect calls for empty stream, got %d", callCount)
	}
}

func TestTap_Process_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input := make(chan Result[int])

	var observed []Result[int]
	tap := NewTap(func(result Result[int]) {
		observed = append(observed, result)
	})

	// Process through tap
	output := tap.Process(ctx, input)

	// Send one item
	input <- NewSuccess(1)

	// Read the first result
	result := <-output
	if result.IsError() || result.Value() != 1 {
		t.Errorf("Expected success result 1, got %+v", result)
	}

	// Cancel context
	cancel()

	// Send another item (should be ignored)
	go func() {
		time.Sleep(10 * time.Millisecond)
		input <- NewSuccess(2)
		close(input)
	}()

	// Try to read more - channel should close due to cancellation
	select {
	case result, ok := <-output:
		if ok {
			t.Errorf("Expected channel to be closed after context cancellation, got result: %+v", result)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected output channel to close quickly after context cancellation")
	}

	// Should have observed exactly one item
	if len(observed) != 1 {
		t.Errorf("Expected exactly 1 observed item, got %d", len(observed))
	}
}

func TestTap_Process_SideEffectPanic(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 3)

	// Create tap with panicking function
	tap := NewTap(func(result Result[int]) {
		if result.IsSuccess() && result.Value() == 2 {
			panic("test panic")
		}
	})

	// Send test data
	input <- NewSuccess(1)
	input <- NewSuccess(2) // This will cause panic in side effect
	input <- NewSuccess(3) // This should still be processed
	close(input)

	// Process should handle panic gracefully and continue
	output := tap.Process(ctx, input)

	// Collect all results - panic should be recovered and processing should continue
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	// Should get all results despite panic in side effect
	if len(results) != 3 {
		t.Errorf("Expected 3 results despite panic, got %d", len(results))
	}

	// Verify first result
	if len(results) > 0 && (results[0].IsError() || results[0].Value() != 1) {
		t.Errorf("Expected first result to be success 1, got %+v", results[0])
	}

	// Verify second result (the one that caused panic in side effect)
	if len(results) > 1 && (results[1].IsError() || results[1].Value() != 2) {
		t.Errorf("Expected second result to be success 2, got %+v", results[1])
	}

	// Verify third result (should still work after panic recovery)
	if len(results) > 2 && (results[2].IsError() || results[2].Value() != 3) {
		t.Errorf("Expected third result to be success 3, got %+v", results[2])
	}
}

func TestTap_Process_MetricsCollection(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 5)

	// Metrics collection example
	var successCount, errorCount atomic.Int64
	var totalValue atomic.Int64

	tap := NewTap(func(result Result[int]) {
		if result.IsError() {
			errorCount.Add(1)
		} else {
			successCount.Add(1)
			totalValue.Add(int64(result.Value()))
		}
	}).WithName("metrics-collector")

	// Send test data
	input <- NewSuccess(10)
	input <- NewSuccess(20)
	input <- NewError(0, errors.New("test error"), "test")
	input <- NewSuccess(30)
	input <- NewError(0, errors.New("another error"), "test")
	close(input)

	// Process through tap
	output := tap.Process(ctx, input)

	// Collect all results
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	// Verify metrics were collected correctly
	if successCount.Load() != 3 {
		t.Errorf("Expected 3 successful items, got %d", successCount.Load())
	}

	if errorCount.Load() != 2 {
		t.Errorf("Expected 2 error items, got %d", errorCount.Load())
	}

	if totalValue.Load() != 60 { // 10 + 20 + 30
		t.Errorf("Expected total value 60, got %d", totalValue.Load())
	}

	// Verify all results passed through unchanged
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	// Verify processor name
	if tap.Name() != "metrics-collector" {
		t.Errorf("Expected name 'metrics-collector', got %s", tap.Name())
	}
}

func TestTap_Process_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 100)

	// Thread-safe counter
	var count atomic.Int64

	tap := NewTap(func(_ Result[int]) {
		count.Add(1)
	})

	// Send many items
	go func() {
		for i := 0; i < 100; i++ {
			input <- NewSuccess(i)
		}
		close(input)
	}()

	// Process through tap
	output := tap.Process(ctx, input)

	// Count results
	resultCount := 0
	for range output {
		resultCount++
	}

	// Should have processed all 100 items
	if resultCount != 100 {
		t.Errorf("Expected 100 results, got %d", resultCount)
	}

	if count.Load() != 100 {
		t.Errorf("Expected side effect called 100 times, got %d", count.Load())
	}
}
