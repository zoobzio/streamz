package streamz

import (
	"context"
	"testing"
)

func TestChunk(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	chunk := NewChunk[int](3)
	out := chunk.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	chunks := [][]int{}
	for chunk := range out {
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(chunks))
	}

	expectedSizes := []int{3, 3, 3, 1}
	for i, chunk := range chunks {
		if len(chunk) != expectedSizes[i] {
			t.Errorf("expected chunk %d to have size %d, got %d", i, expectedSizes[i], len(chunk))
		}
	}

	if chunks[3][0] != 9 {
		t.Errorf("expected last chunk to contain 9, got %d", chunks[3][0])
	}
}

func TestChunkExact(t *testing.T) {
	ctx := context.Background()
	in := make(chan int)

	chunk := NewChunk[int](5)
	out := chunk.Process(ctx, in)

	go func() {
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)
	}()

	chunks := [][]int{}
	for chunk := range out {
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}

	for _, chunk := range chunks {
		if len(chunk) != 5 {
			t.Errorf("expected all chunks to be size 5, got %d", len(chunk))
		}
	}
}
