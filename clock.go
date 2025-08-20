// Package streamz provides streaming data processing capabilities,
// including time-related operations for deterministic testing.
package streamz

import "time"

// Clock provides an interface for time operations, enabling
// both real and mock implementations for testing.
type Clock interface {
	// Now returns the current time.
	Now() time.Time

	// After waits for the duration to elapse and then sends the current time
	// on the returned channel.
	After(d time.Duration) <-chan time.Time

	// AfterFunc waits for the duration to elapse and then executes f
	// in its own goroutine. It returns a Timer that can be used to
	// cancel the call using its Stop method.
	AfterFunc(d time.Duration, f func()) Timer

	// NewTimer creates a new Timer that will send the current time
	// on its channel after at least duration d.
	NewTimer(d time.Duration) Timer

	// NewTicker returns a new Ticker containing a channel that will
	// send the time with a period specified by the duration argument.
	NewTicker(d time.Duration) Ticker
}

// Timer represents a single event timer.
type Timer interface {
	// Stop prevents the Timer from firing.
	// It returns true if the call stops the timer, false if the timer
	// has already expired or been stopped.
	Stop() bool

	// Reset changes the timer to expire after duration d.
	// It returns true if the timer had been active, false if
	// the timer had expired or been stopped.
	Reset(d time.Duration) bool

	// C returns the channel on which the time will be sent.
	C() <-chan time.Time
}

// Ticker holds a channel that delivers ticks of a clock at intervals.
type Ticker interface {
	// Stop turns off a ticker. After Stop, no more ticks will be sent.
	Stop()

	// C returns the channel on which the ticks are delivered.
	C() <-chan time.Time
}

// RealClock is the default Clock implementation using the standard time package.
var RealClock Clock = &realClock{}

// realClock implements Clock using the standard time package.
type realClock struct{}
