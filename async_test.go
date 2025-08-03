package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestAsyncMapper(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	mapper := NewAsyncMapper(3, func(_ context.Context, i int) (string, error) {
		time.Sleep(time.Duration(10-i) * time.Millisecond)
		return fmt.Sprintf("item-%d", i), nil
	})

	out := mapper.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	results := []string{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}

	for i, result := range results {
		expected := fmt.Sprintf("item-%d", i)
		if result != expected {
			t.Errorf("expected %s at position %d, got %s", expected, i, result)
		}
	}
}

func TestAsyncMapperWithErrors(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	mapper := NewAsyncMapper(2, func(_ context.Context, i int) (int, error) {
		if i%2 == 0 {
			return i * 2, nil
		}
		return 0, fmt.Errorf("odd number")
	})

	out := mapper.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	results := []int{}
	for result := range out {
		results = append(results, result)
	}

	expected := []int{0, 4, 8, 12, 16}
	if len(results) != len(expected) {
		t.Errorf("expected %d results (even numbers only), got %d: %v", len(expected), len(results), results)
	}

	for i, result := range results {
		if i < len(expected) && result != expected[i] {
			t.Errorf("expected %d at position %d, got %d", expected[i], i, result)
		}
	}
}

func TestAsyncMapperConcurrency(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	processing := make(chan int, 5)
	mapper := NewAsyncMapper(5, func(_ context.Context, i int) (int, error) {
		processing <- i
		defer func() { <-processing }()
		time.Sleep(50 * time.Millisecond)
		return i, nil
	})

	out := mapper.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	go func() {
		//nolint:revive // empty-block: necessary to drain output channel for concurrency test
		for range out {
		}
	}()

	time.Sleep(30 * time.Millisecond)

	concurrent := len(processing)
	if concurrent != 5 {
		t.Errorf("expected 5 concurrent operations, got %d", concurrent)
	}
}
