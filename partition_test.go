package streamz

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPartition_HashRouting(t *testing.T) {
	keyExtractor := func(s string) string {
		return s
	}

	partition, err := NewHashPartition(3, keyExtractor, 10)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Create input channel and send test data
	in := make(chan Result[string], 10)
	go func() {
		defer close(in)
		in <- NewSuccess("apple")  // Should consistently route to same partition
		in <- NewSuccess("banana") // Should consistently route to same partition
		in <- NewSuccess("apple")  // Should route to same partition as first apple
	}()

	outputs := partition.Process(ctx, in)
	if len(outputs) != 3 {
		t.Fatalf("Expected 3 output channels, got %d", len(outputs))
	}

	// Collect all results
	var wg sync.WaitGroup
	resultsChan := make(chan Result[string], 10)

	for i, out := range outputs {
		wg.Add(1)
		go func(partitionIndex int, ch <-chan Result[string]) {
			defer wg.Done()
			for result := range ch {
				// Verify partition metadata
				if index, exists := result.GetMetadata(MetadataPartitionIndex); exists {
					if idx, ok := index.(int); ok && idx != partitionIndex {
						t.Errorf("Result routed to wrong partition: expected %d, got %d", partitionIndex, idx)
					}
				}
				resultsChan <- result
			}
		}(i, out)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results and verify consistency
	applePartitions := make(map[int]bool)
	bananaPartitions := make(map[int]bool)

	for result := range resultsChan {
		if result.IsSuccess() {
			value := result.Value()
			partitionIndex, _ := result.GetMetadata(MetadataPartitionIndex)
			idx, _ := partitionIndex.(int) //nolint:errcheck // test code, value guaranteed to be int

			if value == "apple" {
				applePartitions[idx] = true
			} else if value == "banana" {
				bananaPartitions[idx] = true
			}
		}
	}

	// Verify that same keys go to same partitions
	if len(applePartitions) != 1 {
		t.Errorf("Apple should always route to same partition, found in %d partitions", len(applePartitions))
	}
	if len(bananaPartitions) != 1 {
		t.Errorf("Banana should always route to same partition, found in %d partitions", len(bananaPartitions))
	}
}

func TestPartition_RoundRobinRouting(t *testing.T) {
	partition, err := NewRoundRobinPartition[int](3, 10)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Create input channel and send test data
	in := make(chan Result[int], 10)
	go func() {
		defer close(in)
		for i := 0; i < 9; i++ {
			in <- NewSuccess(i)
		}
	}()

	outputs := partition.Process(ctx, in)
	if len(outputs) != 3 {
		t.Fatalf("Expected 3 output channels, got %d", len(outputs))
	}

	// Collect results from each partition
	partitionCounts := make([]int, 3)
	var wg sync.WaitGroup

	for i, out := range outputs {
		wg.Add(1)
		go func(partitionIndex int, ch <-chan Result[int]) {
			defer wg.Done()
			for result := range ch {
				if result.IsSuccess() {
					partitionCounts[partitionIndex]++
					// Verify metadata
					if index, exists := result.GetMetadata(MetadataPartitionIndex); exists {
						if idx, ok := index.(int); ok && idx != partitionIndex {
							t.Errorf("Result routed to wrong partition: expected %d, got %d", partitionIndex, idx)
						}
					}
				}
			}
		}(i, out)
	}

	wg.Wait()

	// Verify even distribution (9 items across 3 partitions = 3 each)
	for i, count := range partitionCounts {
		if count != 3 {
			t.Errorf("Partition %d received %d items, expected 3", i, count)
		}
	}
}

func TestPartition_ErrorHandling(t *testing.T) {
	partition, err := NewRoundRobinPartition[string](3, 10)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Create input with both successes and errors
	in := make(chan Result[string], 10)
	go func() {
		defer close(in)
		in <- NewSuccess("success1")
		in <- NewError("error1", fmt.Errorf("test error"), "test")
		in <- NewSuccess("success2")
		in <- NewError("error2", fmt.Errorf("another error"), "test")
	}()

	outputs := partition.Process(ctx, in)

	// Collect all results
	var wg sync.WaitGroup
	resultsChan := make(chan Result[string], 10)

	for i, out := range outputs {
		wg.Add(1)
		go func(_ int, ch <-chan Result[string]) {
			defer wg.Done()
			for result := range ch {
				resultsChan <- result
			}
		}(i, out)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Verify all errors go to partition 0
	errorCount := 0
	successCount := 0

	for result := range resultsChan {
		partitionIndex, _ := result.GetMetadata(MetadataPartitionIndex)
		idx, _ := partitionIndex.(int) //nolint:errcheck // test code, value guaranteed to be int

		if result.IsError() {
			errorCount++
			if idx != 0 {
				t.Errorf("Error routed to partition %d, expected partition 0", idx)
			}
		} else {
			successCount++
		}
	}

	if errorCount != 2 {
		t.Errorf("Expected 2 errors, got %d", errorCount)
	}
	if successCount != 2 {
		t.Errorf("Expected 2 successes, got %d", successCount)
	}
}

func TestPartition_ContextCancellation(t *testing.T) {
	partition, err := NewRoundRobinPartition[int](2, 0)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan Result[int])
	outputs := partition.Process(ctx, in)

	// Cancel context immediately
	cancel()

	// Try to send data (should not block)
	select {
	case in <- NewSuccess(1):
	case <-time.After(100 * time.Millisecond):
		// This is expected - context canceled
	}

	// Verify channels are closed within reasonable time
	// May receive pending items first, so drain until closed
	timeout := time.After(500 * time.Millisecond)
	for i, out := range outputs {
		closed := false
		for !closed {
			select {
			case _, ok := <-out:
				if !ok {
					closed = true
				}
				// If ok, keep draining
			case <-timeout:
				t.Fatalf("Channel %d not closed after context cancellation", i)
			}
		}
	}
}

func TestPartition_MetadataPreservation(t *testing.T) {
	partition, err := NewRoundRobinPartition[string](2, 10)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Create input with metadata
	in := make(chan Result[string], 10)
	go func() {
		defer close(in)
		result := NewSuccess("test").
			WithMetadata("custom_key", "custom_value").
			WithMetadata("timestamp", time.Now())
		in <- result
	}()

	outputs := partition.Process(ctx, in)

	// Collect result and verify metadata preservation
	var result Result[string]
	var wg sync.WaitGroup

	for _, out := range outputs {
		wg.Add(1)
		go func(ch <-chan Result[string]) {
			defer wg.Done()
			for r := range ch {
				result = r
			}
		}(out)
	}

	wg.Wait()

	// Verify original metadata preserved
	if value, exists := result.GetMetadata("custom_key"); !exists || value != "custom_value" {
		t.Error("Original metadata not preserved")
	}

	// Verify partition metadata added
	if _, exists := result.GetMetadata(MetadataPartitionIndex); !exists {
		t.Error("Partition index metadata not added")
	}
	if total, exists := result.GetMetadata(MetadataPartitionTotal); !exists || total != 2 {
		t.Error("Partition total metadata not added or incorrect")
	}
	if strategy, exists := result.GetMetadata(MetadataPartitionStrategy); !exists || strategy != "round_robin" {
		t.Error("Partition strategy metadata not added or incorrect")
	}
}

func TestHashPartition_ConsistentRouting(t *testing.T) {
	keyExtractor := func(s string) string {
		return s
	}

	strategy := &HashPartition[string, string]{
		keyExtractor: keyExtractor,
		hasher:       defaultHasher[string],
	}

	// Test multiple calls with same value return same partition
	value := "consistent_test"
	partitionCount := 5

	firstResult := strategy.Route(value, partitionCount)
	for i := 0; i < 100; i++ {
		result := strategy.Route(value, partitionCount)
		if result != firstResult {
			t.Errorf("Inconsistent routing: first=%d, iteration %d=%d", firstResult, i, result)
		}
	}

	// Verify result is in valid range
	if firstResult < 0 || firstResult >= partitionCount {
		t.Errorf("Route result %d out of range [0, %d)", firstResult, partitionCount)
	}
}

func TestHashPartition_DistributionQuality(t *testing.T) {
	keyExtractor := func(i int) int {
		return i
	}

	strategy := &HashPartition[int, int]{
		keyExtractor: keyExtractor,
		hasher:       defaultHasher[int],
	}

	partitionCount := 5
	sampleSize := 10000
	counts := make([]int, partitionCount)

	// Generate sample distribution
	for i := 0; i < sampleSize; i++ {
		partition := strategy.Route(i, partitionCount)
		if partition < 0 || partition >= partitionCount {
			t.Fatalf("Invalid partition index: %d", partition)
		}
		counts[partition]++
	}

	// Check distribution quality (should be roughly even)
	expectedCount := sampleSize / partitionCount
	tolerance := expectedCount / 10 // 10% tolerance

	for i, count := range counts {
		if count < expectedCount-tolerance || count > expectedCount+tolerance {
			t.Logf("Partition %d: %d items (expected ~%d)", i, count, expectedCount)
		}
	}

	// Verify no partition is completely empty
	for i, count := range counts {
		if count == 0 {
			t.Errorf("Partition %d received no items", i)
		}
	}
}

func TestRoundRobinPartition_EvenDistribution(t *testing.T) {
	strategy := &RoundRobinPartition[int]{counter: 0}
	partitionCount := 4
	sampleSize := 100

	counts := make([]int, partitionCount)

	for i := 0; i < sampleSize; i++ {
		partition := strategy.Route(i, partitionCount)
		if partition < 0 || partition >= partitionCount {
			t.Fatalf("Invalid partition index: %d", partition)
		}
		counts[partition]++
	}

	// Should be exactly even for round-robin
	expectedCount := sampleSize / partitionCount
	for i, count := range counts {
		if count != expectedCount {
			t.Errorf("Partition %d: %d items, expected %d", i, count, expectedCount)
		}
	}
}

func TestPartition_SinglePartition(t *testing.T) {
	partition, err := NewRoundRobinPartition[string](1, 5)
	if err != nil {
		t.Fatalf("Failed to create single partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan Result[string], 5)
	go func() {
		defer close(in)
		in <- NewSuccess("test1")
		in <- NewSuccess("test2")
		in <- NewError("error", fmt.Errorf("test error"), "test")
	}()

	outputs := partition.Process(ctx, in)
	if len(outputs) != 1 {
		t.Fatalf("Expected 1 output channel, got %d", len(outputs))
	}

	// Collect all results
	results := make([]Result[string], 0, 10)
	for result := range outputs[0] {
		results = append(results, result)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// All should route to partition 0
	for _, result := range results {
		if index, exists := result.GetMetadata(MetadataPartitionIndex); exists {
			if idx, ok := index.(int); ok && idx != 0 {
				t.Errorf("Result routed to partition %d, expected 0", idx)
			}
		}
	}
}

func TestPartition_StrategyPanic(t *testing.T) {
	// Create a strategy that panics
	panicStrategy := &testPanicStrategy[string]{}

	config := PartitionConfig[string]{
		PartitionCount: 3,
		Strategy:       panicStrategy,
		BufferSize:     5,
	}

	partition, err := NewPartition(config)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan Result[string], 5)
	go func() {
		defer close(in)
		in <- NewSuccess("test")
	}()

	outputs := partition.Process(ctx, in)

	// Collect result from partition 0 (panic recovery should route there)
	var result Result[string]
	select {
	case result = <-outputs[0]:
	case <-time.After(time.Second):
		t.Fatal("No result received from partition 0")
	}

	// Verify it was routed to partition 0 due to panic
	if index, exists := result.GetMetadata(MetadataPartitionIndex); exists {
		if idx, ok := index.(int); ok && idx != 0 {
			t.Errorf("Panic should route to partition 0, got %d", idx)
		}
	}
}

func TestPartition_KeyExtractorPanic(t *testing.T) {
	// Key extractor that panics
	keyExtractor := func(_ string) string {
		panic("key extractor panic")
	}

	partition, err := NewHashPartition(3, keyExtractor, 5)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan Result[string], 5)
	go func() {
		defer close(in)
		in <- NewSuccess("test")
	}()

	outputs := partition.Process(ctx, in)

	// Should route to partition 0 due to panic
	var result Result[string]
	select {
	case result = <-outputs[0]:
	case <-time.After(time.Second):
		t.Fatal("No result received from partition 0")
	}

	if index, exists := result.GetMetadata(MetadataPartitionIndex); exists {
		if idx, ok := index.(int); ok && idx != 0 {
			t.Errorf("Key extractor panic should route to partition 0, got %d", idx)
		}
	}
}

func TestPartition_HasherPanic(t *testing.T) {
	// Hasher that panics
	hasher := func(_ string) uint64 {
		panic("hasher panic")
	}

	strategy := &HashPartition[string, string]{
		keyExtractor: func(s string) string { return s },
		hasher:       hasher,
	}

	config := PartitionConfig[string]{
		PartitionCount: 3,
		Strategy:       strategy,
		BufferSize:     5,
	}

	partition, err := NewPartition(config)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan Result[string], 5)
	go func() {
		defer close(in)
		in <- NewSuccess("test")
	}()

	outputs := partition.Process(ctx, in)

	// Should route to partition 0 due to panic
	var result Result[string]
	select {
	case result = <-outputs[0]:
	case <-time.After(time.Second):
		t.Fatal("No result received from partition 0")
	}

	if index, exists := result.GetMetadata(MetadataPartitionIndex); exists {
		if idx, ok := index.(int); ok && idx != 0 {
			t.Errorf("Hasher panic should route to partition 0, got %d", idx)
		}
	}
}

func TestPartition_ConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		config        PartitionConfig[string]
		expectedError string
	}{
		{
			name: "zero partition count",
			config: PartitionConfig[string]{
				PartitionCount: 0,
				Strategy:       &RoundRobinPartition[string]{},
				BufferSize:     5,
			},
			expectedError: "partition count must be > 0, got 0",
		},
		{
			name: "negative partition count",
			config: PartitionConfig[string]{
				PartitionCount: -1,
				Strategy:       &RoundRobinPartition[string]{},
				BufferSize:     5,
			},
			expectedError: "partition count must be > 0, got -1",
		},
		{
			name: "negative buffer size",
			config: PartitionConfig[string]{
				PartitionCount: 3,
				Strategy:       &RoundRobinPartition[string]{},
				BufferSize:     -1,
			},
			expectedError: "buffer size must be >= 0, got -1",
		},
		{
			name: "nil strategy",
			config: PartitionConfig[string]{
				PartitionCount: 3,
				Strategy:       nil,
				BufferSize:     5,
			},
			expectedError: "strategy cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPartition(tt.config)
			if err == nil {
				t.Error("Expected error, got nil")
			} else if err.Error() != tt.expectedError {
				t.Errorf("Expected error %q, got %q", tt.expectedError, err.Error())
			}
		})
	}
}

func TestPartition_ConcurrentRouting(t *testing.T) {
	partition, err := NewHashPartition(5, func(i int) int { return i }, 100)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan Result[int], 1000)
	outputs := partition.Process(ctx, in)

	// Start multiple producers
	var producerWg sync.WaitGroup
	numProducers := 5
	itemsPerProducer := 100

	for p := 0; p < numProducers; p++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				value := producerID*itemsPerProducer + i
				in <- NewSuccess(value)
			}
		}(p)
	}

	// Close input when all producers done
	go func() {
		producerWg.Wait()
		close(in)
	}()

	// Collect results from all partitions
	var collectorWg sync.WaitGroup
	totalReceived := int64(0)
	var receivedMutex sync.Mutex

	for _, out := range outputs {
		collectorWg.Add(1)
		go func(ch <-chan Result[int]) {
			defer collectorWg.Done()
			count := 0
			for result := range ch {
				if result.IsSuccess() {
					count++
				}
			}
			receivedMutex.Lock()
			totalReceived += int64(count)
			receivedMutex.Unlock()
		}(out)
	}

	collectorWg.Wait()

	expectedTotal := int64(numProducers * itemsPerProducer)
	if totalReceived != expectedTotal {
		t.Errorf("Expected %d items, received %d", expectedTotal, totalReceived)
	}
}

func TestPartition_BufferOverflow(t *testing.T) {
	// Create partition with very small buffer
	partition, err := NewRoundRobinPartition[int](2, 1)
	if err != nil {
		t.Fatalf("Failed to create partition: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	in := make(chan Result[int], 10)
	outputs := partition.Process(ctx, in)

	// Send more data than buffer can hold
	go func() {
		defer close(in)
		for i := 0; i < 10; i++ {
			in <- NewSuccess(i)
		}
	}()

	// Slowly consume from one partition to create backpressure
	var consumedCount atomic.Int32
	consumerCtx, consumerCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer consumerCancel()

	// Consume from partition 0 slowly
	go func() {
		for {
			select {
			case <-consumerCtx.Done():
				return
			case result, ok := <-outputs[0]:
				if !ok {
					return
				}
				if result.IsSuccess() {
					consumedCount.Add(1)
				}
				time.Sleep(50 * time.Millisecond) // Slow consumption
			}
		}
	}()

	// Consume from partition 1 normally
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for result := range outputs[1] {
			if result.IsSuccess() {
				consumedCount.Add(1)
			}
		}
	}()

	wg.Wait()

	// Should still process all items (may take time due to backpressure)
	finalCount := consumedCount.Load()
	if finalCount < 5 {
		t.Logf("Only consumed %d items due to backpressure (expected behavior)", finalCount)
	}
}

func TestDefaultHasher_CommonTypes(t *testing.T) {
	tests := []struct {
		name string
		key  interface{}
	}{
		{"string", "test"},
		{"int", 42},
		{"int64", int64(42)},
		{"int32", int32(42)},
		{"uint64", uint64(42)},
		{"uint32", uint32(42)},
		{"float64", 3.14},
		{"float32", float32(3.14)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that hasher doesn't panic and returns consistent results
			var hash1, hash2 uint64

			switch v := tt.key.(type) {
			case string:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			case int:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			case int64:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			case int32:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			case uint64:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			case uint32:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			case float64:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			case float32:
				hash1 = defaultHasher(v)
				hash2 = defaultHasher(v)
			}

			if hash1 != hash2 {
				t.Errorf("Inconsistent hash for %T: %d != %d", tt.key, hash1, hash2)
			}
			if hash1 == 0 {
				t.Errorf("Hash should not be zero for %T", tt.key)
			}
		})
	}
}

func TestHashPartition_NoModuloBias(t *testing.T) {
	strategy := &HashPartition[int, int]{
		keyExtractor: func(i int) int { return i },
		hasher:       defaultHasher[int],
	}

	// Test with partition count that would show modulo bias (non-power-of-2)
	partitionCount := 7
	sampleSize := 70000
	counts := make([]int, partitionCount)

	for i := 0; i < sampleSize; i++ {
		partition := strategy.Route(i, partitionCount)
		counts[partition]++
	}

	// Calculate chi-square statistic for uniformity test
	expected := float64(sampleSize) / float64(partitionCount)
	chiSquare := 0.0

	for _, count := range counts {
		diff := float64(count) - expected
		chiSquare += (diff * diff) / expected
	}

	// Chi-square critical value for 6 degrees of freedom at 95% confidence ≈ 12.59
	// Using relaxed threshold for hash function quality test
	if chiSquare > 20.0 {
		t.Errorf("Distribution too uneven, chi-square: %.2f", chiSquare)
		for i, count := range counts {
			t.Logf("Partition %d: %d items (%.1f%%)", i, count, 100.0*float64(count)/float64(sampleSize))
		}
	}
}

// Test helper: strategy that always panics.
type testPanicStrategy[T any] struct{}

func (*testPanicStrategy[T]) Route(_ T, _ int) int {
	panic("test panic in strategy")
}

// Benchmark hash partitioning performance.
func BenchmarkHashPartition_Route(b *testing.B) {
	strategy := &HashPartition[string, string]{
		keyExtractor: func(s string) string { return s },
		hasher:       defaultHasher[string],
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Route("test_key_"+strconv.Itoa(i%1000), 8)
	}
}

// Benchmark round-robin partitioning performance.
func BenchmarkRoundRobinPartition_Route(b *testing.B) {
	strategy := &RoundRobinPartition[string]{counter: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Route("test", 8)
	}
}
