package streamz

import (
	"context"
	"testing"
)

func TestTake(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	take := NewTake[int](3)
	out := take.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 items, got %d", len(results))
	}

	expected := []int{0, 1, 2}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d at position %d, got %d", expected[i], i, v)
		}
	}
}

func TestTakeZero(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	take := NewTake[int](0)
	out := take.Process(ctx, in)

	go func() {
		in <- 1
		in <- 2
		close(in)
	}()

	count := 0
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 items, got %d", count)
	}
}
