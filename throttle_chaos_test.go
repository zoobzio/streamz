package streamz

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

//nolint:gosec // Test code uses non-cryptographic randomness intentionally

// TestThrottle_ChaoticPatterns subjects the throttle to random chaotic patterns
// to uncover edge cases and race conditions.
func TestThrottle_ChaoticPatterns(t *testing.T) {
	// Create seeded random generator for reproducibility
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic seed for reproducible tests

	for round := 0; round < 10; round++ {
		t.Run(fmt.Sprintf("round_%d", round), func(t *testing.T) {
			clock := clockz.NewFakeClock()
			duration := time.Duration(rng.Intn(100)+10) * time.Millisecond
			throttle := NewThrottle[int](duration, clock)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			in := make(chan Result[int], 100)
			out := throttle.Process(ctx, in)

			var wg sync.WaitGroup
			var sent, received atomic.Int64
			errs := make([]error, 0)
			var errorsMu sync.Mutex

			// Chaos producer
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					// Random action
					switch rng.Intn(5) {
					case 0, 1, 2: // Send value (60% chance)
						in <- NewSuccess(i)
						sent.Add(1)
					case 3: // Send error (20% chance)
						in <- NewError(0, errors.New("chaos"), "test")
						errorsMu.Lock()
						errs = append(errs, errors.New("chaos"))
						errorsMu.Unlock()
					case 4: // Small delay (20% chance)
						time.Sleep(time.Duration(rng.Intn(5)) * time.Millisecond)
					}
				}
			}()

			// Chaos time advancer
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					// Random time advance
					advance := time.Duration(rng.Intn(int(duration)*2)) * time.Millisecond
					clock.Advance(advance)

					// Sometimes block, sometimes don't
					if rng.Float32() < 0.5 {
						clock.BlockUntilReady()
					}

					time.Sleep(time.Duration(rng.Intn(10)) * time.Millisecond)
				}
			}()

			// Consumer with timeout
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
						if !result.IsError() {
							received.Add(1)
						}
					case <-timeout:
						t.Log("Consumer timeout (may be normal)")
						return
					}
				}
			}()

			// Random context cancellation
			if rng.Float32() < 0.3 {
				time.AfterFunc(time.Duration(rng.Intn(100))*time.Millisecond, cancel)
			}

			// Let chaos run
			time.Sleep(500 * time.Millisecond)
			close(in)
			wg.Wait()

			// Validate results are sensible
			sentCount := sent.Load()
			receivedCount := received.Load()

			t.Logf("Chaos round %d: sent=%d, received=%d, errors=%d",
				round, sentCount, receivedCount, len(errs))

			// Basic sanity checks
			if receivedCount > sentCount {
				t.Errorf("received more than sent: %d > %d", receivedCount, sentCount)
			}

			// Should throttle at least some items (unless very few sent)
			if sentCount > 10 && receivedCount >= sentCount {
				t.Errorf("no throttling occurred: sent=%d, received=%d", sentCount, receivedCount)
			}
		})
	}
}

// TestThrottle_CascadeFailure simulates cascading failures where
// multiple components fail in sequence.
func TestThrottle_CascadeFailure(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[string](50*time.Millisecond, clock)
	ctx := context.Background()

	// Create a chain of throttles (simulating a pipeline)
	stage1 := make(chan Result[string], 10)
	stage2 := throttle.Process(ctx, stage1)
	stage3 := NewThrottle[string](30*time.Millisecond, clock).Process(ctx, stage2)
	stage4 := NewThrottle[string](20*time.Millisecond, clock).Process(ctx, stage3)

	// Send burst of data
	go func() {
		for i := 0; i < 100; i++ {
			stage1 <- NewSuccess(fmt.Sprintf("item_%d", i))
			if i%10 == 0 {
				// Inject errors periodically
				stage1 <- NewError("", fmt.Errorf("cascade_%d", i), "source")
			}
		}
		close(stage1)
	}()

	// Advance time chaotically
	go func() {
		localRng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic seed for reproducible tests
		for i := 0; i < 20; i++ {
			time.Sleep(10 * time.Millisecond)
			clock.Advance(time.Duration(localRng.Intn(100)) * time.Millisecond)
			clock.BlockUntilReady()
		}
	}()

	// Collect results
	results := make([]Result[string], 0)
	timeout := time.After(2 * time.Second)

	for {
		select {
		case result, ok := <-stage4:
			if !ok {
				goto done
			}
			results = append(results, result)
		case <-timeout:
			goto done
		}
	}
done:

	t.Logf("Cascade test: received %d items through 3 throttle stages", len(results))

	// Should receive at least some items
	if len(results) == 0 {
		t.Error("cascade failure: no items passed through pipeline")
	}

	// Count errors vs successes
	var errorCount, successCount int
	for _, r := range results {
		if r.IsError() {
			errorCount++
		} else {
			successCount++
		}
	}

	t.Logf("Errors: %d, Successes: %d", errorCount, successCount)

	// Errors should pass through
	if errorCount == 0 {
		t.Error("no errors passed through cascade")
	}
}

// TestThrottle_ResourceExhaustion attempts to exhaust system resources
// through various attack vectors.
func TestThrottle_ResourceExhaustion(t *testing.T) {
	tests := []struct {
		name   string
		attack func(*testing.T)
	}{
		{
			name: "channel_saturation",
			attack: func(_ *testing.T) {
				clock := clockz.NewFakeClock()
				throttle := NewThrottle[int](time.Microsecond, clock)
				ctx := context.Background()

				// Create many channels
				channels := make([]<-chan Result[int], 100)
				for i := 0; i < 100; i++ {
					in := make(chan Result[int], 1000)
					channels[i] = throttle.Process(ctx, in)

					// Flood each channel
					for j := 0; j < 1000; j++ {
						select {
						case in <- NewSuccess(j):
						default:
							// Channel full, move on
						}
					}
				}

				// Should not panic or hang
				time.Sleep(100 * time.Millisecond)
			},
		},
		{
			name: "goroutine_leak",
			attack: func(_ *testing.T) {
				initialGoroutines := runtime.NumGoroutine()

				for i := 0; i < 100; i++ {
					clock := clockz.NewFakeClock()
					throttle := NewThrottle[string](time.Millisecond, clock)
					ctx, cancel := context.WithCancel(context.Background())

					in := make(chan Result[string])
					_ = throttle.Process(ctx, in)

					// Immediate cancellation
					cancel()
					close(in)
				}

				// Give goroutines time to clean up
				time.Sleep(100 * time.Millisecond)
				runtime.GC()

				finalGoroutines := runtime.NumGoroutine()
				leaked := finalGoroutines - initialGoroutines

				if leaked > 10 { // Allow some tolerance
					t.Errorf("goroutine leak detected: %d goroutines leaked", leaked)
				}
			},
		},
		{
			name: "timer_accumulation",
			attack: func(_ *testing.T) {
				clock := clockz.NewFakeClock()
				throttle := NewThrottle[int](time.Hour, clock) // Long duration
				ctx := context.Background()

				in := make(chan Result[int])
				out := throttle.Process(ctx, in)

				// Create many timers by sending values
				for i := 0; i < 1000; i++ {
					in <- NewSuccess(i)

					// Drain output to prevent blocking
					select {
					case <-out:
					default:
					}

					// Don't advance time - timers accumulate
				}

				// Should handle accumulated timers gracefully
				close(in)

				// Drain remaining
				for result := range out {
					_ = result // Consume
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Monitor for panics
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic during %s: %v", tt.name, r)
				}
			}()

			tt.attack(t)
		})
	}
}

// TestThrottle_SelectFairness verifies that the two-phase select pattern
// doesn't create unfairness or starvation issues.
func TestThrottle_SelectFairness(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[int](10*time.Millisecond, clock)
	ctx := context.Background()

	in := make(chan Result[int])
	out := throttle.Process(ctx, in)

	// Track when items are processed
	processed := make([]timePoint, 0)
	var mu sync.Mutex

	go func() {
		for result := range out {
			if !result.IsError() {
				mu.Lock()
				processed = append(processed, timePoint{
					value: result.Value(),
					time:  clock.Now(),
				})
				mu.Unlock()
			}
		}
	}()

	// Send items at specific times
	in <- NewSuccess(1)
	time.Sleep(time.Millisecond)

	// Multiple timer expirations with inputs
	for i := 0; i < 10; i++ {
		clock.Advance(10 * time.Millisecond)
		clock.BlockUntilReady()
		in <- NewSuccess(i + 2)
		time.Sleep(time.Millisecond)
	}

	close(in)
	<-time.After(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Verify fair processing
	if len(processed) < 5 {
		t.Errorf("unfair processing: only %d items processed", len(processed))
	}

	// Check timing intervals
	for i := 1; i < len(processed); i++ {
		interval := processed[i].time.Sub(processed[i-1].time)
		if interval < 10*time.Millisecond {
			t.Errorf("items %d and %d too close: %v apart",
				processed[i-1].value, processed[i].value, interval)
		}
	}
}

type timePoint struct {
	value int
	time  time.Time
}

// TestThrottle_ByzantineBehavior tests behavior under Byzantine conditions
// where inputs may be malicious or components may fail arbitrarily.
// Note: This is NOT testing Byzantine fault tolerance (throttle doesn't need that),
// but rather ensuring graceful degradation under adversarial conditions.
func TestThrottle_ByzantineBehavior(t *testing.T) {
	clock := clockz.NewFakeClock()
	throttle := NewThrottle[interface{}](25*time.Millisecond, clock)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic seed for reproducible tests

	in := make(chan Result[interface{}])
	out := throttle.Process(ctx, in)

	// Consumer that handles anything
	go func() {
		for result := range out {
			_ = result // Just consume, don't care about values
		}
	}()

	// Send various malicious inputs
	maliciousInputs := []interface{}{
		nil,                         // nil value
		make(chan int),              // channel as value
		func() {},                   // function as value
		struct{ x [1000000]byte }{}, // large struct
		&struct{ p *int }{},         // pointer with nil field
	}

	for _, input := range maliciousInputs {
		in <- NewSuccess(input)

		// Randomly advance time
		if rng.Float32() < 0.5 {
			clock.Advance(time.Duration(rng.Intn(50)) * time.Millisecond)
		}
	}

	// Send errors with nil and empty messages
	in <- NewError[interface{}](nil, nil, "")
	in <- NewError[interface{}](nil, errors.New(""), "")

	// Should handle all inputs without panic
	close(in)

	// If we get here, Byzantine inputs were handled gracefully
	t.Log("Byzantine behavior test passed - no panics")
}
