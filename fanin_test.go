package streamz

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFanIn_Result_BasicMerging(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[int]()

	// Create source channels with Result[T]
	ch1 := make(chan Result[int], 2)
	ch2 := make(chan Result[int], 2)
	ch3 := make(chan Result[int], 2)

	// Send successful Results to source channels
	ch1 <- NewSuccess(1)
	ch1 <- NewSuccess(2)
	close(ch1)

	ch2 <- NewSuccess(3)
	ch2 <- NewSuccess(4)
	close(ch2)

	ch3 <- NewSuccess(5)
	ch3 <- NewSuccess(6)
	close(ch3)

	// Process through FanIn
	out := fanin.Process(ctx, ch1, ch2, ch3)

	// Collect results
	var values []int
	var errorCount int

	for result := range out {
		if result.IsError() {
			errorCount++
		} else {
			values = append(values, result.Value())
		}
	}

	if errorCount != 0 {
		t.Errorf("Expected no errors, got %d", errorCount)
	}

	// Verify all items were merged (order doesn't matter)
	sort.Ints(values)
	expected := []int{1, 2, 3, 4, 5, 6}

	if len(values) != len(expected) {
		t.Fatalf("Expected %d items, got %d", len(expected), len(values))
	}

	for i, v := range expected {
		if values[i] != v {
			t.Errorf("Expected values[%d] = %d, got %d", i, v, values[i])
		}
	}
}

func TestFanIn_Result_MixedSuccessAndError(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[string]()

	// Create channels with mixed success and error Results
	ch1 := make(chan Result[string], 3)
	ch2 := make(chan Result[string], 3)

	// Channel 1: success, error, success
	ch1 <- NewSuccess("success1")
	ch1 <- NewError("failed1", errors.New("error1"), "test-processor")
	ch1 <- NewSuccess("success2")
	close(ch1)

	// Channel 2: error, success, error
	ch2 <- NewError("failed2", errors.New("error2"), "test-processor")
	ch2 <- NewSuccess("success3")
	ch2 <- NewError("failed3", errors.New("error3"), "test-processor")
	close(ch2)

	// Process through FanIn
	out := fanin.Process(ctx, ch1, ch2)

	// Collect results
	var values []string
	var errors []*StreamError[string]

	for result := range out {
		if result.IsError() {
			errors = append(errors, result.Error())
		} else {
			values = append(values, result.Value())
		}
	}

	// Verify successful values
	sort.Strings(values)
	expectedValues := []string{"success1", "success2", "success3"}
	sort.Strings(expectedValues)

	if len(values) != len(expectedValues) {
		t.Fatalf("Expected %d successful values, got %d", len(expectedValues), len(values))
	}

	for i, v := range expectedValues {
		if values[i] != v {
			t.Errorf("Expected values[%d] = %q, got %q", i, v, values[i])
		}
	}

	// Verify errors
	if len(errors) != 3 {
		t.Fatalf("Expected 3 errors, got %d", len(errors))
	}

	// Check that all error items are present
	errorItems := make([]string, len(errors))
	for i, err := range errors {
		errorItems[i] = err.Item
	}
	sort.Strings(errorItems)
	expectedErrorItems := []string{"failed1", "failed2", "failed3"}
	sort.Strings(expectedErrorItems)

	for i, item := range expectedErrorItems {
		if errorItems[i] != item {
			t.Errorf("Expected error item[%d] = %q, got %q", i, item, errorItems[i])
		}
	}

	// Verify all errors have correct processor name
	for _, err := range errors {
		if err.ProcessorName != "test-processor" {
			t.Errorf("Expected processor name 'test-processor', got %q", err.ProcessorName)
		}
	}
}

func TestFanIn_Result_EmptyInputs(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[string]()

	out := fanin.Process(ctx)

	// Should close immediately with no inputs
	var resultCount int
	for range out {
		resultCount++
	}

	if resultCount != 0 {
		t.Errorf("Expected no results from empty inputs, got %d items", resultCount)
	}
}

func TestFanIn_Result_SingleInput(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[string]()

	ch := make(chan Result[string], 4)
	ch <- NewSuccess("hello")
	ch <- NewError("failed", errors.New("test error"), "single-test")
	ch <- NewSuccess("world")
	ch <- NewSuccess("test")
	close(ch)

	out := fanin.Process(ctx, ch)

	var values []string
	var errorCount int

	for result := range out {
		if result.IsError() {
			errorCount++
			// Verify error details
			err := result.Error()
			if err.Item != "failed" {
				t.Errorf("Expected error item 'failed', got %q", err.Item)
			}
			if err.ProcessorName != "single-test" {
				t.Errorf("Expected processor 'single-test', got %q", err.ProcessorName)
			}
		} else {
			values = append(values, result.Value())
		}
	}

	expected := []string{"hello", "world", "test"}
	if len(values) != len(expected) {
		t.Fatalf("Expected %d successful values, got %d", len(expected), len(values))
	}

	// Since there's only one input, order should be preserved for successful values
	for i, v := range expected {
		if values[i] != v {
			t.Errorf("Expected values[%d] = %q, got %q", i, v, values[i])
		}
	}

	if errorCount != 1 {
		t.Errorf("Expected 1 error, got %d", errorCount)
	}
}

func TestFanIn_Result_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fanin := NewFanIn[int]()

	// Create a slow channel that sends Results periodically
	ch := make(chan Result[int])

	// Start a goroutine that sends Results slowly
	go func() {
		defer close(ch)
		for i := 0; i < 100; i++ {
			select {
			case ch <- NewSuccess(i):
			case <-ctx.Done():
				return // Stop sending when context is canceled
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()

	out := fanin.Process(ctx, ch)

	// Read a few items first
	var itemsReceived int32
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for result := range out {
			if result.IsSuccess() {
				atomic.AddInt32(&itemsReceived, 1)
			}
		}
	}()

	// Let it run briefly then cancel
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait for processing to complete
	wg.Wait()

	// Context cancellation should have stopped further processing
	if atomic.LoadInt32(&itemsReceived) == 0 {
		t.Error("Expected to receive at least some items before cancellation")
	}
}

func TestFanIn_Result_ErrorsFlowThrough(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[int]()

	// Create channel that only sends errors
	errCh := make(chan Result[int], 3)
	errCh <- NewError(1, errors.New("error 1"), "processor-1")
	errCh <- NewError(2, errors.New("error 2"), "processor-2")
	errCh <- NewError(3, errors.New("error 3"), "processor-3")
	close(errCh)

	// Create channel that only sends successes
	successCh := make(chan Result[int], 2)
	successCh <- NewSuccess(10)
	successCh <- NewSuccess(20)
	close(successCh)

	out := fanin.Process(ctx, errCh, successCh)

	var values []int
	var errors []*StreamError[int]

	for result := range out {
		if result.IsError() {
			errors = append(errors, result.Error())
		} else {
			values = append(values, result.Value())
		}
	}

	// Verify errors flow through without blocking
	if len(errors) != 3 {
		t.Fatalf("Expected 3 errors, got %d", len(errors))
	}

	// Verify successes still flow through
	sort.Ints(values)
	expectedValues := []int{10, 20}
	if len(values) != len(expectedValues) {
		t.Fatalf("Expected %d values, got %d", len(expectedValues), len(values))
	}

	for i, v := range expectedValues {
		if values[i] != v {
			t.Errorf("Expected values[%d] = %d, got %d", i, v, values[i])
		}
	}

	// Verify error details are preserved
	errorValues := make([]int, len(errors))
	for i, err := range errors {
		errorValues[i] = err.Item
	}
	sort.Ints(errorValues)
	expectedErrorValues := []int{1, 2, 3}

	for i, v := range expectedErrorValues {
		if errorValues[i] != v {
			t.Errorf("Expected error values[%d] = %d, got %d", i, v, errorValues[i])
		}
	}
}

func TestFanIn_Result_LargeNumberOfInputs(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[int]()

	const numChannels = 50
	const itemsPerChannel = 10
	const errorsPerChannel = 2

	channels := make([]<-chan Result[int], numChannels)

	// Create channels and populate them with mix of success and error Results
	for i := 0; i < numChannels; i++ {
		ch := make(chan Result[int], itemsPerChannel+errorsPerChannel)

		// Add successful Results
		for j := 0; j < itemsPerChannel; j++ {
			ch <- NewSuccess(i*itemsPerChannel + j)
		}

		// Add error Results
		for j := 0; j < errorsPerChannel; j++ {
			ch <- NewError(i*1000+j, errors.New("test error"), "large-test")
		}

		close(ch)
		channels[i] = ch
	}

	out := fanin.Process(ctx, channels...)

	// Collect all results
	var values []int
	var errorCount int

	for result := range out {
		if result.IsError() {
			errorCount++
		} else {
			values = append(values, result.Value())
		}
	}

	// Verify error count
	expectedErrors := numChannels * errorsPerChannel
	if errorCount != expectedErrors {
		t.Errorf("Expected %d errors, got %d", expectedErrors, errorCount)
	}

	// Verify we got all successful items
	expectedSuccesses := numChannels * itemsPerChannel
	if len(values) != expectedSuccesses {
		t.Errorf("Expected %d successful values, got %d", expectedSuccesses, len(values))
	}

	// Verify all expected values are present
	sort.Ints(values)
	for i := 0; i < expectedSuccesses; i++ {
		if values[i] != i {
			t.Errorf("Missing or incorrect value: expected %d at position %d, got %d", i, i, values[i])
		}
	}
}

func TestFanIn_Result_NoGoroutineLeaks(t *testing.T) {
	// Record initial goroutine count
	initialGoroutines := runtime.NumGoroutine()

	ctx := context.Background()
	fanin := NewFanIn[int]()

	// Create channels with Results
	ch1 := make(chan Result[int], 2)
	ch2 := make(chan Result[int], 2)

	ch1 <- NewSuccess(42)
	ch1 <- NewError(99, errors.New("test error"), "leak-test")
	close(ch1)

	ch2 <- NewSuccess(84)
	ch2 <- NewError(88, errors.New("another error"), "leak-test-2")
	close(ch2)

	out := fanin.Process(ctx, ch1, ch2)

	// Consume all output
	//nolint:revive // empty-block: intentional channel draining
	for range out {
	}

	// Give goroutines time to clean up
	time.Sleep(10 * time.Millisecond)
	runtime.GC()
	runtime.GC() // Double GC to ensure cleanup

	// Check that we don't have a goroutine leak
	finalGoroutines := runtime.NumGoroutine()

	// Allow for some variation but shouldn't be significantly higher
	if finalGoroutines > initialGoroutines+1 {
		t.Errorf("Potential goroutine leak: started with %d, ended with %d goroutines",
			initialGoroutines, finalGoroutines)
	}
}

func TestFanIn_Result_ProperChannelClosure(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[int]()

	ch := make(chan Result[int], 2)
	ch <- NewSuccess(42)
	ch <- NewError(99, errors.New("test error"), "closure-test")
	close(ch)

	out := fanin.Process(ctx, ch)

	// Consume output and verify both success and error flow through
	var gotSuccess, gotError bool
	for result := range out {
		if result.IsSuccess() && result.Value() == 42 {
			gotSuccess = true
		}
		if result.IsError() && result.Error().Item == 99 {
			gotError = true
		}
	}

	if !gotSuccess {
		t.Error("Expected to receive success Result")
	}

	if !gotError {
		t.Error("Expected to receive error Result")
	}

	// Verify channel is indeed closed
	select {
	case _, ok := <-out:
		if ok {
			t.Error("Output channel should be closed")
		}
	default:
		// Channel is closed and empty, this is expected
	}
}

func TestFanIn_Result_ConcurrentSafety(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[int]()

	const numGoRoutines = 10
	const itemsPerRoutine = 50
	const errorsPerRoutine = 5

	// Create channels that will be written to concurrently
	channels := make([]chan Result[int], numGoRoutines)
	for i := range channels {
		channels[i] = make(chan Result[int], itemsPerRoutine+errorsPerRoutine)
	}

	// Convert to receive-only channels
	inputChans := make([]<-chan Result[int], numGoRoutines)
	for i := range channels {
		inputChans[i] = channels[i]
	}

	out := fanin.Process(ctx, inputChans...)

	// Start goroutines to write Results to channels
	var wg sync.WaitGroup
	for i := 0; i < numGoRoutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			defer close(channels[routineID])

			// Send success Results
			for j := 0; j < itemsPerRoutine; j++ {
				channels[routineID] <- NewSuccess(routineID*itemsPerRoutine + j)
			}

			// Send error Results
			for j := 0; j < errorsPerRoutine; j++ {
				channels[routineID] <- NewError(routineID*1000+j, errors.New("concurrent test error"), "concurrent-processor")
			}
		}(i)
	}

	// Collect results while writers are running
	var values []int
	var errorCount int
	var mu sync.Mutex

	go func() {
		for result := range out {
			mu.Lock()
			if result.IsSuccess() {
				values = append(values, result.Value())
			} else {
				errorCount++
			}
			mu.Unlock()
		}
	}()

	// Wait for all writers to complete
	wg.Wait()

	// Give some time for FanIn to process all items
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	valueCount := len(values)
	finalErrorCount := errorCount
	mu.Unlock()

	expectedValues := numGoRoutines * itemsPerRoutine
	if valueCount != expectedValues {
		t.Errorf("Expected %d successful values, got %d", expectedValues, valueCount)
	}

	expectedErrors := numGoRoutines * errorsPerRoutine
	if finalErrorCount != expectedErrors {
		t.Errorf("Expected %d errors, got %d", expectedErrors, finalErrorCount)
	}
}

func TestFanIn_Result_OrderPreservation(t *testing.T) {
	ctx := context.Background()
	fanin := NewFanIn[int]()

	// Test that items from individual channels maintain relative order
	ch1 := make(chan Result[int], 5)
	ch1 <- NewSuccess(1)
	ch1 <- NewError(100, errors.New("error between"), "order-test")
	ch1 <- NewSuccess(2)
	ch1 <- NewSuccess(3)
	ch1 <- NewError(200, errors.New("another error"), "order-test")
	close(ch1)

	ch2 := make(chan Result[int], 3)
	ch2 <- NewSuccess(10)
	ch2 <- NewSuccess(20)
	ch2 <- NewSuccess(30)
	close(ch2)

	out := fanin.Process(ctx, ch1, ch2)

	var ch1Successes []int
	var ch2Successes []int
	var errors []*StreamError[int]

	for result := range out {
		if result.IsError() {
			errors = append(errors, result.Error())
		} else {
			value := result.Value()
			if value < 10 {
				ch1Successes = append(ch1Successes, value)
			} else {
				ch2Successes = append(ch2Successes, value)
			}
		}
	}

	// Verify relative ordering within each channel is preserved for successes
	expectedCh1 := []int{1, 2, 3}
	if len(ch1Successes) != len(expectedCh1) {
		t.Errorf("Expected %d successful items from ch1, got %d", len(expectedCh1), len(ch1Successes))
	}

	for i, v := range expectedCh1 {
		if i < len(ch1Successes) && ch1Successes[i] != v {
			t.Errorf("Expected ch1Successes[%d] = %d, got %d", i, v, ch1Successes[i])
		}
	}

	expectedCh2 := []int{10, 20, 30}
	if len(ch2Successes) != len(expectedCh2) {
		t.Errorf("Expected %d successful items from ch2, got %d", len(expectedCh2), len(ch2Successes))
	}

	for i, v := range expectedCh2 {
		if i < len(ch2Successes) && ch2Successes[i] != v {
			t.Errorf("Expected ch2Successes[%d] = %d, got %d", i, v, ch2Successes[i])
		}
	}

	// Verify errors were captured
	if len(errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(errors))
	}
}

// Helper function to create channels with mixed Results for testing.
func createMixedResultChannel[T any](successes []T, errorItems []T, processorName string) <-chan Result[T] {
	ch := make(chan Result[T], len(successes)+len(errorItems))

	for _, success := range successes {
		ch <- NewSuccess(success)
	}

	for _, errorItem := range errorItems {
		ch <- NewError(errorItem, errors.New("test error"), processorName)
	}

	close(ch)
	return ch
}

func TestFanIn_Result_HelperFunctionUsage(t *testing.T) {
	// Test using the helper function
	ctx := context.Background()
	fanin := NewFanIn[string]()

	ch1 := createMixedResultChannel(
		[]string{"success1", "success2"},
		[]string{"error1"},
		"helper-test",
	)

	ch2 := createMixedResultChannel(
		[]string{"success3"},
		[]string{"error2", "error3"},
		"helper-test-2",
	)

	out := fanin.Process(ctx, ch1, ch2)

	var values []string
	var errorCount int

	for result := range out {
		if result.IsSuccess() {
			values = append(values, result.Value())
		} else {
			errorCount++
		}
	}

	sort.Strings(values)
	expected := []string{"success1", "success2", "success3"}

	if len(values) != len(expected) {
		t.Fatalf("Expected %d successful values, got %d", len(expected), len(values))
	}

	for i, v := range expected {
		if values[i] != v {
			t.Errorf("Expected values[%d] = %q, got %q", i, v, values[i])
		}
	}

	if errorCount != 3 {
		t.Errorf("Expected 3 errors, got %d", errorCount)
	}
}
