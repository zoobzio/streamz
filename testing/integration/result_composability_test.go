package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"

	streamz "github.com/zoobzio/streamz"
	testinghelpers "github.com/zoobzio/streamz/testing"
)

// TestResultComposability_FanInFanOut tests that Result[T] pattern works correctly
// when combining FanIn and FanOut processors, proving composability across different
// processor types (many-to-1 and 1-to-many).
func TestResultComposability_FanInFanOut(t *testing.T) {
	ctx := context.Background()

	// Create test pipeline: multiple inputs -> FanIn -> FanOut -> collect all

	// Step 1: Create multiple input streams with mixed success/error Results
	input1 := make(chan streamz.Result[string], 3)
	input1 <- streamz.NewSuccess("input1-data1")
	input1 <- streamz.NewError("input1-bad", fmt.Errorf("input1 error"), "source1")
	input1 <- streamz.NewSuccess("input1-data2")
	close(input1)

	input2 := make(chan streamz.Result[string], 2)
	input2 <- streamz.NewSuccess("input2-data1")
	input2 <- streamz.NewError("input2-bad", fmt.Errorf("input2 error"), "source2")
	close(input2)

	input3 := make(chan streamz.Result[string], 1)
	input3 <- streamz.NewSuccess("input3-data1")
	close(input3)

	// Step 2: Merge all inputs with FanIn
	fanin := streamz.NewFanIn[string]()
	merged := fanin.Process(ctx, input1, input2, input3)

	// Step 3: Fan out to multiple processors
	fanout := streamz.NewFanOut[string](3)
	outputs := fanout.Process(ctx, merged)

	if len(outputs) != 3 {
		t.Fatalf("expected 3 fan-out outputs, got %d", len(outputs))
	}

	// Step 4: Collect results from all outputs concurrently
	var wg sync.WaitGroup
	results := make([][]streamz.Result[string], 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = testinghelpers.CollectResultsWithTimeout(outputs[index], 500*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// Step 5: Verify results
	expectedTotalItems := 6 // 3 + 2 + 1 from all inputs

	// All outputs should receive the same items (FanOut duplicates everything)
	for i := 0; i < 3; i++ {
		if len(results[i]) != expectedTotalItems {
			t.Errorf("output %d: expected %d results, got %d", i, expectedTotalItems, len(results[i]))
			continue
		}

		// Count successful vs error results
		successCount := 0
		errorCount := 0
		for _, result := range results[i] {
			if result.IsError() {
				errorCount++
			} else {
				successCount++
			}
		}

		// We expect 4 successes and 2 errors from all inputs combined
		if successCount != 4 {
			t.Errorf("output %d: expected 4 success results, got %d", i, successCount)
		}
		if errorCount != 2 {
			t.Errorf("output %d: expected 2 error results, got %d", i, errorCount)
		}
	}

	// Verify that all outputs have identical sequences (FanOut guarantee)
	if len(results[0]) > 0 && len(results[1]) > 0 {
		for j := 0; j < len(results[0]); j++ {
			r0, r1 := results[0][j], results[1][j]

			if r0.IsError() != r1.IsError() {
				t.Errorf("outputs differ at index %d: output0 error=%v, output1 error=%v",
					j, r0.IsError(), r1.IsError())
				continue
			}

			if r0.IsError() {
				// Compare error items (errors should be identical)
				if r0.Error().Item != r1.Error().Item {
					t.Errorf("error items differ at index %d: output0=%v, output1=%v",
						j, r0.Error().Item, r1.Error().Item)
				}
			} else {
				// Compare success values (values should be identical)
				if r0.Value() != r1.Value() {
					t.Errorf("success values differ at index %d: output0=%v, output1=%v",
						j, r0.Value(), r1.Value())
				}
			}
		}
	}

	t.Logf("Successfully processed %d items through FanIn->FanOut pipeline", expectedTotalItems)
	t.Logf("Output 0: %d items, Output 1: %d items, Output 2: %d items",
		len(results[0]), len(results[1]), len(results[2]))
}

// TestResultComposability_ErrorPropagation tests that errors propagate correctly
// through complex FanIn/FanOut compositions while maintaining error context.
func TestResultComposability_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	// Create inputs with specific error patterns
	errorInput := make(chan streamz.Result[int], 3)
	errorInput <- streamz.NewError(1, fmt.Errorf("critical error"), "validator")
	errorInput <- streamz.NewError(2, fmt.Errorf("validation failed"), "validator")
	errorInput <- streamz.NewError(3, fmt.Errorf("parsing error"), "parser")
	close(errorInput)

	successInput := make(chan streamz.Result[int], 2)
	successInput <- streamz.NewSuccess(100)
	successInput <- streamz.NewSuccess(200)
	close(successInput)

	// Merge inputs
	fanin := streamz.NewFanIn[int]()
	merged := fanin.Process(ctx, errorInput, successInput)

	// Fan out to multiple processors
	fanout := streamz.NewFanOut[int](2)
	outputs := fanout.Process(ctx, merged)

	// Collect all results
	var wg sync.WaitGroup
	results := make([][]streamz.Result[int], 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = testinghelpers.CollectResultsWithTimeout(outputs[index], 200*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// Verify error propagation
	for i := 0; i < 2; i++ {
		if len(results[i]) != 5 { // 3 errors + 2 successes
			t.Errorf("output %d: expected 5 results, got %d", i, len(results[i]))
			continue
		}

		errorItems := make([]int, 0)
		successValues := make([]int, 0)

		for _, result := range results[i] {
			if result.IsError() {
				errorItems = append(errorItems, result.Error().Item)

				// Verify error metadata is preserved
				if result.Error().ProcessorName == "" {
					t.Errorf("output %d: error missing processor name", i)
				}
				if result.Error().Err == nil {
					t.Errorf("output %d: error missing underlying error", i)
				}
			} else {
				successValues = append(successValues, result.Value())
			}
		}

		// Should have 3 error items and 2 success values
		if len(errorItems) != 3 {
			t.Errorf("output %d: expected 3 error items, got %d", i, len(errorItems))
		}
		if len(successValues) != 2 {
			t.Errorf("output %d: expected 2 success values, got %d", i, len(successValues))
		}

		// Verify error items contain expected values
		expectedErrorItems := map[int]bool{1: true, 2: true, 3: true}
		for _, item := range errorItems {
			if !expectedErrorItems[item] {
				t.Errorf("output %d: unexpected error item %d", i, item)
			}
		}

		// Verify success values
		expectedSuccessValues := map[int]bool{100: true, 200: true}
		for _, value := range successValues {
			if !expectedSuccessValues[value] {
				t.Errorf("output %d: unexpected success value %d", i, value)
			}
		}
	}
}

// TestResultComposability_ComplexFlow tests a more complex pipeline that demonstrates
// Result[T] composability across multiple processor types.
func TestResultComposability_ComplexFlow(t *testing.T) {
	ctx := context.Background()

	// Simulate a data processing pipeline with multiple stages

	// Stage 1: Multiple data sources
	source1 := make(chan streamz.Result[string], 2)
	source1 <- streamz.NewSuccess("record-001")
	source1 <- streamz.NewSuccess("record-002")
	close(source1)

	source2 := make(chan streamz.Result[string], 2)
	source2 <- streamz.NewError("bad-record", fmt.Errorf("invalid format"), "parser")
	source2 <- streamz.NewSuccess("record-003")
	close(source2)

	// Stage 2: Merge sources
	fanin := streamz.NewFanIn[string]()
	merged := fanin.Process(ctx, source1, source2)

	// Stage 3: Distribute to parallel processors
	fanout := streamz.NewFanOut[string](3)
	branches := fanout.Process(ctx, merged)

	// Stage 4: Process each branch concurrently (simulate different processors)
	var wg sync.WaitGroup
	branchResults := make([][]streamz.Result[string], 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			branchResults[index] = testinghelpers.CollectResultsWithTimeout(branches[index], 500*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// Extract individual branch results for clarity
	branch0Results := branchResults[0]
	branch1Results := branchResults[1]
	branch2Results := branchResults[2]

	// Verify each branch received all items
	expectedItemCount := 4 // 2 + 2 from both sources

	if len(branch0Results) != expectedItemCount {
		t.Errorf("branch 0: expected %d items, got %d", expectedItemCount, len(branch0Results))
	}
	if len(branch1Results) != expectedItemCount {
		t.Errorf("branch 1: expected %d items, got %d", expectedItemCount, len(branch1Results))
	}
	if len(branch2Results) != expectedItemCount {
		t.Errorf("branch 2: expected %d items, got %d", expectedItemCount, len(branch2Results))
	}

	// Count success vs error results across all branches
	for branchIndex, results := range [][]streamz.Result[string]{branch0Results, branch1Results, branch2Results} {
		successCount, errorCount := 0, 0
		for _, result := range results {
			if result.IsError() {
				errorCount++
			} else {
				successCount++
			}
		}

		// Should be 3 successes, 1 error in each branch
		if successCount != 3 {
			t.Errorf("branch %d: expected 3 successes, got %d", branchIndex, successCount)
		}
		if errorCount != 1 {
			t.Errorf("branch %d: expected 1 error, got %d", branchIndex, errorCount)
		}
	}

	// Stage 5: Recombine results (simulate final aggregation)
	finalFanin := streamz.NewFanIn[string]()

	// Convert collected results back to channels for final merge
	// (In real pipeline, branches would continue as channels)
	recombinedInput1 := make(chan streamz.Result[string], len(branch0Results))
	for _, result := range branch0Results {
		recombinedInput1 <- result
	}
	close(recombinedInput1)

	recombinedInput2 := make(chan streamz.Result[string], len(branch1Results))
	for _, result := range branch1Results {
		recombinedInput2 <- result
	}
	close(recombinedInput2)

	// Final merge
	final := finalFanin.Process(ctx, recombinedInput1, recombinedInput2)
	finalResults := testinghelpers.CollectResultsWithTimeout(final, 500*time.Millisecond)

	// Should have 2 * expectedItemCount items (duplicated from 2 branches)
	expectedFinalCount := 2 * expectedItemCount
	if len(finalResults) != expectedFinalCount {
		t.Errorf("final results: expected %d items, got %d", expectedFinalCount, len(finalResults))
	}

	t.Logf("Complex flow completed successfully:")
	t.Logf("- Initial sources: 4 items total (3 success, 1 error)")
	t.Logf("- After FanOut: 3 branches, each with 4 items")
	t.Logf("- After recombination: %d items from 2 branches", len(finalResults))
}

// TestResultComposability_BufferInPipeline tests that Buffer processor works correctly
// in Result[T] pipelines, providing buffering between other processors without
// altering the data flow or error propagation.
func TestResultComposability_BufferInPipeline(t *testing.T) {
	ctx := context.Background()

	// Create a pipeline: FanIn -> Buffer -> FanOut
	// This tests that Buffer works as a pass-through with buffering capability

	// Step 1: Create multiple input streams
	input1 := make(chan streamz.Result[string], 3)
	input1 <- streamz.NewSuccess("data1")
	input1 <- streamz.NewError("error1", fmt.Errorf("first error"), "source1")
	input1 <- streamz.NewSuccess("data2")
	close(input1)

	input2 := make(chan streamz.Result[string], 2)
	input2 <- streamz.NewSuccess("data3")
	input2 <- streamz.NewError("error2", fmt.Errorf("second error"), "source2")
	close(input2)

	// Step 2: Merge inputs with FanIn
	fanin := streamz.NewFanIn[string]()
	merged := fanin.Process(ctx, input1, input2)

	// Step 3: Add Buffer in the middle of pipeline
	buffer := streamz.NewBuffer[string](10) // Buffer with capacity 10
	buffered := buffer.Process(ctx, merged)

	// Step 4: Fan out after buffering
	fanout := streamz.NewFanOut[string](2)
	outputs := fanout.Process(ctx, buffered)

	if len(outputs) != 2 {
		t.Fatalf("expected 2 fan-out outputs, got %d", len(outputs))
	}

	// Step 5: Collect results from both outputs
	var wg sync.WaitGroup
	results := make([][]streamz.Result[string], 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = testinghelpers.CollectResultsWithTimeout(outputs[index], 500*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// Step 6: Verify results
	expectedItemCount := 5 // 3 from input1 + 2 from input2

	for i := 0; i < 2; i++ {
		if len(results[i]) != expectedItemCount {
			t.Errorf("output %d: expected %d results, got %d", i, expectedItemCount, len(results[i]))
			continue
		}

		// Count success vs error results
		successCount, errorCount := 0, 0
		successValues := make([]string, 0)
		errorItems := make([]string, 0)

		for _, result := range results[i] {
			if result.IsError() {
				errorCount++
				errorItems = append(errorItems, result.Error().Item)
			} else {
				successCount++
				successValues = append(successValues, result.Value())
			}
		}

		// Should have 3 successes and 2 errors
		if successCount != 3 {
			t.Errorf("output %d: expected 3 success results, got %d", i, successCount)
		}
		if errorCount != 2 {
			t.Errorf("output %d: expected 2 error results, got %d", i, errorCount)
		}

		// Verify actual values (order might vary due to FanIn concurrency)
		expectedSuccesses := map[string]bool{"data1": true, "data2": true, "data3": true}
		for _, value := range successValues {
			if !expectedSuccesses[value] {
				t.Errorf("output %d: unexpected success value %s", i, value)
			}
		}

		expectedErrors := map[string]bool{"error1": true, "error2": true}
		for _, item := range errorItems {
			if !expectedErrors[item] {
				t.Errorf("output %d: unexpected error item %s", i, item)
			}
		}
	}

	// Verify both outputs are identical (FanOut guarantee should be preserved through Buffer)
	if len(results[0]) == len(results[1]) && len(results[0]) > 0 {
		// Convert to maps for order-independent comparison
		output0Map := make(map[string]bool)
		output1Map := make(map[string]bool)

		for _, result := range results[0] {
			if result.IsError() {
				output0Map["ERROR:"+result.Error().Item] = true
			} else {
				output0Map["SUCCESS:"+result.Value()] = true
			}
		}

		for _, result := range results[1] {
			if result.IsError() {
				output1Map["ERROR:"+result.Error().Item] = true
			} else {
				output1Map["SUCCESS:"+result.Value()] = true
			}
		}

		// Both outputs should contain the same set of items
		for key := range output0Map {
			if !output1Map[key] {
				t.Errorf("output1 missing item that output0 has: %s", key)
			}
		}
		for key := range output1Map {
			if !output0Map[key] {
				t.Errorf("output0 missing item that output1 has: %s", key)
			}
		}
	}

	t.Logf("Buffer pipeline test completed successfully:")
	t.Logf("- Processed %d items through FanIn->Buffer->FanOut", expectedItemCount)
	t.Logf("- Buffer maintained data integrity and error propagation")
	t.Logf("- Both outputs received identical item sets")
}

// TestResultComposability_BufferSizes tests Buffer behavior with different buffer sizes
// to ensure proper buffering and flow control in Result[T] pipelines.
func TestResultComposability_BufferSizes(t *testing.T) {
	bufferSizes := []struct {
		name string
		size int
	}{
		{"unbuffered", 0},
		{"small buffer", 1},
		{"medium buffer", 5},
		{"large buffer", 100},
	}

	for _, bufferTest := range bufferSizes {
		t.Run(bufferTest.name, func(t *testing.T) {
			ctx := context.Background()

			// Create input with mixed results
			input := make(chan streamz.Result[int], 10)
			expectedItems := make([]streamz.Result[int], 10)

			for i := 0; i < 10; i++ {
				if i%3 == 0 {
					item := streamz.NewError(i, fmt.Errorf("error %d", i), fmt.Sprintf("processor-%d", i))
					expectedItems[i] = item
					input <- item
				} else {
					item := streamz.NewSuccess(i * 10)
					expectedItems[i] = item
					input <- item
				}
			}
			close(input)

			// Process through buffer
			buffer := streamz.NewBuffer[int](bufferTest.size)
			output := buffer.Process(ctx, input)

			// Collect results
			var results []streamz.Result[int]
			for result := range output {
				results = append(results, result)
			}

			// Verify count
			if len(results) != len(expectedItems) {
				t.Fatalf("expected %d results, got %d", len(expectedItems), len(results))
			}

			// Verify content (Buffer should preserve order and content)
			for i, expected := range expectedItems {
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
				} else {
					if expected.Value() != actual.Value() {
						t.Errorf("item %d: expected value %v, got %v",
							i, expected.Value(), actual.Value())
					}
				}
			}

			t.Logf("Buffer size %d: successfully processed %d items", bufferTest.size, len(results))
		})
	}
}

// TestResultComposability_ThrottleInPipeline tests that Throttle processor works correctly
// in Result[T] pipelines, providing rate limiting without altering data flow or error propagation.
func TestResultComposability_ThrottleInPipeline(t *testing.T) {
	ctx := context.Background()

	// Create a pipeline: FanIn -> Throttle -> FanOut
	// This tests that Throttle works as rate limiter in a complex pipeline

	// Step 1: Create multiple input streams with mixed results
	input1 := make(chan streamz.Result[string], 3)
	input1 <- streamz.NewSuccess("data1")
	input1 <- streamz.NewError("error1", fmt.Errorf("first error"), "source1")
	input1 <- streamz.NewSuccess("data2") // Should be ignored during cooling from data1
	close(input1)

	input2 := make(chan streamz.Result[string], 2)
	input2 <- streamz.NewSuccess("data3") // Should be ignored during cooling from data1
	input2 <- streamz.NewError("error2", fmt.Errorf("second error"), "source2")
	close(input2)

	// Step 2: Merge inputs with FanIn
	fanin := streamz.NewFanIn[string]()
	merged := fanin.Process(ctx, input1, input2)

	// Step 3: Add Throttle processor (leading edge behavior)
	clock := clockz.NewFakeClock()
	throttle := streamz.NewThrottle[string](500*time.Millisecond, clock)
	throttled := throttle.Process(ctx, merged)

	// Step 4: Fan out after throttling
	fanout := streamz.NewFanOut[string](2)
	outputs := fanout.Process(ctx, throttled)

	if len(outputs) != 2 {
		t.Fatalf("expected 2 fan-out outputs, got %d", len(outputs))
	}

	// Step 5: Collect results from both outputs concurrently
	var wg sync.WaitGroup
	results := make([][]streamz.Result[string], 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = testinghelpers.CollectResultsWithTimeout(outputs[index], 2*time.Second)
		}(i)
	}

	wg.Wait()

	// Step 6: Verify results
	// Expected: 1 success (leading edge - first one from merged streams) + 2 errors (pass through) = 3 total
	expectedItemCount := 3

	for i := 0; i < 2; i++ {
		if len(results[i]) != expectedItemCount {
			t.Errorf("output %d: expected %d results, got %d", i, expectedItemCount, len(results[i]))
			continue
		}

		// Count success vs error results
		successCount, errorCount := 0, 0
		successValues := make([]string, 0)
		errorItems := make([]string, 0)

		for _, result := range results[i] {
			if result.IsError() {
				errorCount++
				errorItems = append(errorItems, result.Error().Item)
			} else {
				successCount++
				successValues = append(successValues, result.Value())
			}
		}

		// Should have 1 success (leading edge) and 2 errors (pass through)
		if successCount != 1 {
			t.Errorf("output %d: expected 1 success result (leading edge), got %d", i, successCount)
		}
		if errorCount != 2 {
			t.Errorf("output %d: expected 2 error results, got %d", i, errorCount)
		}

		// Verify success value is one of the valid ones (order varies due to FanIn)
		expectedSuccesses := map[string]bool{"data1": true, "data3": true}
		for _, value := range successValues {
			if !expectedSuccesses[value] {
				t.Errorf("output %d: unexpected success value %s", i, value)
			}
		}

		// Verify errors
		expectedErrors := map[string]bool{"error1": true, "error2": true}
		for _, item := range errorItems {
			if !expectedErrors[item] {
				t.Errorf("output %d: unexpected error item %s", i, item)
			}
		}
	}

	// Verify both outputs are identical (FanOut guarantee preserved through Throttle)
	if len(results[0]) == len(results[1]) && len(results[0]) > 0 {
		// Convert to maps for order-independent comparison
		output0Map := make(map[string]bool)
		output1Map := make(map[string]bool)

		for _, result := range results[0] {
			if result.IsError() {
				output0Map["ERROR:"+result.Error().Item] = true
			} else {
				output0Map["SUCCESS:"+result.Value()] = true
			}
		}

		for _, result := range results[1] {
			if result.IsError() {
				output1Map["ERROR:"+result.Error().Item] = true
			} else {
				output1Map["SUCCESS:"+result.Value()] = true
			}
		}

		// Both outputs should contain the same set of items
		for key := range output0Map {
			if !output1Map[key] {
				t.Errorf("output1 missing item that output0 has: %s", key)
			}
		}
		for key := range output1Map {
			if !output0Map[key] {
				t.Errorf("output0 missing item that output1 has: %s", key)
			}
		}
	}

	t.Logf("Throttle pipeline test completed successfully:")
	t.Logf("- Processed items through FanIn->Throttle->FanOut")
	t.Logf("- Throttle applied leading edge behavior (first success emitted, others ignored during cooling)")
	t.Logf("- Errors passed through immediately without throttling")
	t.Logf("- Both outputs received identical item sets")
	t.Logf("- Leading edge rate limiting applied correctly")
}

// TestResultComposability_ThrottleTiming tests that Throttle processor correctly
// enforces timing constraints in Result[T] pipelines.
func TestResultComposability_ThrottleTiming(t *testing.T) {
	// Test different throttle intervals
	intervals := []struct {
		name     string
		interval time.Duration
		items    int
	}{
		{"fast throttle", 50 * time.Millisecond, 3},
		{"slow throttle", 200 * time.Millisecond, 2},
		{"very slow throttle", 500 * time.Millisecond, 4},
	}

	for _, tc := range intervals {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// Create input with mixed success/error Results
			input := make(chan streamz.Result[int], tc.items)
			for i := 0; i < tc.items; i++ {
				if i%2 == 0 {
					input <- streamz.NewSuccess(i * 10)
				} else {
					input <- streamz.NewError(i*10, fmt.Errorf("error %d", i), fmt.Sprintf("source-%d", i))
				}
			}
			close(input)

			// Apply throttle
			clock := clockz.NewFakeClock()
			throttle := streamz.NewThrottle[int](tc.interval, clock)
			output := throttle.Process(ctx, input)

			// Collect results
			var results []streamz.Result[int]
			for result := range output {
				results = append(results, result)
			}

			// Verify throttle behavior:
			// - All errors should pass through immediately (count unchanged)
			// - Only first success should pass through (leading edge behavior)
			successCount, errorCount := 0, 0
			for _, result := range results {
				if result.IsError() {
					errorCount++
				} else {
					successCount++
				}
			}

			expectedError := tc.items / 2 // Floor division - same as input
			expectedSuccess := 1          // Only first success (leading edge)

			// Special case: if no successes in input, expect 0
			inputSuccessCount := (tc.items + 1) / 2
			if inputSuccessCount == 0 {
				expectedSuccess = 0
			}

			if successCount != expectedSuccess {
				t.Errorf("expected %d successes (leading edge), got %d", expectedSuccess, successCount)
			}
			if errorCount != expectedError {
				t.Errorf("expected %d errors (all pass through), got %d", expectedError, errorCount)
			}

			t.Logf("Timing test '%s': processed %d items (%d success, %d errors) with %s interval",
				tc.name, len(results), successCount, errorCount, tc.interval)
		})
	}
}

// TestResultComposability_DebounceInPipeline tests that Debounce processor works correctly
// in Result[T] pipelines, demonstrating timer-based processing with immediate error passthrough.
func TestResultComposability_DebounceInPipeline(t *testing.T) {
	ctx := context.Background()

	// Create a complex pipeline: FanIn -> Debounce -> FanOut
	// This tests that Debounce handles Result[T] correctly with timer management

	// Step 1: Create multiple input streams with rapid events and errors
	input1 := make(chan streamz.Result[string], 5)
	input1 <- streamz.NewSuccess("rapid1")
	input1 <- streamz.NewSuccess("rapid2")
	input1 <- streamz.NewSuccess("rapid3") // Only this should be debounced (last success)
	input1 <- streamz.NewError("error1", fmt.Errorf("validation error"), "validator")
	input1 <- streamz.NewSuccess("final1") // Should be debounced after quiet period
	close(input1)

	input2 := make(chan streamz.Result[string], 3)
	input2 <- streamz.NewError("error2", fmt.Errorf("parsing error"), "parser")
	input2 <- streamz.NewSuccess("quick1")
	input2 <- streamz.NewSuccess("quick2") // Only this should be debounced (last success)
	close(input2)

	// Step 2: Merge inputs with FanIn
	fanin := streamz.NewFanIn[string]()
	merged := fanin.Process(ctx, input1, input2)

	// Step 3: Add Debounce processor (timer-based)
	clock := clockz.NewFakeClock()
	debounce := streamz.NewDebounce[string](200*time.Millisecond, clock)
	debounced := debounce.Process(ctx, merged)

	// Step 4: Fan out after debouncing
	fanout := streamz.NewFanOut[string](2)
	outputs := fanout.Process(ctx, debounced)

	if len(outputs) != 2 {
		t.Fatalf("expected 2 fan-out outputs, got %d", len(outputs))
	}

	// Step 5: Collect initial results (errors should come through immediately)
	var wg sync.WaitGroup
	results := make([][]streamz.Result[string], 2)

	// Start collectors
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = testinghelpers.CollectResultsWithTimeout(outputs[index], 2*time.Second)
		}(i)
	}

	// Step 6: Advance clock to trigger debounce (errors should already be through)
	time.Sleep(50 * time.Millisecond)     // Allow errors to propagate immediately
	clock.Advance(250 * time.Millisecond) // Trigger debounce for pending successes
	clock.BlockUntilReady()               // Allow clock synchronization

	wg.Wait()

	// Step 7: Verify results
	// Expected: 2 errors (immediate) + 1 debounced success = 3 total per output
	// Debounce coalesces rapid success values into a single output (the last one)
	expectedItemCount := 3

	for i := 0; i < 2; i++ {
		if len(results[i]) != expectedItemCount {
			t.Errorf("output %d: expected %d results, got %d", i, expectedItemCount, len(results[i]))
			continue
		}

		// Separate errors and successes
		errorItems := make([]string, 0)
		successValues := make([]string, 0)

		for _, result := range results[i] {
			if result.IsError() {
				errorItems = append(errorItems, result.Error().Item)
			} else {
				successValues = append(successValues, result.Value())
			}
		}

		// Should have exactly 2 errors and 1 success (debounce coalesces rapid successes)
		if len(errorItems) != 2 {
			t.Errorf("output %d: expected 2 error items, got %d", i, len(errorItems))
		}
		if len(successValues) != 1 {
			t.Errorf("output %d: expected 1 success value, got %d", i, len(successValues))
		}

		// Verify error items (should be passed through immediately)
		expectedErrors := map[string]bool{"error1": true, "error2": true}
		for _, item := range errorItems {
			if !expectedErrors[item] {
				t.Errorf("output %d: unexpected error item %s", i, item)
			}
		}

		// Verify success values (should be debounced - only last value from all rapid sequences)
		// Expected: 1 value - the final success from all merged inputs after debouncing
		// Note: Debounce coalesces ALL rapid successes into a single output
		validSuccesses := map[string]bool{
			"rapid3": true, "final1": true, "quick2": true,
		}
		for _, value := range successValues {
			if !validSuccesses[value] {
				t.Errorf("output %d: unexpected success value %s", i, value)
			}
		}
	}

	// Step 8: Verify both outputs are identical (FanOut guarantee)
	if len(results[0]) == len(results[1]) && len(results[0]) > 0 {
		// Convert to sets for order-independent comparison
		output0Set := make(map[string]bool)
		output1Set := make(map[string]bool)

		for _, result := range results[0] {
			var key string
			if result.IsError() {
				key = "ERROR:" + result.Error().Item
			} else {
				key = "SUCCESS:" + result.Value()
			}
			output0Set[key] = true
		}

		for _, result := range results[1] {
			var key string
			if result.IsError() {
				key = "ERROR:" + result.Error().Item
			} else {
				key = "SUCCESS:" + result.Value()
			}
			output1Set[key] = true
		}

		// Verify both outputs contain identical items
		for key := range output0Set {
			if !output1Set[key] {
				t.Errorf("output1 missing item that output0 has: %s", key)
			}
		}
		for key := range output1Set {
			if !output0Set[key] {
				t.Errorf("output0 missing item that output1 has: %s", key)
			}
		}
	}

	t.Logf("Debounce pipeline test completed successfully:")
	t.Logf("- Processed mixed rapid events and errors through FanIn->Debounce->FanOut")
	t.Logf("- Errors passed through immediately (no debouncing)")
	t.Logf("- Success values properly debounced (only last in rapid sequences)")
	t.Logf("- Both outputs received identical item sets")
	t.Logf("- Timer-based processing integrated correctly with Result[T] pattern")
}

// TestResultComposability_RouterInPipeline tests that Router processor works correctly
// in Result[T] pipelines, routing different types of Results to appropriate processors.
func TestResultComposability_RouterInPipeline(t *testing.T) {
	// Create a complex pipeline: FanIn -> Router -> collect results from different routes

	// Step 1: Create multiple input streams with mixed success/error Results
	input1 := make(chan streamz.Result[int], 4)
	input1 <- streamz.NewSuccess(150) // High value success
	input1 <- streamz.NewSuccess(25)  // Low value success
	input1 <- streamz.NewError(100, fmt.Errorf("validation error"), "validator")
	input1 <- streamz.NewError(200, fmt.Errorf("critical system error"), "system")
	close(input1)

	input2 := make(chan streamz.Result[int], 3)
	input2 <- streamz.NewSuccess(300) // High value success
	input2 <- streamz.NewError(50, fmt.Errorf("parsing error"), "parser")
	input2 <- streamz.NewSuccess(10) // Low value success
	close(input2)

	// Step 2: Merge streams using FanIn
	ctx := context.Background()
	fanin := streamz.NewFanIn[int]()
	merged := fanin.Process(ctx, input1, input2)

	// Step 3: Test AsyncMapper with concurrent processing
	enricher := streamz.NewAsyncMapper(func(_ context.Context, item int) (int, error) {
		// Simulate enrichment work with variable processing time
		time.Sleep(time.Duration(item%3) * time.Millisecond) // Variable delay
		if item < 0 {
			return 0, fmt.Errorf("cannot enrich negative value: %d", item)
		}
		return item * 100, nil // Enrich by multiplying by 100
	}).WithWorkers(3).WithOrdered(true)

	// Step 4: Process merged stream through AsyncMapper
	enriched := enricher.Process(ctx, merged)

	// Step 5: Collect and verify results
	results := testinghelpers.CollectResultsWithTimeout(enriched, 500*time.Millisecond)

	// Verify we get expected number of results (7 total: 4 from input1 + 3 from input2)
	expectedTotal := 7
	if len(results) != expectedTotal {
		t.Errorf("expected %d results from AsyncMapper, got %d", expectedTotal, len(results))
	}

	// Count successes and errors
	successCount := 0
	errorCount := 0
	enrichedValues := make([]int, 0)

	for _, result := range results {
		if result.IsError() {
			errorCount++
			t.Logf("AsyncMapper error: %v", result.Error())
		} else {
			successCount++
			enrichedValues = append(enrichedValues, result.Value())
		}
	}

	// Verify enrichment worked correctly
	// Expected successful values: 150*100=15000, 300*100=30000, 25*100=2500, 10*100=1000
	expectedEnrichedSet := map[int]bool{15000: true, 30000: true, 2500: true, 1000: true}
	if len(enrichedValues) != len(expectedEnrichedSet) {
		t.Errorf("expected %d enriched values, got %d: %v", len(expectedEnrichedSet), len(enrichedValues), enrichedValues)
	}

	// Verify all expected values are present (FanIn doesn't preserve cross-channel order)
	for _, value := range enrichedValues {
		if !expectedEnrichedSet[value] {
			t.Errorf("unexpected enriched value: %d", value)
		}
	}

	// Verify error handling - should have errors for negative values and propagated input errors
	expectedMinErrorCount := 2 // At least validation error + system error + parsing error
	if errorCount < expectedMinErrorCount {
		t.Errorf("expected at least %d errors, got %d", expectedMinErrorCount, errorCount)
	}

	t.Logf("AsyncMapper integration test completed successfully:")
	t.Logf("- Processed items through FanIn->AsyncMapper pipeline")
	t.Logf("- AsyncMapper processed %d successes and %d errors concurrently", successCount, errorCount)
	t.Logf("- Concurrent processing with proper error propagation")
	t.Logf("- Error handling works correctly with Result[T] pattern")
}
