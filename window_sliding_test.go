package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestSlidingWindowBasic tests basic sliding window functionality.
func TestSlidingWindowBasic(t *testing.T) {
	ctx := context.Background()

	// 100ms windows sliding every 50ms.
	clk := NewFakeClock(time.Now())
	windower := NewSlidingWindow[int](100*time.Millisecond, clk).
		WithSlide(50 * time.Millisecond).
		WithName("test-sliding")

	input := make(chan int)
	output := windower.Process(ctx, input)

	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	// Send items over time.
	go func() {
		for i := 0; i < 5; i++ {
			input <- i
			clk.Step(30 * time.Millisecond)
		}
		// Wait for final windows.
		clk.Step(200 * time.Millisecond)
		close(input)
	}()

	// Advance clock to process ticks
	for i := 0; i < 10; i++ {
		clk.Step(50 * time.Millisecond)
		clk.BlockUntilReady()
		time.Sleep(10 * time.Millisecond) // Allow goroutine scheduling
	}

	<-done

	// Should have multiple overlapping windows.
	if len(windows) < 3 {
		t.Errorf("expected at least 3 windows, got %d", len(windows))
	}

	// Windows should overlap (contain some same items).
	hasOverlap := false
	for i := 1; i < len(windows); i++ {
		for _, item1 := range windows[i-1].Items {
			for _, item2 := range windows[i].Items {
				if item1 == item2 {
					hasOverlap = true
					break
				}
			}
		}
	}

	if !hasOverlap {
		t.Error("expected windows to have overlapping items")
	}

	if windower.Name() != "test-sliding" {
		t.Errorf("expected name 'test-sliding', got %s", windower.Name())
	}
}

// TestSlidingWindowTumbling tests tumbling windows (slide = size).
func TestSlidingWindowTumbling(t *testing.T) {
	ctx := context.Background()

	// When slide equals size, it's a tumbling window.
	windower := NewSlidingWindow[int](100*time.Millisecond, RealClock).
		WithSlide(100 * time.Millisecond)

	input := make(chan int)
	output := windower.Process(ctx, input)

	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	go func() {
		// Send 10 items quickly.
		for i := 0; i < 10; i++ {
			input <- i
		}
		// Wait for windows to complete.
		time.Sleep(250 * time.Millisecond)
		close(input)
	}()

	<-done

	// Windows should not overlap.
	seen := make(map[int]bool)
	for _, window := range windows {
		for _, item := range window.Items {
			if seen[item] {
				t.Errorf("item %d appeared in multiple windows (should not overlap)", item)
			}
			seen[item] = true
		}
	}
}

// TestSlidingWindowMaxCount tests window behavior with many items.
func TestSlidingWindowMaxCount(t *testing.T) {
	ctx := context.Background()

	windower := NewSlidingWindow[string](200*time.Millisecond, RealClock).
		WithSlide(100 * time.Millisecond)

	input := make(chan string)
	output := windower.Process(ctx, input)

	var windows []Window[string]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	go func() {
		// Send many items quickly.
		for i := 0; i < 10; i++ {
			input <- fmt.Sprintf("item-%d", i)
		}
		// Wait for windows.
		time.Sleep(300 * time.Millisecond)
		close(input)
	}()

	<-done

	// Check that windows were created properly.
	if len(windows) == 0 {
		t.Error("expected at least one window")
	}

	// Windows should contain items based on timing.
	totalItems := 0
	for _, window := range windows {
		totalItems += len(window.Items)
	}

	// Due to overlapping windows, items may appear multiple times.
	if totalItems == 0 {
		t.Error("expected some items in windows")
	}
}

// TestSlidingWindowEmptyInput tests empty input handling.
func TestSlidingWindowEmptyInput(t *testing.T) {
	ctx := context.Background()

	windower := NewSlidingWindow[int](100*time.Millisecond, RealClock).
		WithSlide(50 * time.Millisecond)

	input := make(chan int)
	close(input)

	output := windower.Process(ctx, input)

	count := 0
	for range output {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 windows for empty input, got %d", count)
	}
}

// TestSlidingWindowSingleItem tests window with single item.
func TestSlidingWindowSingleItem(t *testing.T) {
	ctx := context.Background()

	windower := NewSlidingWindow[int](100*time.Millisecond, RealClock).
		WithSlide(50 * time.Millisecond)

	input := make(chan int)
	output := windower.Process(ctx, input)

	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	go func() {
		input <- 42
		// Wait for windows containing this item.
		time.Sleep(200 * time.Millisecond)
		close(input)
	}()

	<-done

	// Item should appear in multiple overlapping windows.
	itemCount := 0
	for _, window := range windows {
		for _, item := range window.Items {
			if item == 42 {
				itemCount++
			}
		}
	}

	if itemCount < 2 {
		t.Errorf("expected item to appear in at least 2 windows, appeared in %d", itemCount)
	}
}

// TestSlidingWindowContextCancellation tests graceful shutdown.
func TestSlidingWindowContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	windower := NewSlidingWindow[int](100*time.Millisecond, RealClock).
		WithSlide(50 * time.Millisecond)

	input := make(chan int)
	output := windower.Process(ctx, input)

	windowCount := 0
	done := make(chan bool)
	go func() {
		for range output {
			windowCount++
		}
		done <- true
	}()

	// Send continuous data.
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
	time.Sleep(200 * time.Millisecond)
	cancel()

	<-done

	// Should have received some windows.
	if windowCount == 0 {
		t.Error("expected some windows before cancellation")
	}
}

// TestSlidingWindowConfiguration tests configuration validation.
func TestSlidingWindowConfiguration(t *testing.T) {
	ctx := context.Background()

	// Test with slide > size (should adjust).
	windower := NewSlidingWindow[int](50*time.Millisecond, RealClock).
		WithSlide(100 * time.Millisecond) // Larger than size.

	input := make(chan int)
	output := windower.Process(ctx, input)

	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 5; i++ {
			input <- i
			time.Sleep(30 * time.Millisecond)
		}
		time.Sleep(200 * time.Millisecond)
		close(input)
	}()

	<-done

	// Should still produce windows.
	if len(windows) == 0 {
		t.Error("expected windows even with slide > size")
	}
}

// BenchmarkSlidingWindow benchmarks sliding window performance.
func BenchmarkSlidingWindow(b *testing.B) {
	ctx := context.Background()

	windower := NewSlidingWindow[int](100*time.Millisecond, RealClock).
		WithSlide(50 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 100)
		output := windower.Process(ctx, input)

		done := make(chan bool)
		go func() {
			//nolint:revive // empty-block: necessary to drain channel
			for range output {
				// Consume.
			}
			done <- true
		}()

		// Send items.
		for j := 0; j < 100; j++ {
			input <- j
		}
		close(input)

		<-done
	}
}

// Example demonstrates using sliding windows for rolling averages.
func ExampleSlidingWindow() {
	ctx := context.Background()

	// 5-second windows, sliding every second.
	windower := NewSlidingWindow[float64](5*time.Second, RealClock).
		WithSlide(time.Second)

	// Simulate temperature readings.
	readings := make(chan float64)
	go func() {
		temps := []float64{20.5, 21.0, 21.5, 22.0, 22.5, 23.0}
		for _, temp := range temps {
			readings <- temp
			time.Sleep(time.Second)
		}
		// Wait for final windows.
		time.Sleep(6 * time.Second)
		close(readings)
	}()

	// Process windows.
	windows := windower.Process(ctx, readings)

	for window := range windows {
		if len(window.Items) > 0 {
			var sum float64
			for _, temp := range window.Items {
				sum += temp
			}
			avg := sum / float64(len(window.Items))
			fmt.Printf("Window [%s-%s]: %.1f°C average\n",
				window.Start.Format("15:04:05"),
				window.End.Format("15:04:05"),
				avg)
		}
	}
}
