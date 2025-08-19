package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"

	"streamz/clock"
	clocktesting "streamz/clock/testing"
)

func TestDedupe(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	dedupe := NewDedupe(func(i int) int { return i }, clock.Real).WithTTL(100 * time.Millisecond)
	out := dedupe.Process(ctx, in)

	go func() {
		for i := 0; i < 3; i++ {
			in <- 1
			in <- 2
			in <- 1
		}
		close(in)
	}()

	results := []int{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 unique items, got %d: %v", len(results), results)
	}

	if results[0] != 1 || results[1] != 2 {
		t.Errorf("expected [1, 2], got %v", results)
	}
}

func TestDedupeTTL(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	clk := clocktesting.NewFakeClock(time.Now())
	dedupe := NewDedupe(func(i int) int { return i }, clk).WithTTL(50 * time.Millisecond)
	out := dedupe.Process(ctx, in)

	go func() {
		in <- 1
		clk.Step(60 * time.Millisecond)
		in <- 1
		in <- 2
		close(in)
	}()

	results := []int{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 items (1 appeared twice after TTL), got %d: %v", len(results), results)
	}
}

func TestDedupeCustomKey(t *testing.T) {
	ctx := context.Background()
	in := make(chan string)

	dedupe := NewDedupe(func(s string) int { return len(s) }, clock.Real).WithTTL(100 * time.Millisecond)
	out := dedupe.Process(ctx, in)

	go func() {
		in <- "a"
		in <- "bb"
		in <- "c"
		in <- "dd"
		close(in)
	}()

	results := []string{}
	for result := range out {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 items (different lengths), got %d: %v", len(results), results)
	}

	if results[0] != "a" || results[1] != "bb" {
		t.Errorf("expected first item of each length, got %v", results)
	}
}

func TestDedupeCleanup(t *testing.T) {
	ctx := context.Background()
	clk := clocktesting.NewFakeClock(time.Now())

	// Create dedupe with 40ms TTL, cleanup runs every 20ms
	dedupe := NewDedupe(func(i int) int { return i }, clk).WithTTL(40 * time.Millisecond)

	// Start processing in background
	in := make(chan int)
	out := dedupe.Process(ctx, in)

	// Drain output in background
	go func() {
		//nolint:revive // empty-block: necessary to drain channel
		for range out {
		}
	}()

	// Add some items
	for i := 0; i < 10; i++ {
		in <- i
	}

	// Wait for processing
	time.Sleep(10 * time.Millisecond)

	// Check initial state
	dedupe.mu.Lock()
	initialCount := len(dedupe.seen)
	dedupe.mu.Unlock()

	if initialCount != 10 {
		t.Errorf("expected 10 items initially, got %d", initialCount)
	}

	// Advance clock past TTL to trigger cleanup
	// First advance to trigger the ticker (20ms)
	clk.Step(20 * time.Millisecond)
	clk.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let cleanup run

	// Items are 20ms old, TTL is 40ms, so nothing cleaned yet
	dedupe.mu.Lock()
	afterFirstTick := len(dedupe.seen)
	dedupe.mu.Unlock()

	if afterFirstTick != 10 {
		t.Errorf("expected 10 items after first tick, got %d", afterFirstTick)
	}

	// Advance another 20ms (total 40ms) - should trigger another cleanup
	clk.Step(20 * time.Millisecond)
	clk.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let cleanup run

	// Now items are 40ms old, equal to TTL, might not be cleaned yet

	// Advance one more tick (total 60ms) - items should definitely be cleaned
	clk.Step(20 * time.Millisecond)
	clk.BlockUntilReady()
	time.Sleep(10 * time.Millisecond) // Let cleanup run

	// Check that old items were cleaned
	dedupe.mu.Lock()
	finalCount := len(dedupe.seen)
	dedupe.mu.Unlock()

	if finalCount != 0 {
		t.Errorf("expected 0 items after cleanup, got %d", finalCount)
	}

	// Verify that we can add the same items again
	for i := 0; i < 5; i++ {
		in <- i
	}

	time.Sleep(10 * time.Millisecond)

	dedupe.mu.Lock()
	newCount := len(dedupe.seen)
	dedupe.mu.Unlock()

	if newCount != 5 {
		t.Errorf("expected 5 new items, got %d", newCount)
	}

	close(in)
}

// Example demonstrates deduplicating events by ID.
func ExampleDedupe() {
	ctx := context.Background()

	// Event type with ID for deduplication.
	type Event struct {
		ID     string
		Action string
		UserID string
	}

	// Deduplicate events by ID with 1-minute TTL.
	// Using shorter TTL for example.
	deduper := NewDedupe(func(e Event) string {
		return e.ID
	}, clock.Real).WithTTL(100 * time.Millisecond).WithName("event-deduper")

	// Simulate event stream with duplicates.
	events := make(chan Event)
	go func() {
		// Some events may be sent multiple times.
		events <- Event{ID: "evt-001", Action: "login", UserID: "user-123"}
		events <- Event{ID: "evt-002", Action: "view", UserID: "user-123"}
		events <- Event{ID: "evt-001", Action: "login", UserID: "user-123"} // Duplicate
		events <- Event{ID: "evt-003", Action: "click", UserID: "user-123"}
		events <- Event{ID: "evt-002", Action: "view", UserID: "user-123"} // Duplicate
		events <- Event{ID: "evt-004", Action: "logout", UserID: "user-123"}

		// After TTL expires, same ID can appear again.
		time.Sleep(150 * time.Millisecond)
		events <- Event{ID: "evt-001", Action: "login", UserID: "user-456"} // New occurrence

		close(events)
	}()

	// Process deduplicated events.
	unique := deduper.Process(ctx, events)

	fmt.Println("Unique events processed:")
	for event := range unique {
		fmt.Printf("- %s: %s by %s\n", event.ID, event.Action, event.UserID)
	}

	// Output:
	// Unique events processed:
	// - evt-001: login by user-123
	// - evt-002: view by user-123
	// - evt-003: click by user-123
	// - evt-004: logout by user-123
	// - evt-001: login by user-456
}
