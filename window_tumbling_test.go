package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestTumblingWindow(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	clk := NewFakeClock(time.Now())
	window := NewTumblingWindow[int](100*time.Millisecond, clk)
	out := window.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
			if i < 9 {
				clk.Step(30 * time.Millisecond)
			}
		}
		close(in)
	}()

	windows := []Window[int]{}
	done := make(chan bool)
	go func() {
		for w := range out {
			windows = append(windows, w)
		}
		done <- true
	}()

	// Advance clock to trigger window emissions
	for i := 0; i < 4; i++ {
		clk.Step(100 * time.Millisecond)
		clk.BlockUntilReady()
		time.Sleep(10 * time.Millisecond) // Allow goroutine scheduling
	}

	<-done

	if len(windows) < 2 {
		t.Errorf("expected at least 2 windows, got %d", len(windows))
	}

	totalItems := 0
	for _, w := range windows {
		totalItems += len(w.Items)
	}

	if totalItems != 10 {
		t.Errorf("expected 10 total items, got %d", totalItems)
	}
}

// Example demonstrates using tumbling windows for time-based aggregation.
func ExampleTumblingWindow() {
	ctx := context.Background()

	// Aggregate metrics into 1-minute windows.
	window := NewTumblingWindow[float64](time.Minute, RealClock)

	// Simulate metric stream.
	metrics := make(chan float64)
	go func() {
		// Send metrics over 2.5 minutes.
		for i := 0; i < 150; i++ {
			metrics <- float64(i)
			time.Sleep(time.Second)
		}
		close(metrics)
	}()

	// Process windows.
	windows := window.Process(ctx, metrics)

	for w := range windows {
		if len(w.Items) > 0 {
			var sum float64
			for _, v := range w.Items {
				sum += v
			}
			avg := sum / float64(len(w.Items))
			fmt.Printf("Window [%s-%s]: %d items, avg: %.1f\n",
				w.Start.Format("15:04"),
				w.End.Format("15:04"),
				len(w.Items),
				avg)
		}
	}
}
