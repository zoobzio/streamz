package streamz

import (
	"context"
	"testing"
)

func TestFlatten(t *testing.T) {
	ctx := context.Background()
	in := make(chan []int)

	flatten := NewFlatten[int]()
	out := flatten.Process(ctx, in)

	go func() {
		in <- []int{1, 2, 3}
		in <- []int{4, 5}
		in <- []int{}
		in <- []int{6}
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	expected := []int{1, 2, 3, 4, 5, 6}
	if len(results) != len(expected) {
		t.Errorf("expected %d items, got %d", len(expected), len(results))
	}

	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d at position %d, got %d", expected[i], i, v)
		}
	}
}

func TestFlattenEmpty(t *testing.T) {
	ctx := context.Background()
	in := make(chan []int)

	flatten := NewFlatten[int]()
	out := flatten.Process(ctx, in)

	go func() {
		in <- []int{}
		in <- []int{}
		close(in)
	}()

	count := 0
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 items from empty slices, got %d", count)
	}
}
