package streamz

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestTap(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	var sum atomic.Int64
	tap := NewTap(func(i int) {
		sum.Add(int64(i))
	})

	out := tap.Process(ctx, in)

	go func() {
		for i := 1; i <= 5; i++ {
			in <- i
		}
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 items, got %d", len(results))
	}

	for i, v := range results {
		if v != i+1 {
			t.Errorf("expected %d at position %d, got %d", i+1, i, v)
		}
	}

	if sum.Load() != 15 {
		t.Errorf("expected sum of 15 from tap function, got %d", sum.Load())
	}
}

func TestTapDoesNotModify(t *testing.T) {
	ctx := context.Background()
	in := make(chan *struct{ value int })

	tap := NewTap(func(s *struct{ value int }) {
		s.value = 999
	})

	out := tap.Process(ctx, in)

	original := &struct{ value int }{value: 42}
	go func() {
		in <- original
		close(in)
	}()

	result := <-out

	if result != original {
		t.Error("tap should pass through the same pointer")
	}

	if result.value != 999 {
		t.Error("tap function should be able to modify the item")
	}
}
