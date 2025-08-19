package streamz

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestFanOutBasic tests basic fan-out functionality.
func TestFanOutBasic(t *testing.T) {
	ctx := context.Background()

	// Create input channel.
	input := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		input <- i
	}
	close(input)

	// Create fan-out with 3 outputs.
	fanout := NewFanOut[int](3)
	outputs := fanout.Process(ctx, input)

	if len(outputs) != 3 {
		t.Fatalf("expected 3 output channels, got %d", len(outputs))
	}

	// Collect from all outputs.
	var wg sync.WaitGroup
	results := make([][]int, 3)

	for i, out := range outputs {
		wg.Add(1)
		go func(idx int, ch <-chan int) {
			defer wg.Done()
			for val := range ch {
				results[idx] = append(results[idx], val)
			}
		}(i, out)
	}

	wg.Wait()

	// Each output should have all values.
	for i, result := range results {
		if len(result) != 5 {
			t.Errorf("output %d: expected 5 values, got %d", i, len(result))
		}
		for j, val := range result {
			if val != j+1 {
				t.Errorf("output %d, position %d: expected %d, got %d", i, j, j+1, val)
			}
		}
	}

	// FanOut processor created successfully.
}

// TestFanOutConcurrentConsumers tests concurrent consumption.
func TestFanOutConcurrentConsumers(t *testing.T) {
	ctx := context.Background()

	input := make(chan string)
	fanout := NewFanOut[string](3)
	outputs := fanout.Process(ctx, input)

	// Start consumers with different processing speeds.
	var wg sync.WaitGroup
	counts := make([]int, 3)
	var mu sync.Mutex

	for i, out := range outputs {
		wg.Add(1)
		go func(idx int, ch <-chan string) {
			defer wg.Done()
			for range ch {
				// Simulate different processing speeds.
				time.Sleep(time.Duration(idx*5) * time.Millisecond)
				mu.Lock()
				counts[idx]++
				mu.Unlock()
			}
		}(i, out)
	}

	// Send data.
	go func() {
		for i := 0; i < 10; i++ {
			input <- fmt.Sprintf("msg-%d", i)
		}
		close(input)
	}()

	wg.Wait()

	// All consumers should receive all messages.
	mu.Lock()
	defer mu.Unlock()
	for i, count := range counts {
		if count != 10 {
			t.Errorf("consumer %d: expected 10 messages, got %d", i, count)
		}
	}
}

// TestFanOutEmptyInput tests fan-out with empty input.
func TestFanOutEmptyInput(t *testing.T) {
	ctx := context.Background()

	input := make(chan int)
	close(input)

	fanout := NewFanOut[int](2)
	outputs := fanout.Process(ctx, input)

	for i, out := range outputs {
		count := 0
		for range out {
			count++
		}
		if count != 0 {
			t.Errorf("output %d: expected 0 values, got %d", i, count)
		}
	}
}

// TestFanOutSingleOutput tests fan-out with n=1.
func TestFanOutSingleOutput(t *testing.T) {
	ctx := context.Background()

	input := make(chan int, 3)
	input <- 1
	input <- 2
	input <- 3
	close(input)

	fanout := NewFanOut[int](1)
	outputs := fanout.Process(ctx, input)

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output channel, got %d", len(outputs))
	}

	results := make([]int, 0, 3)
	for val := range outputs[0] {
		results = append(results, val)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 values, got %d", len(results))
	}
}

// TestFanOutContextCancellation tests graceful shutdown.
func TestFanOutContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	input := make(chan int)
	fanout := NewFanOut[int](2)
	outputs := fanout.Process(ctx, input)

	// Start consumers.
	var wg sync.WaitGroup
	counts := make([]int, 2)

	for i, out := range outputs {
		wg.Add(1)
		go func(idx int, ch <-chan int) {
			defer wg.Done()
			for range ch {
				counts[idx]++
			}
		}(i, out)
	}

	// Send data continuously.
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

	// Let it run for a bit.
	time.Sleep(100 * time.Millisecond)
	cancel()

	wg.Wait()

	// Should have received some values.
	for i, count := range counts {
		if count == 0 {
			t.Errorf("consumer %d: expected some values, got 0", i)
		}
	}
}

// TestFanOutLargeData tests fan-out with large data volume.
func TestFanOutLargeData(t *testing.T) {
	ctx := context.Background()

	input := make(chan int, 100)
	fanout := NewFanOut[int](5)
	outputs := fanout.Process(ctx, input)

	// Send lots of data.
	go func() {
		for i := 0; i < 1000; i++ {
			input <- i
		}
		close(input)
	}()

	// Concurrent consumers.
	var wg sync.WaitGroup
	sums := make([]int, 5)

	for i, out := range outputs {
		wg.Add(1)
		go func(idx int, ch <-chan int) {
			defer wg.Done()
			sum := 0
			for val := range ch {
				sum += val
			}
			sums[idx] = sum
		}(i, out)
	}

	wg.Wait()

	// All sums should be equal.
	expectedSum := (999 * 1000) / 2 // Sum of 0..999
	for i, sum := range sums {
		if sum != expectedSum {
			t.Errorf("consumer %d: expected sum %d, got %d", i, expectedSum, sum)
		}
	}
}

// TestFanOutZeroOutputs tests handling of n=0.
func TestFanOutZeroOutputs(t *testing.T) {
	ctx := context.Background()

	input := make(chan int, 1)
	input <- 42
	close(input)

	fanout := NewFanOut[int](0) // Creates 0 outputs
	outputs := fanout.Process(ctx, input)

	if len(outputs) != 0 {
		t.Fatalf("expected 0 output channels, got %d", len(outputs))
	}

	// No outputs to consume from - input is effectively discarded
}

// BenchmarkFanOut benchmarks fan-out performance.
func BenchmarkFanOut(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 100)
		fanout := NewFanOut[int](3)
		outputs := fanout.Process(ctx, input)

		// Start consumers.
		var wg sync.WaitGroup
		for _, out := range outputs {
			wg.Add(1)
			go func(ch <-chan int) {
				defer wg.Done()
				//nolint:revive // empty-block: necessary to drain channel
				for range ch {
					// Consume.
				}
			}(out)
		}

		// Send data.
		for j := 0; j < 100; j++ {
			input <- j
		}
		close(input)

		wg.Wait()
	}
}

// Example demonstrates distributing work to multiple workers.
func ExampleFanOut() {
	ctx := context.Background()

	// Create tasks to distribute.
	tasks := make(chan string, 6)
	tasks <- "process-order-123"
	tasks <- "send-email-456"
	tasks <- "update-inventory-789"
	tasks <- "generate-report-012"
	tasks <- "backup-data-345"
	tasks <- "analyze-metrics-678"
	close(tasks)

	// Distribute to 3 workers.
	fanout := NewFanOut[string](3)
	workerQueues := fanout.Process(ctx, tasks)

	// Simulate workers processing tasks.
	var wg sync.WaitGroup
	for i, queue := range workerQueues {
		wg.Add(1)
		go func(workerID int, tasks <-chan string) {
			defer wg.Done()
			for task := range tasks {
				fmt.Printf("Worker %d: %s\n", workerID, task)
			}
		}(i+1, queue)
	}

	wg.Wait()

	// Output (order may vary):
	// Worker 1: process-order-123
	// Worker 2: send-email-456
	// Worker 3: update-inventory-789
	// Worker 1: generate-report-012
	// Worker 2: backup-data-345
	// Worker 3: analyze-metrics-678
}
