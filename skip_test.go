package streamz

import (
	"context"
	"fmt"
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

// Example demonstrates skipping header rows in data processing.
func ExampleSkip() {
	ctx := context.Background()

	// Skip the first 3 lines (headers and metadata).
	skipper := NewSkip[string](3)

	// Simulate CSV data with headers.
	lines := make(chan string, 8)
	lines <- "# Generated on 2024-01-18"
	lines <- "# Data format: ID,Name,Value"
	lines <- "ID,Name,Value"
	lines <- "1,Alice,100"
	lines <- "2,Bob,200"
	lines <- "3,Carol,300"
	lines <- "4,Dave,400"
	lines <- "5,Eve,500"
	close(lines)

	// Process data rows only.
	dataRows := skipper.Process(ctx, lines)

	fmt.Println("Data rows:")
	for row := range dataRows {
		fmt.Printf("- %s\n", row)
	}

	// Output:
	// Data rows:
	// - 1,Alice,100
	// - 2,Bob,200
	// - 3,Carol,300
	// - 4,Dave,400
	// - 5,Eve,500
}
