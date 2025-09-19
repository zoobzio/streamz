// Package streamz provides streaming data processing capabilities,
// including time-related operations for deterministic testing.
package streamz

import "github.com/zoobzio/clockz"

// Clock provides time operations for deterministic testing.
type Clock = clockz.Clock

// Timer represents a single event timer.
type Timer = clockz.Timer

// Ticker delivers ticks at intervals.
type Ticker = clockz.Ticker

// RealClock is the default Clock using standard time.
var RealClock Clock = clockz.RealClock
