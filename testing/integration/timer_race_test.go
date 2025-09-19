package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/clockz"

	"github.com/zoobzio/streamz"
)

// TestTimerRaceConditions demonstrates the race condition between
// FakeClock.Advance() and timer-based processors.
//
// Root Cause: When FakeClock advances time, it sends on timer channels
// non-blockingly. The receiving goroutine needs scheduler time to process
// the timer event. If an item is sent immediately after Advance(), it may
// be processed before the timer event, causing incorrect behavior.
func TestTimerRaceConditions(t *testing.T) {
	t.Run("Throttle", func(t *testing.T) {
		testThrottleRace(t)
	})

	t.Run("Debounce", func(t *testing.T) {
		testDebounceRace(t)
	})
}

func testThrottleRace(t *testing.T) {
	const runs = 20
	failures := 0
	var mu sync.Mutex

	for i := 0; i < runs; i++ {
		clock := clockz.NewFakeClock()
		throttle := streamz.NewThrottle[int](10*time.Millisecond, clock)
		ctx := context.Background()

		in := make(chan streamz.Result[int])
		out := throttle.Process(ctx, in)

		// Send first item - starts cooling period
		in <- streamz.NewSuccess(1)
		<-out // Receive first item

		// Minimal yield to let timer register
		time.Sleep(time.Microsecond)

		// Advance clock to expire cooling period
		clock.Advance(10 * time.Millisecond)

		// RACE POINT: Send immediately after advance
		// The timer event and this item race to be processed first
		in <- streamz.NewSuccess(2)

		select {
		case <-out:
			// Success - timer was processed first
		case <-time.After(5 * time.Millisecond):
			// Failure - item was processed while still cooling
			mu.Lock()
			failures++
			mu.Unlock()
		}

		close(in)
		for range out { //nolint:revive // intentionally draining channel
		}
	}

	// We expect a high failure rate due to the race
	if failures == 0 {
		t.Log("Lucky run - no races detected. Try running test multiple times.")
	} else {
		t.Logf("Race condition confirmed: %d/%d runs failed (%.0f%%)",
			failures, runs, float64(failures)/float64(runs)*100)
	}
}

func testDebounceRace(t *testing.T) {
	const runs = 20
	failures := 0
	var mu sync.Mutex

	for i := 0; i < runs; i++ {
		clock := clockz.NewFakeClock()
		debounce := streamz.NewDebounce[int](10*time.Millisecond, clock)
		ctx := context.Background()

		in := make(chan streamz.Result[int])
		out := debounce.Process(ctx, in)

		// Send item to start debounce timer
		in <- streamz.NewSuccess(1)

		// Minimal yield to let timer register
		time.Sleep(time.Microsecond)

		// Advance clock to trigger debounce
		clock.Advance(10 * time.Millisecond)
		clock.BlockUntilReady()

		// RACE POINT: Expect output immediately after advance
		// The timer event races with our receive
		select {
		case result := <-out:
			if result.Value() != 1 {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		case <-time.After(5 * time.Millisecond):
			// Timeout - timer event not processed yet
			mu.Lock()
			failures++
			mu.Unlock()
		}

		close(in)
		for range out { //nolint:revive // intentionally draining channel
		}
	}

	if failures == 0 {
		t.Log("Lucky run - no races detected. Try running test multiple times.")
	} else {
		t.Logf("Race condition confirmed: %d/%d runs failed (%.0f%%)",
			failures, runs, float64(failures)/float64(runs)*100)
	}
}

// TestTimerRaceWithWorkaround demonstrates that adding a small sleep
// after clock.Advance() prevents the race condition by allowing the
// scheduler to process timer events.
func TestTimerRaceWithWorkaround(t *testing.T) {
	const runs = 20

	t.Run("ThrottleWithSleep", func(t *testing.T) {
		failures := 0

		for i := 0; i < runs; i++ {
			clock := clockz.NewFakeClock()
			throttle := streamz.NewThrottle[int](10*time.Millisecond, clock)
			ctx := context.Background()

			in := make(chan streamz.Result[int])
			out := throttle.Process(ctx, in)

			in <- streamz.NewSuccess(1)
			<-out

			time.Sleep(time.Microsecond)
			clock.Advance(10 * time.Millisecond)

			// WORKAROUND: Sleep to let timer event be processed
			time.Sleep(10 * time.Millisecond)

			in <- streamz.NewSuccess(2)

			select {
			case <-out:
				// Success
			case <-time.After(5 * time.Millisecond):
				failures++
			}

			close(in)
			for range out { //nolint:revive // intentionally draining channel
			}
		}

		if failures > 0 {
			t.Errorf("Workaround failed: %d/%d runs still have race", failures, runs)
		} else {
			t.Logf("Workaround successful: 0/%d failures with sleep after Advance()", runs)
		}
	})
}
