package streamz

import (
	"context"
	"testing"
	"time"
)

func TestDebounce(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	debounce := NewDebounce[int](50 * time.Millisecond)
	out := debounce.Process(ctx, in)

	go func() {
		in <- 1
		time.Sleep(10 * time.Millisecond)
		in <- 2
		time.Sleep(10 * time.Millisecond)
		in <- 3
		time.Sleep(60 * time.Millisecond)
		in <- 4
		time.Sleep(60 * time.Millisecond)
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 debounced values, got %d: %v", len(results), results)
	}

	if len(results) >= 2 {
		if results[0] != 3 {
			t.Errorf("expected first debounced value to be 3, got %d", results[0])
		}
		if results[1] != 4 {
			t.Errorf("expected second debounced value to be 4, got %d", results[1])
		}
	}
}

func TestDebounceRapidFire(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	debounce := NewDebounce[int](100 * time.Millisecond)
	out := debounce.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
			time.Sleep(10 * time.Millisecond)
		}
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 debounced value from rapid fire, got %d", len(results))
	}

	if len(results) > 0 && results[0] != 9 {
		t.Errorf("expected debounced value to be last item (9), got %d", results[0])
	}
}

func TestDebounceFinalFlush(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	debounce := NewDebounce[int](50 * time.Millisecond)
	out := debounce.Process(ctx, in)

	go func() {
		in <- 42
		close(in)
	}()

	result := <-out

	if result != 42 {
		t.Errorf("expected final item to be flushed on close, got %d", result)
	}
}
