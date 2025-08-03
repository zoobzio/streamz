package streamz

import (
	"time"
)

// Window represents a time-bounded collection of items.
// It's used by windowing processors to group items for aggregation.
type Window[T any] struct {
	Start time.Time
	End   time.Time
	Items []T
}
