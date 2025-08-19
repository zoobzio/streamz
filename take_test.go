package streamz

import (
	"context"
	"fmt"
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

// Example demonstrates limiting results to first N items.
func ExampleTake() {
	ctx := context.Background()

	// Take only the first 5 search results.
	limiter := NewTake[string](5)

	// Simulate paginated search results.
	results := make(chan string)
	go func() {
		// Simulate multiple pages of results.
		for page := 1; page <= 3; page++ {
			for i := 1; i <= 10; i++ {
				results <- fmt.Sprintf("Page %d, Result %d", page, i)
			}
		}
		close(results)
	}()

	// Process only first 5 results.
	topResults := limiter.Process(ctx, results)

	fmt.Println("Top 5 search results:")
	for i, result := range collectResults(topResults) {
		fmt.Printf("%d. %s\n", i+1, result)
	}

	// Output:
	// Top 5 search results:
	// 1. Page 1, Result 1
	// 2. Page 1, Result 2
	// 3. Page 1, Result 3
	// 4. Page 1, Result 4
	// 5. Page 1, Result 5
}

// Helper function to collect results.
func collectResults(ch <-chan string) []string {
	//nolint:prealloc // size unknown at compile time
	var results []string
	for result := range ch {
		results = append(results, result)
	}
	return results
}
