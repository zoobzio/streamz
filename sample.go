package streamz

import (
	"context"
	"crypto/rand"
	"encoding/binary"
)

// Sample randomly selects items from a stream based on a sampling rate.
// It uses cryptographically secure randomness to ensure unbiased sampling,
// making it suitable for statistical sampling and data reduction.
type Sample[T any] struct {
	name string
	rate float64
}

// NewSample creates a processor that randomly samples items at the specified rate.
// Each item has an independent probability of being selected, ensuring statistical validity.
//
// When to use:
//   - Reducing data volume for analysis or monitoring
//   - Statistical sampling for quality control
//   - Load testing with a percentage of traffic
//   - Creating data subsets for development/testing
//   - Implementing trace sampling in observability
//
// Example:
//
//	// Sample 10% of events for analysis
//	sampler := streamz.NewSample[Event](0.1)
//
//	sampled := sampler.Process(ctx, events)
//	for event := range sampled {
//		// Approximately 10% of events will be processed
//		analyzeEvent(event)
//	}
//
//	// Trace sampling for observability
//	traceSampler := streamz.NewSample[Request](0.01) // 1% sampling
//	sampled := traceSampler.Process(ctx, requests)
//	for req := range sampled {
//		// Detailed tracing for 1% of requests
//		trace.Enable(req)
//		processRequest(req)
//	}
//
// Parameters:
//   - rate: Sampling rate between 0.0 and 1.0 (0.1 = 10%, 1.0 = 100%)
//
// Returns a new Sample processor.
func NewSample[T any](rate float64) *Sample[T] {
	return &Sample[T]{
		rate: rate,
		name: "sample",
	}
}

func (s *Sample[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		for item := range in {
			if s.shouldSample() {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

func (s *Sample[T]) shouldSample() bool {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		// On error, default to not sampling
		return false
	}

	// Convert to float64 in range [0, 1)
	val := binary.BigEndian.Uint64(b[:]) >> 11 // Use 53 bits for mantissa
	return float64(val)/(1<<53) < s.rate
}

func (s *Sample[T]) Name() string {
	return s.name
}
