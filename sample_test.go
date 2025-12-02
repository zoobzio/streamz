package streamz

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestNewSample(t *testing.T) {
	sample := NewSample[int](0.5)

	if sample.name != "sample" {
		t.Errorf("Expected default name 'sample', got %s", sample.name)
	}

	if sample.rate != 0.5 {
		t.Errorf("Expected rate 0.5, got %f", sample.rate)
	}

	if sample.Rate() != 0.5 {
		t.Errorf("Expected Rate() to return 0.5, got %f", sample.Rate())
	}
}

func TestNewSample_ValidRates(t *testing.T) {
	validRates := []struct {
		name string
		rate float64
	}{
		{"rate_0.0", 0.0},
		{"rate_0.1", 0.1},
		{"rate_0.5", 0.5},
		{"rate_0.9", 0.9},
		{"rate_1.0", 1.0},
	}

	for _, tc := range validRates {
		t.Run(tc.name, func(t *testing.T) {
			sample := NewSample[int](tc.rate)
			if sample.rate != tc.rate {
				t.Errorf("Expected rate %f, got %f", tc.rate, sample.rate)
			}
		})
	}
}

func TestNewSample_InvalidRates(t *testing.T) {
	invalidRates := []struct {
		name string
		rate float64
	}{
		{"negative", -0.1},
		{"negative_large", -1.0},
		{"above_one", 1.1},
		{"large", 2.0},
		{"infinity", math.Inf(1)},
		{"nan", math.NaN()},
	}

	for _, tc := range invalidRates {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected panic for invalid rate %f", tc.rate)
				}
			}()
			NewSample[int](tc.rate)
		})
	}
}

func TestSample_WithName(t *testing.T) {
	sample := NewSample[int](0.3).WithName("custom-sample")

	if sample.name != "custom-sample" {
		t.Errorf("Expected name 'custom-sample', got %s", sample.name)
	}

	if sample.Name() != "custom-sample" {
		t.Errorf("Expected Name() to return 'custom-sample', got %s", sample.Name())
	}
}

func TestSample_Name(t *testing.T) {
	sample := NewSample[int](0.7)

	if sample.Name() != "sample" {
		t.Errorf("Expected Name() to return 'sample', got %s", sample.Name())
	}
}

func TestSample_Process_RateZero(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 5)

	sample := NewSample[int](0.0) // Drop everything

	// Send test data
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	input <- NewSuccess(4)
	input <- NewSuccess(5)
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// Should receive no items with rate 0.0
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results with rate 0.0, got %d", len(results))
	}
}

func TestSample_Process_RateOne(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 3)

	sample := NewSample[int](1.0) // Keep everything

	// Send test data
	input <- NewSuccess(10)
	input <- NewSuccess(20)
	input <- NewSuccess(30)
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// Should receive all items with rate 1.0
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results with rate 1.0, got %d", len(results))
	}

	expectedValues := []int{10, 20, 30}
	for i, result := range results {
		if result.IsError() {
			t.Errorf("Expected success result at index %d, got error: %v", i, result.Error())
			continue
		}
		if result.Value() != expectedValues[i] {
			t.Errorf("Expected value %d at index %d, got %d", expectedValues[i], i, result.Value())
		}
	}
}

func TestSample_Process_ErrorsAlwaysPassThrough(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int], 4)

	sample := NewSample[int](0.0) // Drop all successful items

	// Send mixed data including errors
	testErr1 := errors.New("error1")
	testErr2 := errors.New("error2")
	input <- NewSuccess(1)                   // Should be dropped
	input <- NewError(99, testErr1, "proc1") // Should pass through
	input <- NewSuccess(2)                   // Should be dropped
	input <- NewError(88, testErr2, "proc2") // Should pass through
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// Should receive only the 2 errors, no successful items
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 error results, got %d", len(results))
	}

	// Verify both results are errors
	for i, result := range results {
		if !result.IsError() {
			t.Errorf("Expected error result at index %d, got success: %v", i, result.Value())
		}
	}

	// Verify first error
	if len(results) > 0 && results[0].Error().Err.Error() != "error1" {
		t.Errorf("Expected first error 'error1', got %v", results[0].Error().Err)
	}

	// Verify second error
	if len(results) > 1 && results[1].Error().Err.Error() != "error2" {
		t.Errorf("Expected second error 'error2', got %v", results[1].Error().Err)
	}
}

func TestSample_Process_ErrorsWithNonZeroRate(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[string], 3)

	sample := NewSample[string](0.5) // Sample successful items at 50%

	// Send data with errors mixed in
	testErr := errors.New("test error")
	input <- NewError("fail1", testErr, "processor1")
	input <- NewError("fail2", testErr, "processor2")
	input <- NewError("fail3", testErr, "processor3")
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// All errors should pass through regardless of rate
	results := make([]Result[string], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 error results to pass through, got %d", len(results))
	}

	// Verify all are errors
	for i, result := range results {
		if !result.IsError() {
			t.Errorf("Expected error result at index %d, got success: %v", i, result.Value())
		}
		if result.Error().Err.Error() != "test error" {
			t.Errorf("Expected error 'test error' at index %d, got %v", i, result.Error().Err)
		}
	}
}

func TestSample_Process_EmptyStream(t *testing.T) {
	ctx := context.Background()
	input := make(chan Result[int])

	sample := NewSample[int](0.5)

	// Close empty input immediately
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// Should receive no items
	results := make([]Result[int], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 0 {
		t.Errorf("Expected no results from empty stream, got %d", len(results))
	}
}

func TestSample_Process_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input := make(chan Result[int])

	sample := NewSample[int](1.0) // Keep everything to make test deterministic

	// Process through sample
	output := sample.Process(ctx, input)

	// Send one item
	input <- NewSuccess(42)

	// Read the first result
	result := <-output
	if result.IsError() || result.Value() != 42 {
		t.Errorf("Expected success result 42, got %+v", result)
	}

	// Cancel context
	cancel()

	// Send another item (should be ignored due to cancellation)
	go func() {
		time.Sleep(10 * time.Millisecond)
		input <- NewSuccess(99)
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
}

func TestSample_Process_StatisticalBehavior(t *testing.T) {
	ctx := context.Background()

	// Test with a moderate rate and enough samples for statistical validity
	rate := 0.3
	numItems := 10000
	sample := NewSample[int](rate)

	// Create input with many items
	input := make(chan Result[int], numItems)
	for i := 0; i < numItems; i++ {
		input <- NewSuccess(i)
	}
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// Count results
	resultCount := 0
	for range output {
		resultCount++
	}

	// Check if result count is within reasonable statistical bounds
	// With 10k samples at 30% rate, expect ~3000 results
	// Use wide tolerance (15%) to avoid flaky tests in CI while still catching gross errors
	expected := float64(numItems) * rate
	tolerance := expected * 0.15 // 15% tolerance - wide enough to be reliable

	if float64(resultCount) < expected-tolerance || float64(resultCount) > expected+tolerance {
		t.Errorf("Statistical test failed: expected ~%.0f results (±%.0f), got %d",
			expected, tolerance, resultCount)
	}
}

func TestSample_Process_MixedSuccessAndError(t *testing.T) {
	ctx := context.Background()

	// Use rate 0.0 to make success behavior deterministic
	sample := NewSample[string](0.0)

	// Create input with alternating success/error pattern
	input := make(chan Result[string], 6)
	testErr := errors.New("test error")
	input <- NewSuccess("drop1")                 // Should be dropped
	input <- NewError("keep1", testErr, "proc1") // Should pass through
	input <- NewSuccess("drop2")                 // Should be dropped
	input <- NewError("keep2", testErr, "proc2") // Should pass through
	input <- NewSuccess("drop3")                 // Should be dropped
	input <- NewError("keep3", testErr, "proc3") // Should pass through
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// Should receive only the 3 errors
	results := make([]Result[string], 0, 10)
	for result := range output {
		results = append(results, result)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 error results, got %d", len(results))
	}

	// Verify all are errors and in correct order
	expectedErrorData := []string{"keep1", "keep2", "keep3"}
	for i, result := range results {
		if !result.IsError() {
			t.Errorf("Expected error result at index %d, got success", i)
			continue
		}
		if result.Error().Item != expectedErrorData[i] {
			t.Errorf("Expected error item %s at index %d, got %s",
				expectedErrorData[i], i, result.Error().Item)
		}
	}
}

func TestSample_Process_LoadShedding(t *testing.T) {
	ctx := context.Background()

	// Simulate load shedding scenario - keep only 10% during high load
	sample := NewSample[int](0.1).WithName("load-shedder")

	// Create high-volume input
	input := make(chan Result[int], 1000)
	for i := 0; i < 1000; i++ {
		input <- NewSuccess(i)
	}
	close(input)

	// Process through sample
	output := sample.Process(ctx, input)

	// Count processed items
	processedCount := 0
	for range output {
		processedCount++
	}

	// Should process significantly fewer items (around 100 ± variance)
	if processedCount < 50 || processedCount > 150 {
		t.Errorf("Load shedding test: expected ~100 items (50-150), got %d", processedCount)
	}

	// Verify processor name
	if sample.Name() != "load-shedder" {
		t.Errorf("Expected name 'load-shedder', got %s", sample.Name())
	}
}

func TestSample_Process_ABTestingScenario(t *testing.T) {
	ctx := context.Background()

	// A/B testing scenario - 50/50 split
	groupA := NewSample[string](0.5).WithName("group-a")

	// Simulate user traffic
	input := make(chan Result[string], 200)
	for i := 0; i < 200; i++ {
		input <- NewSuccess("user" + string(rune(i+'0')))
	}
	close(input)

	// Process through sample for Group A
	output := groupA.Process(ctx, input)

	// Count Group A users
	groupACount := 0
	for range output {
		groupACount++
	}

	// Should get approximately half (80-120 with variance)
	if groupACount < 70 || groupACount > 130 {
		t.Errorf("A/B testing: expected ~100 users for Group A (70-130), got %d", groupACount)
	}
}

func TestSample_Process_MonitoringSample(t *testing.T) {
	ctx := context.Background()

	// Monitoring scenario - sample 1% for detailed analysis
	monitor := NewSample[int](0.01).WithName("monitoring-sample")

	// High-volume production data
	input := make(chan Result[int], 5000)
	for i := 0; i < 5000; i++ {
		input <- NewSuccess(i)
	}
	close(input)

	// Process through sample
	output := monitor.Process(ctx, input)

	// Count sampled items
	sampledCount := 0
	for range output {
		sampledCount++
	}

	// Should get approximately 1% (25-75 with variance for this sample size)
	if sampledCount > 100 {
		t.Errorf("Monitoring sample: expected ~50 items (0-100), got %d", sampledCount)
	}

	// Verify processor name
	if monitor.Name() != "monitoring-sample" {
		t.Errorf("Expected name 'monitoring-sample', got %s", monitor.Name())
	}
}
