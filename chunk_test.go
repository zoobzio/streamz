package streamz

import (
	"context"
	"fmt"
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

// Example demonstrates splitting a stream into fixed-size chunks.
func ExampleChunk() {
	ctx := context.Background()

	// Create a chunker that groups items into chunks of 3.
	chunker := NewChunk[string](3)

	// Simulate a stream of log entries.
	logs := make(chan string, 8)
	logs <- "2024-01-18 10:00:00 INFO Starting application"
	logs <- "2024-01-18 10:00:01 DEBUG Loading configuration"
	logs <- "2024-01-18 10:00:02 INFO Configuration loaded"
	logs <- "2024-01-18 10:00:03 INFO Connecting to database"
	logs <- "2024-01-18 10:00:04 DEBUG Database connection established"
	logs <- "2024-01-18 10:00:05 INFO Starting web server"
	logs <- "2024-01-18 10:00:06 INFO Web server started on port 8080"
	logs <- "2024-01-18 10:00:07 INFO Ready to accept requests"
	close(logs)

	// Process logs in chunks for batch operations.
	chunks := chunker.Process(ctx, logs)

	// Process each chunk (e.g., batch insert to database).
	chunkNum := 1
	for chunk := range chunks {
		fmt.Printf("Chunk %d (%d logs):\n", chunkNum, len(chunk))
		for i, log := range chunk {
			fmt.Printf("  [%d] %s\n", i+1, log)
		}
		chunkNum++
	}

	// Output:
	// Chunk 1 (3 logs):
	//   [1] 2024-01-18 10:00:00 INFO Starting application
	//   [2] 2024-01-18 10:00:01 DEBUG Loading configuration
	//   [3] 2024-01-18 10:00:02 INFO Configuration loaded
	// Chunk 2 (3 logs):
	//   [1] 2024-01-18 10:00:03 INFO Connecting to database
	//   [2] 2024-01-18 10:00:04 DEBUG Database connection established
	//   [3] 2024-01-18 10:00:05 INFO Starting web server
	// Chunk 3 (2 logs):
	//   [1] 2024-01-18 10:00:06 INFO Web server started on port 8080
	//   [2] 2024-01-18 10:00:07 INFO Ready to accept requests
}
