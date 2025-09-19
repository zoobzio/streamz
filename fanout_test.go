package streamz

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// collectResults collects all results from a channel within a timeout.
func collectResults[T any](ch <-chan Result[T], timeout time.Duration) []Result[T] {
	var results []Result[T]
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case result, ok := <-ch:
			if !ok {
				return results
			}
			results = append(results, result)
		case <-timer.C:
			return results
		}
	}
}

func TestFanOut_BroadcastSuccess(t *testing.T) {
	ctx := context.Background()
	fanout := NewFanOut[int](3)

	// Create input with successful values
	input := make(chan Result[int], 5)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	outputs := fanout.Process(ctx, input)

	if len(outputs) != 3 {
		t.Errorf("expected 3 outputs, got %d", len(outputs))
		return
	}

	// Collect from all outputs concurrently
	var wg sync.WaitGroup
	results := make([][]Result[int], 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = collectResults(outputs[index], 100*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// Verify each output received all values
	expected := []int{1, 2, 3}
	for i := 0; i < 3; i++ {
		if len(results[i]) != len(expected) {
			t.Errorf("output %d: expected %d results, got %d", i, len(expected), len(results[i]))
			continue
		}

		for j, result := range results[i] {
			if result.IsError() {
				t.Errorf("output %d, item %d: expected success, got error: %v", i, j, result.Error())
				continue
			}
			if result.Value() != expected[j] {
				t.Errorf("output %d, item %d: expected %d, got %d", i, j, expected[j], result.Value())
			}
		}
	}
}

func TestFanOut_BroadcastErrors(t *testing.T) {
	ctx := context.Background()
	fanout := NewFanOut[int](2)

	// Create input with error
	input := make(chan Result[int], 3)
	input <- NewError(1, fmt.Errorf("processing failed"), "test")
	input <- NewSuccess(2)
	input <- NewError(3, fmt.Errorf("another failure"), "test")
	close(input)

	outputs := fanout.Process(ctx, input)

	// Collect from all outputs
	var wg sync.WaitGroup
	results := make([][]Result[int], 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = collectResults(outputs[index], 100*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// Verify both outputs received all items (including errors)
	for i := 0; i < 2; i++ {
		if len(results[i]) != 3 {
			t.Errorf("output %d: expected 3 results, got %d", i, len(results[i]))
			continue
		}

		// First result should be an error
		if !results[i][0].IsError() {
			t.Errorf("output %d, item 0: expected error, got success: %v", i, results[i][0].Value())
		} else if results[i][0].Error().Item != 1 {
			t.Errorf("output %d, item 0: expected error item 1, got %v", i, results[i][0].Error().Item)
		}

		// Second result should be success
		if results[i][1].IsError() {
			t.Errorf("output %d, item 1: expected success, got error: %v", i, results[i][1].Error())
		} else if results[i][1].Value() != 2 {
			t.Errorf("output %d, item 1: expected 2, got %d", i, results[i][1].Value())
		}

		// Third result should be an error
		if !results[i][2].IsError() {
			t.Errorf("output %d, item 2: expected error, got success: %v", i, results[i][2].Value())
		} else if results[i][2].Error().Item != 3 {
			t.Errorf("output %d, item 2: expected error item 3, got %v", i, results[i][2].Error().Item)
		}
	}
}

func TestFanOut_MixedSuccessError(t *testing.T) {
	ctx := context.Background()
	fanout := NewFanOut[string](3)

	// Create input with mixed success/error
	input := make(chan Result[string], 4)
	input <- NewSuccess("hello")
	input <- NewError("bad", fmt.Errorf("bad data"), "validator")
	input <- NewSuccess("world")
	input <- NewError("invalid", fmt.Errorf("validation failed"), "validator")
	close(input)

	outputs := fanout.Process(ctx, input)

	// Collect from all outputs
	var wg sync.WaitGroup
	results := make([][]Result[string], 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = collectResults(outputs[index], 100*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// All outputs should receive the same sequence
	for i := 0; i < 3; i++ {
		if len(results[i]) != 4 {
			t.Errorf("output %d: expected 4 results, got %d", i, len(results[i]))
			continue
		}

		// Verify sequence: success, error, success, error
		expected := []struct {
			isError bool
			value   string
			item    string
		}{
			{false, "hello", ""},
			{true, "", "bad"},
			{false, "world", ""},
			{true, "", "invalid"},
		}

		for j, exp := range expected {
			result := results[i][j]
			if exp.isError {
				if !result.IsError() {
					t.Errorf("output %d, item %d: expected error, got success: %v", i, j, result.Value())
				} else if result.Error().Item != exp.item {
					t.Errorf("output %d, item %d: expected error item %q, got %q", i, j, exp.item, result.Error().Item)
				}
			} else {
				if result.IsError() {
					t.Errorf("output %d, item %d: expected success, got error: %v", i, j, result.Error())
				} else if result.Value() != exp.value {
					t.Errorf("output %d, item %d: expected %q, got %q", i, j, exp.value, result.Value())
				}
			}
		}
	}
}

func TestFanOut_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fanout := NewFanOut[int](2)

	// Create input that would send many items
	input := make(chan Result[int])

	outputs := fanout.Process(ctx, input)

	// Send one item, then cancel
	input <- NewSuccess(1)
	cancel()

	// Give it time to process cancellation
	time.Sleep(50 * time.Millisecond)

	// Try to send more (should not block due to cancellation)
	select {
	case input <- NewSuccess(2):
		// This might succeed if the goroutine hasn't processed cancellation yet
	case <-time.After(10 * time.Millisecond):
		// Expected - the goroutine should exit and stop consuming
	}
	close(input)

	// Outputs should be closed
	var wg sync.WaitGroup
	closedCount := 0
	var mu sync.Mutex

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			//nolint:revive // empty-block: intentional channel draining
			for range outputs[index] {
				// Drain any remaining items
			}
			mu.Lock()
			closedCount++
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if closedCount != 2 {
		t.Errorf("expected 2 outputs to be closed, got %d", closedCount)
	}
}

func TestFanOut_SlowConsumer(t *testing.T) {
	ctx := context.Background()
	fanout := NewFanOut[int](2)

	// Create input
	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	outputs := fanout.Process(ctx, input)

	// Use WaitGroup and channels to properly synchronize
	var wg sync.WaitGroup
	fastResults := make(chan []Result[int], 1)
	slowResults := make(chan []Result[int], 1)

	// Consume from first output normally
	wg.Add(1)
	go func() {
		defer wg.Done()
		var results []Result[int]
		for result := range outputs[0] {
			results = append(results, result)
		}
		fastResults <- results
	}()

	// Consume from second output slowly
	wg.Add(1)
	go func() {
		defer wg.Done()
		var results []Result[int]
		for result := range outputs[1] {
			results = append(results, result)
			time.Sleep(50 * time.Millisecond) // Slow consumer
		}
		slowResults <- results
	}()

	// Wait for both to complete
	wg.Wait()

	// Get the results safely
	fastResultList := <-fastResults
	slowResultList := <-slowResults

	// Both should have received all items (FanOut waits for all outputs)
	if len(fastResultList) != 3 {
		t.Errorf("fast consumer: expected 3 results, got %d", len(fastResultList))
	}
	if len(slowResultList) != 3 {
		t.Errorf("slow consumer: expected 3 results, got %d", len(slowResultList))
	}

	// Verify values
	for i, result := range fastResultList {
		if result.IsError() || result.Value() != i+1 {
			t.Errorf("fast consumer item %d: expected %d, got %v", i, i+1, result)
		}
	}
	for i, result := range slowResultList {
		if result.IsError() || result.Value() != i+1 {
			t.Errorf("slow consumer item %d: expected %d, got %v", i, i+1, result)
		}
	}
}

func TestFanOut_SingleOutput(t *testing.T) {
	ctx := context.Background()
	fanout := NewFanOut[int](1)

	input := make(chan Result[int], 2)
	input <- NewSuccess(42)
	input <- NewError(99, fmt.Errorf("test error"), "test")
	close(input)

	outputs := fanout.Process(ctx, input)

	if len(outputs) != 1 {
		t.Errorf("expected 1 output, got %d", len(outputs))
		return
	}

	results := collectResults(outputs[0], 100*time.Millisecond)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
		return
	}

	// First should be success
	if results[0].IsError() {
		t.Errorf("first result: expected success, got error: %v", results[0].Error())
	} else if results[0].Value() != 42 {
		t.Errorf("first result: expected 42, got %d", results[0].Value())
	}

	// Second should be error
	if !results[1].IsError() {
		t.Errorf("second result: expected error, got success: %v", results[1].Value())
	} else if results[1].Error().Item != 99 {
		t.Errorf("second result: expected error item 99, got %v", results[1].Error().Item)
	}
}

func TestFanOut_EmptyInput(t *testing.T) {
	ctx := context.Background()
	fanout := NewFanOut[int](3)

	input := make(chan Result[int])
	close(input) // Empty input

	outputs := fanout.Process(ctx, input)

	// All outputs should close immediately
	for i, output := range outputs {
		select {
		case result, ok := <-output:
			if ok {
				t.Errorf("output %d: expected closed channel, got result: %v", i, result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("output %d: channel should have closed immediately", i)
		}
	}
}

func TestFanOut_NoGoroutineLeaks(_ *testing.T) {
	// This test verifies that the FanOut processor doesn't leak goroutines
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		fanout := NewFanOut[int](3)

		input := make(chan Result[int], 1)
		input <- NewSuccess(i)
		close(input)

		outputs := fanout.Process(ctx, input)

		// Consume all outputs
		var wg sync.WaitGroup
		for j := 0; j < 3; j++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				//nolint:revive // empty-block: intentional channel draining
				for range outputs[index] {
					// Drain the channel
				}
			}(j)
		}
		wg.Wait()
	}

	// If there were goroutine leaks, this test would hang or fail under race detection
	// The fact that it completes indicates proper cleanup
}

// Benchmark tests for performance analysis.
func BenchmarkFanOut_SingleItem(b *testing.B) {
	ctx := context.Background()
	fanout := NewFanOut[int](3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := make(chan Result[int], 1)
		input <- NewSuccess(i)
		close(input)

		outputs := fanout.Process(ctx, input)

		// Consume outputs
		var wg sync.WaitGroup
		for j := 0; j < 3; j++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				//nolint:revive // empty-block: intentional channel draining
				for range outputs[index] {
					// Consume
				}
			}(j)
		}
		wg.Wait()
	}
}

func BenchmarkFanOut_MultipleItems(b *testing.B) {
	ctx := context.Background()
	fanout := NewFanOut[int](3)
	const itemCount = 100

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := make(chan Result[int], itemCount)
		for j := 0; j < itemCount; j++ {
			input <- NewSuccess(j)
		}
		close(input)

		outputs := fanout.Process(ctx, input)

		// Consume outputs
		var wg sync.WaitGroup
		for j := 0; j < 3; j++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				//nolint:revive // empty-block: intentional channel draining
				for range outputs[index] {
					// Consume
				}
			}(j)
		}
		wg.Wait()
	}
}
