# Benchmarks

Performance benchmarks for streamz processors.

## Running Benchmarks

Run all benchmarks:
```bash
make test-bench
```

Or directly:
```bash
go test -bench=. -benchmem ./...
```

Run specific processor benchmarks:
```bash
go test -bench=BenchmarkMapper -benchmem ./...
go test -bench=BenchmarkBatcher -benchmem ./...
```

## Benchmark Organisation

Benchmarks are co-located with their respective processor test files:

| Processor | File | Benchmarks |
|-----------|------|------------|
| Mapper | `mapper_test.go` | `BenchmarkMapper` |
| Filter | `filter_test.go` | `BenchmarkFilter`, `BenchmarkFilterFiltered` |
| AsyncMapper | `async_mapper_test.go` | `BenchmarkAsyncMapper_Ordered`, `BenchmarkAsyncMapper_Unordered` |
| Batcher | `batcher_test.go` | `BenchmarkBatcher_SizeBasedBatching`, `BenchmarkBatcher_TimeBasedBatching`, `BenchmarkBatcher_ErrorPassthrough`, `BenchmarkBatcher_MixedItems` |
| FanOut | `fanout_test.go` | `BenchmarkFanOut_SingleItem`, `BenchmarkFanOut_MultipleItems` |
| Partition | `partition_test.go` | `BenchmarkHashPartition_Route`, `BenchmarkRoundRobinPartition_Route` |
| Throttle | `throttle_test.go` | `BenchmarkThrottle_TimestampSuccess`, `BenchmarkThrottle_TimestampErrors` |
| Debounce | `debounce_test.go` | `BenchmarkDebounce_Success`, `BenchmarkDebounce_Errors`, `BenchmarkDebounce_RapidItems` |
| Result | `result_test.go` | `BenchmarkMetadata_Operations` |

## Performance Guidelines

When adding or modifying processors:

1. **Include benchmarks** for new processor implementations
2. **Test throughput** under varying item counts
3. **Measure allocation** using `-benchmem`
4. **Compare before/after** for performance-related changes

## Example Benchmark

```go
func BenchmarkMyProcessor(b *testing.B) {
    ctx := context.Background()
    input := make(chan streamz.Result[int], b.N)

    for i := 0; i < b.N; i++ {
        input <- streamz.Success(i)
    }
    close(input)

    processor := streamz.NewMyProcessor[int]()
    output := processor.Process(ctx, input)

    b.ResetTimer()

    for range output {
        // Consume output
    }
}
```

## Interpreting Results

```
BenchmarkMapper-8    1000000    1042 ns/op    256 B/op    3 allocs/op
```

- `BenchmarkMapper-8`: Benchmark name, 8 CPU cores
- `1000000`: Number of iterations
- `1042 ns/op`: Nanoseconds per operation
- `256 B/op`: Bytes allocated per operation
- `3 allocs/op`: Heap allocations per operation

Lower values indicate better performance. Focus on:
- **ns/op**: Latency per item
- **allocs/op**: Memory pressure (affects GC)
