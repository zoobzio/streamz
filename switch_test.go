package streamz

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSwitchBasicFunctionality tests basic switch operations.
func TestSwitchBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	// Create processors for each case.
	evenProcessor := NewMapper(func(n int) int { return n * 2 }).WithName("even")
	oddProcessor := NewMapper(func(n int) int { return n * 3 }).WithName("odd")
	defaultProcessor := NewMapper(func(n int) int { return n + 100 }).WithName("default")

	// Create switch.
	sw := NewSwitch[int]().
		Case("even", func(n int) bool { return n%2 == 0 }, evenProcessor).
		Case("odd", func(n int) bool { return n%2 == 1 }, oddProcessor).
		Default(defaultProcessor)

	// Process numbers.
	input := make(chan int, 11)
	for i := 0; i <= 10; i++ {
		input <- i
	}
	close(input)

	output := sw.Process(ctx, input)

	// Collect results..
	results := make(map[int]int)
	for result := range output {
		// Reverse engineer the input.
		switch {
		case result%2 == 0 && result < 100:
			// Even case: result = input * 2.
			results[result/2] = result
		case result%3 == 0 && result < 100:
			// Odd case: result = input * 3.
			results[result/3] = result
		case result > 100:
			// Default case: result = input + 100.
			results[result-100] = result
		}
	}

	// Verify all inputs were processed.
	if len(results) != 11 {
		t.Errorf("expected 11 results, got %d", len(results))
	}

	// Verify correct routing.
	//nolint:gocritic // Comments explain expected calculations
	expectedResults := map[int]int{
		0:  0,  // even: 0 * 2 = 0 //nolint:gocritic // calculation comment
		1:  3,  // odd: 1 * 3 = 3 //nolint:gocritic // calculation comment
		2:  4,  // even: 2 * 2 = 4 //nolint:gocritic // calculation comment
		3:  9,  // odd: 3 * 3 = 9 //nolint:gocritic // calculation comment
		4:  8,  // even: 4 * 2 = 8 //nolint:gocritic // calculation comment
		5:  15, // odd: 5 * 3 = 15 //nolint:gocritic // calculation comment
		6:  12, // even: 6 * 2 = 12 //nolint:gocritic // calculation comment
		7:  21, // odd: 7 * 3 = 21 //nolint:gocritic // calculation comment
		8:  16, // even: 8 * 2 = 16 //nolint:gocritic // calculation comment
		9:  27, // odd: 9 * 3 = 27 //nolint:gocritic // calculation comment
		10: 20, // even: 10 * 2 = 20 //nolint:gocritic // calculation comment
	}

	for input, expected := range expectedResults {
		if results[input] != expected {
			t.Errorf("input %d: expected %d, got %d", input, expected, results[input])
		}
	}

	// Check statistics.
	stats := sw.GetStats()
	if stats.CaseMatches["even"] != 6 { // 0, 2, 4, 6, 8, 10
		t.Errorf("expected 6 even matches, got %d", stats.CaseMatches["even"])
	}
	if stats.CaseMatches["odd"] != 5 { // 1, 3, 5, 7, 9
		t.Errorf("expected 5 odd matches, got %d", stats.CaseMatches["odd"])
	}
	if stats.CaseMatches["_default"] != 0 {
		t.Errorf("expected 0 default matches, got %d", stats.CaseMatches["_default"])
	}
}

// TestSwitchFirstMatchWins tests that only the first matching case is executed.
func TestSwitchFirstMatchWins(t *testing.T) {
	ctx := context.Background()

	var firstCaseCount, secondCaseCount atomic.Int32

	// Create processors that track execution.
	firstProcessor := ProcessorFunc[int, int](func(_ context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for item := range in {
				firstCaseCount.Add(1)
				out <- item * 10
			}
		}()
		return out
	})

	secondProcessor := ProcessorFunc[int, int](func(_ context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for item := range in {
				secondCaseCount.Add(1)
				out <- item * 100
			}
		}()
		return out
	})

	// Create switch with overlapping conditions.
	sw := NewSwitch[int]().
		Case("greater-than-5", func(n int) bool { return n > 5 }, firstProcessor).
		Case("greater-than-3", func(n int) bool { return n > 3 }, secondProcessor)

	// Process numbers
	input := make(chan int, 10)
	for i := 1; i <= 10; i++ {
		input <- i
	}
	close(input)

	output := sw.Process(ctx, input)

	// Collect results.
	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Numbers > 5 should go to first case.
	// Numbers > 3 but <= 5 should go to second case.
	// Numbers <= 3 should be dropped (no default).

	if firstCaseCount.Load() != 5 { // 6, 7, 8, 9, 10
		t.Errorf("expected 5 items in first case, got %d", firstCaseCount.Load())
	}

	if secondCaseCount.Load() != 2 { // 4, 5
		t.Errorf("expected 2 items in second case, got %d", secondCaseCount.Load())
	}

	if len(results) != 7 { // Total items processed
		t.Errorf("expected 7 results, got %d", len(results))
	}
}

// TestSwitchDefaultCase tests default case handling.
func TestSwitchDefaultCase(t *testing.T) {
	ctx := context.Background()

	// Processor for specific case.
	multipleOfThree := NewMapper(func(n int) int { return n * 10 })

	// Default processor.
	defaultProc := NewMapper(func(n int) int { return n + 1000 })

	// Switch with specific case and default.
	sw := NewSwitch[int]().
		Case("multiple-of-3", func(n int) bool { return n%3 == 0 }, multipleOfThree).
		Default(defaultProc)

	// Process numbers
	input := make(chan int, 10)
	for i := 1; i <= 10; i++ {
		input <- i
	}
	close(input)

	output := sw.Process(ctx, input)

	// Collect and categorize results.
	var multiplesOfThree, defaults []int
	for result := range output {
		if result < 100 {
			multiplesOfThree = append(multiplesOfThree, result/10)
		} else {
			defaults = append(defaults, result-1000)
		}
	}

	// Verify routing.
	expectedMultiples := []int{3, 6, 9}
	if len(multiplesOfThree) != len(expectedMultiples) {
		t.Errorf("expected %d multiples of 3, got %d", len(expectedMultiples), len(multiplesOfThree))
	}

	expectedDefaults := []int{1, 2, 4, 5, 7, 8, 10}
	if len(defaults) != len(expectedDefaults) {
		t.Errorf("expected %d defaults, got %d", len(expectedDefaults), len(defaults))
	}

	// Check stats.
	stats := sw.GetStats()
	if stats.CaseMatches["multiple-of-3"] != 3 {
		t.Errorf("expected 3 multiple-of-3 matches, got %d", stats.CaseMatches["multiple-of-3"])
	}
	if stats.CaseMatches["_default"] != 7 {
		t.Errorf("expected 7 default matches, got %d", stats.CaseMatches["_default"])
	}
}

// TestSwitchNoDefault tests behavior when no default is set.
func TestSwitchNoDefault(t *testing.T) {
	ctx := context.Background()

	// Single case processor.
	processor := NewMapper(func(n int) int { return n * 2 })

	// Switch without default.
	sw := NewSwitch[int]().
		Case("even", func(n int) bool { return n%2 == 0 }, processor)

	// Process numbers
	input := make(chan int, 10)
	for i := 1; i <= 10; i++ {
		input <- i
	}
	close(input)

	output := sw.Process(ctx, input)

	// Collect results.
	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result/2) // Reverse the multiplication
	}

	// Only even numbers should be processed.
	if len(results) != 5 {
		t.Errorf("expected 5 results (even numbers), got %d", len(results))
	}

	// Verify only even numbers were processed.
	for _, n := range results {
		if n%2 != 0 {
			t.Errorf("unexpected odd number in results: %d", n)
		}
	}
}

// TestSwitchConcurrentProcessing tests concurrent execution of cases.
func TestSwitchConcurrentProcessing(t *testing.T) {
	ctx := context.Background()

	// Track concurrent execution.
	var activeWorkers atomic.Int32
	maxConcurrent := atomic.Int32{}

	// Slow processor to test concurrency.
	slowProcessor := ProcessorFunc[int, int](func(_ context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for item := range in {
				current := activeWorkers.Add(1)

				// Track max concurrent workers.
				for {
					maxVal := maxConcurrent.Load()
					if current <= maxVal || maxConcurrent.CompareAndSwap(maxVal, current) {
						break
					}
				}

				// Simulate work.
				time.Sleep(10 * time.Millisecond)

				out <- item
				activeWorkers.Add(-1)
			}
		}()
		return out
	})

	// Create switch with multiple cases using same slow processor.
	sw := NewSwitch[int]().
		Case("low", func(n int) bool { return n < 10 }, slowProcessor).
		Case("medium", func(n int) bool { return n >= 10 && n < 20 }, slowProcessor).
		Case("high", func(n int) bool { return n >= 20 }, slowProcessor).
		WithBufferSize(10)

	// Send items from multiple ranges.
	input := make(chan int)
	go func() {
		defer close(input)
		for i := 0; i < 30; i++ {
			input <- i
		}
	}()

	output := sw.Process(ctx, input)

	// Consume results.
	count := 0
	for range output {
		count++
	}

	if count != 30 {
		t.Errorf("expected 30 results, got %d", count)
	}

	// Should have had some concurrent processing.
	if maxConcurrent.Load() < 2 {
		t.Errorf("expected concurrent processing, max concurrent was %d", maxConcurrent.Load())
	}
}

// TestSwitchContextCancellation tests graceful shutdown.
func TestSwitchContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Slow processor.
	slowProcessor := ProcessorFunc[int, int](func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for item := range in {
				select {
				case <-time.After(50 * time.Millisecond):
					out <- item * 2
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	})

	sw := NewSwitch[int]().
		Case("all", func(_ int) bool { return true }, slowProcessor)

	input := make(chan int)
	go func() {
		defer close(input)
		for i := 0; i < 100; i++ {
			select {
			case input <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	output := sw.Process(ctx, input)

	// Process some items then cancel.
	count := 0
	done := make(chan struct{})

	go func() {
		defer close(done)
		for range output {
			count++
		}
	}()

	// Let it process a few items.
	time.Sleep(150 * time.Millisecond)

	// Cancel context.
	cancel()

	// Wait for completion.
	<-done

	// Should have processed some but not all.
	if count == 0 {
		t.Error("expected some items to be processed")
	}
	if count >= 100 {
		t.Errorf("expected cancellation to stop processing, but processed %d items", count)
	}
}

// TestSwitchComplexConditions tests complex routing logic.
func TestSwitchComplexConditions(t *testing.T) {
	ctx := context.Background()

	type Order struct {
		ID       string
		Amount   float64
		Priority string
		Country  string
	}

	// Different processors for different order types.
	urgentProcessor := NewMapper(func(o Order) Order {
		o.Priority = "processed-urgent"
		return o
	})

	highValueProcessor := NewMapper(func(o Order) Order {
		o.Priority = "processed-high-value"
		return o
	})

	internationalProcessor := NewMapper(func(o Order) Order {
		o.Priority = "processed-international"
		return o
	})

	standardProcessor := NewMapper(func(o Order) Order {
		o.Priority = "processed-standard"
		return o
	})

	// Complex routing logic.
	orderSwitch := NewSwitch[Order]().
		Case("urgent", func(o Order) bool {
			return o.Priority == "urgent" || o.Amount > 10000
		}, urgentProcessor).
		Case("high-value", func(o Order) bool {
			return o.Amount > 5000
		}, highValueProcessor).
		Case("international", func(o Order) bool {
			return o.Country != "US"
		}, internationalProcessor).
		Default(standardProcessor).
		WithName("order-router")

	// Test orders.
	orders := []Order{
		{ID: "1", Amount: 100, Priority: "urgent", Country: "US"},   // urgent
		{ID: "2", Amount: 15000, Priority: "normal", Country: "US"}, // urgent (high amount)
		{ID: "3", Amount: 7000, Priority: "normal", Country: "US"},  // high-value
		{ID: "4", Amount: 100, Priority: "normal", Country: "UK"},   // international
		{ID: "5", Amount: 100, Priority: "normal", Country: "US"},   // standard
	}

	input := make(chan Order, len(orders))
	for _, order := range orders {
		input <- order
	}
	close(input)

	output := orderSwitch.Process(ctx, input)

	// Collect results.
	results := make(map[string]string)
	for order := range output {
		results[order.ID] = order.Priority
	}

	// Verify routing.
	expected := map[string]string{
		"1": "processed-urgent",
		"2": "processed-urgent",
		"3": "processed-high-value",
		"4": "processed-international",
		"5": "processed-standard",
	}

	for id, expectedPriority := range expected {
		if results[id] != expectedPriority {
			t.Errorf("order %s: expected priority %s, got %s", id, expectedPriority, results[id])
		}
	}

	// Check stats.
	stats := orderSwitch.GetStats()
	if stats.CaseMatches["urgent"] != 2 {
		t.Errorf("expected 2 urgent matches, got %d", stats.CaseMatches["urgent"])
	}
	if stats.MatchRate("urgent") != 40.0 {
		t.Errorf("expected 40%% urgent match rate, got %.2f%%", stats.MatchRate("urgent"))
	}
}

// TestSwitchFluentAPI tests fluent configuration.
func TestSwitchFluentAPI(t *testing.T) {
	processor := NewMapper(func(n int) int { return n * 2 })

	sw := NewSwitch[int]().
		Case("case1", func(n int) bool { return n > 10 }, processor).
		Case("case2", func(n int) bool { return n < 0 }, processor).
		Default(processor).
		WithBufferSize(100).
		WithName("test-switch")

	if sw.name != "test-switch" {
		t.Errorf("expected name 'test-switch', got %s", sw.name)
	}

	if sw.bufferSize != 100 {
		t.Errorf("expected buffer size 100, got %d", sw.bufferSize)
	}

	if len(sw.cases) != 2 {
		t.Errorf("expected 2 cases, got %d", len(sw.cases))
	}

	if sw.defaultProc == nil {
		t.Error("expected default processor to be set")
	}
}

// TestSwitchEmptyInput tests handling of empty input.
func TestSwitchEmptyInput(t *testing.T) {
	ctx := context.Background()

	processor := NewMapper(func(n int) int { return n * 2 })
	sw := NewSwitch[int]().
		Case("even", func(n int) bool { return n%2 == 0 }, processor)

	input := make(chan int)
	close(input)

	output := sw.Process(ctx, input)

	// Should complete without hanging.
	count := 0
	for range output {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 results from empty input, got %d", count)
	}
}

// TestSwitchStatsConcurrency tests thread-safe statistics.
func TestSwitchStatsConcurrency(t *testing.T) {
	ctx := context.Background()

	processor := NewMapper(func(n int) int { return n * 2 })
	sw := NewSwitch[int]().
		Case("even", func(n int) bool { return n%2 == 0 }, processor).
		Case("odd", func(n int) bool { return n%2 == 1 }, processor)

	// Process items concurrently.
	var wg sync.WaitGroup
	numGoroutines := 10
	itemsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()

			input := make(chan int, itemsPerGoroutine)
			for j := 0; j < itemsPerGoroutine; j++ {
				input <- offset*itemsPerGoroutine + j
			}
			close(input)

			output := sw.Process(ctx, input)
			for range output { //nolint:revive // Intentionally draining channel
				// Drain.
			}
		}(i)
	}

	wg.Wait()

	// Check total stats.
	stats := sw.GetStats()
	expectedTotal := int64(numGoroutines * itemsPerGoroutine)

	if stats.TotalItems != expectedTotal {
		t.Errorf("expected %d total items, got %d", expectedTotal, stats.TotalItems)
	}

	// Even and odd should be roughly equal.
	evenCount := stats.CaseMatches["even"]
	oddCount := stats.CaseMatches["odd"]

	if evenCount+oddCount != expectedTotal {
		t.Errorf("sum of cases (%d) doesn't match total (%d)", evenCount+oddCount, expectedTotal)
	}
}

// BenchmarkSwitchTwoCases benchmarks switch with two cases.
func BenchmarkSwitchTwoCases(b *testing.B) {
	ctx := context.Background()

	processor := NewMapper(func(n int) int { return n * 2 })
	sw := NewSwitch[int]().
		Case("even", func(n int) bool { return n%2 == 0 }, processor).
		Case("odd", func(n int) bool { return n%2 == 1 }, processor)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		output := sw.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Consume.
		}
	}
}

// BenchmarkSwitchManyCases benchmarks switch with many cases.
func BenchmarkSwitchManyCases(b *testing.B) {
	ctx := context.Background()

	processor := NewMapper(func(n int) int { return n * 2 })
	sw := NewSwitch[int]()

	// Add 10 cases.
	for i := 0; i < 10; i++ {
		threshold := i * 10
		caseThreshold := threshold // Capture in closure
		sw.Case(fmt.Sprintf("case-%d", i), func(n int) bool {
			return n > caseThreshold && n <= caseThreshold+10
		}, processor)
	}
	sw.Default(processor)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i % 100
		close(input)

		output := sw.Process(ctx, input)
		for range output { //nolint:revive // Intentionally draining channel
			// Consume.
		}
	}
}

// Example demonstrates basic switch usage.
func ExampleSwitch() {
	ctx := context.Background()

	// Create processors for different cases.
	smallProcessor := NewMapper(func(n int) int { return n + 10 })
	mediumProcessor := NewMapper(func(n int) int { return n * 2 })
	largeProcessor := NewMapper(func(n int) int { return n * 10 })

	// Create switch based on value ranges.
	sw := NewSwitch[int]().
		Case("small", func(n int) bool { return n < 10 }, smallProcessor).
		Case("medium", func(n int) bool { return n < 100 }, mediumProcessor).
		Case("large", func(n int) bool { return n < 1000 }, largeProcessor).
		WithName("size-router")

	// Process values.
	// Test cases:
	// - 5: small processor (5 + 10 = 15)
	// - 50: medium processor (50 * 2 = 100)
	// - 500: large processor (500 * 10 = 5000)
	// - 5000: no match, will be dropped
	input := make(chan int, 4)
	input <- 5
	input <- 50
	input <- 500
	input <- 5000
	close(input)

	output := sw.Process(ctx, input)

	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for result := range output {
		results = append(results, result)
	}

	// Sort for consistent output.
	sort.Ints(results)
	fmt.Println(results)
	// Output: [15 100 5000]
}
