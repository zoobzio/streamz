package streamz

import (
	"context"
	"testing"
)

func TestSample(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	sample := NewSample[int](0.5)
	out := sample.Process(ctx, in)

	go func() {
		for i := 0; i < 1000; i++ {
			in <- i
		}
		close(in)
	}()

	count := 0
	for range out {
		count++
	}

	if count < 400 || count > 600 {
		t.Errorf("expected ~500 items (50%% of 1000), got %d", count)
	}
}

func TestSampleNone(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	sample := NewSample[int](0.0)
	out := sample.Process(ctx, in)

	go func() {
		for i := 0; i < 100; i++ {
			in <- i
		}
		close(in)
	}()

	count := 0
	for range out {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 items with 0%% sample rate, got %d", count)
	}
}
