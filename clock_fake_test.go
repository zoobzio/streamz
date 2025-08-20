// Package streamz provides a fake clock implementation for deterministic testing.
package streamz

import (
	"sync"
	"time"
)

// FakeClock implements Clock for testing purposes.
// It allows manual control of time progression.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type FakeClock struct {
	mu      sync.RWMutex
	wg      sync.WaitGroup
	time    time.Time
	waiters []*waiter
}

// waiter represents a timer or ticker waiting for a specific time.
type waiter struct {
	targetTime time.Time
	destChan   chan time.Time
	afterFunc  func()
	period     time.Duration // For tickers
	active     bool
}

// NewFakeClock creates a new FakeClock set to the given time.
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{
		time: t,
	}
}

// Now returns the current time of the fake clock.
func (f *FakeClock) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.time
}

// After waits for the duration to elapse and then sends the current time.
func (f *FakeClock) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	targetTime := f.time.Add(d)

	w := &waiter{
		targetTime: targetTime,
		destChan:   ch,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return ch
}

// AfterFunc waits for the duration to elapse and then executes f.
func (f *FakeClock) AfterFunc(d time.Duration, fn func()) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()

	targetTime := f.time.Add(d)
	w := &waiter{
		targetTime: targetTime,
		afterFunc:  fn,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return &fakeTimer{
		clock:  f,
		waiter: w,
	}
}

// NewTimer creates a new Timer.
func (f *FakeClock) NewTimer(d time.Duration) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	targetTime := f.time.Add(d)

	w := &waiter{
		targetTime: targetTime,
		destChan:   ch,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return &fakeTimer{
		clock:  f,
		waiter: w,
	}
}

// NewTicker returns a new Ticker.
func (f *FakeClock) NewTicker(d time.Duration) Ticker {
	f.mu.Lock()
	defer f.mu.Unlock()

	ch := make(chan time.Time, 1)
	targetTime := f.time.Add(d)

	w := &waiter{
		targetTime: targetTime,
		destChan:   ch,
		period:     d,
		active:     true,
	}
	f.waiters = append(f.waiters, w)

	return &fakeTicker{
		clock:  f,
		waiter: w,
	}
}

// Step advances the fake clock by the given duration.
func (f *FakeClock) Step(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.setTimeLocked(f.time.Add(d))
}

// SetTime sets the fake clock to the given time.
func (f *FakeClock) SetTime(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.setTimeLocked(t)
}

// HasWaiters returns true if there are any waiters.
func (f *FakeClock) HasWaiters() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, w := range f.waiters {
		if w.active {
			return true
		}
	}
	return false
}

// BlockUntilReady blocks until all pending timer callbacks have completed.
func (f *FakeClock) BlockUntilReady() {
	f.wg.Wait()
}

// setTimeLocked sets the time and triggers any waiters. Caller must hold f.mu.
func (f *FakeClock) setTimeLocked(t time.Time) {
	if t.Before(f.time) {
		panic("cannot move fake clock backwards")
	}

	f.time = t

	// Process waiters
	newWaiters := make([]*waiter, 0, len(f.waiters))
	for _, w := range f.waiters {
		if !w.active {
			continue
		}

		if !w.targetTime.After(t) {
			// Time has passed, trigger the waiter
			if w.destChan != nil {
				select {
				case w.destChan <- t:
				default:
				}
			}

			if w.afterFunc != nil {
				f.wg.Add(1)
				go func() {
					defer f.wg.Done()
					w.afterFunc()
				}()
			}

			// Handle tickers
			if w.period > 0 {
				w.targetTime = w.targetTime.Add(w.period)
				for !w.targetTime.After(t) {
					select {
					case w.destChan <- w.targetTime:
					default:
					}
					w.targetTime = w.targetTime.Add(w.period)
				}
				newWaiters = append(newWaiters, w)
			}
		} else {
			// Not ready yet
			newWaiters = append(newWaiters, w)
		}
	}

	f.waiters = newWaiters
}

// fakeTimer implements Timer.
type fakeTimer struct {
	clock  *FakeClock
	waiter *waiter
}

// Stop prevents the Timer from firing.
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	active := t.waiter.active
	t.waiter.active = false
	return active
}

// Reset changes the timer to expire after duration d.
func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	active := t.waiter.active
	t.waiter.active = true
	t.waiter.targetTime = t.clock.time.Add(d)

	return active
}

// C returns the channel on which the time will be sent.
func (t *fakeTimer) C() <-chan time.Time {
	return t.waiter.destChan
}

// fakeTicker implements Ticker.
type fakeTicker struct {
	clock  *FakeClock
	waiter *waiter
}

// Stop turns off the ticker.
func (t *fakeTicker) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	t.waiter.active = false
}

// C returns the channel on which the ticks are delivered.
func (t *fakeTicker) C() <-chan time.Time {
	return t.waiter.destChan
}
