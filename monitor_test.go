package streamz

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitor(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	var stats atomic.Value
	monitor := NewMonitor[int](100*time.Millisecond, func(s StreamStats) {
		stats.Store(s)
	})

	out := monitor.Process(ctx, in)

	go func() {
		for i := 0; i < 50; i++ {
			in <- i
			time.Sleep(2 * time.Millisecond)
		}
		close(in)
	}()

	count := 0
	for range out {
		count++
	}

	if count != 50 {
		t.Errorf("expected 50 items, got %d", count)
	}

	time.Sleep(150 * time.Millisecond)

	if s, ok := stats.Load().(StreamStats); ok {
		if s.Count == 0 {
			t.Error("expected stats to be reported")
		}
		if s.Rate == 0 {
			t.Error("expected rate to be calculated")
		}
	} else {
		t.Error("no stats reported")
	}
}

func TestMonitorRate(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	var mu sync.Mutex
	rates := []float64{}
	monitor := NewMonitor[int](50*time.Millisecond, func(s StreamStats) {
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
