package streamz

import (
	"context"
	"testing"
	"time"
)

func TestTumblingWindow(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	window := NewTumblingWindow[int](100 * time.Millisecond)
	out := window.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
			time.Sleep(30 * time.Millisecond)
		}
		close(in)
	}()

	windows := []Window[int]{}
	for w := range out {
		windows = append(windows, w)
	}

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
