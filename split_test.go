package streamz

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestSplitBasicFunctionality tests basic splitting operations.
func TestSplitBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	// Create splitter for even/odd numbers.
	splitter := NewSplit[int](func(n int) bool {
		return n%2 == 0
	})

	// Send test data.
	input := make(chan int, 10)
	for i := 1; i <= 10; i++ {
		input <- i
	}
	close(input)

	// Process.
	outputs := splitter.Process(ctx, input)

	// Collect results
	var evens, odds []int
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := range outputs.True {
			evens = append(evens, n)
		}
	}()

	go func() {
		defer wg.Done()
		for n := range outputs.False {
			odds = append(odds, n)
		}
	}()

	wg.Wait()

	// Verify results.
	if len(evens) != 5 {
		t.Errorf("expected 5 even numbers, got %d", len(evens))
	}
	if len(odds) != 5 {
		t.Errorf("expected 5 odd numbers, got %d", len(odds))
	}

	// Verify correct classification.
	for _, n := range evens {
		if n%2 != 0 {
			t.Errorf("odd number %d in evens", n)
		}
	}
	for _, n := range odds {
		if n%2 == 0 {
			t.Errorf("even number %d in odds", n)
		}
	}

	// Check statistics.
	stats := splitter.GetStats()
	if stats.TotalItems != 10 {
		t.Errorf("expected 10 total items, got %d", stats.TotalItems)
	}
	if stats.TrueCount != 5 {
		t.Errorf("expected 5 true items, got %d", stats.TrueCount)
	}
	if stats.FalseCount != 5 {
		t.Errorf("expected 5 false items, got %d", stats.FalseCount)
	}
	if stats.TrueRatio != 0.5 {
		t.Errorf("expected true ratio 0.5, got %f", stats.TrueRatio)
	}
}

// TestSplitWithStructs tests splitting complex types.
func TestSplitWithStructs(t *testing.T) {
	ctx := context.Background()

	type Order struct {
		ID     string
		Amount float64
		Rush   bool
	}

	// Split high value orders.
	splitter := NewSplit[Order](func(o Order) bool {
		return o.Amount > 100 || o.Rush
	}).WithName("priority-splitter")

	// Test data.
	orders := []Order{
		{ID: "1", Amount: 50, Rush: false},  // false
		{ID: "2", Amount: 150, Rush: false}, // true
		{ID: "3", Amount: 75, Rush: true},   // true
		{ID: "4", Amount: 200, Rush: true},  // true
		{ID: "5", Amount: 25, Rush: false},  // false
	}

	input := make(chan Order, len(orders))
	for _, order := range orders {
		input <- order
	}
	close(input)

	outputs := splitter.Process(ctx, input)

	// Collect results
	var priority, normal []Order
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for order := range outputs.True {
			priority = append(priority, order)
		}
	}()

	go func() {
		defer wg.Done()
		for order := range outputs.False {
			normal = append(normal, order)
		}
	}()

	wg.Wait()

	// Verify counts.
	if len(priority) != 3 {
		t.Errorf("expected 3 priority orders, got %d", len(priority))
	}
	if len(normal) != 2 {
		t.Errorf("expected 2 normal orders, got %d", len(normal))
	}

	// Verify name.
	if splitter.Name() != "priority-splitter" {
		t.Errorf("expected name 'priority-splitter', got %s", splitter.Name())
	}
}

// TestSplitBuffering tests buffered operation.
func TestSplitBuffering(t *testing.T) {
	ctx := context.Background()

	// Create splitter with buffer.
	splitter := NewSplit[int](func(n int) bool {
		return n > 5
	}).WithBufferSize(10)

	input := make(chan int)
	outputs := splitter.Process(ctx, input)

	// Start consumers first.
	var high, low int
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for range outputs.True {
			high++
		}
	}()

	go func() {
		defer wg.Done()
		for range outputs.False {
			low++
		}
	}()

	// Now send items - buffer allows some to queue.
	go func() {
		for i := 1; i <= 10; i++ {
			input <- i
		}
		close(input)
	}()

	wg.Wait()

	if high != 5 {
		t.Errorf("expected 5 high values (6-10), got %d", high)
	}
	if low != 5 {
		t.Errorf("expected 5 low values (1-5), got %d", low)
	}
}

// TestSplitContextCancellation tests graceful shutdown.
func TestSplitContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	splitter := NewSplit[int](func(n int) bool {
		return n%2 == 0
	})

	input := make(chan int)
	outputs := splitter.Process(ctx, input)

	// Start consumers.
	var wg sync.WaitGroup
	processed := make(chan int, 100)

	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := range outputs.True {
			processed <- n
		}
	}()

	go func() {
		defer wg.Done()
		for n := range outputs.False {
			processed <- n
		}
	}()

	// Send some items.
	go func() {
		for i := 1; i <= 100; i++ {
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

	// Cancel after some time.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for shutdown.
	wg.Wait()
	close(processed)

	// Should have processed some but not all.
	count := 0
	for range processed {
		count++
	}

	if count == 0 {
		t.Error("expected some items to be processed")
	}
	if count >= 100 {
		t.Errorf("expected cancellation to stop processing, but processed %d items", count)
	}
}

// TestSplitEmptyInput tests behavior with empty input.
func TestSplitEmptyInput(t *testing.T) {
	ctx := context.Background()

	splitter := NewSplit[string](func(s string) bool {
		return len(s) > 5
	})

	input := make(chan string)
	close(input)

	outputs := splitter.Process(ctx, input)

	// Both channels should close immediately.
	_, ok1 := <-outputs.True
	_, ok2 := <-outputs.False

	if ok1 || ok2 {
		t.Error("expected both channels to be closed")
	}

	stats := splitter.GetStats()
	if stats.TotalItems != 0 {
		t.Errorf("expected 0 items, got %d", stats.TotalItems)
	}
}

// TestSplitAllTrue tests when all items go to true output.
func TestSplitAllTrue(t *testing.T) {
	ctx := context.Background()

	splitter := NewSplit[int](func(_ int) bool {
		return true // All items are "true"
	})

	input := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		input <- i
	}
	close(input)

	outputs := splitter.Process(ctx, input)

	var trueCount, falseCount int
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for range outputs.True {
			trueCount++
		}
	}()

	go func() {
		defer wg.Done()
		for range outputs.False {
			falseCount++
		}
	}()

	wg.Wait()

	if trueCount != 5 {
		t.Errorf("expected 5 true items, got %d", trueCount)
	}
	if falseCount != 0 {
		t.Errorf("expected 0 false items, got %d", falseCount)
	}

	stats := splitter.GetStats()
	if stats.TrueRatio != 1.0 {
		t.Errorf("expected true ratio 1.0, got %f", stats.TrueRatio)
	}
	if stats.FalseRatio != 0.0 {
		t.Errorf("expected false ratio 0.0, got %f", stats.FalseRatio)
	}
}

// TestSplitConcurrentProcessing tests concurrent access.
func TestSplitConcurrentProcessing(t *testing.T) {
	ctx := context.Background()

	splitter := NewSplit[int](func(n int) bool {
		return n%3 == 0 // Divisible by 3
	}).WithBufferSize(10)

	// Multiple producers.
	input := make(chan int)
	var producerWg sync.WaitGroup

	for p := 0; p < 5; p++ {
		producerWg.Add(1)
		go func(producer int) {
			defer producerWg.Done()
			for i := 0; i < 100; i++ {
				select {
				case input <- producer*1000 + i:
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

	outputs := splitter.Process(ctx, input)

	// Multiple consumers.
	var trueCount, falseCount int
	var mu sync.Mutex
	var consumerWg sync.WaitGroup

	// True consumers.
	for c := 0; c < 3; c++ {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			for range outputs.True {
				mu.Lock()
				trueCount++
				mu.Unlock()
			}
		}()
	}

	// False consumers.
	for c := 0; c < 3; c++ {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			for range outputs.False {
				mu.Lock()
				falseCount++
				mu.Unlock()
			}
		}()
	}

	consumerWg.Wait()

	total := trueCount + falseCount
	if total != 500 {
		t.Errorf("expected 500 total items, got %d", total)
	}

	// Roughly 1/3 should be divisible by 3.
	expectedTrue := 167 // ~500/3
	tolerance := 10
	if trueCount < expectedTrue-tolerance || trueCount > expectedTrue+tolerance {
		t.Errorf("expected ~%d true items (±%d), got %d", expectedTrue, tolerance, trueCount)
	}
}

// TestSplitFluentAPI tests the fluent configuration.
func TestSplitFluentAPI(t *testing.T) {
	splitter := NewSplit[string](func(s string) bool {
		return len(s) > 10
	}).WithBufferSize(100).WithName("length-splitter")

	if splitter.bufferSize != 100 {
		t.Errorf("expected buffer size 100, got %d", splitter.bufferSize)
	}

	if splitter.Name() != "length-splitter" {
		t.Errorf("expected name 'length-splitter', got %s", splitter.Name())
	}

	// Test negative buffer size.
	splitter2 := NewSplit[int](func(n int) bool {
		return n > 0
	}).WithBufferSize(-10)

	if splitter2.bufferSize != 0 {
		t.Errorf("expected buffer size 0 for negative input, got %d", splitter2.bufferSize)
	}
}

// BenchmarkSplit benchmarks splitting performance.
func BenchmarkSplit(b *testing.B) {
	ctx := context.Background()

	splitter := NewSplit[int](func(n int) bool {
		return n%2 == 0
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		outputs := splitter.Process(ctx, input)

		// Consume both outputs.
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			for range outputs.True { //nolint:revive // Intentionally draining channel
			}
		}()

		go func() {
			defer wg.Done()
			for range outputs.False { //nolint:revive // Intentionally draining channel
			}
		}()

		wg.Wait()
	}
}

// BenchmarkSplitThroughput benchmarks high-throughput splitting.
func BenchmarkSplitThroughput(b *testing.B) {
	ctx := context.Background()

	splitter := NewSplit[int](func(n int) bool {
		return n > b.N/2
	}).WithBufferSize(100)

	b.ResetTimer()
	b.ReportAllocs()

	input := make(chan int, 1000)
	outputs := splitter.Process(ctx, input)

	// Start consumers.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range outputs.True { //nolint:revive // Intentionally draining channel
		}
	}()

	go func() {
		defer wg.Done()
		for range outputs.False { //nolint:revive // Intentionally draining channel
		}
	}()

	// Send items.
	go func() {
		for i := 0; i < b.N; i++ {
			input <- i
		}
		close(input)
	}()

	wg.Wait()
}

// Example demonstrates basic splitting usage.
func ExampleSplit() {
	ctx := context.Background()

	// Split strings by length.
	splitter := NewSplit[string](func(s string) bool {
		return len(s) > 5
	})

	// Create input.
	words := make(chan string, 6)
	words <- "hello"      // 5 chars - false
	words <- "world"      // 5 chars - false
	words <- "streaming"  // 9 chars - true
	words <- "data"       // 4 chars - false
	words <- "processing" // 10 chars - true
	words <- "go"         // 2 chars - false
	close(words)

	// Process splits.
	outputs := splitter.Process(ctx, words)

	// Handle both outputs.
	var longWords, shortWords []string
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for word := range outputs.True {
			longWords = append(longWords, word)
		}
	}()

	go func() {
		defer wg.Done()
		for word := range outputs.False {
			shortWords = append(shortWords, word)
		}
	}()

	wg.Wait()

	// Print in deterministic order.
	fmt.Println("Long words:")
	for _, word := range longWords {
		fmt.Printf("  %s (%d chars)\n", word, len(word))
	}
	fmt.Println("Short words:")
	for _, word := range shortWords {
		fmt.Printf("  %s (%d chars)\n", word, len(word))
	}

	// Output:
	// Long words:
	//   streaming (9 chars)
	//   processing (10 chars)
	// Short words:
	//   hello (5 chars)
	//   world (5 chars)
	//   data (4 chars)
	//   go (2 chars)
}
