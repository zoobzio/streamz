package integration

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/streamz"
)

// Test Result with metadata field - proving viability.
type ResultWithMetadata[T any] struct {
	value    T
	err      *streamz.StreamError[T] //nolint:unused // field for future error support in prototype
	metadata map[string]interface{}
}

func NewSuccessWithMetadata[T any](value T) ResultWithMetadata[T] {
	return ResultWithMetadata[T]{value: value, metadata: make(map[string]interface{})}
}

func (r ResultWithMetadata[T]) WithMetadata(key string, value interface{}) ResultWithMetadata[T] {
	if r.metadata == nil {
		r.metadata = make(map[string]interface{})
	}
	r.metadata[key] = value
	return r
}

func (r ResultWithMetadata[T]) GetMetadata(key string) (interface{}, bool) {
	if r.metadata == nil {
		return nil, false
	}
	val, ok := r.metadata[key]
	return val, ok
}

func (r ResultWithMetadata[T]) Value() T {
	return r.value
}

// TestMetadataFlowThroughChannels proves metadata survives channel transmission.
func TestMetadataFlowThroughChannels(t *testing.T) {
	// Create channel for augmented results
	ch := make(chan ResultWithMetadata[int], 10)

	// Send result with metadata
	result := NewSuccessWithMetadata(42)
	result = result.WithMetadata("route", "high-priority")
	result = result.WithMetadata("timestamp", time.Now())
	result = result.WithMetadata("window_start", time.Now().Add(-time.Minute))
	result = result.WithMetadata("window_end", time.Now())

	ch <- result
	close(ch)

	// Receive and verify metadata intact
	received := <-ch

	route, hasRoute := received.GetMetadata("route")
	if !hasRoute || route != "high-priority" {
		t.Errorf("Route metadata lost: %v", route)
	}

	_, hasTimestamp := received.GetMetadata("timestamp")
	if !hasTimestamp {
		t.Error("Timestamp metadata lost")
	}

	if received.Value() != 42 {
		t.Errorf("Value corrupted: %d", received.Value())
	}
}

// TestWindowMetadataAttachment simulates window processor attaching metadata.
func TestWindowMetadataAttachment(t *testing.T) {
	input := make(chan ResultWithMetadata[int], 10)
	output := make(chan ResultWithMetadata[[]int], 10)

	// Simulate window processor
	go func() {
		defer close(output)

		windowStart := time.Now()
		var batch []int

		for result := range input {
			batch = append(batch, result.Value())
		}

		// Create windowed result with metadata instead of Window[T]
		windowedResult := NewSuccessWithMetadata(batch)
		windowedResult = windowedResult.WithMetadata("window_start", windowStart)
		windowedResult = windowedResult.WithMetadata("window_end", time.Now())
		windowedResult = windowedResult.WithMetadata("count", len(batch))

		output <- windowedResult
	}()

	// Send values
	for i := 0; i < 5; i++ {
		input <- NewSuccessWithMetadata(i)
	}
	close(input)

	// Verify windowed result has metadata
	result := <-output

	start, hasStart := result.GetMetadata("window_start")
	end, hasEnd := result.GetMetadata("window_end")
	count, hasCount := result.GetMetadata("count")

	if !hasStart || !hasEnd || !hasCount {
		t.Error("Window metadata missing")
	}

	if count != 5 {
		t.Errorf("Wrong count: %v", count)
	}

	// Check window duration makes sense
	startTime, ok := start.(time.Time)
	if !ok {
		t.Error("start is not a time.Time")
		return
	}
	endTime, ok := end.(time.Time)
	if !ok {
		t.Error("end is not a time.Time")
		return
	}
	if endTime.Before(startTime) {
		t.Error("Invalid window times")
	}
}

// TestRouteMetadataPassthrough simulates route processor adding metadata.
func TestRouteMetadataPassthrough(t *testing.T) {
	input := make(chan ResultWithMetadata[string], 10)
	highPriority := make(chan ResultWithMetadata[string], 10)
	lowPriority := make(chan ResultWithMetadata[string], 10)

	// Simulate route processor that adds route metadata
	go func() {
		defer close(highPriority)
		defer close(lowPriority)

		for result := range input {
			value := result.Value()

			if value == "urgent" {
				// Add route metadata before sending
				result = result.WithMetadata("route", "high-priority")
				highPriority <- result
			} else {
				result = result.WithMetadata("route", "low-priority")
				lowPriority <- result
			}
		}
	}()

	// Send test data
	input <- NewSuccessWithMetadata("urgent")
	input <- NewSuccessWithMetadata("normal")
	close(input)

	// Check high priority
	high := <-highPriority
	route, _ := high.GetMetadata("route")
	if route != "high-priority" {
		t.Errorf("Wrong route: %v", route)
	}

	// Check low priority
	low := <-lowPriority
	route, _ = low.GetMetadata("route")
	if route != "low-priority" {
		t.Errorf("Wrong route: %v", route)
	}
}

// TestMetadataIgnoredByUnaware proves processors can ignore metadata they don't need.
func TestMetadataIgnoredByUnaware(t *testing.T) {
	input := make(chan ResultWithMetadata[int], 10)
	output := make(chan ResultWithMetadata[int], 10)

	// Processor that doesn't care about metadata
	go func() {
		defer close(output)
		for result := range input {
			// Just double the value, ignore metadata
			doubled := result.Value() * 2
			output <- NewSuccessWithMetadata(doubled)
		}
	}()

	// Send result with metadata
	result := NewSuccessWithMetadata(10)
	result = result.WithMetadata("irrelevant", "data")
	input <- result
	close(input)

	// Processor worked despite not understanding metadata
	processed := <-output
	if processed.Value() != 20 {
		t.Errorf("Processing failed: %d", processed.Value())
	}
}

// TestMetadataPreservingProcessor shows processor that preserves metadata.
func TestMetadataPreservingProcessor(t *testing.T) {
	input := make(chan ResultWithMetadata[int], 10)
	output := make(chan ResultWithMetadata[int], 10)

	// Processor that preserves metadata while transforming value
	go func() {
		defer close(output)
		for result := range input {
			// Transform value but preserve metadata
			doubled := result.Value() * 2
			newResult := NewSuccessWithMetadata(doubled)

			// Copy all metadata
			if result.metadata != nil {
				for k, v := range result.metadata {
					newResult = newResult.WithMetadata(k, v)
				}
			}

			output <- newResult
		}
	}()

	// Send result with metadata
	result := NewSuccessWithMetadata(10)
	result = result.WithMetadata("route", "test")
	result = result.WithMetadata("timestamp", time.Now())
	input <- result
	close(input)

	// Check metadata preserved
	processed := <-output
	if processed.Value() != 20 {
		t.Errorf("Value wrong: %d", processed.Value())
	}

	route, hasRoute := processed.GetMetadata("route")
	if !hasRoute || route != "test" {
		t.Error("Metadata lost during processing")
	}
}

// TestMetadataComposability shows processors can be chained without wrapper types.
func TestMetadataComposability(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Just proving we could use real processors

	// All channels same type - no wrapper types needed
	source := make(chan ResultWithMetadata[int], 10)
	routed := make(chan ResultWithMetadata[int], 10)
	windowed := make(chan ResultWithMetadata[[]int], 10)

	// Route processor adds route metadata
	go func() {
		defer close(routed)
		for result := range source {
			result = result.WithMetadata("route", "main")
			routed <- result
		}
	}()

	// Window processor reads route, adds window metadata
	go func() {
		defer close(windowed)
		var batch []int
		windowStart := time.Now()

		for result := range routed {
			// Can read route if needed
			route, _ := result.GetMetadata("route")
			_ = route // Could use for window logic

			batch = append(batch, result.Value())
		}

		// Return batch with window metadata
		windowResult := NewSuccessWithMetadata(batch)
		windowResult = windowResult.WithMetadata("window_start", windowStart)
		windowResult = windowResult.WithMetadata("window_end", time.Now())
		windowed <- windowResult
	}()

	// Send test data
	for i := 0; i < 3; i++ {
		source <- NewSuccessWithMetadata(i)
	}
	close(source)

	// Verify composition worked
	result := <-windowed
	if len(result.Value()) != 3 {
		t.Errorf("Wrong batch size: %d", len(result.Value()))
	}

	_, hasStart := result.GetMetadata("window_start")
	if !hasStart {
		t.Error("Window metadata missing")
	}
}
