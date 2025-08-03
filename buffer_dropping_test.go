package streamz

import (
	"context"
	"testing"
	"time"
)

func TestDroppingBuffer(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	dropped := []int{}
	buffer := NewDroppingBuffer(2, func(i int) {
		dropped = append(dropped, i)
	})
	out := buffer.Process(ctx, in)

	for i := 0; i < 5; i++ {
		in <- i
	}
	close(in)

	time.Sleep(50 * time.Millisecond)

	received := []int{}
	for v := range out {
		received = append(received, v)
	}

	if len(received)+len(dropped) != 5 {
		t.Errorf("total items should be 5, got %d received + %d dropped",
			len(received), len(dropped))
	}
}
