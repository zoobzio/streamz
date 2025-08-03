package streamz

import (
	"context"
	"testing"
)

func TestSkip(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	skip := NewSkip[int](3)
	out := skip.Process(ctx, in)

	go func() {
		for i := 0; i < 6; i++ {
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

	expected := []int{3, 4, 5}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d at position %d, got %d", expected[i], i, v)
		}
	}
}

func TestSkipAll(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	skip := NewSkip[int](10)
	out := skip.Process(ctx, in)

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

	if count != 0 {
		t.Errorf("expected 0 items when skipping more than available, got %d", count)
	}
}
