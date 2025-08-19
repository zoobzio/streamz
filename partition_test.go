package streamz

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPartitionBasicFunctionality tests basic partitioning operations.
func TestPartitionBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	// Create partitioner for orders by customer ID
	partitioner := NewPartition[PartitionOrder](func(o PartitionOrder) string {
		return o.CustomerID
	}).WithPartitions(3)

	// Create test orders
	orders := []PartitionOrder{
		{ID: "1", CustomerID: "cust-1", Amount: 100},
		{ID: "2", CustomerID: "cust-2", Amount: 200},
		{ID: "3", CustomerID: "cust-1", Amount: 150}, // Same customer as order 1
		{ID: "4", CustomerID: "cust-3", Amount: 300},
		{ID: "5", CustomerID: "cust-2", Amount: 250}, // Same customer as order 2
		{ID: "6", CustomerID: "cust-1", Amount: 175}, // Same customer as order 1
	}

	input := make(chan PartitionOrder, len(orders))
	for _, order := range orders {
		input <- order
	}
	close(input)

	// Process and partition
	output := partitioner.Process(ctx, input)

	// Collect orders from each partition
	partitionOrders := make([][]PartitionOrder, 3)
	var wg sync.WaitGroup

	for i, partition := range output.Partitions {
		wg.Add(1)
		go func(idx int, p <-chan PartitionOrder) {
			defer wg.Done()
			for order := range p {
				partitionOrders[idx] = append(partitionOrders[idx], order)
			}
		}(i, partition)
	}

	wg.Wait()

	// Verify all orders were processed
	totalOrders := 0
	for _, orders := range partitionOrders {
		totalOrders += len(orders)
	}
	if totalOrders != 6 {
		t.Errorf("expected 6 orders, got %d", totalOrders)
	}

	// Verify orders from same customer went to same partition
	customerPartitions := make(map[string]int)
	for partIdx, orders := range partitionOrders {
		for _, order := range orders {
			if existingPart, exists := customerPartitions[order.CustomerID]; exists {
				if existingPart != partIdx {
					t.Errorf("customer %s found in multiple partitions: %d and %d",
						order.CustomerID, existingPart, partIdx)
				}
			} else {
				customerPartitions[order.CustomerID] = partIdx
			}
		}
	}

	// Verify order preservation within partitions
	for partIdx, orders := range partitionOrders {
		for i := 1; i < len(orders); i++ {
			// Orders from same customer should maintain order
			if orders[i-1].CustomerID == orders[i].CustomerID {
				prevID := orders[i-1].ID
				currID := orders[i].ID
				if prevID > currID {
					t.Errorf("partition %d: order violated for customer %s: %s came after %s",
						partIdx, orders[i].CustomerID, prevID, currID)
				}
			}
		}
	}

	// Check statistics
	stats := partitioner.GetStats()
	if stats.TotalItems != 6 {
		t.Errorf("expected 6 total items, got %d", stats.TotalItems)
	}
	if stats.NumPartitions != 3 {
		t.Errorf("expected 3 partitions, got %d", stats.NumPartitions)
	}
}

// TestPartitionConsistentRouting tests that same key always goes to same partition.
func TestPartitionConsistentRouting(t *testing.T) {
	ctx := context.Background()

	partitioner := NewPartition[string](func(s string) string {
		return s // Use string itself as key
	}).WithPartitions(5)

	// Track which partition each key goes to
	keyToPartition := make(map[string]int)
	var mu sync.Mutex

	// Process items multiple times
	for round := 0; round < 3; round++ {
		input := make(chan string, 10)
		keys := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

		for _, key := range keys {
			input <- key
		}
		close(input)

		output := partitioner.Process(ctx, input)

		// Wait for all partitions to be consumed
		var wg sync.WaitGroup

		// Check routing
		for partIdx, partition := range output.Partitions {
			wg.Add(1)
			go func(idx int, p <-chan string) {
				defer wg.Done()
				for item := range p {
					mu.Lock()
					if existingPart, exists := keyToPartition[item]; exists {
						if existingPart != idx {
							t.Errorf("key %s routed to different partitions: %d and %d",
								item, existingPart, idx)
						}
					} else {
						keyToPartition[item] = idx
					}
					mu.Unlock()
				}
			}(partIdx, partition)
		}

		wg.Wait()
	}
}

// TestPartitionDistribution tests even distribution of items.
func TestPartitionDistribution(t *testing.T) {
	ctx := context.Background()

	// Use a simple incrementing key for even distribution
	counter := 0
	partitioner := NewPartition[int](func(n int) string {
		return fmt.Sprintf("key-%d", n)
	}).WithPartitions(4)

	// Send many items
	numItems := 1000
	input := make(chan int, numItems)
	for i := 0; i < numItems; i++ {
		input <- counter
		counter++
	}
	close(input)

	output := partitioner.Process(ctx, input)

	// Count items per partition
	partitionCounts := make([]int, 4)
	var wg sync.WaitGroup

	for i, partition := range output.Partitions {
		wg.Add(1)
		go func(idx int, p <-chan int) {
			defer wg.Done()
			count := 0
			for range p {
				count++
			}
			partitionCounts[idx] = count
		}(i, partition)
	}

	wg.Wait()

	// Check distribution
	stats := partitioner.GetStats()
	expectedPerPartition := numItems / 4
	tolerance := float64(expectedPerPartition) * 0.2 // 20% tolerance

	for i, count := range partitionCounts {
		if float64(count) < float64(expectedPerPartition)-tolerance ||
			float64(count) > float64(expectedPerPartition)+tolerance {
			t.Errorf("partition %d has %d items, expected ~%d (±20%%)",
				i, count, expectedPerPartition)
		}
	}

	// Check balance metric
	balance := stats.DistributionBalance()
	if balance > 0.5 {
		t.Errorf("distribution balance too high: %.2f", balance)
	}
}

// TestPartitionCustomPartitioner tests custom partitioning logic.
func TestPartitionCustomPartitioner(t *testing.T) {
	ctx := context.Background()

	// Custom partitioner that puts even numbers in partition 0, odd in partition 1
	customPartitioner := func(key string, _ int) int {
		// Extract number from key
		var n int
		_, _ = fmt.Sscanf(key, "num-%d", &n) //nolint:errcheck // test code ignoring parse errors
		return n % 2
	}

	partitioner := NewPartition[int](func(n int) string {
		return fmt.Sprintf("num-%d", n)
	}).WithPartitions(2).WithPartitioner(customPartitioner)

	// Send numbers
	input := make(chan int, 10)
	for i := 0; i < 10; i++ {
		input <- i
	}
	close(input)

	output := partitioner.Process(ctx, input)

	// Collect from partitions
	var evens, odds []int
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := range output.Partitions[0] {
			evens = append(evens, n)
		}
	}()

	go func() {
		defer wg.Done()
		for n := range output.Partitions[1] {
			odds = append(odds, n)
		}
	}()

	wg.Wait()

	// Verify partitioning
	for _, n := range evens {
		if n%2 != 0 {
			t.Errorf("found odd number %d in even partition", n)
		}
	}

	for _, n := range odds {
		if n%2 != 1 {
			t.Errorf("found even number %d in odd partition", n)
		}
	}

	if len(evens) != 5 || len(odds) != 5 {
		t.Errorf("expected 5 evens and 5 odds, got %d and %d", len(evens), len(odds))
	}
}

// TestPartitionGetPartition tests the GetPartition method.
func TestPartitionGetPartition(t *testing.T) {
	ctx := context.Background()

	partitioner := NewPartition[int](func(n int) string {
		return fmt.Sprintf("%d", n)
	}).WithPartitions(3)

	input := make(chan int)
	close(input)

	output := partitioner.Process(ctx, input)

	// Test valid indices
	for i := 0; i < 3; i++ {
		partition := output.GetPartition(i)
		if partition == nil {
			t.Errorf("GetPartition(%d) returned nil", i)
		}
		if partition != output.Partitions[i] {
			t.Error("GetPartition returned different channel than Partitions array")
		}
	}

	// Test invalid indices
	if output.GetPartition(-1) != nil {
		t.Error("GetPartition(-1) should return nil")
	}
	if output.GetPartition(3) != nil {
		t.Error("GetPartition(3) should return nil for 3 partitions")
	}
}

// TestPartitionContextCancellation tests graceful shutdown.
func TestPartitionContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	partitioner := NewPartition[int](func(n int) string {
		return fmt.Sprintf("%d", n)
	}).WithPartitions(2)

	input := make(chan int)
	output := partitioner.Process(ctx, input)

	// Start consumers
	var wg sync.WaitGroup
	var processedCount atomic.Int32

	for _, partition := range output.Partitions {
		wg.Add(1)
		go func(p <-chan int) {
			defer wg.Done()
			for range p {
				processedCount.Add(1)
			}
		}(partition)
	}

	// Send some items
	go func() {
		for i := 0; i < 100; i++ {
			select {
			case input <- i:
			case <-ctx.Done():
				close(input)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		close(input)
	}()

	// Let some items process
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for completion
	wg.Wait()

	// Should have processed some but not all
	count := processedCount.Load()
	if count == 0 {
		t.Error("expected some items to be processed")
	}
	if count >= 100 {
		t.Errorf("expected cancellation to stop processing, but processed %d items", count)
	}
}

// TestPartitionConcurrentProcessing tests concurrent access.
func TestPartitionConcurrentProcessing(t *testing.T) {
	ctx := context.Background()

	partitioner := NewPartition[int](func(n int) string {
		return fmt.Sprintf("key-%d", n%10) // 10 different keys
	}).WithPartitions(4).WithBufferSize(10)

	// Multiple producers
	input := make(chan int)
	var producerWg sync.WaitGroup
	numProducers := 5
	itemsPerProducer := 100

	for p := 0; p < numProducers; p++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				select {
				case input <- producerID*1000 + i:
				case <-ctx.Done():
					return
				}
			}
		}(p)
	}

	go func() {
		producerWg.Wait()
		close(input)
	}()

	output := partitioner.Process(ctx, input)

	// Multiple consumers per partition
	var consumerWg sync.WaitGroup
	totalProcessed := atomic.Int32{}

	for partIdx, partition := range output.Partitions {
		for c := 0; c < 2; c++ { // 2 consumers per partition
			consumerWg.Add(1)
			go func(_, _ int, p <-chan int) {
				defer consumerWg.Done()
				count := 0
				for range p {
					count++
					totalProcessed.Add(1)
				}
			}(partIdx, c, partition)
		}
	}

	consumerWg.Wait()

	// Verify all items processed
	expected := int32(numProducers * itemsPerProducer) // #nosec G115 - test code with known values
	if totalProcessed.Load() != expected {
		t.Errorf("expected %d items processed, got %d", expected, totalProcessed.Load())
	}

	// Verify stats
	stats := partitioner.GetStats()
	if stats.TotalItems != int64(expected) {
		t.Errorf("stats show %d items, expected %d", stats.TotalItems, expected)
	}
}

// TestPartitionFluentAPI tests fluent configuration.
func TestPartitionFluentAPI(t *testing.T) {
	partitioner := NewPartition[string](func(s string) string { return s }).
		WithPartitions(8).
		WithBufferSize(100).
		WithName("test-partitioner").
		WithPartitioner(func(key string, n int) int {
			return len(key) % n
		})

	if partitioner.numPartitions != 8 {
		t.Errorf("expected 8 partitions, got %d", partitioner.numPartitions)
	}

	if partitioner.bufferSize != 100 {
		t.Errorf("expected buffer size 100, got %d", partitioner.bufferSize)
	}

	if partitioner.Name() != "test-partitioner" {
		t.Errorf("expected name 'test-partitioner', got %s", partitioner.Name())
	}

	// Test custom partitioner
	partition := partitioner.partitioner("hello", 8) // length 5
	if partition != 5 {
		t.Errorf("custom partitioner returned %d, expected 5", partition)
	}
}

// TestPartitionEdgeCases tests edge cases.
func TestPartitionEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("ZeroPartitions", func(t *testing.T) {
		// Should default to 1 partition
		partitioner := NewPartition[int](func(n int) string {
			return fmt.Sprintf("%d", n)
		}).WithPartitions(0)

		if partitioner.numPartitions != 1 {
			t.Errorf("expected 1 partition for 0 input, got %d", partitioner.numPartitions)
		}
	})

	t.Run("NegativeBufferSize", func(t *testing.T) {
		// Should default to 0
		partitioner := NewPartition[int](func(n int) string {
			return fmt.Sprintf("%d", n)
		}).WithBufferSize(-10)

		if partitioner.bufferSize != 0 {
			t.Errorf("expected 0 buffer size for negative input, got %d", partitioner.bufferSize)
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		partitioner := NewPartition[int](func(n int) string {
			return fmt.Sprintf("%d", n)
		}).WithPartitions(3)

		input := make(chan int)
		close(input)

		output := partitioner.Process(ctx, input)

		// All partitions should close immediately
		for i, partition := range output.Partitions {
			_, ok := <-partition
			if ok {
				t.Errorf("partition %d should be closed for empty input", i)
			}
		}
	})

	t.Run("NilPartitioner", func(t *testing.T) {
		partitioner := NewPartition[int](func(n int) string {
			return fmt.Sprintf("%d", n)
		}).WithPartitioner(nil)

		// Should keep default partitioner
		if partitioner.partitioner == nil {
			t.Error("partitioner should not be nil")
		}
	})
}

// BenchmarkPartition benchmarks partitioning performance.
func BenchmarkPartition(b *testing.B) {
	ctx := context.Background()

	partitioner := NewPartition[int](func(n int) string {
		return fmt.Sprintf("key-%d", n)
	}).WithPartitions(4)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		output := partitioner.Process(ctx, input)

		// Drain all partitions
		var wg sync.WaitGroup
		for _, partition := range output.Partitions {
			wg.Add(1)
			go func(p <-chan int) {
				defer wg.Done()
				for range p { //nolint:revive // Intentionally draining channel
					// Consume
				}
			}(partition)
		}
		wg.Wait()
	}
}

// BenchmarkPartitionThroughput benchmarks high-throughput partitioning.
func BenchmarkPartitionThroughput(b *testing.B) {
	ctx := context.Background()

	partitioner := NewPartition[int](func(n int) string {
		return fmt.Sprintf("customer-%d", n%1000)
	}).WithPartitions(10).WithBufferSize(100)

	b.ResetTimer()
	b.ReportAllocs()

	input := make(chan int, 1000)
	output := partitioner.Process(ctx, input)

	// Start consumers
	var wg sync.WaitGroup
	for _, partition := range output.Partitions {
		wg.Add(1)
		go func(p <-chan int) {
			defer wg.Done()
			for range p { //nolint:revive // Intentionally draining channel
				// Consume
			}
		}(partition)
	}

	// Send items
	go func() {
		for i := 0; i < b.N; i++ {
			input <- i
		}
		close(input)
	}()

	wg.Wait()
}

// PartitionOrder type for testing.
type PartitionOrder struct {
	ID         string
	CustomerID string
	Amount     float64
}

// Example demonstrates basic partitioning usage.
func ExamplePartition() {
	ctx := context.Background()

	// Partition messages by user ID for parallel processing
	partitioner := NewPartition[PartitionMessage](func(m PartitionMessage) string {
		return m.UserID
	}).WithPartitions(3)

	// Create input messages
	messages := make(chan PartitionMessage, 6)
	messages <- PartitionMessage{ID: "1", UserID: "alice", Content: "Hello"}
	messages <- PartitionMessage{ID: "2", UserID: "bob", Content: "Hi"}
	messages <- PartitionMessage{ID: "3", UserID: "alice", Content: "How are you?"}
	messages <- PartitionMessage{ID: "4", UserID: "charlie", Content: "Hey"}
	messages <- PartitionMessage{ID: "5", UserID: "bob", Content: "What's up?"}
	messages <- PartitionMessage{ID: "6", UserID: "alice", Content: "Good, thanks!"}
	close(messages)

	// Process partitions
	output := partitioner.Process(ctx, messages)

	// Process each partition independently
	var wg sync.WaitGroup
	for i, partition := range output.Partitions {
		wg.Add(1)
		go func(partIdx int, p <-chan PartitionMessage) {
			defer wg.Done()
			fmt.Printf("Partition %d:\n", partIdx)
			for msg := range p {
				fmt.Printf("  %s: %s\n", msg.UserID, msg.Content)
			}
		}(i, partition)
	}
	wg.Wait()

	// Note: Output order may vary between partitions, but messages
	// from the same user will always be in the same partition and in order
}

// PartitionMessage type for example.
type PartitionMessage struct {
	ID      string
	UserID  string
	Content string
}
