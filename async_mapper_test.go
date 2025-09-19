package streamz

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestAsyncMapper_OrderedProcessing(t *testing.T) {
	ctx := context.Background()
	in := make(chan Result[int])

	// Create mapper with variable processing times to test ordering
	mapper := NewAsyncMapper(func(_ context.Context, i int) (string, error) {
		// Later items process faster to test reordering
		time.Sleep(time.Duration(10-i) * time.Millisecond)
		return fmt.Sprintf("item-%d", i), nil
	}).WithWorkers(3).WithOrdered(true)

	out := mapper.Process(ctx, in)

	// Send items
	go func() {
		defer close(in)
		for i := 0; i < 10; i++ {
			in <- NewSuccess(i)
		}
	}()

	// Collect results
	results := make([]string, 0, 10)
	for result := range out {
		if result.IsError() {
			t.Errorf("unexpected error: %v", result.Error())
			continue
		}
		results = append(results, result.Value())
	}

	// Verify count
	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}

	// Verify order is preserved despite variable processing times
	for i, result := range results {
		expected := fmt.Sprintf("item-%d", i)
		if result != expected {
			t.Errorf("expected %s at position %d, got %s", expected, i, result)
		}
	}
}

func TestAsyncMapper_UnorderedProcessing(t *testing.T) {
	ctx := context.Background()
	in := make(chan Result[int])

	// Create mapper with unordered processing
	mapper := NewAsyncMapper(func(_ context.Context, i int) (string, error) {
		// Variable processing times - results should come back unordered
		time.Sleep(time.Duration(10-i) * time.Millisecond)
		return fmt.Sprintf("item-%d", i), nil
	}).WithWorkers(3).WithOrdered(false)

	out := mapper.Process(ctx, in)

	// Send items
	go func() {
		defer close(in)
		for i := 0; i < 10; i++ {
			in <- NewSuccess(i)
		}
	}()

	// Collect results
	results := make([]string, 0, 10)
	for result := range out {
		if result.IsError() {
			t.Errorf("unexpected error: %v", result.Error())
			continue
		}
		results = append(results, result.Value())
	}

	// Verify count
	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}

	// Verify all expected items are present (order may vary)
	sort.Strings(results)
	for i, result := range results {
		expected := fmt.Sprintf("item-%d", i)
		if result != expected {
			t.Errorf("expected %s at sorted position %d, got %s", expected, i, result)
		}
	}
}

func TestAsyncMapper_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	in := make(chan Result[int])

	// Create mapper that errors on odd numbers
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		if i%2 == 1 {
			return 0, fmt.Errorf("odd number: %d", i)
		}
		return i * 2, nil
	}).WithWorkers(2)

	out := mapper.Process(ctx, in)

	// Send mixed success and error items
	go func() {
		defer close(in)
		for i := 0; i < 6; i++ {
			in <- NewSuccess(i)
		}
	}()

	// Collect results
	var successes []int
	var errors []*StreamError[int]

	for result := range out {
		if result.IsError() {
			errors = append(errors, result.Error())
		} else {
			successes = append(successes, result.Value())
		}
	}

	// Verify successful results (even numbers doubled)
	expectedSuccesses := []int{0, 4, 8} // 0*2, 2*2, 4*2
	if len(successes) != len(expectedSuccesses) {
		t.Errorf("expected %d successes, got %d: %v", len(expectedSuccesses), len(successes), successes)
	}

	for i, success := range successes {
		if i < len(expectedSuccesses) && success != expectedSuccesses[i] {
			t.Errorf("expected success %d at position %d, got %d", expectedSuccesses[i], i, success)
		}
	}

	// Verify errors (odd numbers)
	expectedErrors := 3 // 1, 3, 5
	if len(errors) != expectedErrors {
		t.Errorf("expected %d errors, got %d", expectedErrors, len(errors))
	}

	// Verify error details
	for _, err := range errors {
		if err.ProcessorName != "async-mapper" {
			t.Errorf("expected processor name 'async-mapper', got %s", err.ProcessorName)
		}
	}
}

func TestAsyncMapper_InputErrorPropagation(t *testing.T) {
	ctx := context.Background()
	in := make(chan Result[int])

	// Create mapper - function should not be called for input errors
	mapper := NewAsyncMapper(func(_ context.Context, i int) (string, error) {
		return fmt.Sprintf("processed-%d", i), nil
	}).WithWorkers(2)

	out := mapper.Process(ctx, in)

	// Send mix of success and input errors
	go func() {
		defer close(in)
		in <- NewSuccess(1)
		in <- NewError(2, fmt.Errorf("input error"), "upstream")
		in <- NewSuccess(3)
	}()

	// Collect results
	var successes []string
	var errors []*StreamError[string]

	for result := range out {
		if result.IsError() {
			errors = append(errors, result.Error())
		} else {
			successes = append(successes, result.Value())
		}
	}

	// Should have 2 successes and 1 error
	if len(successes) != 2 {
		t.Errorf("expected 2 successes, got %d: %v", len(successes), successes)
	}

	if len(errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(errors))
	}

	// Verify input error is propagated
	if len(errors) > 0 {
		if errors[0].ProcessorName != "async-mapper" {
			t.Errorf("expected processor name 'async-mapper', got %s", errors[0].ProcessorName)
		}
	}
}

func TestAsyncMapper_ConcurrentProcessing(t *testing.T) {
	ctx := context.Background()
	in := make(chan Result[int])

	// Track concurrent executions
	concurrent := make(chan int, 5)
	maxConcurrent := 0
	var mu sync.Mutex

	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		concurrent <- i
		defer func() { <-concurrent }()

		// Track max concurrency
		mu.Lock()
		if len(concurrent) > maxConcurrent {
			maxConcurrent = len(concurrent)
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)
		return i * 2, nil
	}).WithWorkers(5).WithOrdered(false)

	out := mapper.Process(ctx, in)

	// Send items
	go func() {
		defer close(in)
		for i := 0; i < 10; i++ {
			in <- NewSuccess(i)
		}
	}()

	// Drain output
	count := 0
	for result := range out {
		if result.IsError() {
			t.Errorf("unexpected error: %v", result.Error())
		} else {
			count++
		}
	}

	// Verify all items processed
	if count != 10 {
		t.Errorf("expected 10 results, got %d", count)
	}

	// Verify concurrency was achieved
	mu.Lock()
	defer mu.Unlock()
	if maxConcurrent < 3 { // Should achieve reasonable concurrency
		t.Errorf("expected at least 3 concurrent operations, got %d", maxConcurrent)
	}
}

func TestAsyncMapper_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan Result[int])

	mapper := NewAsyncMapper(func(ctx context.Context, i int) (string, error) {
		// Simulate work that respects context
		select {
		case <-time.After(100 * time.Millisecond):
			return fmt.Sprintf("item-%d", i), nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}).WithWorkers(2)

	out := mapper.Process(ctx, in)

	// Send a few items
	go func() {
		defer close(in)
		for i := 0; i < 5; i++ {
			select {
			case in <- NewSuccess(i):
			case <-ctx.Done():
				return
			}
		}
	}()

	// Cancel context after short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Collect results - should terminate due to cancellation
	results := 0
	for range out {
		results++
	}

	// Should process fewer than all items due to cancellation
	if results >= 5 {
		t.Errorf("expected fewer than 5 results due to cancellation, got %d", results)
	}
}

func TestAsyncMapper_DefaultConfiguration(t *testing.T) {
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		return i, nil
	})

	// Verify defaults
	if mapper.workers <= 0 {
		t.Errorf("expected positive default worker count, got %d", mapper.workers)
	}

	if !mapper.ordered {
		t.Error("expected ordered=true by default")
	}

	if mapper.bufferSize <= 0 {
		t.Errorf("expected positive default buffer size, got %d", mapper.bufferSize)
	}

	if mapper.name != "async-mapper" {
		t.Errorf("expected default name 'async-mapper', got %s", mapper.name)
	}
}

func TestAsyncMapper_FluentConfiguration(t *testing.T) {
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		return i, nil
	}).WithWorkers(8).WithOrdered(false).WithBufferSize(200).WithName("test-mapper")

	// Verify configuration
	if mapper.workers != 8 {
		t.Errorf("expected 8 workers, got %d", mapper.workers)
	}

	if mapper.ordered {
		t.Error("expected ordered=false")
	}

	if mapper.bufferSize != 200 {
		t.Errorf("expected buffer size 200, got %d", mapper.bufferSize)
	}

	if mapper.name != "test-mapper" {
		t.Errorf("expected name 'test-mapper', got %s", mapper.name)
	}
}

func TestAsyncMapper_InvalidConfiguration(t *testing.T) {
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		return i, nil
	})

	originalWorkers := mapper.workers
	originalBufferSize := mapper.bufferSize

	// Invalid values should not change configuration
	mapper.WithWorkers(0).WithWorkers(-1).WithBufferSize(0).WithBufferSize(-1)

	if mapper.workers != originalWorkers {
		t.Errorf("expected workers unchanged at %d, got %d", originalWorkers, mapper.workers)
	}

	if mapper.bufferSize != originalBufferSize {
		t.Errorf("expected buffer size unchanged at %d, got %d", originalBufferSize, mapper.bufferSize)
	}
}

func TestAsyncMapper_Name(t *testing.T) {
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		return i, nil
	}).WithName("custom-name")

	if mapper.Name() != "custom-name" {
		t.Errorf("expected name 'custom-name', got %s", mapper.Name())
	}
}

// Benchmarks

func BenchmarkAsyncMapper_Ordered(b *testing.B) {
	ctx := context.Background()
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		// Simulate some work
		time.Sleep(time.Microsecond)
		return i * 2, nil
	}).WithWorkers(4).WithOrdered(true)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			in := make(chan Result[int], 100)
			out := mapper.Process(ctx, in)

			// Send items
			go func() {
				defer close(in)
				for i := 0; i < 100; i++ {
					in <- NewSuccess(i)
				}
			}()

			// Consume results
			count := 0
			for range out {
				count++
			}

			if count != 100 {
				b.Errorf("expected 100 results, got %d", count)
			}
		}
	})
}

func BenchmarkAsyncMapper_Unordered(b *testing.B) {
	ctx := context.Background()
	mapper := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		// Simulate some work
		time.Sleep(time.Microsecond)
		return i * 2, nil
	}).WithWorkers(4).WithOrdered(false)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			in := make(chan Result[int], 100)
			out := mapper.Process(ctx, in)

			// Send items
			go func() {
				defer close(in)
				for i := 0; i < 100; i++ {
					in <- NewSuccess(i)
				}
			}()

			// Consume results
			count := 0
			for range out {
				count++
			}

			if count != 100 {
				b.Errorf("expected 100 results, got %d", count)
			}
		}
	})
}

// Example demonstrates ordered concurrent processing.
func ExampleAsyncMapper_ordered() {
	ctx := context.Background()

	// Simulate API enrichment with preserved order
	type User struct {
		ID   int
		Name string
	}

	type EnrichedUser struct {
		ID      int
		Name    string
		Profile string
	}

	// Create ordered async mapper
	enricher := NewAsyncMapper(func(_ context.Context, u User) (EnrichedUser, error) {
		// Simulate API call - processing times vary but order is preserved
		return EnrichedUser{
			ID:      u.ID,
			Name:    u.Name,
			Profile: fmt.Sprintf("Profile for %s", u.Name),
		}, nil
	}).WithWorkers(3).WithOrdered(true)

	// Create input stream
	users := make(chan Result[User])
	go func() {
		defer close(users)
		users <- NewSuccess(User{ID: 1, Name: "Alice"})
		users <- NewSuccess(User{ID: 2, Name: "Bob"})
		users <- NewSuccess(User{ID: 3, Name: "Carol"})
	}()

	// Process with preserved order
	enriched := enricher.Process(ctx, users)
	for result := range enriched {
		if result.IsError() {
			fmt.Printf("Error: %v\n", result.Error())
		} else {
			user := result.Value()
			fmt.Printf("User %d: %s - %s\n", user.ID, user.Name, user.Profile)
		}
	}

	// Output:
	// User 1: Alice - Profile for Alice
	// User 2: Bob - Profile for Bob
	// User 3: Carol - Profile for Carol
}

// Example demonstrates unordered concurrent processing for maximum throughput.
func ExampleAsyncMapper_unordered() {
	ctx := context.Background()

	// Create unordered async mapper for maximum throughput
	processor := NewAsyncMapper(func(_ context.Context, i int) (int, error) {
		// Simulate CPU-intensive work
		return i * i, nil
	}).WithWorkers(4).WithOrdered(false)

	// Create input stream
	numbers := make(chan Result[int])
	go func() {
		defer close(numbers)
		for i := 1; i <= 5; i++ {
			numbers <- NewSuccess(i)
		}
	}()

	// Process without order preservation
	squares := processor.Process(ctx, numbers)
	results := make([]int, 0, 5)
	for result := range squares {
		if result.IsError() {
			fmt.Printf("Error: %v\n", result.Error())
		} else {
			results = append(results, result.Value())
		}
	}

	// Sort for consistent output (order may vary in real usage)
	sort.Ints(results)
	fmt.Printf("Squares: %v\n", results)

	// Output:
	// Squares: [1 4 9 16 25]
}
