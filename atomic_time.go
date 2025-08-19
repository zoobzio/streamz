package streamz

import (
	"sync/atomic"
	"time"
)

// AtomicTime provides atomic operations for time.Time values.
// It internally stores time as Unix nanoseconds to avoid type assertions.
type AtomicTime struct {
	nanos atomic.Int64
}

// Store atomically stores a time value.
func (at *AtomicTime) Store(t time.Time) {
	at.nanos.Store(t.UnixNano())
}

// Load atomically loads the time value.
func (at *AtomicTime) Load() time.Time {
	nanos := at.nanos.Load()
	if nanos == 0 {
		return time.Time{} // Return zero time if not set.
	}
	return time.Unix(0, nanos)
}

// IsZero returns true if the time is zero.
func (at *AtomicTime) IsZero() bool {
	return at.nanos.Load() == 0
}
