package streamz

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMapper_BasicTransformation(t *testing.T) {
	// Create a mapper that doubles integers
	mapper := NewMapper(func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})

	// Test data
	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	input <- NewSuccess(3)
	close(input)

	// Process
	ctx := context.Background()
	results := mapper.Process(ctx, input)

	// Collect results
	outputs := make([]int, 0, 3)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		outputs = append(outputs, result.Value())
	}

	// Verify
	expected := []int{2, 4, 6}
	if len(outputs) != len(expected) {
		t.Fatalf("Expected %d results, got %d", len(expected), len(outputs))
	}
	for i, exp := range expected {
		if outputs[i] != exp {
			t.Errorf("Expected outputs[%d] = %d, got %d", i, exp, outputs[i])
		}
	}
}

func TestMapper_TypeConversion(t *testing.T) {
	// Create a mapper that converts int to string
	mapper := NewMapper(func(_ context.Context, n int) (string, error) {
		return strconv.Itoa(n), nil
	})

	// Test data
	input := make(chan Result[int], 3)
	input <- NewSuccess(42)
	input <- NewSuccess(100)
	input <- NewSuccess(-5)
	close(input)

	// Process
	ctx := context.Background()
	results := mapper.Process(ctx, input)

	// Collect results
	outputs := make([]string, 0, 3)
	for result := range results {
		if result.IsError() {
			t.Fatalf("Unexpected error: %v", result.Error())
		}
		outputs = append(outputs, result.Value())
	}

	// Verify
	expected := []string{"42", "100", "-5"}
	if len(outputs) != len(expected) {
		t.Fatalf("Expected %d results, got %d", len(expected), len(outputs))
	}
	for i, exp := range expected {
		if outputs[i] != exp {
			t.Errorf("Expected outputs[%d] = %s, got %s", i, exp, outputs[i])
		}
	}
}

func TestMapper_ErrorHandling(t *testing.T) {
	// Create a mapper that returns an error for negative numbers
	mapper := NewMapper(func(_ context.Context, n int) (int, error) {
		if n < 0 {
			return 0, errors.New("negative number not allowed")
		}
		return n * 2, nil
	})

	// Test data with mixed success and error cases
	input := make(chan Result[int], 4)
	input <- NewSuccess(1)
	input <- NewSuccess(-1) // This will cause an error
	input <- NewSuccess(2)
	input <- NewSuccess(-2) // This will cause an error
	close(input)

	// Process
	ctx := context.Background()
	results := mapper.Process(ctx, input)

	// Collect results
	var successes []int
	var errorCount int
	for result := range results {
		if result.IsError() {
			errorCount++
			// Verify error contains expected message
			if !strings.Contains(result.Error().Error(), "negative number not allowed") {
				t.Errorf("Expected error message to contain 'negative number not allowed', got: %v", result.Error())
			}
		} else {
			successes = append(successes, result.Value())
		}
	}

	// Verify
	expectedSuccesses := []int{2, 4}
	if len(successes) != len(expectedSuccesses) {
		t.Fatalf("Expected %d successes, got %d", len(expectedSuccesses), len(successes))
	}
	if errorCount != 2 {
		t.Fatalf("Expected 2 errors, got %d", errorCount)
	}

	for i, exp := range expectedSuccesses {
		if successes[i] != exp {
			t.Errorf("Expected successes[%d] = %d, got %d", i, exp, successes[i])
		}
	}
}

func TestMapper_PassThroughErrors(t *testing.T) {
	// Create a simple mapper
	mapper := NewMapper(func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})

	// Test data with input errors
	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewError(0, errors.New("input error"), "test-source")
	input <- NewSuccess(2)
	close(input)

	// Process
	ctx := context.Background()
	results := mapper.Process(ctx, input)

	// Collect results
	var successes []int
	var errors []error
	for result := range results {
		if result.IsError() {
			errors = append(errors, result.Error())
		} else {
			successes = append(successes, result.Value())
		}
	}

	// Verify
	expectedSuccesses := []int{2, 4}
	if len(successes) != len(expectedSuccesses) {
		t.Fatalf("Expected %d successes, got %d", len(expectedSuccesses), len(successes))
	}
	if len(errors) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errors))
	}

	for i, exp := range expectedSuccesses {
		if successes[i] != exp {
			t.Errorf("Expected successes[%d] = %d, got %d", i, exp, successes[i])
		}
	}

	// Verify the error contains original error information
	if !strings.Contains(errors[0].Error(), "input error") {
		t.Errorf("Expected passed-through error to contain 'input error', got: %v", errors[0])
	}
}

func TestMapper_ContextCancellation(t *testing.T) {
	// Create a mapper that could potentially block
	mapper := NewMapper(func(ctx context.Context, n int) (int, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return n * 2, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})

	// Test data
	input := make(chan Result[int], 2)
	input <- NewSuccess(1)
	input <- NewSuccess(2)

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	results := mapper.Process(ctx, input)

	// Get the first result
	result := <-results
	if result.IsError() {
		t.Fatalf("Unexpected error in first result: %v", result.Error())
	}
	if result.Value() != 2 {
		t.Fatalf("Expected first result to be 2, got %d", result.Value())
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

func TestMapper_EmptyInput(t *testing.T) {
	// Create a simple mapper
	mapper := NewMapper(func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})

	// Empty input
	input := make(chan Result[int])
	close(input)

	// Process
	ctx := context.Background()
	results := mapper.Process(ctx, input)

	// Verify no results
	var count int
	for range results {
		count++
	}

	if count != 0 {
		t.Errorf("Expected no results from empty input, got %d", count)
	}
}

func TestMapper_WithName(t *testing.T) {
	// Create mapper with custom name
	mapper := NewMapper(func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	}).WithName("test-mapper")

	// Verify name
	if mapper.Name() != "test-mapper" {
		t.Errorf("Expected name 'test-mapper', got '%s'", mapper.Name())
	}

	// Test that errors include the custom name
	input := make(chan Result[int], 1)
	input <- NewSuccess(-1)
	close(input)

	// Create mapper that returns errors to test error processor name
	errorMapper := NewMapper(func(_ context.Context, _ int) (int, error) {
		return 0, errors.New("test error")
	}).WithName("error-mapper")

	ctx := context.Background()
	results := errorMapper.Process(ctx, input)

	result := <-results
	if !result.IsError() {
		t.Fatal("Expected error result")
	}

	if !strings.Contains(result.Error().Error(), "error-mapper") {
		t.Errorf("Expected error to contain processor name 'error-mapper', got: %v", result.Error())
	}
}

func TestMapper_DefaultName(t *testing.T) {
	// Create mapper without custom name
	mapper := NewMapper(func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})

	// Verify default name
	if mapper.Name() != "mapper" {
		t.Errorf("Expected default name 'mapper', got '%s'", mapper.Name())
	}
}

func TestMapper_ComplexTransformation(t *testing.T) {
	// Test a more complex transformation with struct types
	type User struct {
		Name string
		Age  int
	}

	type UserDisplay struct {
		Display string
		IsAdult bool
	}

	// Create mapper for struct transformation
	mapper := NewMapper(func(_ context.Context, u User) (UserDisplay, error) {
		if u.Name == "" {
			return UserDisplay{}, errors.New("name cannot be empty")
		}
		return UserDisplay{
			Display: fmt.Sprintf("%s (%d)", u.Name, u.Age),
			IsAdult: u.Age >= 18,
		}, nil
	})

	// Test data
	input := make(chan Result[User], 3)
	input <- NewSuccess(User{Name: "Alice", Age: 25})
	input <- NewSuccess(User{Name: "Bob", Age: 16})
	input <- NewSuccess(User{Name: "", Age: 30}) // This will cause an error
	close(input)

	// Process
	ctx := context.Background()
	results := mapper.Process(ctx, input)

	// Collect results
	var successes []UserDisplay
	var errorCount int
	for result := range results {
		if result.IsError() {
			errorCount++
		} else {
			successes = append(successes, result.Value())
		}
	}

	// Verify
	if len(successes) != 2 {
		t.Fatalf("Expected 2 successes, got %d", len(successes))
	}
	if errorCount != 1 {
		t.Fatalf("Expected 1 error, got %d", errorCount)
	}

	// Verify specific transformations
	if successes[0].Display != "Alice (25)" || !successes[0].IsAdult {
		t.Errorf("Unexpected first result: %+v", successes[0])
	}
	if successes[1].Display != "Bob (16)" || successes[1].IsAdult {
		t.Errorf("Unexpected second result: %+v", successes[1])
	}
}

// Benchmark to ensure the mapper has minimal overhead.
func BenchmarkMapper(b *testing.B) {
	mapper := NewMapper(func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			input := make(chan Result[int], 1)
			input <- NewSuccess(42)
			close(input)

			results := mapper.Process(ctx, input)
			result := <-results

			if result.IsError() || result.Value() != 84 {
				b.Fatal("Unexpected result")
			}
		}
	})
}
