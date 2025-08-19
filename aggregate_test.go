package streamz

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"streamz/clock"
)

// TestAggregateSum tests basic sum aggregation.
func TestAggregateSum(t *testing.T) {
	ctx := context.Background()

	// Sum with count-based windows
	summer := NewAggregate(0, Sum[int](), clock.Real).
		WithCountWindow(3).
		WithName("summer")

	input := make(chan int, 9)
	for i := 1; i <= 9; i++ {
		input <- i
	}
	close(input)

	windows := summer.Process(ctx, input)

	expected := []int{6, 15, 24} // 1+2+3=6, 4+5+6=15, 7+8+9=24
	var results []int            //nolint:prealloc // dynamic growth acceptable in test code

	for window := range windows {
		results = append(results, window.Result)
		if window.Count != 3 {
			t.Errorf("expected count 3, got %d", window.Count)
		}
	}

	if len(results) != 3 {
		t.Errorf("expected 3 windows, got %d", len(results))
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("window %d: expected sum %d, got %d", i, expected[i], result)
		}
	}

	if summer.Name() != "summer" {
		t.Errorf("expected name 'summer', got %s", summer.Name())
	}
}

// TestAggregateTimeWindow tests time-based aggregation.
func TestAggregateTimeWindow(t *testing.T) {
	ctx := context.Background()

	// Use real clock for simpler timing in this test
	counter := NewAggregate(0, Count[string](), clock.Real).
		WithTimeWindow(50 * time.Millisecond).
		WithEmptyWindows(false)

	input := make(chan string)
	windows := counter.Process(ctx, input)

	// Collect windows
	var results []AggregateWindow[int] //nolint:prealloc // dynamic growth acceptable in test code
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for window := range windows {
			results = append(results, window)
		}
	}()

	// Send items in bursts
	go func() {
		// First burst
		for i := 0; i < 5; i++ {
			input <- fmt.Sprintf("item-%d", i)
		}

		// Wait for window
		time.Sleep(70 * time.Millisecond)

		// Second burst
		for i := 5; i < 8; i++ {
			input <- fmt.Sprintf("item-%d", i)
		}

		// Wait for window
		time.Sleep(70 * time.Millisecond)

		close(input)
	}()

	wg.Wait()

	// Should have at least 2 windows with data
	if len(results) < 2 {
		t.Errorf("expected at least 2 windows, got %d", len(results))
	}

	// First window should have 5 items
	if results[0].Count != 5 {
		t.Errorf("expected first window to have 5 items, got %d", results[0].Count)
	}

	// Check window times are reasonable
	for i, window := range results {
		duration := window.End.Sub(window.Start)
		if duration < 40*time.Millisecond || duration > 100*time.Millisecond {
			t.Errorf("window %d duration out of range: %v", i, duration)
		}
	}
}

// TestAggregateAverage tests average calculation.
func TestAggregateAverage(t *testing.T) {
	ctx := context.Background()

	averager := NewAggregate(Average{}, Avg[float64](), clock.Real).
		WithCountWindow(4)

	input := make(chan float64, 8)
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80}
	for _, v := range values {
		input <- v
	}
	close(input)

	windows := averager.Process(ctx, input)

	expected := []float64{25.0, 65.0} // (10+20+30+40)/4=25, (50+60+70+80)/4=65
	var results []float64             //nolint:prealloc // dynamic growth acceptable in test code

	for window := range windows {
		results = append(results, window.Result.Value())
	}

	if len(results) != 2 {
		t.Errorf("expected 2 windows, got %d", len(results))
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("window %d: expected average %.1f, got %.1f", i, expected[i], result)
		}
	}
}

// TestAggregateMinMax tests min/max tracking.
func TestAggregateMinMax(t *testing.T) {
	ctx := context.Background()

	minMaxer := NewAggregate(MinMax[int]{}, MinMaxAgg[int](), clock.Real).
		WithCountWindow(5)

	input := make(chan int, 10)
	values := []int{5, 2, 8, 1, 9, 3, 7, 4, 6, 10}
	for _, v := range values {
		input <- v
	}
	close(input)

	windows := minMaxer.Process(ctx, input)

	var results []MinMax[int] //nolint:prealloc // dynamic growth acceptable in test code
	for window := range windows {
		results = append(results, window.Result)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 windows, got %d", len(results))
	}

	// First window: 5,2,8,1,9 -> min=1, max=9
	if results[0].Min != 1 || results[0].Max != 9 {
		t.Errorf("window 0: expected min=1, max=9, got %s", results[0])
	}

	// Second window: 3,7,4,6,10 -> min=3, max=10
	if results[1].Min != 3 || results[1].Max != 10 {
		t.Errorf("window 1: expected min=3, max=10, got %s", results[1])
	}
}

// TestAggregateCustomFunction tests custom aggregation.
func TestAggregateCustomFunction(t *testing.T) {
	ctx := context.Background()

	// Custom aggregator: concatenate strings
	type StringList struct {
		Items []string
	}

	concatenator := NewAggregate(
		StringList{},
		func(list StringList, item string) StringList {
			list.Items = append(list.Items, item)
			return list
		},
		clock.Real,
	).WithCountWindow(3)

	input := make(chan string, 6)
	words := []string{"hello", "world", "from", "go", "channels", "!"}
	for _, w := range words {
		input <- w
	}
	close(input)

	windows := concatenator.Process(ctx, input)

	var results []StringList //nolint:prealloc // dynamic growth acceptable in test code
	for window := range windows {
		results = append(results, window.Result)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 windows, got %d", len(results))
	}

	// First window
	if len(results[0].Items) != 3 {
		t.Errorf("expected 3 items in first window, got %d", len(results[0].Items))
	}
	if results[0].Items[0] != "hello" || results[0].Items[2] != "from" {
		t.Errorf("unexpected items in first window: %v", results[0].Items)
	}
}

// TestAggregateMixedTriggers tests count and time triggers together.
func TestAggregateMixedTriggers(t *testing.T) {
	ctx := context.Background()

	// Emit on count OR time
	summer := NewAggregate(0, Sum[int](), clock.Real).
		WithCountWindow(5).
		WithTimeWindow(150 * time.Millisecond)

	input := make(chan int)
	windows := summer.Process(ctx, input)

	var results []AggregateWindow[int] //nolint:prealloc // dynamic growth acceptable in test code
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for window := range windows {
			results = append(results, window)
		}
	}()

	// Send items slowly
	go func() {
		// Quick burst - should trigger count
		for i := 1; i <= 5; i++ {
			input <- i
		}

		// Slow items - should trigger time
		time.Sleep(100 * time.Millisecond)
		input <- 6
		input <- 7

		time.Sleep(100 * time.Millisecond)

		close(input)
	}()

	wg.Wait()

	if len(results) < 2 {
		t.Errorf("expected at least 2 windows, got %d", len(results))
	}

	// First window should be count-triggered (1+2+3+4+5=15)
	if results[0].Count != 5 || results[0].Result != 15 {
		t.Errorf("first window unexpected: count=%d, sum=%d", results[0].Count, results[0].Result)
	}
}

// TestAggregateEmptyWindows tests empty window emission.
func TestAggregateEmptyWindows(t *testing.T) {
	ctx := context.Background()

	// With empty windows
	counter1 := NewAggregate(0, Count[int](), clock.Real).
		WithTimeWindow(50 * time.Millisecond).
		WithEmptyWindows(true)

	// Without empty windows
	counter2 := NewAggregate(0, Count[int](), clock.Real).
		WithTimeWindow(50 * time.Millisecond).
		WithEmptyWindows(false)

	input1 := make(chan int)
	input2 := make(chan int)

	windows1 := counter1.Process(ctx, input1)
	windows2 := counter2.Process(ctx, input2)

	var results1, results2 []AggregateWindow[int] //nolint:prealloc // dynamic growth acceptable in test code
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for window := range windows1 {
			results1 = append(results1, window)
		}
	}()

	go func() {
		defer wg.Done()
		for window := range windows2 {
			results2 = append(results2, window)
		}
	}()

	// Wait for empty windows
	time.Sleep(150 * time.Millisecond)

	close(input1)
	close(input2)
	wg.Wait()

	// With empty windows should have multiple windows
	if len(results1) < 2 {
		t.Errorf("expected multiple empty windows, got %d", len(results1))
	}

	// Without empty windows should have none
	if len(results2) != 0 {
		t.Errorf("expected no windows without data, got %d", len(results2))
	}
}

// TestAggregateContextCancellation tests graceful shutdown.
func TestAggregateContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	summer := NewAggregate(0, Sum[int](), clock.Real).
		WithTimeWindow(100 * time.Millisecond)

	input := make(chan int)
	windows := summer.Process(ctx, input)

	var resultCount int
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range windows {
			resultCount++
		}
	}()

	// Send items continuously
	go func() {
		for i := 1; ; i++ {
			select {
			case input <- i:
				time.Sleep(10 * time.Millisecond)
			case <-ctx.Done():
				close(input)
				return
			}
		}
	}()

	// Cancel after some windows
	time.Sleep(250 * time.Millisecond)
	cancel()

	wg.Wait()

	// Should have emitted at least 2 windows
	if resultCount < 2 {
		t.Errorf("expected at least 2 windows before cancellation, got %d", resultCount)
	}
}

// TestAggregateGetCurrentState tests state inspection.
func TestAggregateGetCurrentState(t *testing.T) {
	ctx := context.Background()

	summer := NewAggregate(0, Sum[int](), clock.Real).
		WithCountWindow(10) // High count so we can inspect mid-aggregation

	input := make(chan int)
	_ = summer.Process(ctx, input)

	// Send some items
	for i := 1; i <= 5; i++ {
		input <- i
	}

	// Check current state
	time.Sleep(10 * time.Millisecond) // Let processing happen
	state, count := summer.GetCurrentState()

	if state != 15 { // 1+2+3+4+5
		t.Errorf("expected current sum 15, got %d", state)
	}
	if count != 5 {
		t.Errorf("expected current count 5, got %d", count)
	}

	close(input)
}

// TestAggregateEdgeCases tests edge cases.
func TestAggregateEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("ZeroCountWindow", func(t *testing.T) {
		// Should default to 1
		agg := NewAggregate(0, Sum[int](), clock.Real).WithCountWindow(0)
		if agg.maxCount != 1 {
			t.Errorf("expected count window to default to 1, got %d", agg.maxCount)
		}
	})

	t.Run("NegativeTimeWindow", func(t *testing.T) {
		// Should default to 0 (disabled)
		agg := NewAggregate(0, Sum[int](), clock.Real).WithTimeWindow(-1 * time.Second)
		if agg.maxLatency != 0 {
			t.Errorf("expected time window to be disabled, got %v", agg.maxLatency)
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		summer := NewAggregate(0, Sum[int](), clock.Real).WithCountWindow(5)

		input := make(chan int)
		close(input)

		windows := summer.Process(ctx, input)

		count := 0
		for range windows {
			count++
		}

		if count != 0 {
			t.Errorf("expected no windows for empty input, got %d", count)
		}
	})

	t.Run("SingleItem", func(t *testing.T) {
		summer := NewAggregate(0, Sum[int](), clock.Real).WithCountWindow(5)

		input := make(chan int, 1)
		input <- 42
		close(input)

		windows := summer.Process(ctx, input)

		var results []AggregateWindow[int] //nolint:prealloc // dynamic growth acceptable in test code
		for window := range windows {
			results = append(results, window)
		}

		// Should emit final window with partial data
		if len(results) != 1 {
			t.Errorf("expected 1 window, got %d", len(results))
		}
		if results[0].Result != 42 {
			t.Errorf("expected sum 42, got %d", results[0].Result)
		}
		if results[0].Count != 1 {
			t.Errorf("expected count 1, got %d", results[0].Count)
		}
	})
}

// BenchmarkAggregate benchmarks aggregation performance.
func BenchmarkAggregate(b *testing.B) {
	ctx := context.Background()

	summer := NewAggregate(0, Sum[int](), clock.Real).WithCountWindow(100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 100)
		for j := 0; j < 100; j++ {
			input <- j
		}
		close(input)

		windows := summer.Process(ctx, input)
		for range windows { //nolint:revive // Intentionally draining channel
			// Consume
		}
	}
}

// BenchmarkAggregateThroughput benchmarks high-throughput aggregation.
func BenchmarkAggregateThroughput(b *testing.B) {
	ctx := context.Background()

	summer := NewAggregate(0, Sum[int](), clock.Real).WithCountWindow(1000)

	b.ResetTimer()
	b.ReportAllocs()

	input := make(chan int, 1000)
	windows := summer.Process(ctx, input)

	// Consumer
	go func() {
		for range windows { //nolint:revive // Intentionally draining channel
			// Consume
		}
	}()

	// Send items
	for i := 0; i < b.N; i++ {
		input <- i
	}
	close(input)
}

// Example demonstrates time-based aggregation.
func ExampleAggregate_timeWindows() {
	ctx := context.Background()

	// Count events per second
	counter := NewAggregate(0, Count[string](), clock.Real).
		WithTimeWindow(1 * time.Second).
		WithName("event-counter")

	events := make(chan string)
	windows := counter.Process(ctx, events)

	// Simulate events
	go func() {
		events <- "login"
		events <- "page_view"
		time.Sleep(500 * time.Millisecond)
		events <- "click"
		events <- "logout"
		time.Sleep(600 * time.Millisecond)
		events <- "login"
		close(events)
	}()

	// Print windows
	for window := range windows {
		fmt.Printf("Window %s-%s: %d events\n",
			window.Start.Format("15:04:05"),
			window.End.Format("15:04:05"),
			window.Result)
	}
}

// Example demonstrates custom aggregation.
func ExampleAggregate_custom() {
	ctx := context.Background()

	// Track unique users
	type UserSet map[string]bool

	uniqueUsers := NewAggregate(
		make(UserSet),
		func(users UserSet, userID string) UserSet {
			// Create new map to avoid mutation
			newUsers := make(UserSet)
			for k, v := range users {
				newUsers[k] = v
			}
			newUsers[userID] = true
			return newUsers
		},
		clock.Real,
	).WithCountWindow(5)

	// User activity stream
	activity := make(chan string, 10)
	activity <- "alice"
	activity <- "bob"
	activity <- "alice" // duplicate
	activity <- "charlie"
	activity <- "bob" // duplicate
	activity <- "david"
	activity <- "eve"
	activity <- "alice" // duplicate
	close(activity)

	// Process windows
	windows := uniqueUsers.Process(ctx, activity)

	windowNum := 1
	for window := range windows {
		fmt.Printf("Window %d: %d unique users\n", windowNum, len(window.Result))
		windowNum++
	}

	// Output:
	// Window 1: 3 unique users
	// Window 2: 3 unique users
}
