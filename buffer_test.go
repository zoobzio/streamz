package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestBuffer(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	buffer := NewBuffer[int](5)
	out := buffer.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	count := 0
	for range out {
		count++
	}

	if count != 10 {
		t.Errorf("expected 10 items, got %d", count)
	}
}

func TestBufferBackpressure(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	buffer := NewBuffer[int](3)
	out := buffer.Process(ctx, in)

	sent := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			in <- i
		}
		close(sent)
		close(in)
	}()

	time.Sleep(10 * time.Millisecond)

	select {
	case <-sent:
		t.Error("should not have sent all items yet due to backpressure")
	default:
	}

	for i := 0; i < 5; i++ {
		<-out
	}
}

// Example demonstrates using a buffer to handle backpressure in a pipeline.
func ExampleBuffer() {
	ctx := context.Background()

	// Create a buffer to handle bursts of events.
	buffer := NewBuffer[string](100)

	// Simulate burst of incoming events.
	events := make(chan string)
	go func() {
		// Burst of events.
		for i := 0; i < 5; i++ {
			events <- fmt.Sprintf("event-%d", i)
		}
		close(events)
	}()

	// Process events with buffer handling backpressure.
	buffered := buffer.Process(ctx, events)

	// Slow consumer.
	for event := range buffered {
		fmt.Printf("Processing: %s\n", event)
		// In real scenario, this might be slow I/O operation.
	}

	// Output:
	// Processing: event-0
	// Processing: event-1
	// Processing: event-2
	// Processing: event-3
	// Processing: event-4
}
