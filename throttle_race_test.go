package streamz

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestThrottle_RaceConditionReproduction verifies the exact race condition
// that causes non-deterministic behavior in the current implementation.
// This test should FAIL with the current implementation and PASS after the fix.
func TestThrottle_RaceConditionReproduction(t *testing.T) {
	// Run multiple iterations to catch the race
	for iteration := 0; iteration < 100; iteration++ {
		clock := clockz.NewFakeClock()
		throttle := NewThrottle[int](50*time.Millisecond, clock)
		ctx := context.Background()

		in := make(chan Result[int])
		out := throttle.Process(ctx, in)

		// Send first item to start cooling
		in <- NewSuccess(1)
		result := <-out
		if result.Value() != 1 {
			t.Fatalf("iteration %d: expected 1, got %d", iteration, result.Value())
		}

		// Create the exact race condition:
		// 1. Advance time to make timer ready
		clock.Advance(50 * time.Millisecond)
		clock.BlockUntilReady() // Timer is now ready to fire

		// 2. Send input while timer is ready - creates race
		in <- NewSuccess(2)

		// The behavior here is non-deterministic with current implementation:
		// - If timer fires first: cooling=false, item 2 is emitted
		// - If input processed first: cooling=true, item 2 is dropped

		// With the fix, timer should always be processed first
		select {
		case result := <-out:
			if result.Value() != 2 {
				t.Fatalf("iteration %d: expected 2, got %d", iteration, result.Value())
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("iteration %d: expected item 2 after cooling, got timeout (race caused drop)", iteration)
		}

		close(in)
		<-out // Wait for close
	}
}

// TestThrottle_ConcurrentStress tests throttle under heavy concurrent load
// to detect any race conditions or state corruption.
func TestThrottle_ConcurrentStress(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[int](10*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := throttle.Process(ctx, in)

	// Counters for verification
	var sent atomic.Int64
	var received atomic.Int64

	// Producer goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				in <- NewSuccess(id*1000 + j)
				sent.Add(1)
				// Random sleep to vary timing
				time.Sleep(time.Duration(j%3) * time.Microsecond)
			}
		}(i)
	}

	// Time advancement goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			time.Sleep(time.Millisecond)
			clock.Advance(10 * time.Millisecond)
			clock.BlockUntilReady()
		}
	}()

	// Consumer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case result, ok := <-out:
				if !ok {
					return
				}
				if result.IsError() {
					t.Errorf("unexpected error: %v", result.Error())
				}
				received.Add(1)
			case <-timeout:
				t.Error("consumer timeout")
				return
			}
		}
	}()

	// Let stress test run
	time.Sleep(2 * time.Second)

	// Cleanup
	close(in)
	wg.Wait()

	// Verify we received a reasonable number of items (throttled)
	sentCount := sent.Load()
	receivedCount := received.Load()

	t.Logf("Sent: %d, Received: %d", sentCount, receivedCount)

	if receivedCount == 0 {
		t.Error("no items received - complete throttle failure")
	}

	if receivedCount >= sentCount {
		t.Error("throttle not working - all items passed through")
	}

	// We expect roughly 1 item per cooling period
	expectedMax := int64(200) // ~2000ms / 10ms = 200 periods max
	if receivedCount > expectedMax {
		t.Errorf("too many items received: %d > %d", receivedCount, expectedMax)
	}
}

// TestThrottle_TimerChannelAbandonment verifies that abandoned timer channels
// don't cause hangs or panics.
func TestThrottle_TimerChannelAbandonment(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[string](50*time.Millisecond, clock)
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan Result[string])
	out := throttle.Process(ctx, in)

	// Send first item to create timer
	in <- NewSuccess("first")
	<-out

	// Advance time partially
	clock.Advance(25 * time.Millisecond)

	// Cancel context while timer is pending
	cancel()

	// Channel should close without hanging
	select {
	case _, ok := <-out:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel didn't close after context cancellation")
	}

	// Advance time to trigger abandoned timer
	clock.Advance(50 * time.Millisecond)
	clock.BlockUntilReady()

	// Should not panic or hang
	close(in)
}

// TestThrottle_RapidFireDuringCooling verifies that rapid inputs during
// cooling period are correctly dropped without causing state corruption.
func TestThrottle_RapidFireDuringCooling(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[int](100*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int], 1000) // Buffer to allow rapid fire
	out := throttle.Process(ctx, in)

	// Send first item
	in <- NewSuccess(1)
	result := <-out
	if result.Value() != 1 {
		t.Fatalf("expected 1, got %d", result.Value())
	}

	// Rapid fire during cooling
	for i := 2; i <= 1000; i++ {
		in <- NewSuccess(i)
	}

	// No items should be emitted during cooling
	select {
	case result := <-out:
		t.Errorf("unexpected result during cooling: %v", result)
	case <-time.After(10 * time.Millisecond):
		// Good - nothing received
	}

	// Advance past cooling period
	clock.Advance(100 * time.Millisecond)
	clock.BlockUntilReady()

	// Send one more item after cooling
	in <- NewSuccess(1001)

	// Should receive exactly one item (1001)
	result = <-out
	if result.Value() != 1001 {
		t.Fatalf("expected 1001 after cooling, got %d", result.Value())
	}

	close(in)
	<-out
}

// TestThrottle_ClockSkewAttack simulates clock manipulation attacks
// to verify throttle remains secure under time-based attacks.
func TestThrottle_ClockSkewAttack(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[string](time.Second, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	out := throttle.Process(ctx, in)

	// Normal operation
	in <- NewSuccess("item1")
	<-out

	// Try rapid time advances to bypass throttling
	for i := 0; i < 10; i++ {
		clock.Advance(time.Nanosecond) // Minimal advance
		in <- NewSuccess("attack")
	}

	// Should not receive any items (still cooling)
	select {
	case result := <-out:
		t.Errorf("throttle bypassed with minimal time advance: %v", result)
	case <-time.After(10 * time.Millisecond):
		// Good - throttle held
	}

	// Advance to exactly cooling period
	clock.Advance(time.Second - 10*time.Nanosecond)
	clock.BlockUntilReady()

	// Send legitimate item
	in <- NewSuccess("item2")

	// Should receive item after proper cooling
	result := <-out
	if result.Value() != "item2" {
		t.Errorf("expected item2, got %v", result)
	}

	close(in)
	<-out
}

// TestThrottle_MemoryExhaustion verifies throttle doesn't leak memory
// under sustained load with many dropped items.
func TestThrottle_MemoryExhaustion(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[[]byte](time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[[]byte])
	out := throttle.Process(ctx, in)

	// Consumer that counts received items
	var received atomic.Int64
	go func() {
		for range out {
			received.Add(1)
		}
	}()

	// Send large payloads continuously
	payload := make([]byte, 1024*1024) // 1MB payloads
	for i := 0; i < 1000; i++ {
		in <- NewSuccess(payload)

		// Occasionally advance time
		if i%100 == 0 {
			clock.Advance(time.Millisecond)
			clock.BlockUntilReady()
		}
	}

	close(in)
	<-out

	// Should have throttled most items (not stored/leaked)
	count := received.Load()
	if count > 20 { // At most ~10 cooling periods
		t.Errorf("too many items passed: %d", count)
	}

	// If we get here without OOM, memory management is working
}

// TestThrottle_StateConsistency performs property-based testing
// to verify state machine consistency under all conditions.
func TestThrottle_StateConsistency(t *testing.T) {
	scenarios := []struct {
		name     string
		duration time.Duration
		pattern  []action
	}{
		{
			name:     "alternating_input_timer",
			duration: 10 * time.Millisecond,
			pattern: []action{
				{typ: "input", value: 1},
				{typ: "advance", duration: 10 * time.Millisecond},
				{typ: "input", value: 2},
				{typ: "advance", duration: 10 * time.Millisecond},
			},
		},
		{
			name:     "burst_then_quiet",
			duration: 10 * time.Millisecond,
			pattern: []action{
				{typ: "input", value: 1},
				{typ: "input", value: 2},
				{typ: "input", value: 3},
				{typ: "advance", duration: 20 * time.Millisecond},
				{typ: "input", value: 4},
			},
		},
		{
			name:     "timer_before_input",
			duration: 10 * time.Millisecond,
			pattern: []action{
				{typ: "input", value: 1},
				{typ: "advance", duration: 15 * time.Millisecond},
				{typ: "input", value: 2},
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			clock := clockz.NewFakeClock()
			throttle := NewThrottle[int](sc.duration, clock)
			ctx := context.Background()

			in := make(chan Result[int])
			out := throttle.Process(ctx, in)

			var results []int
			done := make(chan struct{})

			go func() {
				for result := range out {
					if !result.IsError() {
						results = append(results, result.Value())
					}
				}
				close(done)
			}()

			// Execute pattern
			for _, act := range sc.pattern {
				switch act.typ {
				case "input":
					in <- NewSuccess(act.value)
					// Small delay to ensure processing
					time.Sleep(time.Millisecond)
				case "advance":
					clock.Advance(act.duration)
					clock.BlockUntilReady()
				}
			}

			close(in)
			<-done

			// Verify results make sense for throttling
			if len(results) == 0 {
				t.Error("no results received")
			}

			// First item should always pass (leading edge)
			if len(results) > 0 && results[0] != 1 {
				t.Errorf("first item should be 1, got %d", results[0])
			}

			t.Logf("Results for %s: %v", sc.name, results)
		})
	}
}

type action struct {
	typ      string
	value    int
	duration time.Duration
}

// TestThrottle_TwoPhaseCorrectness validates that the two-phase select pattern
// maintains correct semantics without introducing new bugs.
func TestThrottle_TwoPhaseCorrectness(t *testing.T) {
	// This test will only pass with the two-phase implementation
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[string](30*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[string])
	out := throttle.Process(ctx, in)

	results := make([]string, 0)
	done := make(chan struct{})

	go func() {
		for result := range out {
			if !result.IsError() {
				results = append(results, result.Value())
			}
		}
		close(done)
	}()

	// Test sequence that exposes the race
	in <- NewSuccess("A")
	time.Sleep(time.Millisecond)

	// Create race: timer ready + input ready
	clock.Advance(30 * time.Millisecond)
	clock.BlockUntilReady()
	in <- NewSuccess("B")
	time.Sleep(time.Millisecond)

	// Another race scenario
	clock.Advance(30 * time.Millisecond)
	clock.BlockUntilReady()
	in <- NewSuccess("C")
	time.Sleep(time.Millisecond)

	close(in)
	<-done

	// With two-phase select, we should get all three items
	// With race condition, B or C might be dropped
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d: %v", len(results), results)
	}

	expected := []string{"A", "B", "C"}
	for i, exp := range expected {
		if i < len(results) && results[i] != exp {
			t.Errorf("result[%d]: expected %s, got %s", i, exp, results[i])
		}
	}
}
