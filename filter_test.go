package streamz

import (
	"context"
	"testing"
)

func TestFilter(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	isEven := NewFilter("even", func(n int) bool { return n%2 == 0 })
	out := isEven.Process(ctx, in)

	go func() {
		for i := 1; i <= 6; i++ {
			in <- i
		}
		close(in)
	}()

	results := []int{}
	for val := range out {
		results = append(results, val)
	}

	expected := []int{2, 4, 6}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d, got %d", expected[i], v)
		}
	}
}
