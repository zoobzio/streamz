package benchmarks

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/clockz"

	streamz "github.com/zoobzio/streamz"
)

// BenchmarkPipeline_FanInFanOut benchmarks a FanIn->FanOut pipeline.
func BenchmarkPipeline_FanInFanOut(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Create inputs
		input1 := make(chan streamz.Result[int], 100)
		input2 := make(chan streamz.Result[int], 100)
		for j := 0; j < 100; j++ {
			input1 <- streamz.NewSuccess(j)
			input2 <- streamz.NewSuccess(j + 100)
		}
		close(input1)
		close(input2)

		// Build pipeline
		fanin := streamz.NewFanIn[int]()
		merged := fanin.Process(ctx, input1, input2)

		fanout := streamz.NewFanOut[int](2)
		outputs := fanout.Process(ctx, merged)

		b.StartTimer()

		// Consume all outputs
		done := make(chan struct{})
		go func() {
			for range outputs[0] {
			}
			done <- struct{}{}
		}()
		go func() {
			for range outputs[1] {
			}
			done <- struct{}{}
		}()
		<-done
		<-done
	}
}

// BenchmarkPipeline_FilterMapBatch benchmarks a Filter->Mapper->Batcher pipeline.
func BenchmarkPipeline_FilterMapBatch(b *testing.B) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		input := make(chan streamz.Result[int], 1000)
		for j := 0; j < 1000; j++ {
			input <- streamz.NewSuccess(j)
		}
		close(input)

		// Build pipeline
		filter := streamz.NewFilter(func(n int) bool { return n%2 == 0 })
		mapper := streamz.NewMapper(func(_ context.Context, n int) (int, error) { return n * 2, nil })
		batcher := streamz.NewBatcher[int](streamz.BatchConfig{
			MaxSize:    50,
			MaxLatency: time.Hour,
		}, clock)

		filtered := filter.Process(ctx, input)
		mapped := mapper.Process(ctx, filtered)
		batched := batcher.Process(ctx, mapped)

		b.StartTimer()

		// Consume output
		for range batched {
		}
	}
}

// BenchmarkPipeline_Throughput measures raw throughput of a simple pipeline.
func BenchmarkPipeline_Throughput(b *testing.B) {
	ctx := context.Background()

	input := make(chan streamz.Result[int], b.N)
	for i := 0; i < b.N; i++ {
		input <- streamz.NewSuccess(i)
	}
	close(input)

	mapper := streamz.NewMapper(func(_ context.Context, n int) (int, error) { return n + 1, nil })
	output := mapper.Process(ctx, input)

	b.ResetTimer()

	for range output {
	}
}

// BenchmarkPipeline_ErrorHandling benchmarks error passthrough performance.
func BenchmarkPipeline_ErrorHandling(b *testing.B) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		input := make(chan streamz.Result[int], 100)
		for j := 0; j < 100; j++ {
			if j%10 == 0 {
				input <- streamz.NewError(j, nil, "test")
			} else {
				input <- streamz.NewSuccess(j)
			}
		}
		close(input)

		batcher := streamz.NewBatcher[int](streamz.BatchConfig{
			MaxSize:    10,
			MaxLatency: time.Hour,
		}, clock)
		output := batcher.Process(ctx, input)

		b.StartTimer()

		for range output {
		}
	}
}
