package streamz

import (
	"context"
	"testing"
	"time"
)

// BenchmarkWindowCollector_StructKeys benchmarks the struct key optimization in WindowCollector.
func BenchmarkWindowCollector_StructKeys(b *testing.B) {
	// Create window metadata
	windowStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(5 * time.Minute)

	meta := WindowMetadata{
		Start: windowStart,
		End:   windowEnd,
		Type:  "tumbling",
		Size:  5 * time.Minute,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		collector := NewWindowCollector[int]()
		input := make(chan Result[int], 100)

		// Add 100 Results with identical metadata
		for j := 0; j < 100; j++ {
			input <- AddWindowMetadata(NewSuccess(j), meta)
		}
		close(input)

		ctx := context.Background()
		collections := collector.Process(ctx, input)

		// Consume all output
		for collection := range collections {
			_ = collection // Just consume the output
		}
	}
}

// BenchmarkWindowCollector_MultipleWindows benchmarks with multiple different windows.
func BenchmarkWindowCollector_MultipleWindows(b *testing.B) {
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		collector := NewWindowCollector[int]()
		input := make(chan Result[int], 300)

		// Create 3 different windows with 100 items each
		for w := 0; w < 3; w++ {
			windowStart := baseTime.Add(time.Duration(w) * 5 * time.Minute)
			windowEnd := windowStart.Add(5 * time.Minute)

			meta := WindowMetadata{
				Start: windowStart,
				End:   windowEnd,
				Type:  "tumbling",
				Size:  5 * time.Minute,
			}

			for j := 0; j < 100; j++ {
				input <- AddWindowMetadata(NewSuccess(w*100+j), meta)
			}
		}
		close(input)

		ctx := context.Background()
		collections := collector.Process(ctx, input)

		// Consume all output - should be 3 windows
		count := 0
		for range collections {
			count++
		}

		if count != 3 {
			b.Fatalf("expected 3 windows, got %d", count)
		}
	}
}
