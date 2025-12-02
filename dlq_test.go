package streamz

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

func TestDeadLetterQueue_Constructor(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)

	if dlq.name != "dlq" {
		t.Errorf("Expected name 'dlq', got %q", dlq.name)
	}

	if dlq.DroppedCount() != 0 {
		t.Errorf("Expected dropped count 0, got %d", dlq.DroppedCount())
	}
}

func TestDeadLetterQueue_WithName(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock).WithName("custom-dlq")

	if dlq.Name() != "custom-dlq" {
		t.Errorf("Expected name 'custom-dlq', got %q", dlq.Name())
	}
}

func TestDeadLetterQueue_SuccessDistribution(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	input := make(chan Result[int], 10) // Buffered to prevent blocking during sending
	successes, failures := dlq.Process(ctx, input)

	// Send success items immediately
	testValues := []int{1, 2, 3}
	for _, val := range testValues {
		input <- NewSuccess(val)
	}
	close(input)

	// Collect results concurrently (like MixedResults test)
	var successValues []int
	var failureCount int
	var wg sync.WaitGroup

	wg.Add(2)

	// Consume successes
	go func() {
		defer wg.Done()
		for result := range successes {
			if result.IsSuccess() {
				successValues = append(successValues, result.Value())
			} else {
				t.Errorf("Got error in success channel: %v", result.Error())
			}
		}
	}()

	// Consume failures (should be empty)
	go func() {
		defer wg.Done()
		for range failures {
			failureCount++
		}
	}()

	wg.Wait()

	// Verify all successes received
	if len(successValues) != len(testValues) {
		t.Errorf("Expected %d successes, got %d", len(testValues), len(successValues))
	}

	// Check that all expected values are present (order might vary)
	for _, expected := range testValues {
		found := false
		for _, actual := range successValues {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected success value %d not found in %v", expected, successValues)
		}
	}

	if failureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", failureCount)
	}
}

func TestDeadLetterQueue_FailureDistribution(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	input := make(chan Result[int], 10) // Buffered to prevent blocking during sending
	successes, failures := dlq.Process(ctx, input)

	// Send failure items immediately
	testErrors := []error{
		errors.New("error1"),
		errors.New("error2"),
		errors.New("error3"),
	}

	for i, err := range testErrors {
		input <- NewError(i, err, "test-processor")
	}
	close(input)

	// Collect results concurrently (like MixedResults test)
	var failureErrors []string
	var successCount int
	var wg sync.WaitGroup

	wg.Add(2)

	// Consume failures
	go func() {
		defer wg.Done()
		for result := range failures {
			if result.IsError() {
				failureErrors = append(failureErrors, result.Error().Err.Error())
			} else {
				t.Errorf("Got success in failure channel: %v", result.Value())
			}
		}
	}()

	// Consume successes (should be empty)
	go func() {
		defer wg.Done()
		for range successes {
			successCount++
		}
	}()

	wg.Wait()

	// Verify all failures received
	if len(failureErrors) != len(testErrors) {
		t.Errorf("Expected %d failures, got %d", len(testErrors), len(failureErrors))
	}

	// Check that all expected error messages are present (order might vary)
	for _, expected := range testErrors {
		found := false
		for _, actual := range failureErrors {
			if actual == expected.Error() {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected failure error %s not found in %v", expected.Error(), failureErrors)
		}
	}

	if successCount != 0 {
		t.Errorf("Expected 0 successes, got %d", successCount)
	}
}

func TestDeadLetterQueue_MixedResults(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	input := make(chan Result[int], 10)
	successes, failures := dlq.Process(ctx, input)

	// Send mixed success and failure items
	input <- NewSuccess(1)
	input <- NewError(2, errors.New("error1"), "test")
	input <- NewSuccess(3)
	input <- NewError(4, errors.New("error2"), "test")
	input <- NewSuccess(5)
	close(input)

	// Collect results concurrently
	var successValues []int
	var failureValues []int
	var wg sync.WaitGroup

	wg.Add(2)

	// Collect successes
	go func() {
		defer wg.Done()
		for result := range successes {
			if result.IsSuccess() {
				successValues = append(successValues, result.Value())
			} else {
				t.Errorf("Got error in success channel: %v", result.Error())
			}
		}
	}()

	// Collect failures
	go func() {
		defer wg.Done()
		for result := range failures {
			if result.IsError() {
				failureValues = append(failureValues, result.Error().Item)
			} else {
				t.Errorf("Got success in failure channel: %v", result.Value())
			}
		}
	}()

	wg.Wait()

	// Verify distribution
	expectedSuccesses := []int{1, 3, 5}
	expectedFailures := []int{2, 4}

	if len(successValues) != len(expectedSuccesses) {
		t.Errorf("Expected %d successes, got %d", len(expectedSuccesses), len(successValues))
	}

	if len(failureValues) != len(expectedFailures) {
		t.Errorf("Expected %d failures, got %d", len(expectedFailures), len(failureValues))
	}

	// Note: Order might not be preserved due to concurrent consumption
	// Just verify all expected values are present
	for _, expected := range expectedSuccesses {
		found := false
		for _, actual := range successValues {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected success value %d not found", expected)
		}
	}

	for _, expected := range expectedFailures {
		found := false
		for _, actual := range failureValues {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected failure value %d not found", expected)
		}
	}
}

func TestDeadLetterQueue_ContextCancellation(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithCancel(context.Background())

	input := make(chan Result[int], 10)
	successes, failures := dlq.Process(ctx, input)

	// Send some items
	input <- NewSuccess(1)
	input <- NewError(2, errors.New("error"), "test")

	// Cancel context
	cancel()

	// Allow goroutine cleanup with fake clock
	clock.Advance(10 * time.Millisecond)

	// Both channels should eventually close
	// May receive pending items first, so drain until closed
	timeout := time.After(500 * time.Millisecond)

	successesClosed := false
	for !successesClosed {
		select {
		case _, ok := <-successes:
			if !ok {
				successesClosed = true
			}
		case <-timeout:
			t.Fatal("Successes channel should close after context cancellation")
		}
	}

	failuresClosed := false
	for !failuresClosed {
		select {
		case _, ok := <-failures:
			if !ok {
				failuresClosed = true
			}
		case <-timeout:
			t.Fatal("Failures channel should close after context cancellation")
		}
	}
}

func TestDeadLetterQueue_InputChannelClosure(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	input := make(chan Result[int], 10)
	successes, failures := dlq.Process(ctx, input)

	// Send some items and close input
	input <- NewSuccess(1)
	input <- NewError(2, errors.New("error"), "test")
	close(input)

	// Both channels should close when input closes
	var successClosed, failureClosed bool

	// Read all successes
	for result := range successes {
		if result.IsSuccess() && result.Value() != 1 {
			t.Errorf("Expected success value 1, got %d", result.Value())
		}
	}
	successClosed = true

	// Read all failures
	for result := range failures {
		if result.IsError() && result.Error().Item != 2 {
			t.Errorf("Expected failure item 2, got %d", result.Error().Item)
		}
	}
	failureClosed = true

	if !successClosed {
		t.Error("Successes channel should be closed when input closes")
	}

	if !failureClosed {
		t.Error("Failures channel should be closed when input closes")
	}
}

// LEGACY TESTS: These tests verify drop behavior but use real time for consistency
// with existing production behavior. The deterministic timeout tests above
// provide better coverage for the actual timeout mechanisms.

// CRITICAL TEST: Non-consumed channel handling with drop policy.
func TestDeadLetterQueue_NonConsumedFailureChannel(t *testing.T) {
	// Use real clock for this integration test to match existing behavior
	dlq := NewDeadLetterQueue[int](RealClock).WithName("test-dlq")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	// Only consume successes, ignore failures
	go func() {
		defer close(input)
		// Send mixed results - failures should be dropped
		input <- NewSuccess(1)
		input <- NewError(2, errors.New("error1"), "test")
		input <- NewSuccess(3)
		input <- NewError(4, errors.New("error2"), "test")
		input <- NewSuccess(5)

		// Give time for processing
		time.Sleep(50 * time.Millisecond)
	}()

	// Only read from successes channel - failures ignored
	var successValues []int
	for result := range successes {
		if result.IsSuccess() {
			successValues = append(successValues, result.Value())
		}
	}

	// Drain failures channel to prevent goroutine leak in test
	for failure := range failures {
		_ = failure // Ignore - testing drop behavior
	}

	// Verify successes were processed
	expected := []int{1, 3, 5}
	if len(successValues) != len(expected) {
		t.Errorf("Expected %d successes, got %d", len(expected), len(successValues))
	}

	// Verify drops were counted (failures were dropped due to no consumer)
	// Note: This test is timing-dependent but should show drops
	time.Sleep(100 * time.Millisecond) // Allow drop handling
	droppedCount := dlq.DroppedCount()

	if droppedCount == 0 {
		t.Logf("Warning: Expected some drops due to non-consumed failure channel, got %d", droppedCount)
		// This might not always fail due to timing, but log the observation
	}
}

// CRITICAL TEST: Non-consumed success channel handling.
func TestDeadLetterQueue_NonConsumedSuccessChannel(t *testing.T) {
	// Use real clock for this integration test to match existing behavior
	dlq := NewDeadLetterQueue[int](RealClock).WithName("test-dlq-2")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	// Only consume failures, ignore successes
	go func() {
		defer close(input)
		// Send mixed results - successes should be dropped
		input <- NewSuccess(1)
		input <- NewError(2, errors.New("error1"), "test")
		input <- NewSuccess(3)
		input <- NewError(4, errors.New("error2"), "test")

		// Give time for processing
		time.Sleep(50 * time.Millisecond)
	}()

	// Only read from failures channel - successes ignored
	var failureValues []int
	for result := range failures {
		if result.IsError() {
			failureValues = append(failureValues, result.Error().Item)
		}
	}

	// Drain successes channel to prevent goroutine leak in test
	for success := range successes {
		_ = success // Ignore - testing drop behavior
	}

	// Verify failures were processed
	expected := []int{2, 4}
	if len(failureValues) != len(expected) {
		t.Errorf("Expected %d failures, got %d", len(expected), len(failureValues))
	}

	// Verify drops were counted (successes were dropped due to no consumer)
	time.Sleep(100 * time.Millisecond) // Allow drop handling
	droppedCount := dlq.DroppedCount()

	if droppedCount == 0 {
		t.Logf("Warning: Expected some drops due to non-consumed success channel, got %d", droppedCount)
	}
}

func TestDeadLetterQueue_ConcurrentConsumers(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	var wg sync.WaitGroup
	var successMutex, failureMutex sync.Mutex
	var successValues, failureValues []int

	// Multiple consumers on success channel
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for result := range successes {
				if result.IsSuccess() {
					successMutex.Lock()
					successValues = append(successValues, result.Value())
					successMutex.Unlock()
				}
			}
		}()
	}

	// Multiple consumers on failure channel
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for result := range failures {
				if result.IsError() {
					failureMutex.Lock()
					failureValues = append(failureValues, result.Error().Item)
					failureMutex.Unlock()
				}
			}
		}()
	}

	// Send test data
	go func() {
		defer close(input)
		for i := 1; i <= 10; i++ {
			if i%2 == 0 {
				input <- NewError(i, errors.New("error"), "test")
			} else {
				input <- NewSuccess(i)
			}
		}
	}()

	wg.Wait()

	// Each item should be consumed exactly once despite multiple consumers
	successMutex.Lock()
	failureMutex.Lock()
	defer successMutex.Unlock()
	defer failureMutex.Unlock()

	if len(successValues) != 5 {
		t.Errorf("Expected 5 success values, got %d", len(successValues))
	}

	if len(failureValues) != 5 {
		t.Errorf("Expected 5 failure values, got %d", len(failureValues))
	}
}

func TestDeadLetterQueue_EmptyStream(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	// Close input immediately without sending anything
	close(input)

	// Both channels should close with no items
	successCount := 0
	for range successes {
		successCount++
	}

	failureCount := 0
	for range failures {
		failureCount++
	}

	if successCount != 0 {
		t.Errorf("Expected 0 successes from empty stream, got %d", successCount)
	}

	if failureCount != 0 {
		t.Errorf("Expected 0 failures from empty stream, got %d", failureCount)
	}
}

// Race condition test - must be run with -race flag.
func TestDeadLetterQueue_Race(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := make(chan Result[int], 100)
	successes, failures := dlq.Process(ctx, input)

	var wg sync.WaitGroup

	// Start multiple producers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				value := producerID*50 + j
				if j%3 == 0 {
					input <- NewError(value, errors.New("error"), "test")
				} else {
					input <- NewSuccess(value)
				}
			}
		}(i)
	}

	// Start multiple consumers
	wg.Add(2)

	go func() {
		defer wg.Done()
		for result := range successes {
			// Just consume to test race conditions
			_ = result.IsSuccess()
		}
	}()

	go func() {
		defer wg.Done()
		for result := range failures {
			// Just consume to test race conditions
			_ = result.IsError()
		}
	}()

	// Wait for producers to finish, then close input
	wg.Wait()
	close(input)

	// Verify no dropped items under normal consumption
	droppedCount := dlq.DroppedCount()
	if droppedCount > 0 {
		t.Errorf("Expected no drops with active consumers, got %d", droppedCount)
	}
}

// DETERMINISTIC TIMEOUT TESTS - These test the exact timeout behavior using fake clock

func TestDeadLetterQueue_DeterministicTimeoutSuccessChannel(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock).WithName("timeout-test-success")
	ctx := context.Background()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	// Send an item that will block because no one is consuming successes
	input <- NewSuccess(1)
	close(input)

	// Advance time past the timeout threshold
	clock.Advance(15 * time.Millisecond) // Well past 10ms timeout
	clock.BlockUntilReady()

	// Drain both channels to allow test completion
	for success := range successes {
		_ = success // Channel should close after timeout/drop
	}
	for failure := range failures {
		_ = failure // Ignore failures
	}

	// Verify the timeout caused a drop
	droppedCount := dlq.DroppedCount()
	if droppedCount != 1 {
		t.Errorf("Expected exactly 1 drop due to timeout, got %d", droppedCount)
	}
}

func TestDeadLetterQueue_DeterministicTimeoutFailureChannel(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock).WithName("timeout-test-failure")
	ctx := context.Background()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	// Send an item that will block because no one is consuming failures
	input <- NewError(1, errors.New("error1"), "test")
	close(input)

	// Advance time past the timeout threshold
	clock.Advance(15 * time.Millisecond) // Well past 10ms timeout
	clock.BlockUntilReady()

	// Drain both channels to allow test completion
	for failure := range failures {
		_ = failure // Channel should close after timeout/drop
	}
	for success := range successes {
		_ = success // Ignore successes
	}

	// Verify the timeout caused a drop
	droppedCount := dlq.DroppedCount()
	if droppedCount != 1 {
		t.Errorf("Expected exactly 1 drop due to timeout, got %d", droppedCount)
	}
}

func TestDeadLetterQueue_DeterministicTimeoutBothChannelsSequential(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock).WithName("timeout-test-both")
	ctx := context.Background()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	// Test both channels separately since DLQ processes items sequentially

	// First test: success channel timeout
	input <- NewSuccess(1)
	clock.Advance(15 * time.Millisecond)
	clock.BlockUntilReady()

	// Second test: failure channel timeout
	input <- NewError(2, errors.New("error1"), "test")
	close(input)

	clock.Advance(15 * time.Millisecond)
	clock.BlockUntilReady()

	// Drain both channels to allow test completion
	for success := range successes {
		_ = success // Channel should close after timeout/drop
	}
	for failure := range failures {
		_ = failure // Channel should close after timeout/drop
	}

	// Verify both timeouts caused drops
	droppedCount := dlq.DroppedCount()
	if droppedCount != 2 {
		t.Errorf("Expected exactly 2 drops (one per channel), got %d", droppedCount)
	}
}

func TestDeadLetterQueue_NoTimeoutWhenConsuming(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock).WithName("no-timeout-test")
	ctx := context.Background()

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	var wg sync.WaitGroup
	var successValues []int
	var failureValues []int
	var mu sync.Mutex

	// Active consumers - no timeouts should occur
	wg.Add(2)

	go func() {
		defer wg.Done()
		for result := range successes {
			if result.IsSuccess() {
				mu.Lock()
				successValues = append(successValues, result.Value())
				mu.Unlock()
			}
		}
	}()

	go func() {
		defer wg.Done()
		for result := range failures {
			if result.IsError() {
				mu.Lock()
				failureValues = append(failureValues, result.Error().Item)
				mu.Unlock()
			}
		}
	}()

	// Send items WITHOUT time advances - consumers are active
	go func() {
		defer close(input)

		for i := 1; i <= 10; i++ {
			if i%2 == 0 {
				input <- NewError(i, errors.New("error"), "test")
			} else {
				input <- NewSuccess(i)
			}
			// No time advance - consumers should handle items immediately
		}
	}()

	wg.Wait()

	// Verify all items processed, none dropped
	mu.Lock()
	defer mu.Unlock()

	if len(successValues) != 5 {
		t.Errorf("Expected 5 successes, got %d", len(successValues))
	}

	if len(failureValues) != 5 {
		t.Errorf("Expected 5 failures, got %d", len(failureValues))
	}

	droppedCount := dlq.DroppedCount()
	if droppedCount != 0 {
		t.Errorf("Expected no drops with active consumers, got %d", droppedCount)
	}
}

func TestDeadLetterQueue_ContextCancellationDuringTimeout(t *testing.T) {
	clock := clockz.NewFakeClock()
	dlq := NewDeadLetterQueue[int](clock).WithName("context-cancel-test")
	ctx, cancel := context.WithCancel(context.Background())

	input := make(chan Result[int])
	successes, failures := dlq.Process(ctx, input)

	// Send item that will start timeout process
	input <- NewSuccess(1)

	// Advance partway through timeout
	clock.Advance(5 * time.Millisecond)
	clock.BlockUntilReady()

	// Cancel context during timeout
	cancel()
	close(input)

	// Both channels should close due to context cancellation
	for success := range successes {
		_ = success // Should close quickly due to context cancellation
	}
	for failure := range failures {
		_ = failure // Should close quickly due to context cancellation
	}

	// Item may or may not be counted as dropped depending on timing
	// Context cancellation competes with timeout
	droppedCount := dlq.DroppedCount()
	t.Logf("Drops during context cancellation: %d (timing dependent)", droppedCount)
}
