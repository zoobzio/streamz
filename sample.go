package streamz

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
)

// Sample randomly selects items from a stream based on a probability rate.
// It keeps successful items based on the configured rate (0.0 to 1.0) and always
// passes through errors unchanged.
//
// Sample is used for:
//   - Load shedding in high-volume streams
//   - Creating statistical samples for monitoring
//   - Random downsampling for performance optimization
//   - A/B testing traffic distribution
//
// The sampling decision is made independently for each item using
// cryptographically secure randomness. Items are either kept completely
// or dropped completely - no modification occurs.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Sample[T any] struct {
	name string
	rate float64
}

// NewSample creates a processor that randomly selects items based on probability.
// The rate parameter determines the probability (0.0 to 1.0) that each successful
// item will be kept in the stream. A rate of 0.0 drops all items, 1.0 keeps all items.
//
// Error items are always passed through unchanged regardless of the rate.
//
// When to use:
//   - High-volume streams needing load reduction
//   - Statistical sampling for monitoring/analytics
//   - Performance optimization through data reduction
//   - Random traffic splitting for testing
//   - Memory pressure relief in processing pipelines
//
// Example:
//
//	// Keep 10% of successful orders for monitoring
//	monitor := streamz.NewSample[Order](0.1)
//
//	// Half of metrics for storage optimization
//	storage := streamz.NewSample[Metric](0.5)
//
//	// Load testing with 1% of production traffic
//	loadTest := streamz.NewSample[Request](0.01).WithName("load-test-sample")
//
//	// A/B testing - 50/50 split
//	groupA := streamz.NewSample[User](0.5)
//
//	results := monitor.Process(ctx, input)
//	for result := range results {
//		// Approximately 10% of successful items, all errors
//		processMonitoringData(result)
//	}
//
// Parameters:
//   - rate: Probability (0.0-1.0) that successful items will be kept
//
// Returns a new Sample processor.
// Panics if rate is outside the valid range [0.0, 1.0].
func NewSample[T any](rate float64) *Sample[T] {
	if rate < 0.0 || rate > 1.0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		panic("sample rate must be between 0.0 and 1.0")
	}

	return &Sample[T]{
		name: "sample",
		rate: rate,
	}
}

// WithName sets a custom name for this processor.
// If not set, defaults to "sample".
// The name is used for debugging, monitoring, and error reporting.
func (s *Sample[T]) WithName(name string) *Sample[T] {
	s.name = name
	return s
}

// Process randomly selects successful items based on the configured rate.
// Each successful item has an independent probability of being kept.
// Error items are always passed through unchanged.
//
// The selection uses crypto/rand for secure randomness.
func (s *Sample[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		for item := range in {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Always pass through errors
			if item.IsError() {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
				continue
			}

			// Sample successful items based on rate using crypto/rand
			if cryptoFloat64() < s.rate {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
			// Items not selected are dropped (no output)
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (s *Sample[T]) Name() string {
	return s.name
}

// Rate returns the current sampling rate.
func (s *Sample[T]) Rate() float64 {
	return s.rate
}

// cryptoFloat64 returns a cryptographically secure random float64 in [0.0, 1.0).
func cryptoFloat64() float64 {
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		// On crypto/rand failure (extremely rare), fail open by keeping item
		return 0.0
	}
	// Convert to uint64 and normalize to [0.0, 1.0)
	n := binary.LittleEndian.Uint64(buf[:])
	return float64(n) / float64(1<<64)
}
