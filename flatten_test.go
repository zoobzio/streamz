package streamz

import (
	"context"
	"fmt"
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

// Example demonstrates flattening batched results.
func ExampleFlatten() {
	ctx := context.Background()

	// Simulate receiving batched search results.
	type SearchResult struct {
		Query string
		URL   string
		Score float64
	}

	// Flatten batches of search results into individual items.
	flattener := NewFlatten[SearchResult]()

	// Simulate batched results from multiple search engines.
	batches := make(chan []SearchResult, 3)

	// Results from search engine A.
	batches <- []SearchResult{
		{Query: "golang", URL: "https://golang.org", Score: 0.95},
		{Query: "golang", URL: "https://go.dev", Score: 0.93},
		{Query: "golang", URL: "https://tour.golang.org", Score: 0.90},
	}

	// Results from search engine B.
	batches <- []SearchResult{
		{Query: "golang", URL: "https://github.com/golang/go", Score: 0.92},
		{Query: "golang", URL: "https://pkg.go.dev", Score: 0.88},
	}

	// Results from search engine C (might be empty).
	batches <- []SearchResult{}

	close(batches)

	// Process flattened results.
	results := flattener.Process(ctx, batches)

	fmt.Println("All search results:")
	for result := range results {
		fmt.Printf("- %.2f: %s\n", result.Score, result.URL)
	}

	// Output:
	// All search results:
	// - 0.95: https://golang.org
	// - 0.93: https://go.dev
	// - 0.90: https://tour.golang.org
	// - 0.92: https://github.com/golang/go
	// - 0.88: https://pkg.go.dev
}
