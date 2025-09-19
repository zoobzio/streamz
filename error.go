package streamz

import (
	"fmt"
	"time"
)

// StreamError represents an error that occurred during stream processing.
// It captures both the item that caused the error and the error itself,
// enabling better debugging and error handling strategies.
//
//nolint:govet // fieldalignment: struct layout optimized for readability over memory
type StreamError[T any] struct {
	// Item is the original item that caused the processing error.
	Item T

	// Err is the underlying error that occurred during processing.
	Err error

	// ProcessorName identifies which processor generated the error.
	ProcessorName string

	// Timestamp records when the error occurred.
	Timestamp time.Time
}

// NewStreamError creates a new StreamError with the current timestamp.
func NewStreamError[T any](item T, err error, processorName string) *StreamError[T] {
	return &StreamError[T]{
		Item:          item,
		Err:           err,
		ProcessorName: processorName,
		Timestamp:     time.Now(),
	}
}

// String returns a human-readable representation of the error.
func (se *StreamError[T]) String() string {
	return fmt.Sprintf("StreamError[%s]: %v (item: %v, time: %s)",
		se.ProcessorName, se.Err, se.Item, se.Timestamp.Format(time.RFC3339))
}

// Unwrap returns the underlying error, enabling error wrapping chains.
func (se *StreamError[T]) Unwrap() error {
	return se.Err
}

// Error implements the error interface.
func (se *StreamError[T]) Error() string {
	return se.String()
}
