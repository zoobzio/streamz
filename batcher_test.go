package streamz

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"streamz/clock"
)

// TestBatcherBySize tests batching by size.
func TestBatcherBySize(t *testing.T) {
	ctx := context.Background()

	config := BatchConfig{
		MaxSize:    3,
		MaxLatency: 1 * time.Hour, // Effectively disabled.
	}

	batcher := NewBatcher[int](config, clock.Real)

	input := make(chan int, 10)
	for i := 1; i <= 10; i++ {
		input <- i
	}
	close(input)

	output := batcher.Process(ctx, input)

	expectedBatches := [][]int{
		{1, 2, 3},
		{4, 5, 6},
		{7, 8, 9},
		{10}, // Final partial batch.
	}

	batches := make([][]int, 0, len(expectedBatches))
	for batch := range output {
		batches = append(batches, batch)
	}

	if len(batches) != len(expectedBatches) {
		t.Fatalf("expected %d batches, got %d", len(expectedBatches), len(batches))
	}

	for i, batch := range batches {
		if len(batch) != len(expectedBatches[i]) {
			t.Errorf("batch %d: expected size %d, got %d", i, len(expectedBatches[i]), len(batch))
		}
		for j, item := range batch {
			if item != expectedBatches[i][j] {
				t.Errorf("batch %d, item %d: expected %d, got %d", i, j, expectedBatches[i][j], item)
			}
		}
	}

	// Batcher has default name.
	if batcher.Name() == "" {
		t.Errorf("expected non-empty name, got empty string")
	}
}

// TestBatcherByTime tests batching by time.
// Note: This test uses real clock because the batcher's timer logic
// requires careful coordination between item arrival and timer resets.
func TestBatcherByTime(t *testing.T) {
	ctx := context.Background()

	config := BatchConfig{
		MaxSize:    100, // Effectively disabled.
		MaxLatency: 50 * time.Millisecond,
	}

	batcher := NewBatcher[int](config, clock.Real)

	input := make(chan int)
	output := batcher.Process(ctx, input)

	var batches [][]int
	var mu sync.Mutex
	done := make(chan bool)

	// Collector.
	go func() {
		for batch := range output {
			mu.Lock()
			batches = append(batches, batch)
			mu.Unlock()
		}
		done <- true
	}()

	// Send items in bursts.
	// First burst.
	for i := 1; i <= 3; i++ {
		input <- i
	}

	// Wait for time trigger.
	time.Sleep(100 * time.Millisecond)

	// Second burst.
	for i := 4; i <= 6; i++ {
		input <- i
	}

	// Wait for time trigger.
	time.Sleep(100 * time.Millisecond)

	close(input)
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(batches) != 2 {
		t.Fatalf("expected 2 time-based batches, got %d", len(batches))
	}

	if len(batches[0]) != 3 {
		t.Errorf("first batch: expected 3 items, got %d", len(batches[0]))
	}

	if len(batches[1]) != 3 {
		t.Errorf("second batch: expected 3 items, got %d", len(batches[1]))
	}
}

// TestBatcherMixedTriggers tests both size and time triggers.
func TestBatcherMixedTriggers(t *testing.T) {
	ctx := context.Background()

	config := BatchConfig{
		MaxSize:    5,
		MaxLatency: 100 * time.Millisecond,
	}

	batcher := NewBatcher[string](config, clock.Real)

	input := make(chan string)
	output := batcher.Process(ctx, input)

	var batches [][]string
	var mu sync.Mutex
	done := make(chan bool)

	go func() {
		for batch := range output {
			mu.Lock()
			batches = append(batches, batch)
			mu.Unlock()
		}
		done <- true
	}()

	// Quick burst - should trigger size.
	for i := 0; i < 5; i++ {
		input <- fmt.Sprintf("quick-%d", i)
	}

	// Wait a bit.
	time.Sleep(50 * time.Millisecond)

	// Slow items - should trigger time.
	input <- "slow-1"
	input <- "slow-2"

	// Wait for time trigger.
	time.Sleep(150 * time.Millisecond)

	close(input)
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches (1 size, 1 time), got %d", len(batches))
	}

	if len(batches[0]) != 5 {
		t.Errorf("first batch (size-triggered): expected 5 items, got %d", len(batches[0]))
	}

	if len(batches[1]) != 2 {
		t.Errorf("second batch (time-triggered): expected 2 items, got %d", len(batches[1]))
	}
}

// TestBatcherEmptyInput tests batcher with empty input.
func TestBatcherEmptyInput(t *testing.T) {
	ctx := context.Background()

	config := BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}

	batcher := NewBatcher[int](config, clock.Real)

	input := make(chan int)
	close(input)

	output := batcher.Process(ctx, input)

	count := 0
	for range output {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 batches for empty input, got %d", count)
	}
}

// TestBatcherSingleItem tests batching with a single item.
func TestBatcherSingleItem(t *testing.T) {
	ctx := context.Background()

	config := BatchConfig{
		MaxSize:    10,
		MaxLatency: 100 * time.Millisecond,
	}

	batcher := NewBatcher[int](config, clock.Real)

	input := make(chan int, 1)
	input <- 42
	close(input)

	output := batcher.Process(ctx, input)

	batches := make([][]int, 0, 1)
	for batch := range output {
		batches = append(batches, batch)
	}

	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}

	if len(batches[0]) != 1 || batches[0][0] != 42 {
		t.Errorf("expected batch [42], got %v", batches[0])
	}
}

// TestBatcherContextCancellation tests graceful shutdown.
func TestBatcherContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := BatchConfig{
		MaxSize:    10,
		MaxLatency: 50 * time.Millisecond,
	}

	batcher := NewBatcher[int](config, clock.Real)

	input := make(chan int)
	output := batcher.Process(ctx, input)

	var batchCount int
	done := make(chan bool)

	go func() {
		for range output {
			batchCount++
		}
		done <- true
	}()

	// Send items continuously.
	go func() {
		for i := 0; ; i++ {
			select {
			case input <- i:
				time.Sleep(10 * time.Millisecond)
			case <-ctx.Done():
				close(input)
				return
			}
		}
	}()

	// Let some batches process.
	time.Sleep(200 * time.Millisecond)
	cancel()

	<-done

	// Should have received some batches.
	if batchCount == 0 {
		t.Error("expected some batches before cancellation")
	}
}

// TestBatcherConfiguration tests configuration edge cases.
func TestBatcherConfiguration(t *testing.T) {
	ctx := context.Background()

	// Test zero/negative size defaults to 1.
	config1 := BatchConfig{
		MaxSize:    0,
		MaxLatency: 100 * time.Millisecond,
	}
	batcher1 := NewBatcher[int](config1, clock.Real)

	input1 := make(chan int, 3)
	input1 <- 1
	input1 <- 2
	input1 <- 3
	close(input1)

	output1 := batcher1.Process(ctx, input1)

	batchCount := 0
	for batch := range output1 {
		batchCount++
		if len(batch) != 1 {
			t.Errorf("expected batch size 1 (min), got %d", len(batch))
		}
	}

	if batchCount != 3 {
		t.Errorf("expected 3 batches of size 1, got %d", batchCount)
	}

	// Test zero latency triggers immediately.
	config2 := BatchConfig{
		MaxSize:    2,
		MaxLatency: 0,
	}
	batcher2 := NewBatcher[int](config2, clock.Real)

	input2 := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		input2 <- i
	}
	close(input2)

	output2 := batcher2.Process(ctx, input2)

	batches := make([][]int, 0, 3) // 5 items with batch size 2 = 3 batches
	for batch := range output2 {
		batches = append(batches, batch)
	}

	// With zero latency and size 2, we should get multiple batches.
	if len(batches) < 2 {
		t.Fatalf("expected multiple batches with zero latency, got %d", len(batches))
	}

	// Total items should still be 5.
	totalItems := 0
	for _, batch := range batches {
		totalItems += len(batch)
	}
	if totalItems != 5 {
		t.Errorf("expected 5 total items, got %d", totalItems)
	}
}

// BenchmarkBatcher benchmarks batching performance.
func BenchmarkBatcher(b *testing.B) {
	ctx := context.Background()

	config := BatchConfig{
		MaxSize:    100,
		MaxLatency: 10 * time.Millisecond,
	}

	batcher := NewBatcher[int](config, clock.Real)

	b.ResetTimer()
	b.ReportAllocs()

	input := make(chan int, b.N)
	output := batcher.Process(ctx, input)

	// Consumer.
	done := make(chan bool)
	go func() {
		//nolint:revive // empty-block: necessary to drain channel
		for range output {
			// Consume.
		}
		done <- true
	}()

	// Producer.
	for i := 0; i < b.N; i++ {
		input <- i
	}
	close(input)

	<-done
}

// Example demonstrates batching for efficient database operations.
func ExampleBatcher() {
	ctx := context.Background()

	// Configure batching for database inserts.
	config := BatchConfig{
		MaxSize:    100,                    // Insert up to 100 records at once.
		MaxLatency: 500 * time.Millisecond, // Or every 500ms.
	}

	batcher := NewBatcher[string](config, clock.Real)

	// Simulate incoming user events.
	events := make(chan string, 10)
	events <- "user-login:alice"
	events <- "user-login:bob"
	events <- "page-view:home"
	events <- "user-login:carol"
	events <- "page-view:about"
	close(events)

	// Process batches.
	batches := batcher.Process(ctx, events)

	batchNum := 1
	for batch := range batches {
		fmt.Printf("Batch %d (%d events):\n", batchNum, len(batch))
		for _, event := range batch {
			fmt.Printf("  - %s\n", event)
		}
		batchNum++
	}

	// Output:
	// Batch 1 (5 events):
	//   - user-login:alice
	//   - user-login:bob
	//   - page-view:home
	//   - user-login:carol
	//   - page-view:about
}
