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

// TestRouterBasicRouting tests basic routing functionality.
func TestRouterBasicRouting(t *testing.T) {
	ctx := context.Background()

	// Create processors for different routes
	evenProcessor := NewTestProcessor("even")
	oddProcessor := NewTestProcessor("odd")

	// Create router
	router := NewRouter[int]().
		AddRoute("even", func(n int) bool {
			return n%2 == 0
		}, evenProcessor).
		AddRoute("odd", func(n int) bool {
			return n%2 == 1
		}, oddProcessor)

	// Create input
	input := make(chan int)
	go func() {
		defer close(input)
		for i := 1; i <= 10; i++ {
			input <- i
		}
	}()

	// Process
	outputs := router.Process(ctx, input)

	// Collect results
	var evenResults []int
	var oddResults []int

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for item := range outputs.Routes["even"] {
			evenResults = append(evenResults, item)
		}
	}()

	go func() {
		defer wg.Done()
		for item := range outputs.Routes["odd"] {
			oddResults = append(oddResults, item)
		}
	}()

	wg.Wait()

	// Verify results
	if len(evenResults) != 5 {
		t.Errorf("expected 5 even numbers, got %d", len(evenResults))
	}
	if len(oddResults) != 5 {
		t.Errorf("expected 5 odd numbers, got %d", len(oddResults))
	}

	// Check specific values (TestProcessor doubles values)
	// Even inputs (2, 4, 6, 8, 10) become (4, 8, 12, 16, 20)
	for _, n := range evenResults {
		if n%4 != 0 {
			t.Errorf("expected doubled even number, got %d", n)
		}
	}
	// Odd inputs (1, 3, 5, 7, 9) become (2, 6, 10, 14, 18)
	for _, n := range oddResults {
		if n%2 != 0 {
			t.Errorf("expected doubled odd number to be even, got %d", n)
		}
	}
}

// TestRouterWithDefault tests default route functionality.
func TestRouterWithDefault(t *testing.T) {
	ctx := context.Background()

	// Create processors
	positiveProcessor := NewTestProcessor("positive")
	negativeProcessor := NewTestProcessor("negative")
	defaultProcessor := NewTestProcessor("default")

	// Create router with default
	router := NewRouter[int]().
		AddRoute("positive", func(n int) bool {
			return n > 0
		}, positiveProcessor).
		AddRoute("negative", func(n int) bool {
			return n < 0
		}, negativeProcessor).
		WithDefault(defaultProcessor)

	// Create input with zero (matches no routes)
	input := make(chan int, 5)
	input <- -5
	input <- 0
	input <- 5
	close(input)

	// Process
	outputs := router.Process(ctx, input)

	// Collect results
	results := make(map[string][]int)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, ch := range outputs.Routes {
		wg.Add(1)
		go func(routeName string, routeChan <-chan int) {
			defer wg.Done()
			var items []int
			for item := range routeChan {
				items = append(items, item)
			}
			if len(items) > 0 {
				mu.Lock()
				results[routeName] = items
				mu.Unlock()
			}
		}(name, ch)
	}

	wg.Wait()

	// Verify results
	if len(results["positive"]) != 1 || results["positive"][0] != 10 {
		t.Errorf("expected positive route to receive 5->10, got %v", results["positive"])
	}
	if len(results["negative"]) != 1 || results["negative"][0] != -10 {
		t.Errorf("expected negative route to receive -5->-10, got %v", results["negative"])
	}
	if len(results["default"]) != 1 || results["default"][0] != 0 {
		t.Errorf("expected default route to receive 0->0, got %v", results["default"])
	}
}

// countingProcessor for test.
type countingProcessor struct {
	counter *int64
}

func (cp *countingProcessor) Process(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for item := range in {
			atomic.AddInt64(cp.counter, 1)
			select {
			case out <- item:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (*countingProcessor) Name() string {
	return "counting"
}

// TestRouterAllMatches tests routing to multiple matching routes.
func TestRouterAllMatches(t *testing.T) {
	ctx := context.Background()

	// Track which routes received items
	var route1Count int64
	var route2Count int64
	var route3Count int64

	// Create router with overlapping routes
	router := NewRouter[int]().
		AllMatches(). // Enable all-matches mode
		AddRoute("divisible-by-2", func(n int) bool {
			return n%2 == 0
		}, &countingProcessor{counter: &route1Count}).
		AddRoute("divisible-by-3", func(n int) bool {
			return n%3 == 0
		}, &countingProcessor{counter: &route2Count}).
		AddRoute("divisible-by-5", func(n int) bool {
			return n%5 == 0
		}, &countingProcessor{counter: &route3Count})

	// Create input with specific test cases
	input := make(chan int, 5)
	input <- 6  // divisible by 2 and 3
	input <- 10 // divisible by 2 and 5
	input <- 15 // divisible by 2, 3, and 5
	input <- 9  // divisible by 3 only
	input <- 7  // matches none
	close(input)

	// Process
	outputs := router.Process(ctx, input)

	// Drain all outputs
	var wg sync.WaitGroup
	for _, ch := range outputs.Routes {
		wg.Add(1)
		go func(routeChan <-chan int) {
			defer wg.Done()
			for range routeChan { //nolint:revive // Intentionally draining channel
				// Just drain
			}
		}(ch)
	}
	wg.Wait()

	// Verify counts
	// route1 (div by 2): 6, 10
	if route1Count != 2 {
		t.Errorf("expected route1 (div by 2) to receive 2 items, got %d", route1Count)
	}
	// route2 (div by 3): 6, 15, 9
	if route2Count != 3 {
		t.Errorf("expected route2 (div by 3) to receive 3 items, got %d", route2Count)
	}
	// route3 (div by 5): 10, 15
	if route3Count != 2 {
		t.Errorf("expected route3 (div by 5) to receive 2 items, got %d", route3Count)
	}
}

// TestRouterProcessToSingle tests merging route outputs.
func TestRouterProcessToSingle(t *testing.T) {
	ctx := context.Background()

	// Create router
	router := NewRouter[int]().
		AddRoute("small", func(n int) bool {
			return n <= 10
		}, NewTestProcessor("small")).
		AddRoute("large", func(n int) bool {
			return n > 10
		}, NewTestProcessor("large"))

	// Create input
	input := make(chan int)
	go func() {
		defer close(input)
		for i := 1; i <= 20; i++ {
			input <- i
		}
	}()

	// Process to single output
	output := router.ProcessToSingle(ctx, input)

	// Collect all results
	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for item := range output {
		results = append(results, item)
	}

	// Should have all 20 items (each doubled by TestProcessor)
	if len(results) != 20 {
		t.Errorf("expected 20 results, got %d", len(results))
	}

	// Sort to check values
	sort.Ints(results)
	for i, val := range results {
		expected := (i + 1) * 2
		if val != expected {
			t.Errorf("result[%d] = %d, expected %d", i, val, expected)
		}
	}
}

// TestRouterNoMatchingRoute tests behavior when no routes match.
func TestRouterNoMatchingRoute(t *testing.T) {
	ctx := context.Background()

	// Create router without default
	router := NewRouter[int]().
		AddRoute("positive", func(n int) bool {
			return n > 0
		}, NewTestProcessor("positive"))

	// Send items that don't match
	input := make(chan int, 3)
	input <- -1
	input <- -2
	input <- -3
	close(input)

	// Process
	outputs := router.Process(ctx, input)

	// Collect results
	var results []int //nolint:prealloc // dynamic growth acceptable in test code
	for _, ch := range outputs.Routes {
		for item := range ch {
			results = append(results, item)
		}
	}

	// Should have no results
	if len(results) != 0 {
		t.Errorf("expected no results for non-matching items, got %d", len(results))
	}
}

// slowProcessor for test.
type slowProcessor struct{}

func (*slowProcessor) Process(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for item := range in {
			select {
			case <-time.After(100 * time.Millisecond):
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (*slowProcessor) Name() string {
	return "slow"
}

// TestRouterContextCancellation tests graceful shutdown.
func TestRouterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create slow processor
	slowProcessor := &slowProcessor{}

	// Create router
	router := NewRouter[int]().
		AddRoute("slow", func(_ int) bool { return true }, slowProcessor)

	// Create continuous input
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

	// Process
	outputs := router.Process(ctx, input)

	// Start collecting
	var processed int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range outputs.Routes["slow"] {
			atomic.AddInt64(&processed, 1)
		}
	}()

	// Let it process a few items
	time.Sleep(250 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for completion
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good, completed
	case <-time.After(1 * time.Second):
		t.Error("router did not shut down within timeout")
	}

	// Should have processed some but not all items
	count := atomic.LoadInt64(&processed)
	if count == 0 {
		t.Error("expected some items to be processed")
	}
	if count >= 100 {
		t.Error("expected cancellation to stop processing")
	}
}

// TestRouterFluentAPI tests fluent configuration.
func TestRouterFluentAPI(t *testing.T) {
	processor := NewTestProcessor("test")

	router := NewRouter[int]().
		WithName("test-router").
		WithBufferSize(10).
		AllMatches().
		AddRoute("route1", func(_ int) bool { return true }, processor).
		AddRoute("route2", func(_ int) bool { return true }, processor).
		WithDefault(processor).
		FirstMatch() // Switch back to first-match

	if router.Name() != "test-router" {
		t.Errorf("expected name 'test-router', got %s", router.Name())
	}
	if router.bufferSize != 10 {
		t.Errorf("expected buffer size 10, got %d", router.bufferSize)
	}
	if router.allMatches {
		t.Error("expected first-match mode after calling FirstMatch()")
	}
	if len(router.routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(router.routes))
	}
	if router.defaultRoute == nil {
		t.Error("expected default route to be set")
	}
}

// TestRouterGetRouteNames tests route name retrieval.
func TestRouterGetRouteNames(t *testing.T) {
	router := NewRouter[int]().
		AddRoute("route1", func(_ int) bool { return true }, NewTestProcessor("p1")).
		AddRoute("route2", func(_ int) bool { return true }, NewTestProcessor("p2")).
		WithDefault(NewTestProcessor("default"))

	names := router.GetRouteNames()
	sort.Strings(names) // Sort for consistent comparison

	expected := []string{"default", "route1", "route2"}
	if len(names) != len(expected) {
		t.Errorf("expected %d route names, got %d", len(expected), len(names))
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d] = %s, expected %s", i, name, expected[i])
		}
	}
}

// TestRouterBuilder tests the builder pattern.
func TestRouterBuilder(t *testing.T) {
	ctx := context.Background()

	// Create processors
	p1 := NewTestProcessor("p1")
	p2 := NewTestProcessor("p2")
	p3 := NewTestProcessor("p3")

	// Build router using builder
	router := NewRouterBuilder[int]().
		Route("multiples-of-3").When(func(n int) bool { return n%3 == 0 }).To(p1).
		Route("multiples-of-5").When(func(n int) bool { return n%5 == 0 }).To(p2).
		Route("others").When(func(n int) bool { return n%3 != 0 && n%5 != 0 }).To(p3).
		Build()

	// Test routing
	input := make(chan int)
	go func() {
		defer close(input)
		for i := 1; i <= 15; i++ {
			input <- i
		}
	}()

	outputs := router.Process(ctx, input)

	// Count results per route
	counts := make(map[string]int)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, ch := range outputs.Routes {
		wg.Add(1)
		go func(routeName string, routeChan <-chan int) {
			defer wg.Done()
			count := 0
			for range routeChan {
				count++
			}
			if count > 0 {
				mu.Lock()
				counts[routeName] = count
				mu.Unlock()
			}
		}(name, ch)
	}

	wg.Wait()

	// Verify counts (first-match routing)
	// multiples-of-3: 3,6,9,12,15 (gets 15 before multiples-of-5)
	if counts["multiples-of-3"] != 5 {
		t.Errorf("expected 5 multiples of 3, got %d", counts["multiples-of-3"])
	}
	// multiples-of-5: 5,10 (15 already went to multiples-of-3)
	if counts["multiples-of-5"] != 2 {
		t.Errorf("expected 2 multiples of 5, got %d", counts["multiples-of-5"])
	}
	// others: 1,2,4,7,8,11,13,14
	if counts["others"] != 8 {
		t.Errorf("expected 8 others, got %d", counts["others"])
	}
}

// TestRouterBuilderPanic tests builder panic on missing predicate.
func TestRouterBuilderPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for route without predicate")
		}
	}()

	NewRouterBuilder[int]().
		Route("invalid").
		To(NewTestProcessor("test")). // Missing When() call
		Build()
}

// Order type for complex type tests.
type Order struct {
	ID       int
	Amount   float64
	Priority string
	Country  string
}

// orderTestProcessor for test.
type orderTestProcessor struct {
	name string
}

func (*orderTestProcessor) Process(ctx context.Context, in <-chan Order) <-chan Order {
	out := make(chan Order)
	go func() {
		defer close(out)
		for order := range in {
			// Pass through orders unchanged
			select {
			case out <- order:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (p *orderTestProcessor) Name() string {
	return p.name
}

// TestRouterComplexTypes tests routing with complex types.
func TestRouterComplexTypes(t *testing.T) {
	ctx := context.Background()

	// Create processors
	highValueProcessor := &orderTestProcessor{name: "high-value"}
	urgentProcessor := &orderTestProcessor{name: "urgent"}
	internationalProcessor := &orderTestProcessor{name: "international"}

	// Create router
	router := NewRouter[Order]().
		AllMatches(). // Orders can match multiple categories
		AddRoute("high-value", func(o Order) bool {
			return o.Amount > 1000
		}, highValueProcessor).
		AddRoute("urgent", func(o Order) bool {
			return o.Priority == "urgent"
		}, urgentProcessor).
		AddRoute("international", func(o Order) bool {
			return o.Country != "US"
		}, internationalProcessor)

	// Create test orders
	orders := []Order{
		{ID: 1, Amount: 500, Priority: "normal", Country: "US"},
		{ID: 2, Amount: 1500, Priority: "urgent", Country: "US"},
		{ID: 3, Amount: 2000, Priority: "normal", Country: "UK"},
		{ID: 4, Amount: 100, Priority: "urgent", Country: "CA"},
	}

	input := make(chan Order)
	go func() {
		defer close(input)
		for _, order := range orders {
			input <- order
		}
	}()

	// Process
	outputs := router.Process(ctx, input)

	// Count results per route
	routeCounts := make(map[string]int)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, ch := range outputs.Routes {
		wg.Add(1)
		go func(routeName string, routeChan <-chan Order) {
			defer wg.Done()
			count := 0
			for range routeChan {
				count++
			}
			mu.Lock()
			routeCounts[routeName] = count
			mu.Unlock()
		}(name, ch)
	}

	wg.Wait()

	// Verify routing
	// Order 1: no routes
	// Order 2: high-value, urgent (2 routes)
	// Order 3: high-value, international (2 routes)
	// Order 4: urgent, international (2 routes)

	if routeCounts["high-value"] != 2 {
		t.Errorf("expected 2 high-value orders, got %d", routeCounts["high-value"])
	}
	if routeCounts["urgent"] != 2 {
		t.Errorf("expected 2 urgent orders, got %d", routeCounts["urgent"])
	}
	if routeCounts["international"] != 2 {
		t.Errorf("expected 2 international orders, got %d", routeCounts["international"])
	}
}

// passthroughProcessor for test.
type passthroughProcessor struct{}

func (*passthroughProcessor) Process(_ context.Context, in <-chan int) <-chan int {
	return in
}

func (*passthroughProcessor) Name() string {
	return "passthrough"
}

// BenchmarkRouterFirstMatch benchmarks first-match routing.
func BenchmarkRouterFirstMatch(b *testing.B) {
	ctx := context.Background()

	// Create a pass-through processor
	passthrough := &passthroughProcessor{}

	router := NewRouter[int]().
		AddRoute("even", func(n int) bool { return n%2 == 0 }, passthrough).
		AddRoute("odd", func(n int) bool { return n%2 == 1 }, passthrough)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		outputs := router.Process(ctx, input)

		// Drain outputs concurrently
		var wg sync.WaitGroup
		for _, ch := range outputs.Routes {
			wg.Add(1)
			go func(c <-chan int) {
				defer wg.Done()
				for range c { //nolint:revive // Intentionally draining channel
				}
			}(ch)
		}
		wg.Wait()
	}
}

// BenchmarkRouterAllMatches benchmarks all-matches routing.
func BenchmarkRouterAllMatches(b *testing.B) {
	ctx := context.Background()

	passthrough := &passthroughProcessor{}

	router := NewRouter[int]().
		AllMatches().
		AddRoute("div2", func(n int) bool { return n%2 == 0 }, passthrough).
		AddRoute("div3", func(n int) bool { return n%3 == 0 }, passthrough).
		AddRoute("div5", func(n int) bool { return n%5 == 0 }, passthrough)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 1)
		input <- i
		close(input)

		outputs := router.Process(ctx, input)

		// Drain outputs concurrently
		var wg sync.WaitGroup
		for _, ch := range outputs.Routes {
			wg.Add(1)
			go func(c <-chan int) {
				defer wg.Done()
				for range c { //nolint:revive // Intentionally draining channel
				}
			}(ch)
		}
		wg.Wait()
	}
}

// Example demonstrates basic content-based routing.
func ExampleRouter() {
	ctx := context.Background()

	// Create processors for different types
	smallProcessor := NewTestProcessor("small")
	largeProcessor := NewTestProcessor("large")

	// Create router
	router := NewRouter[int]().
		AddRoute("small", func(n int) bool {
			return n <= 10
		}, smallProcessor).
		AddRoute("large", func(n int) bool {
			return n > 10
		}, largeProcessor)

	// Create input
	input := make(chan int, 3)
	input <- 5
	input <- 15
	input <- 8
	close(input)

	// Process
	outputs := router.Process(ctx, input)

	// Collect results
	fmt.Println("Small numbers:")
	for n := range outputs.Routes["small"] {
		fmt.Printf("  %d\n", n)
	}

	fmt.Println("Large numbers:")
	for n := range outputs.Routes["large"] {
		fmt.Printf("  %d\n", n)
	}

	// Output:
	// Small numbers:
	//   10
	//   16
	// Large numbers:
	//   30
}

// Message type for example.
type Message struct {
	Type    string
	Content string
}

// messageProcessor for test.
type messageProcessor struct {
	name string
}

func (*messageProcessor) Process(_ context.Context, in <-chan Message) <-chan Message {
	return in // Pass through for example
}

func (mp *messageProcessor) Name() string {
	return mp.name
}

// Example demonstrates routing with default fallback.
func ExampleRouter_withDefault() {
	ctx := context.Background()

	// Create processors
	errorProcessor := &messageProcessor{name: "error"}
	warningProcessor := &messageProcessor{name: "warning"}
	defaultProcessor := &messageProcessor{name: "default"}

	// Create router with default
	router := NewRouter[Message]().
		AddRoute("errors", func(m Message) bool {
			return m.Type == "error"
		}, errorProcessor).
		AddRoute("warnings", func(m Message) bool {
			return m.Type == "warning"
		}, warningProcessor).
		WithDefault(defaultProcessor)

	// Create messages
	messages := []Message{
		{Type: "error", Content: "Failed to connect"},
		{Type: "warning", Content: "High memory usage"},
		{Type: "info", Content: "Server started"},
	}

	input := make(chan Message)
	go func() {
		defer close(input)
		for _, msg := range messages {
			input <- msg
		}
	}()

	// Process
	outputs := router.Process(ctx, input)

	// Count messages per route
	var wg sync.WaitGroup
	counts := make(map[string]int)
	var mu sync.Mutex

	for name, ch := range outputs.Routes {
		wg.Add(1)
		go func(routeName string, routeChan <-chan Message) {
			defer wg.Done()
			count := 0
			for range routeChan {
				count++
			}
			if count > 0 {
				mu.Lock()
				counts[routeName] = count
				mu.Unlock()
			}
		}(name, ch)
	}

	wg.Wait()

	// Print in deterministic order
	routes := []string{"errors", "warnings", "default"}
	for _, route := range routes {
		if count, ok := counts[route]; ok {
			fmt.Printf("%s: %d messages\n", route, count)
		}
	}

	// Output:
	// errors: 1 messages
	// warnings: 1 messages
	// default: 1 messages
}
