package streamz

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// DeadLetterQueue separates successful results from failed results into two distinct channels.
// Unlike standard processors that return a single Result[T] channel, DLQ returns two channels:
// one for successes and one for failures. This enables different downstream processing
// strategies for successful vs failed items.
//
// Non-Consumed Channel Handling:
// If either output channel is not consumed, DLQ will drop items that cannot be sent
// to prevent deadlocks. Dropped items are logged and counted for monitoring.
//
// Concurrent Behavior:
// DeadLetterQueue is safe for concurrent use. Multiple goroutines can consume from
// both output channels simultaneously. The internal distribution logic runs in a
// single goroutine to prevent race conditions.
//
// Usage Examples:
//
//	// Separate successes and failures for different handling
//	dlq := streamz.NewDeadLetterQueue[Order](streamz.RealClock)
//	successes, failures := dlq.Process(ctx, orders)
//
//	// Process successes in main path
//	go func() {
//		for success := range successes {
//			processOrder(success.Value())
//		}
//	}()
//
//	// Handle failures separately (logging, metrics, retry queue)
//	go func() {
//		for failure := range failures {
//			log.Printf("Order processing failed: %v", failure.Error())
//			retryQueue.Send(failure)
//		}
//	}()
//
//	// Or ignore failures if only successes matter
//	successes, _ := dlq.Process(ctx, orders)
//	// failures channel ignored - items will be dropped and logged
type DeadLetterQueue[T any] struct {
	clock        Clock         // 8 bytes (pointer)
	name         string        // 16 bytes (pointer + len)
	droppedCount atomic.Uint64 // 8 bytes
}

// NewDeadLetterQueue creates a new DeadLetterQueue processor.
// Uses the provided clock for timeout operations - use RealClock for production,
// fake clock for deterministic testing.
func NewDeadLetterQueue[T any](clock Clock) *DeadLetterQueue[T] {
	return &DeadLetterQueue[T]{
		name:  "dlq",
		clock: clock,
	}
}

// WithName sets a custom name for the DeadLetterQueue (for logging and monitoring).
func (dlq *DeadLetterQueue[T]) WithName(name string) *DeadLetterQueue[T] {
	dlq.name = name
	return dlq
}

// Name returns the processor name.
func (dlq *DeadLetterQueue[T]) Name() string {
	return dlq.name
}

// DroppedCount returns the number of items dropped due to non-consumed channels.
func (dlq *DeadLetterQueue[T]) DroppedCount() uint64 {
	return dlq.droppedCount.Load()
}

// Process separates the input stream into success and failure channels.
// Returns two channels: (successes, failures).
//
// The distribution logic runs in a single goroutine to prevent race conditions.
// Both output channels are closed when the input channel closes or context is canceled.
//
// If either output channel cannot accept an item (blocked consumer), the item is
// dropped and logged to prevent deadlocks. This is particularly important when
// only one of the two channels is consumed.
func (dlq *DeadLetterQueue[T]) Process(ctx context.Context, in <-chan Result[T]) (success <-chan Result[T], failure <-chan Result[T]) {
	successCh := make(chan Result[T])
	failureCh := make(chan Result[T])

	go dlq.distribute(ctx, in, successCh, failureCh)

	return successCh, failureCh
}

// distribute handles the core logic of routing items to appropriate channels.
// Runs in a single goroutine to ensure race-free operation.
func (dlq *DeadLetterQueue[T]) distribute(ctx context.Context, in <-chan Result[T], successCh, failureCh chan Result[T]) {
	defer close(successCh)
	defer close(failureCh)

	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-in:
			if !ok {
				return
			}

			if result.IsError() {
				dlq.sendToFailures(ctx, result, failureCh)
			} else {
				dlq.sendToSuccesses(ctx, result, successCh)
			}
		}
	}
}

// sendToSuccesses attempts to send a success result to the success channel.
// If the channel is blocked or context is canceled, drops the item and logs the event.
func (dlq *DeadLetterQueue[T]) sendToSuccesses(ctx context.Context, result Result[T], successCh chan Result[T]) {
	select {
	case successCh <- result:
		// Sent successfully
	case <-ctx.Done():
		// Context canceled, exit gracefully
		return
	case <-dlq.clock.After(10 * time.Millisecond): // Small timeout for sustained blocking
		// Channel blocked for too long - drop and log
		dlq.handleDroppedItem(result, "success")
	}
}

// sendToFailures attempts to send a failure result to the failure channel.
// If the channel is blocked or context is canceled, drops the item and logs the event.
func (dlq *DeadLetterQueue[T]) sendToFailures(ctx context.Context, result Result[T], failureCh chan Result[T]) {
	select {
	case failureCh <- result:
		// Sent successfully
	case <-ctx.Done():
		// Context canceled, exit gracefully
		return
	case <-dlq.clock.After(10 * time.Millisecond): // Small timeout for sustained blocking
		// Channel blocked for too long - drop and log
		dlq.handleDroppedItem(result, "failure")
	}
}

// handleDroppedItem logs dropped items and increments the counter.
// This prevents deadlocks when consumers don't read from both channels.
func (dlq *DeadLetterQueue[T]) handleDroppedItem(result Result[T], channelType string) {
	dlq.droppedCount.Add(1)

	if result.IsError() {
		log.Printf("DLQ[%s]: Dropped item from %s channel - %v", dlq.name, channelType, result.Error())
	} else {
		log.Printf("DLQ[%s]: Dropped item from %s channel - value: %+v", dlq.name, channelType, result.Value())
	}
}
