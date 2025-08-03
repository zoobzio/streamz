package streamz

import (
	"context"
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
