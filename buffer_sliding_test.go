package streamz

import (
	"context"
	"testing"
	"time"
)

func TestSlidingBuffer(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	buffer := NewSlidingBuffer[int](2)
	out := buffer.Process(ctx, in)

	go func() {
		for i := 0; i < 5; i++ {
			in <- i
		}
		close(in)
	}()

	time.Sleep(50 * time.Millisecond)

	received := []int{}
	for v := range out {
		received = append(received, v)
	}

	if len(received) < 2 || len(received) > 5 {
		t.Errorf("expected 2-5 items, got %d", len(received))
	}

	if received[len(received)-1] != 4 {
		t.Errorf("expected last item to be 4, got %d", received[len(received)-1])
	}
}
