package streamz

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"streamz/clock"
	clocktesting "streamz/clock/testing"
)

func TestMonitor(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 100) // Buffered to avoid blocking

	var stats atomic.Value
	clk := clocktesting.NewFakeClock(time.Now())
	monitor := NewMonitor[int](100*time.Millisecond, clk).OnStats(func(s StreamStats) {
		stats.Store(s)
	})

	out := monitor.Process(ctx, in)

	// Consume output
	consumed := 0
	done := make(chan bool)
	go func() {
		for range out {
			consumed++
		}
		done <- true
	}()

	// Send items
	for i := 0; i < 50; i++ {
		in <- i
	}

	// Wait for all items to be consumed
	time.Sleep(10 * time.Millisecond)

	// Advance time to trigger stats report
	clk.Step(150 * time.Millisecond)
	clk.BlockUntilReady()
	time.Sleep(20 * time.Millisecond) // Allow goroutine scheduling

	// Check stats were reported
	if s, ok := stats.Load().(StreamStats); ok {
		if s.Count != 50 {
			t.Errorf("expected count of 50, got %d", s.Count)
		}
		if s.Rate == 0 {
			t.Error("expected rate to be calculated")
		}
		// Rate should be 50 items / 0.15 seconds = ~333.33 items/sec
		expectedRate := 50.0 / 0.15
		if s.Rate < expectedRate*0.9 || s.Rate > expectedRate*1.1 {
			t.Errorf("expected rate around %.2f, got %.2f", expectedRate, s.Rate)
		}
	} else {
		t.Error("no stats reported")
	}

	close(in)
}

func TestMonitorRate(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	var mu sync.Mutex
	rates := []float64{}
	monitor := NewMonitor[int](50*time.Millisecond, clock.Real).OnStats(func(s StreamStats) {
		if s.Rate > 0 {
			mu.Lock()
			rates = append(rates, s.Rate)
			mu.Unlock()
		}
	})

	out := monitor.Process(ctx, in)

	go func() {
		for i := 0; i < 100; i++ {
			in <- i
			time.Sleep(time.Millisecond)
		}
		close(in)
	}()

	go func() {
		//nolint:revive // empty-block: necessary to drain output channel for test
		for range out {
		}
	}()

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	ratesCopy := make([]float64, len(rates))
	copy(ratesCopy, rates)
	mu.Unlock()

	if len(ratesCopy) < 2 {
		t.Errorf("expected at least 2 rate reports, got %d", len(ratesCopy))
	}

	for _, rate := range ratesCopy {
		if rate < 100 || rate > 1100 {
			t.Errorf("expected rate around 1000/sec, got %f", rate)
		}
	}
}
