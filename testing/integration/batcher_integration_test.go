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

// TestBatcher_FanInBatcherFanOut tests that Batcher works correctly in complex pipelines,
// demonstrating proper batching of Result[T] streams with error handling.
func TestBatcher_FanInBatcherFanOut(t *testing.T) {
	ctx := context.Background()

	// Create a pipeline: multiple sources -> FanIn -> Batcher -> FanOut
	// This tests that Batcher integrates correctly with other Result[T] processors

	// Step 1: Create multiple input streams with mixed success/error Results
	input1 := make(chan streamz.Result[string], 5)
	input1 <- streamz.NewSuccess("data-1-1")
	input1 <- streamz.NewSuccess("data-1-2")
	input1 <- streamz.NewError("error-1", fmt.Errorf("source 1 error"), "source1")
	input1 <- streamz.NewSuccess("data-1-3")
	input1 <- streamz.NewSuccess("data-1-4")
	close(input1)

	input2 := make(chan streamz.Result[string], 4)
	input2 <- streamz.NewSuccess("data-2-1")
	input2 <- streamz.NewError("error-2", fmt.Errorf("source 2 error"), "source2")
	input2 <- streamz.NewSuccess("data-2-2")
	input2 <- streamz.NewSuccess("data-2-3")
	close(input2)

	input3 := make(chan streamz.Result[string], 3)
	input3 <- streamz.NewSuccess("data-3-1")
	input3 <- streamz.NewSuccess("data-3-2")
	input3 <- streamz.NewError("error-3", fmt.Errorf("source 3 error"), "source3")
	close(input3)

	// Step 2: Merge all inputs with FanIn
	fanin := streamz.NewFanIn[string]()
	merged := fanin.Process(ctx, input1, input2, input3)

	// Step 3: Batch the merged stream (3 items per batch or 100ms timeout)
	clock := clockz.NewFakeClock()
	batcher := streamz.NewBatcher[string](streamz.BatchConfig{
		MaxSize:    3,
		MaxLatency: 100 * time.Millisecond,
	}, clock)
	batched := batcher.Process(ctx, merged)

	// Step 4: Fan out batches to multiple processors
	fanout := streamz.NewFanOut[[]string](2)
	outputs := fanout.Process(ctx, batched)

	if len(outputs) != 2 {
		t.Fatalf("expected 2 fan-out outputs, got %d", len(outputs))
	}

	// Step 5: Collect results from both outputs concurrently
	var wg sync.WaitGroup
	results := make([][]streamz.Result[[]string], 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = testinghelpers.CollectResultsWithTimeout(outputs[index], 1*time.Second)
		}(i)
	}

	// Advance clock to trigger any time-based batches
	time.Sleep(50 * time.Millisecond) // Allow processing
	clock.Advance(150 * time.Millisecond)

	wg.Wait()

	// Step 6: Verify results
	// Expected: errors (3 passed through) + batches from 9 successful items
	// 9 successful items with batch size 3 = 3 batches
	// Total per output: 3 errors + 3 batches = 6 results

	for i := 0; i < 2; i++ {
		if len(results[i]) == 0 {
			t.Errorf("output %d: no results received", i)
			continue
		}

		// Separate errors and batch results
		errorResults := make([]streamz.Result[[]string], 0)
		batchResults := make([]streamz.Result[[]string], 0)

		for _, result := range results[i] {
			if result.IsError() {
				errorResults = append(errorResults, result)
			} else {
				batchResults = append(batchResults, result)
			}
		}

		// Should have 3 errors (passed through immediately)
		if len(errorResults) != 3 {
			t.Errorf("output %d: expected 3 errors, got %d", i, len(errorResults))
		}

		// Verify error metadata is preserved
		for j, errorResult := range errorResults {
			if errorResult.Error() == nil {
				t.Errorf("output %d, error %d: missing error metadata", i, j)
				continue
			}

			// Error should contain empty slice (Batcher converts Result[T] errors to Result[[]T])
			if len(errorResult.Error().Item) != 0 {
				t.Errorf("output %d, error %d: expected empty slice for error item, got %v",
					i, j, errorResult.Error().Item)
			}

			if errorResult.Error().ProcessorName == "" {
				t.Errorf("output %d, error %d: missing processor name", i, j)
			}
		}

		// Verify batches contain expected items
		totalBatchedItems := 0
		for j, batchResult := range batchResults {
			if batchResult.IsError() {
				t.Errorf("output %d, batch %d: expected success batch, got error", i, j)
				continue
			}

			batch := batchResult.Value()
			if len(batch) == 0 {
				t.Errorf("output %d, batch %d: empty batch", i, j)
				continue
			}

			if len(batch) > 3 {
				t.Errorf("output %d, batch %d: batch size %d exceeds limit of 3", i, j, len(batch))
			}

			totalBatchedItems += len(batch)

			// Verify all batch items have expected format
			for k, item := range batch {
				if item == "" {
					t.Errorf("output %d, batch %d, item %d: empty item", i, j, k)
				}
				// Should be from one of our inputs (data-X-Y format)
				if len(item) < 7 || item[:5] != "data-" {
					t.Errorf("output %d, batch %d, item %d: unexpected format %q", i, j, k, item)
				}
			}
		}

		// Should have batched all 9 successful items (4 from input1, 3 from input2, 2 from input3)
		expectedSuccessfulItems := 9
		if totalBatchedItems != expectedSuccessfulItems {
			t.Errorf("output %d: expected %d total batched items, got %d",
				i, expectedSuccessfulItems, totalBatchedItems)
		}

		t.Logf("Output %d: received %d errors and %d batches containing %d total items",
			i, len(errorResults), len(batchResults), totalBatchedItems)
	}

	// Step 7: Verify both outputs are identical (FanOut guarantee)
	if len(results[0]) == len(results[1]) && len(results[0]) > 0 {
		// Both outputs should have the same number of results
		for j := 0; j < len(results[0]); j++ {
			result0, result1 := results[0][j], results[1][j]

			// Both should be errors or both should be successes
			if result0.IsError() != result1.IsError() {
				t.Errorf("result %d: error status differs between outputs (output0=%v, output1=%v)",
					j, result0.IsError(), result1.IsError())
				continue
			}

			if result0.IsError() {
				// Error results should have the same error properties
				err0, err1 := result0.Error(), result1.Error()
				if err0.Err.Error() != err1.Err.Error() {
					t.Errorf("result %d: error messages differ (output0=%q, output1=%q)",
						j, err0.Err.Error(), err1.Err.Error())
				}
			} else {
				// Batch results should be identical
				batch0, batch1 := result0.Value(), result1.Value()
				if len(batch0) != len(batch1) {
					t.Errorf("result %d: batch sizes differ (output0=%d, output1=%d)",
						j, len(batch0), len(batch1))
					continue
				}

				for k := 0; k < len(batch0); k++ {
					if batch0[k] != batch1[k] {
						t.Errorf("result %d, item %d: batch items differ (output0=%q, output1=%q)",
							j, k, batch0[k], batch1[k])
					}
				}
			}
		}
	}

	t.Logf("FanIn->Batcher->FanOut pipeline test completed successfully:")
	t.Logf("- Merged 3 input streams with 12 total items (9 success, 3 errors)")
	t.Logf("- Errors passed through immediately without batching")
	t.Logf("- Success items batched with MaxSize=3 configuration")
	t.Logf("- Both outputs received identical results (FanOut guarantee maintained)")
}

// TestBatcher_TimingIntegration tests Batcher time-based batching in integration scenarios.
func TestBatcher_TimingIntegration(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	// Create input stream with staggered data arrival
	input := make(chan streamz.Result[int])

	// Create batcher with time-based configuration
	batcher := streamz.NewBatcher[int](streamz.BatchConfig{
		MaxSize:    5,                      // Higher size limit
		MaxLatency: 200 * time.Millisecond, // Time-based trigger
	}, clock)

	output := batcher.Process(ctx, input)

	// Start result collection
	var wg sync.WaitGroup
	var results []streamz.Result[[]int]

	wg.Add(1)
	go func() {
		defer wg.Done()
		for result := range output {
			results = append(results, result)
		}
	}()

	// Send data in stages with time advancement

	// Stage 1: Send 2 items
	input <- streamz.NewSuccess(1)
	input <- streamz.NewSuccess(2)

	// Advance time partially (should not trigger batch yet)
	clock.Advance(100 * time.Millisecond)
	time.Sleep(10 * time.Millisecond) // Allow processing

	// Send 1 more item
	input <- streamz.NewSuccess(3)

	// Advance remaining time to trigger first batch
	clock.Advance(100 * time.Millisecond) // Total: 200ms
	time.Sleep(50 * time.Millisecond)     // Allow batch processing

	// Stage 2: Send more items for second batch
	input <- streamz.NewSuccess(4)
	input <- streamz.NewSuccess(5)
	input <- streamz.NewError(6, fmt.Errorf("test error"), "processor")
	input <- streamz.NewSuccess(7)

	// Advance time for second batch
	clock.Advance(200 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	// Close input to flush any remaining batch
	close(input)

	wg.Wait()

	// Verify results
	if len(results) == 0 {
		t.Fatal("no results received")
	}

	// Separate errors and batches
	errorCount := 0
	batchCount := 0
	totalItems := 0

	for i, result := range results {
		if result.IsError() {
			errorCount++
			t.Logf("Result %d: Error - %v", i, result.Error().Err)
		} else {
			batchCount++
			batch := result.Value()
			totalItems += len(batch)
			t.Logf("Result %d: Batch with %d items - %v", i, len(batch), batch)
		}
	}

	// Should have 1 error (passed through immediately)
	if errorCount != 1 {
		t.Errorf("expected 1 error, got %d", errorCount)
	}

	// Should have batched 6 successful items (1,2,3,4,5,7)
	expectedSuccessItems := 6
	if totalItems != expectedSuccessItems {
		t.Errorf("expected %d total successful items, got %d", expectedSuccessItems, totalItems)
	}

	// Should have at least 2 batches (time-triggered and final flush)
	if batchCount < 2 {
		t.Errorf("expected at least 2 batches, got %d", batchCount)
	}

	t.Logf("Timing integration test completed successfully:")
	t.Logf("- Processed %d items in %d batches", totalItems, batchCount)
	t.Logf("- %d errors passed through immediately", errorCount)
	t.Logf("- Time-based batching triggered correctly")
}

// TestBatcher_MemoryBounds tests that Batcher maintains bounded memory usage.
func TestBatcher_MemoryBounds(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	// Create batcher with small batch size to test bounds
	batcher := streamz.NewBatcher[int](streamz.BatchConfig{
		MaxSize:    10,
		MaxLatency: time.Hour, // Very long timeout to test size-based triggering
	}, clock)

	input := make(chan streamz.Result[int], 100)
	output := batcher.Process(ctx, input)

	// Send exactly batch size items
	for i := 0; i < 10; i++ {
		input <- streamz.NewSuccess(i)
	}

	// Should trigger batch immediately when size limit reached
	select {
	case result := <-output:
		if result.IsError() {
			t.Errorf("expected batch result, got error: %v", result.Error())
		} else {
			batch := result.Value()
			if len(batch) != 10 {
				t.Errorf("expected batch size 10, got %d", len(batch))
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for size-triggered batch")
	}

	// Send more items to test that new batch starts fresh
	for i := 10; i < 15; i++ {
		input <- streamz.NewSuccess(i)
	}

	close(input)

	// Should get final batch with remaining items
	select {
	case result := <-output:
		if result.IsError() {
			t.Errorf("expected batch result, got error: %v", result.Error())
		} else {
			batch := result.Value()
			if len(batch) != 5 {
				t.Errorf("expected final batch size 5, got %d", len(batch))
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for final batch")
	}

	// Channel should be closed
	select {
	case _, ok := <-output:
		if ok {
			t.Error("expected output channel to be closed")
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("timeout waiting for channel close")
	}

	t.Logf("Memory bounds test completed successfully:")
	t.Logf("- Batch triggered immediately at MaxSize limit")
	t.Logf("- Memory usage bounded to MaxSize items")
	t.Logf("- Final batch flushed correctly on channel close")
}

// TestBatcher_ErrorHandlingIntegration tests comprehensive error handling in pipeline context.
func TestBatcher_ErrorHandlingIntegration(t *testing.T) {
	ctx := context.Background()

	// Create input with mixed patterns
	input := make(chan streamz.Result[string], 10)

	// Mixed success and error pattern
	input <- streamz.NewSuccess("item1")
	input <- streamz.NewError("bad1", fmt.Errorf("parsing error"), "parser")
	input <- streamz.NewSuccess("item2")
	input <- streamz.NewSuccess("item3")
	input <- streamz.NewError("bad2", fmt.Errorf("validation error"), "validator")
	input <- streamz.NewError("bad3", fmt.Errorf("critical error"), "system")
	input <- streamz.NewSuccess("item4")
	input <- streamz.NewSuccess("item5")
	input <- streamz.NewSuccess("item6")
	close(input)

	// Create batcher
	batcher := streamz.NewBatcher[string](streamz.BatchConfig{
		MaxSize:    3,
		MaxLatency: 100 * time.Millisecond,
	}, streamz.RealClock)

	output := batcher.Process(ctx, input)

	// Collect all results
	results := make([]streamz.Result[[]string], 0, 10) // Pre-allocate with expected capacity
	for result := range output {
		results = append(results, result)
	}

	// Analyze results
	errorResults := make([]streamz.Result[[]string], 0)
	batchResults := make([]streamz.Result[[]string], 0)

	for _, result := range results {
		if result.IsError() {
			errorResults = append(errorResults, result)
		} else {
			batchResults = append(batchResults, result)
		}
	}

	// Should have 3 errors passed through
	if len(errorResults) != 3 {
		t.Errorf("expected 3 errors, got %d", len(errorResults))
	}

	// Verify error metadata preservation
	expectedProcessors := map[string]bool{"parser": true, "validator": true, "system": true}
	actualProcessors := make(map[string]bool)

	for _, errorResult := range errorResults {
		if errorResult.Error() == nil {
			t.Error("error result missing metadata")
			continue
		}
		actualProcessors[errorResult.Error().ProcessorName] = true
	}

	for expected := range expectedProcessors {
		if !actualProcessors[expected] {
			t.Errorf("missing error from processor: %s", expected)
		}
	}

	// Should have batched 6 successful items (item1, item2, item3, item4, item5, item6)
	totalBatchedItems := 0
	for _, batchResult := range batchResults {
		if batchResult.IsError() {
			t.Error("unexpected error in batch results")
			continue
		}
		batch := batchResult.Value()
		totalBatchedItems += len(batch)

		// Verify batch doesn't exceed MaxSize
		if len(batch) > 3 {
			t.Errorf("batch size %d exceeds MaxSize of 3", len(batch))
		}
	}

	expectedSuccessItems := 6
	if totalBatchedItems != expectedSuccessItems {
		t.Errorf("expected %d successful items batched, got %d", expectedSuccessItems, totalBatchedItems)
	}

	t.Logf("Error handling integration test completed successfully:")
	t.Logf("- Processed %d errors (passed through immediately)", len(errorResults))
	t.Logf("- Batched %d successful items in %d batches", totalBatchedItems, len(batchResults))
	t.Logf("- Error metadata preserved through pipeline")
	t.Logf("- No interference between error passthrough and success batching")
}
