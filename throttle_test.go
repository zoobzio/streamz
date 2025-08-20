package streamz

import (
	"context"
	"testing"
	"time"
)

func TestThrottle(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	throttle := NewThrottle[int](10, RealClock)
	out := throttle.Process(ctx, in)

	start := time.Now()

	go func() {
		for i := 0; i < 5; i++ {
			in <- i
		}
		close(in)
	}()

	count := 0
	for range out {
		count++
	}

	elapsed := time.Since(start)
	expectedMin := 400 * time.Millisecond
	expectedMax := 600 * time.Millisecond

	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("expected ~500ms for 5 items at 10/sec, got %v", elapsed)
	}

	if count != 5 {
		t.Errorf("expected 5 items, got %d", count)
	}
}

func TestThrottleHighRate(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	throttle := NewThrottle[int](1000, RealClock)
	out := throttle.Process(ctx, in)

	go func() {
		for i := 0; i < 100; i++ {
			in <- i
		}
		close(in)
	}()

	start := time.Now()
	count := 0
	for range out {
		count++
	}
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("high rate throttle too slow: %v", elapsed)
	}

	if count != 100 {
		t.Errorf("expected 100 items, got %d", count)
	}
}
