package streamz

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestBuffer_Basic(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
		items      []Result[string]
	}{
		{
			name:       "unbuffered channel",
			bufferSize: 0,
			items: []Result[string]{
				NewSuccess("item1"),
				NewSuccess("item2"),
			},
		},
		{
			name:       "small buffer",
			bufferSize: 1,
			items: []Result[string]{
				NewSuccess("data1"),
				NewError("bad", fmt.Errorf("error1"), "processor1"),
				NewSuccess("data2"),
			},
		},
		{
			name:       "large buffer",
			bufferSize: 100,
			items: []Result[string]{
				NewSuccess("a"),
				NewError("b", fmt.Errorf("error"), "proc"),
				NewSuccess("c"),
				NewError("d", fmt.Errorf("another"), "proc2"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			buffer := NewBuffer[string](tt.bufferSize)

			// Create input channel
			input := make(chan Result[string], len(tt.items))
			for _, item := range tt.items {
				input <- item
			}
			close(input)

			// Process through buffer
			output := buffer.Process(ctx, input)

			// Collect results
			var results []Result[string]
			for result := range output {
				results = append(results, result)
			}

			// Verify all items passed through unchanged
			if len(results) != len(tt.items) {
				t.Fatalf("expected %d results, got %d", len(tt.items), len(results))
			}

			for i, expected := range tt.items {
				actual := results[i]

				if expected.IsError() != actual.IsError() {
					t.Errorf("item %d: expected error=%v, got error=%v",
						i, expected.IsError(), actual.IsError())
					continue
				}

				if expected.IsError() {
					if expected.Error().Item != actual.Error().Item {
						t.Errorf("item %d: expected error item %v, got %v",
							i, expected.Error().Item, actual.Error().Item)
					}
					if expected.Error().ProcessorName != actual.Error().ProcessorName {
						t.Errorf("item %d: expected processor name %s, got %s",
							i, expected.Error().ProcessorName, actual.Error().ProcessorName)
					}
				} else if expected.Value() != actual.Value() {
					t.Errorf("item %d: expected value %v, got %v",
						i, expected.Value(), actual.Value())
				}
			}
		})
	}
}

func TestBuffer_ContextCancellation(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
	}{
		{"unbuffered", 0},
		{"small buffer", 5},
		{"large buffer", 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			buffer := NewBuffer[int](tt.bufferSize)

			// Create a blocking input channel
			input := make(chan Result[int])

			// Start processing
			output := buffer.Process(ctx, input)

			// Send one item to ensure goroutine is running
			input <- NewSuccess(1)

			// Cancel context after short delay
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()

			// Collect results with timeout
			results := collectResultsWithTimeout(output, 100*time.Millisecond)

			// Should get the one item we sent
			if len(results) != 1 {
				t.Errorf("expected 1 result, got %d", len(results))
			}

			if len(results) > 0 && (!results[0].IsSuccess() || results[0].Value() != 1) {
				t.Errorf("expected success result with value 1, got %+v", results[0])
			}

			// Close input to clean up
			close(input)
		})
	}
}

func TestBuffer_ContextCancellationDuringSend(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
	}{
		{"unbuffered", 0},
		{"buffered", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			buffer := NewBuffer[int](tt.bufferSize)

			// Create input channel
			input := make(chan Result[int])

			// Start processing - but don't read from output to force blocking
			output := buffer.Process(ctx, input)

			// Fill the buffer completely (if buffered) plus one more to block
			itemsToSend := tt.bufferSize + 1
			// Send items to fill buffer
			for i := 0; i < itemsToSend; i++ {
				input <- NewSuccess(i)
			}

			// Give some time for the goroutine to reach the blocking send
			time.Sleep(10 * time.Millisecond)

			// At this point, the buffer goroutine should be blocked on the select
			// trying to send to a full output channel (no consumer)

			// Cancel the context while it's blocked
			cancel()

			// The goroutine should exit due to context cancellation
			// and close the output channel

			// Collect results with timeout - we should only get the buffer size number of items
			results := collectResultsWithTimeout(output, 100*time.Millisecond)

			// We should get exactly bufferSize items (or 0 for unbuffered)
			expectedResults := tt.bufferSize
			if len(results) != expectedResults {
				t.Errorf("expected %d results, got %d", expectedResults, len(results))
			}

			// Verify the items we got are correct
			for i, result := range results {
				if !result.IsSuccess() || result.Value() != i {
					t.Errorf("result %d: expected success %d, got %+v", i, i, result)
				}
			}

			// Close input to clean up
			close(input)
		})
	}
}

func TestBuffer_ContextCancellationWhileBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	buffer := NewBuffer[int](0) // Unbuffered - forces immediate blocking

	// Create input channel
	input := make(chan Result[int], 10) // Buffered input so we can send multiple items
	// Fill input with items
	for i := 0; i < 5; i++ {
		input <- NewSuccess(i)
	}

	// Start processing but DON'T consume output - this will block the goroutine
	output := buffer.Process(ctx, input)
	// Give a moment for goroutine to start and hit the blocking send
	time.Sleep(10 * time.Millisecond)
	// Cancel context while the goroutine is blocked trying to send
	cancel()
	// Now consume output with timeout - should be closed due to context cancellation
	results := collectResultsWithTimeout(output, 50*time.Millisecond)
	// With unbuffered channel, we might get 0 or 1 items depending on timing
	// The key is that the channel should be closed due to context cancellation
	if len(results) > 1 {
		t.Errorf("expected 0-1 results from unbuffered channel, got %d", len(results))
	}
	// Verify output channel is properly closed
	select {
	case _, ok := <-output:
		if ok {
			t.Error("expected output channel to be closed after context cancellation")
		}
	default:
		t.Error("output channel should be closed and readable")
	}
	// Close input
	close(input)
}

func TestBuffer_ProducerConsumerDecoupling(t *testing.T) {
	ctx := context.Background()
	buffer := NewBuffer[int](10) // Buffer should decouple producer from consumer

	input := make(chan Result[int], 15) // Input larger than buffer

	// Fill input with more items than buffer size
	for i := 0; i < 15; i++ {
		if i%3 == 0 {
			input <- NewError(i, fmt.Errorf("error %d", i), "producer")
		} else {
			input <- NewSuccess(i)
		}
	}
	close(input)

	output := buffer.Process(ctx, input)

	// Track timing to verify decoupling
	var producerDone, firstItemReceived time.Time

	// Producer should finish quickly due to buffering
	producerDone = time.Now()

	// Consume slowly to test decoupling
	results := make([]Result[int], 0, 10) // Pre-allocate with expected capacity
	for result := range output {
		if len(results) == 0 {
			firstItemReceived = time.Now()
		}
		results = append(results, result)
		time.Sleep(1 * time.Millisecond) // Slow consumer
	}

	// Verify all items received
	if len(results) != 15 {
		t.Fatalf("expected 15 results, got %d", len(results))
	}

	// Verify content correctness
	successCount, errorCount := 0, 0
	for i, result := range results {
		if i%3 == 0 {
			// Should be error
			if !result.IsError() {
				t.Errorf("item %d: expected error, got success", i)
			} else {
				errorCount++
				if result.Error().Item != i {
					t.Errorf("item %d: expected error item %d, got %d",
						i, i, result.Error().Item)
				}
			}
		} else {
			// Should be success
			if !result.IsSuccess() {
				t.Errorf("item %d: expected success, got error", i)
			} else {
				successCount++
				if result.Value() != i {
					t.Errorf("item %d: expected value %d, got %d",
						i, i, result.Value())
				}
			}
		}
	}

	if successCount != 10 { // 15 - 5 errors (every 3rd item)
		t.Errorf("expected 10 successful items, got %d", successCount)
	}
	if errorCount != 5 {
		t.Errorf("expected 5 error items, got %d", errorCount)
	}

	// Verify timing shows decoupling (producer finished before slow consumption)
	if firstItemReceived.Before(producerDone) {
		t.Log("Buffer successfully decoupled producer from consumer")
	}
}

func TestBuffer_ChannelClosure(t *testing.T) {
	ctx := context.Background()
	buffer := NewBuffer[string](5)

	// Test that output channel closes when input closes
	input := make(chan Result[string], 2)
	input <- NewSuccess("test1")
	input <- NewError("test2", fmt.Errorf("test error"), "tester")
	close(input)

	output := buffer.Process(ctx, input)

	// Collect all results
	results := make([]Result[string], 0, 2) // Pre-allocate with expected capacity
	for result := range output {
		results = append(results, result)
	}

	// Verify we got both items
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Verify first is success
	if !results[0].IsSuccess() || results[0].Value() != "test1" {
		t.Errorf("first result: expected success 'test1', got %+v", results[0])
	}

	// Verify second is error
	if !results[1].IsError() || results[1].Error().Item != "test2" {
		t.Errorf("second result: expected error with item 'test2', got %+v", results[1])
	}

	// Try to receive from closed channel - should not block
	select {
	case _, ok := <-output:
		if ok {
			t.Error("expected output channel to be closed")
		}
	default:
		t.Error("output channel should be closed and readable")
	}
}

func TestBuffer_NoGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	for i := 0; i < 100; i++ {
		ctx := context.Background()
		buffer := NewBuffer[int](10)

		input := make(chan Result[int], 1)
		input <- NewSuccess(i)
		close(input)

		output := buffer.Process(ctx, input)

		// Consume all results
		//nolint:revive // empty-block: intentional channel draining
		for range output {
			// Drain channel completely
		}
	}

	// Give time for goroutines to clean up
	time.Sleep(10 * time.Millisecond)
	runtime.GC()
	runtime.GC() // Double GC to ensure cleanup

	finalGoroutines := runtime.NumGoroutine()

	// Allow some leeway for test infrastructure
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("potential goroutine leak: started with %d, ended with %d",
			initialGoroutines, finalGoroutines)
	}
}

func TestBuffer_Name(t *testing.T) {
	buffer := NewBuffer[string](42)
	if buffer.Name() != "buffer" {
		t.Errorf("expected name 'buffer', got %s", buffer.Name())
	}
}

func TestBuffer_MixedResultTypes(t *testing.T) {
	ctx := context.Background()
	buffer := NewBuffer[interface{}](5)

	// Create input with mixed Result types
	input := make(chan Result[interface{}], 6)
	input <- NewSuccess[interface{}]("string")
	input <- NewSuccess[interface{}](123)
	input <- NewError[interface{}]("failed string", fmt.Errorf("string error"), "str-proc")
	input <- NewSuccess[interface{}](true)
	input <- NewError[interface{}](456, fmt.Errorf("int error"), "int-proc")
	input <- NewSuccess[interface{}](3.14)
	close(input)

	output := buffer.Process(ctx, input)

	// Collect results
	results := make([]Result[interface{}], 0, 6) // Pre-allocate with expected capacity
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(results))
	}

	// Verify each result type and content
	expectedResults := []struct {
		isError bool
		value   interface{}
		errItem interface{}
		errProc string
	}{
		{false, "string", nil, ""},
		{false, 123, nil, ""},
		{true, nil, "failed string", "str-proc"},
		{false, true, nil, ""},
		{true, nil, 456, "int-proc"},
		{false, 3.14, nil, ""},
	}

	for i, expected := range expectedResults {
		actual := results[i]

		if expected.isError != actual.IsError() {
			t.Errorf("result %d: expected error=%v, got error=%v",
				i, expected.isError, actual.IsError())
			continue
		}

		if expected.isError {
			if actual.Error().Item != expected.errItem {
				t.Errorf("result %d: expected error item %v, got %v",
					i, expected.errItem, actual.Error().Item)
			}
			if actual.Error().ProcessorName != expected.errProc {
				t.Errorf("result %d: expected processor name %s, got %s",
					i, expected.errProc, actual.Error().ProcessorName)
			}
		} else if actual.Value() != expected.value {
			t.Errorf("result %d: expected value %v, got %v",
				i, expected.value, actual.Value())
		}
	}
}

func TestBuffer_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	buffer := NewBuffer[int](20)

	// Create input channel
	input := make(chan Result[int], 100)

	// Start multiple producers
	var producerWG sync.WaitGroup
	numProducers := 5
	itemsPerProducer := 20

	for p := 0; p < numProducers; p++ {
		producerWG.Add(1)
		go func(producerID int) {
			defer producerWG.Done()
			for i := 0; i < itemsPerProducer; i++ {
				value := producerID*1000 + i
				if i%4 == 0 {
					input <- NewError(value, fmt.Errorf("error from producer %d", producerID),
						fmt.Sprintf("producer-%d", producerID))
				} else {
					input <- NewSuccess(value)
				}
			}
		}(p)
	}

	// Close input when all producers are done
	go func() {
		producerWG.Wait()
		close(input)
	}()

	// Process through buffer
	output := buffer.Process(ctx, input)

	// Collect results
	results := make([]Result[int], 0, 5) // Pre-allocate with expected capacity
	for result := range output {
		results = append(results, result)
	}

	expectedTotal := numProducers * itemsPerProducer
	if len(results) != expectedTotal {
		t.Fatalf("expected %d results, got %d", expectedTotal, len(results))
	}

	// Verify result distribution
	successCount, errorCount := 0, 0
	valueMap := make(map[int]bool)
	errorMap := make(map[int]bool)

	for _, result := range results {
		if result.IsError() {
			errorCount++
			item := result.Error().Item
			if errorMap[item] {
				t.Errorf("duplicate error item: %d", item)
			}
			errorMap[item] = true
		} else {
			successCount++
			value := result.Value()
			if valueMap[value] {
				t.Errorf("duplicate success value: %d", value)
			}
			valueMap[value] = true
		}
	}

	// Every 4th item should be error, others success
	expectedErrors := numProducers * (itemsPerProducer / 4)
	expectedSuccesses := expectedTotal - expectedErrors

	if errorCount != expectedErrors {
		t.Errorf("expected %d errors, got %d", expectedErrors, errorCount)
	}
	if successCount != expectedSuccesses {
		t.Errorf("expected %d successes, got %d", expectedSuccesses, successCount)
	}
}

// collectResultsWithTimeout collects all results from a channel with a timeout.
func collectResultsWithTimeout[T any](ch <-chan Result[T], timeout time.Duration) []Result[T] {
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
