package streamz

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestFanInBasic tests basic fan-in functionality.
func TestFanInBasic(t *testing.T) {
	ctx := context.Background()

	// Create multiple input channels.
	ch1 := make(chan int, 3)
	ch2 := make(chan int, 3)
	ch3 := make(chan int, 3)

	// Send data to channels.
	ch1 <- 1
	ch1 <- 2
	ch1 <- 3
	close(ch1)

	ch2 <- 4
	ch2 <- 5
	ch2 <- 6
	close(ch2)

	ch3 <- 7
	ch3 <- 8
	ch3 <- 9
	close(ch3)

	// Create fan-in.
	fanin := NewFanIn[int]()
	output := fanin.Process(ctx, ch1, ch2, ch3)

	// Collect results.
	results := make([]int, 0, 9)
	for val := range output {
		results = append(results, val)
	}

	// Verify we got all values.
	if len(results) != 9 {
		t.Errorf("expected 9 values, got %d", len(results))
	}

	// Sort to verify all values are present.
	sort.Ints(results)
	for i, val := range results {
		if val != i+1 {
			t.Errorf("position %d: expected %d, got %d", i, i+1, val)
		}
	}

	// FanIn processor created successfully.
}

// TestFanInConcurrent tests concurrent processing.
func TestFanInConcurrent(t *testing.T) {
	ctx := context.Background()

	// Create channels that will send data concurrently.
	ch1 := make(chan string)
	ch2 := make(chan string)
	ch3 := make(chan string)

	// Create fan-in.
	fanin := NewFanIn[string]()
	output := fanin.Process(ctx, ch1, ch2, ch3)

	// Collect results.
	var results []string
	var mu sync.Mutex
	done := make(chan bool)

	go func() {
		for val := range output {
			mu.Lock()
			results = append(results, val)
			mu.Unlock()
		}
		done <- true
	}()

	// Send data concurrently.
	go func() {
		for i := 0; i < 3; i++ {
			ch1 <- fmt.Sprintf("ch1-%d", i)
			time.Sleep(10 * time.Millisecond)
		}
		close(ch1)
	}()

	go func() {
		for i := 0; i < 3; i++ {
			ch2 <- fmt.Sprintf("ch2-%d", i)
			time.Sleep(15 * time.Millisecond)
		}
		close(ch2)
	}()

	go func() {
		for i := 0; i < 3; i++ {
			ch3 <- fmt.Sprintf("ch3-%d", i)
			time.Sleep(5 * time.Millisecond)
		}
		close(ch3)
	}()

	// Wait for completion.
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(results) != 9 {
		t.Errorf("expected 9 values, got %d", len(results))
	}

	// Verify all values are present.
	counts := make(map[string]int)
	for _, val := range results {
		counts[val]++
	}

	for i := 0; i < 3; i++ {
		for _, prefix := range []string{"ch1", "ch2", "ch3"} {
			key := fmt.Sprintf("%s-%d", prefix, i)
			if counts[key] != 1 {
				t.Errorf("expected exactly one %s, got %d", key, counts[key])
			}
		}
	}
}

// TestFanInEmptyChannels tests fan-in with empty channels.
func TestFanInEmptyChannels(t *testing.T) {
	ctx := context.Background()

	ch1 := make(chan int)
	ch2 := make(chan int)
	close(ch1)
	close(ch2)

	fanin := NewFanIn[int]()
	output := fanin.Process(ctx, ch1, ch2)

	count := 0
	for range output {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 values from empty channels, got %d", count)
	}
}

// TestFanInSingleChannel tests fan-in with just one channel.
func TestFanInSingleChannel(t *testing.T) {
	ctx := context.Background()

	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)

	fanin := NewFanIn[int]()
	output := fanin.Process(ctx, ch)

	results := make([]int, 0, 3)
	for val := range output {
		results = append(results, val)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 values, got %d", len(results))
	}
}

// TestFanInContextCancellation tests graceful shutdown.
func TestFanInContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create channels that continuously send data.
	ch1 := make(chan int)
	ch2 := make(chan int)

	go func() {
		for i := 0; ; i++ {
			select {
			case ch1 <- i:
				time.Sleep(10 * time.Millisecond)
			case <-ctx.Done():
				close(ch1)
				return
			}
		}
	}()

	go func() {
		for i := 100; ; i++ {
			select {
			case ch2 <- i:
				time.Sleep(10 * time.Millisecond)
			case <-ctx.Done():
				close(ch2)
				return
			}
		}
	}()

	fanin := NewFanIn[int]()
	output := fanin.Process(ctx, ch1, ch2)

	// Collect some values.
	count := 0
	done := make(chan bool)
	go func() {
		for range output {
			count++
			if count >= 10 {
				done <- true
				return
			}
		}
		done <- true
	}()

	// Wait for some processing then cancel.
	<-done
	cancel()

	if count < 10 {
		t.Errorf("expected at least 10 values before cancellation, got %d", count)
	}
}

// TestFanInDifferentRates tests channels sending at different rates.
func TestFanInDifferentRates(t *testing.T) {
	ctx := context.Background()

	fast := make(chan string)
	slow := make(chan string)

	// Fast sender.
	go func() {
		for i := 0; i < 10; i++ {
			fast <- fmt.Sprintf("fast-%d", i)
		}
		close(fast)
	}()

	// Slow sender.
	go func() {
		for i := 0; i < 3; i++ {
			time.Sleep(20 * time.Millisecond)
			slow <- fmt.Sprintf("slow-%d", i)
		}
		close(slow)
	}()

	fanin := NewFanIn[string]()
	output := fanin.Process(ctx, fast, slow)

	results := make([]string, 0, 13)
	for val := range output {
		results = append(results, val)
	}

	if len(results) != 13 {
		t.Errorf("expected 13 values total, got %d", len(results))
	}

	// Count by type.
	fastCount, slowCount := 0, 0
	for _, val := range results {
		if len(val) >= 4 && val[:4] == "fast" {
			fastCount++
		} else if len(val) >= 4 && val[:4] == "slow" {
			slowCount++
		}
	}

	if fastCount != 10 {
		t.Errorf("expected 10 fast values, got %d", fastCount)
	}
	if slowCount != 3 {
		t.Errorf("expected 3 slow values, got %d", slowCount)
	}
}

// BenchmarkFanIn benchmarks fan-in performance.
func BenchmarkFanIn(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ch1 := make(chan int, 100)
		ch2 := make(chan int, 100)
		ch3 := make(chan int, 100)

		// Fill channels.
		for j := 0; j < 100; j++ {
			ch1 <- j
			ch2 <- j + 100
			ch3 <- j + 200
		}
		close(ch1)
		close(ch2)
		close(ch3)

		fanin := NewFanIn[int]()
		output := fanin.Process(ctx, ch1, ch2, ch3)

		// Drain output.
		//nolint:revive // empty-block: necessary to drain channel
		for range output {
			// Consume.
		}
	}
}

// Example demonstrates merging multiple event streams into one.
func ExampleFanIn() {
	ctx := context.Background()

	// Multiple event sources.
	userEvents := make(chan string, 2)
	systemEvents := make(chan string, 2)
	alertEvents := make(chan string, 1)

	// Send events.
	userEvents <- "user:login:alice"
	userEvents <- "user:logout:bob"
	close(userEvents)

	systemEvents <- "system:cpu:high"
	systemEvents <- "system:memory:normal"
	close(systemEvents)

	alertEvents <- "alert:disk:full"
	close(alertEvents)

	// Merge all event streams.
	fanin := NewFanIn[string]()
	allEvents := fanin.Process(ctx, userEvents, systemEvents, alertEvents)

	// Process all events from single stream.
	events := []string{}
	for event := range allEvents {
		events = append(events, event)
	}

	// Sort for consistent output.
	sort.Strings(events)
	for _, event := range events {
		fmt.Println(event)
	}

	// Output:
	// alert:disk:full
	// system:cpu:high
	// system:memory:normal
	// user:login:alice
	// user:logout:bob
}
