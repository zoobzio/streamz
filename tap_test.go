package streamz

import (
	"context"
	"fmt"
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

// Example demonstrates side effects without modifying the stream.
func ExampleTap() {
	ctx := context.Background()

	// Use tap for counting without affecting the stream.
	var processedCount atomic.Int64

	counter := NewTap(func(_ string) {
		processedCount.Add(1)
	})

	// Simulate event stream.
	events := make(chan string, 4)
	events <- "user:login"
	events <- "user:click"
	events <- "user:purchase"
	events <- "user:logout"
	close(events)

	// Process events with counting.
	counted := counter.Process(ctx, events)

	// Continue processing the unchanged stream.
	fmt.Println("Events processed:")
	for event := range counted {
		fmt.Printf("- %s\n", event)
	}

	fmt.Printf("\nTotal events: %d\n", processedCount.Load())

	// Output:
	// Events processed:
	// - user:login
	// - user:click
	// - user:purchase
	// - user:logout
	//
	// Total events: 4
}
