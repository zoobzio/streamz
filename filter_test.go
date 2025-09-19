package streamz

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFilter_BasicFiltering(t *testing.T) {
	// Create a filter that keeps positive numbers
	filter := NewFilter(func(n int) bool {
		return n > 0
	})

	// Test data with mixed positive and negative numbers
	input := make(chan Result[int], 5)
	input <- NewSuccess(-2)
	input <- NewSuccess(-1)
	input <- NewSuccess(0)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	close(input)

	// Process
	ctx := context.Background()
	results := filter.Process(ctx, input)

	// Collect results
	outputs := make([]int, 0, 3)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		outputs = append(outputs, result.Value())
	}

	// Verify only positive numbers passed through
	expected := []int{1, 2}
	if len(outputs) != len(expected) {
		t.Fatalf("Expected %d results, got %d", len(expected), len(outputs))
	}
	for i, exp := range expected {
		if outputs[i] != exp {
			t.Errorf("Expected outputs[%d] = %d, got %d", i, exp, outputs[i])
		}
	}
}

func TestFilter_StringFiltering(t *testing.T) {
	// Create a filter that keeps non-empty strings
	filter := NewFilter(func(s string) bool {
		return strings.TrimSpace(s) != ""
	})

	// Test data with empty and non-empty strings
	input := make(chan Result[string], 5)
	input <- NewSuccess("hello")
	input <- NewSuccess("")
	input <- NewSuccess("  ")
	input <- NewSuccess("world")
	input <- NewSuccess("\t")
	close(input)

	// Process
	ctx := context.Background()
	results := filter.Process(ctx, input)

	// Collect results
	outputs := make([]string, 0, 2)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		outputs = append(outputs, result.Value())
	}

	// Verify only non-empty strings passed through
	expected := []string{"hello", "world"}
	if len(outputs) != len(expected) {
		t.Fatalf("Expected %d results, got %d", len(expected), len(outputs))
	}
	for i, exp := range expected {
		if outputs[i] != exp {
			t.Errorf("Expected outputs[%d] = %s, got %s", i, exp, outputs[i])
		}
	}
}

func TestFilter_AllPass(t *testing.T) {
	// Create a filter that accepts everything
	filter := NewFilter(func(_ int) bool {
		return true
	})

	// Test data
	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	// Process
	ctx := context.Background()
	results := filter.Process(ctx, input)

	// Collect results
	outputs := make([]int, 0, 3)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		outputs = append(outputs, result.Value())
	}

	// Verify all items passed through
	expected := []int{1, 2, 3}
	if len(outputs) != len(expected) {
		t.Fatalf("Expected %d results, got %d", len(expected), len(outputs))
	}
	for i, exp := range expected {
		if outputs[i] != exp {
			t.Errorf("Expected outputs[%d] = %d, got %d", i, exp, outputs[i])
		}
	}
}

func TestFilter_NonePass(t *testing.T) {
	// Create a filter that rejects everything
	filter := NewFilter(func(_ int) bool {
		return false
	})

	// Test data
	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	// Process
	ctx := context.Background()
	results := filter.Process(ctx, input)

	// Collect results
	outputs := make([]int, 0, 3)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		outputs = append(outputs, result.Value())
	}

	// Verify no items passed through
	if len(outputs) != 0 {
		t.Errorf("Expected no results, got %d", len(outputs))
	}
}

func TestFilter_PassThroughErrors(t *testing.T) {
	// Create a simple filter
	filter := NewFilter(func(n int) bool {
		return n > 0
	})

	// Test data with input errors
	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewError(0, errors.New("input error"), "test-source")
	input <- NewSuccess(2)
	close(input)

	// Process
	ctx := context.Background()
	results := filter.Process(ctx, input)

	// Collect results
	var successes []int
	var errorResults []error
	for result := range results {
		if result.IsError() {
			errorResults = append(errorResults, result.Error())
		} else {
			successes = append(successes, result.Value())
		}
	}

	// Verify successes and errors
	expectedSuccesses := []int{1, 2}
	if len(successes) != len(expectedSuccesses) {
		t.Fatalf("Expected %d successes, got %d", len(expectedSuccesses), len(successes))
	}
	if len(errorResults) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errorResults))
	}

	for i, exp := range expectedSuccesses {
		if successes[i] != exp {
			t.Errorf("Expected successes[%d] = %d, got %d", i, exp, successes[i])
		}
	}

	// Verify the error contains original error information
	if !strings.Contains(errorResults[0].Error(), "input error") {
		t.Errorf("Expected passed-through error to contain 'input error', got: %v", errorResults[0])
	}
}

func TestFilter_ContextCancellation(t *testing.T) {
	// Create a filter that would pass everything
	filter := NewFilter(func(_ int) bool {
		return true
	})

	// Test data
	input := make(chan Result[int], 2)
	input <- NewSuccess(1)
	input <- NewSuccess(2)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	results := filter.Process(ctx, input)

	// Get the first result
	result := <-results
	if result.IsError() {
		t.Fatalf("Unexpected error in first result: %v", result.Error())
	}
	if result.Value() != 1 {
		t.Fatalf("Expected first result to be 1, got %d", result.Value())
	}

	// Cancel context immediately
	cancel()
	close(input)

	// Verify that processing stops quickly
	done := make(chan bool)
	go func() {
		for result := range results {
			_ = result // Consume any remaining results
		}
		done <- true
	}()

	select {
	case <-done:
		// Good - processing stopped
	case <-time.After(500 * time.Millisecond):
		t.Error("Processing did not stop promptly after context cancellation")
	}
}

func TestFilter_EmptyInput(t *testing.T) {
	// Create a simple filter
	filter := NewFilter(func(n int) bool {
		return n > 0
	})

	// Empty input
	input := make(chan Result[int])
	close(input)

	// Process
	ctx := context.Background()
	results := filter.Process(ctx, input)

	// Verify no results
	var count int
	for range results {
		count++
	}

	if count != 0 {
		t.Errorf("Expected no results from empty input, got %d", count)
	}
}

func TestFilter_WithName(t *testing.T) {
	// Create filter with custom name
	filter := NewFilter(func(n int) bool {
		return n > 0
	}).WithName("test-filter")

	// Verify name
	if filter.Name() != "test-filter" {
		t.Errorf("Expected name 'test-filter', got '%s'", filter.Name())
	}

	// Test that errors include the custom name when passing through
	input := make(chan Result[int], 1)
	input <- NewError(0, errors.New("test error"), "original-source")
	close(input)

	ctx := context.Background()
	results := filter.Process(ctx, input)

	result := <-results
	if !result.IsError() {
		t.Fatal("Expected error result")
	}

	if !strings.Contains(result.Error().Error(), "test-filter") {
		t.Errorf("Expected error to contain processor name 'test-filter', got: %v", result.Error())
	}
}

func TestFilter_DefaultName(t *testing.T) {
	// Create filter without custom name
	filter := NewFilter(func(n int) bool {
		return n > 0
	})

	// Verify default name
	if filter.Name() != "filter" {
		t.Errorf("Expected default name 'filter', got '%s'", filter.Name())
	}
}

func TestFilter_ComplexPredicate(t *testing.T) {
	// Test a more complex predicate with struct types
	type User struct {
		Name     string
		Age      int
		Email    string
		Verified bool
	}

	// Create filter for valid users
	filter := NewFilter(func(u User) bool {
		return u.Name != "" &&
			u.Age >= 18 &&
			strings.Contains(u.Email, "@") &&
			u.Verified
	})

	// Test data
	input := make(chan Result[User], 5)
	input <- NewSuccess(User{Name: "Alice", Age: 25, Email: "alice@example.com", Verified: true})  // Valid
	input <- NewSuccess(User{Name: "Bob", Age: 16, Email: "bob@example.com", Verified: true})      // Too young
	input <- NewSuccess(User{Name: "", Age: 30, Email: "empty@example.com", Verified: true})       // No name
	input <- NewSuccess(User{Name: "Charlie", Age: 22, Email: "invalid-email", Verified: true})    // Bad email
	input <- NewSuccess(User{Name: "Diana", Age: 28, Email: "diana@example.com", Verified: false}) // Not verified
	close(input)

	// Process
	ctx := context.Background()
	results := filter.Process(ctx, input)

	// Collect results
	validUsers := make([]User, 0, 2)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		validUsers = append(validUsers, result.Value())
	}

	// Verify only Alice passed the filter
	if len(validUsers) != 1 {
		t.Fatalf("Expected 1 valid user, got %d", len(validUsers))
	}

	if validUsers[0].Name != "Alice" {
		t.Errorf("Expected Alice to pass the filter, got: %+v", validUsers[0])
	}
}

func TestFilter_EvenOddFiltering(t *testing.T) {
	// Test filtering even numbers
	evenFilter := NewFilter(func(n int) bool {
		return n%2 == 0
	}).WithName("even")

	// Test data
	input := make(chan Result[int], 6)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	input <- NewSuccess(4)
	input <- NewSuccess(5)
	input <- NewSuccess(6)
	close(input)

	// Process
	ctx := context.Background()
	results := evenFilter.Process(ctx, input)

	// Collect results
	evens := make([]int, 0, 3)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		evens = append(evens, result.Value())
	}

	// Verify only even numbers passed through
	expected := []int{2, 4, 6}
	if len(evens) != len(expected) {
		t.Fatalf("Expected %d even numbers, got %d", len(expected), len(evens))
	}
	for i, exp := range expected {
		if evens[i] != exp {
			t.Errorf("Expected evens[%d] = %d, got %d", i, exp, evens[i])
		}
	}
}

// Benchmark to ensure the filter has minimal overhead.
func BenchmarkFilter(b *testing.B) {
	filter := NewFilter(func(n int) bool {
		return n > 0
	})

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			input := make(chan Result[int], 1)
			input <- NewSuccess(42)
			close(input)

			results := filter.Process(ctx, input)
			result := <-results

			if result.IsError() || result.Value() != 42 {
				b.Fatal("Unexpected result")
			}
		}
	})
}

// Benchmark with filtering (items that don't pass).
func BenchmarkFilterFiltered(b *testing.B) {
	filter := NewFilter(func(n int) bool {
		return n > 100 // Will filter out 42
	})

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			input := make(chan Result[int], 1)
			input <- NewSuccess(42)
			close(input)

			results := filter.Process(ctx, input)

			// Should receive no results
			var count int
			for range results {
				count++
			}

			if count != 0 {
				b.Fatal("Expected no results")
			}
		}
	})
}
