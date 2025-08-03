package streamz

import (
	"context"
	"testing"
)

func TestUnbatcher(t *testing.T) {
	ctx := context.Background()
	in := make(chan []int)

	unbatcher := NewUnbatcher[int]()
	out := unbatcher.Process(ctx, in)

	go func() {
		in <- []int{1, 2, 3}
		in <- []int{4, 5}
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	expected := []int{1, 2, 3, 4, 5}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d, got %d", expected[i], v)
		}
	}
}
