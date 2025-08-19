package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"

	"streamz/clock"
	clocktesting "streamz/clock/testing"
)

func TestDebounce(t *testing.T) {
	ctx := context.Background()
	in := make(chan int, 10) // Buffered to avoid blocking

	clk := clocktesting.NewFakeClock(time.Now())
	debounce := NewDebounce[int](50*time.Millisecond, clk)
	out := debounce.Process(ctx, in)

	results := []int{}
	done := make(chan bool)
	go func() {
		for val := range out {
			results = append(results, val)
		}
		done <- true
	}()

	// Rapid succession of values
	in <- 1
	clk.Step(10 * time.Millisecond)
	in <- 2
	clk.Step(10 * time.Millisecond)
	in <- 3

	// Let debounce timer expire
	clk.Step(60 * time.Millisecond)
	clk.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Allow goroutine scheduling

	// Another value after gap
	in <- 4

	// Let second debounce timer expire
	clk.Step(60 * time.Millisecond)
	clk.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Allow goroutine scheduling

	close(in)
	<-done

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

	debounce := NewDebounce[int](100*time.Millisecond, clock.Real)
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

	debounce := NewDebounce[int](50*time.Millisecond, clock.Real)
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

// Example demonstrates debouncing rapid user input.
func ExampleDebounce() {
	ctx := context.Background()

	// Debounce search queries to avoid excessive API calls.
	// Wait 100ms after last keystroke before searching.
	debouncer := NewDebounce[string](100*time.Millisecond, clock.Real)

	// Simulate user typing a search query.
	queries := make(chan string)
	go func() {
		// User types quickly.
		queries <- "h"
		time.Sleep(20 * time.Millisecond)
		queries <- "he"
		time.Sleep(20 * time.Millisecond)
		queries <- "hel"
		time.Sleep(20 * time.Millisecond)
		queries <- "hell"
		time.Sleep(20 * time.Millisecond)
		queries <- "hello"

		// User pauses (debounce triggers).
		time.Sleep(150 * time.Millisecond)

		// User continues typing.
		queries <- "hello w"
		time.Sleep(20 * time.Millisecond)
		queries <- "hello wo"
		time.Sleep(20 * time.Millisecond)
		queries <- "hello wor"
		time.Sleep(20 * time.Millisecond)
		queries <- "hello worl"
		time.Sleep(20 * time.Millisecond)
		queries <- "hello world"

		// Wait for final debounce.
		time.Sleep(150 * time.Millisecond)
		close(queries)
	}()

	// Process debounced queries.
	debounced := debouncer.Process(ctx, queries)

	fmt.Println("Search queries sent:")
	for query := range debounced {
		fmt.Printf("- Searching for: '%s'\n", query)
	}

	// Output:
	// Search queries sent:
	// - Searching for: 'hello'
	// - Searching for: 'hello world'
}
