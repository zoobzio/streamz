# Testing Stream Processing Pipelines

Learn how to effectively test streamz processors and pipelines to ensure reliability and correctness.

> 📖 **Note:** For comprehensive testing documentation including our testing architecture, quality standards, and contribution guidelines, see [TESTING.md](../../TESTING.md) in the project root.

## Testing Philosophy

Stream processing systems have unique testing challenges:

1. **Asynchronous behavior** - channels and goroutines
2. **Timing dependencies** - batching, windowing, debouncing
3. **Concurrency** - race conditions and ordering
4. **Error scenarios** - failure handling and recovery
5. **Performance characteristics** - throughput and latency

streamz processors are designed to be **easily testable** with clear inputs and outputs.

## Unit Testing Processors

### Basic Processor Testing

Test individual processors in isolation:

```go
func TestFilter(t *testing.T) {
    ctx := context.Background()
    
    // Create processor
    isEven := streamz.NewFilter(func(n int) bool {
        return n%2 == 0
    }).WithName("even")
    
    // Create test channels
    input := make(chan int)
    output := isEven.Process(ctx, input)
    
    // Send test data
    go func() {
        defer close(input)
        for i := 1; i <= 6; i++ {
            input <- i
        }
    }()
    
    // Collect results
    var results []int
    for result := range output {
        results = append(results, result)
    }
    
    // Verify expectations
    expected := []int{2, 4, 6}
    assert.Equal(t, expected, results)
}
```

### Testing with Helper Functions

Create reusable test utilities:

```go
// Helper function to collect all items from a channel
func collectAll[T any](ch <-chan T) []T {
    var results []T
    for item := range ch {
        results = append(results, item)
    }
    return results
}

// Helper function to send slice data to channel
func sendData[T any](ctx context.Context, data []T) <-chan T {
    ch := make(chan T)
    go func() {
        defer close(ch)
        for _, item := range data {
            select {
            case ch <- item:
            case <-ctx.Done():
                return
            }
        }
    }()
    return ch
}

// Helper function to collect with timeout
func collectWithTimeout[T any](ch <-chan T, timeout time.Duration) ([]T, error) {
    var results []T
    timer := time.NewTimer(timeout)
    defer timer.Stop()
    
    for {
        select {
        case item, ok := <-ch:
            if !ok {
                return results, nil
            }
            results = append(results, item)
        case <-timer.C:
            return results, fmt.Errorf("timeout after %v", timeout)
        }
    }
}

// Simplified test using helpers
func TestMapperWithHelpers(t *testing.T) {
    ctx := context.Background()
    
    mapper := streamz.NewMapper(func(n int) int {
        return n * 2
    }).WithName("double")
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5})
    output := mapper.Process(ctx, input)
    results := collectAll(output)
    
    expected := []int{2, 4, 6, 8, 10}
    assert.Equal(t, expected, results)
}
```

### Testing Error Handling

Test how processors handle errors:

```go
func TestAsyncMapperErrorHandling(t *testing.T) {
    ctx := context.Background()
    
    // Processor that fails on specific input
    processor := streamz.NewAsyncMapper(func(ctx context.Context, n int) (int, error) {
        if n == 3 {
            return 0, errors.New("test error")
        }
        return n * 2, nil
    }).WithWorkers(2)
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5})
    output := processor.Process(ctx, input)
    results := collectAll(output)
    
    // Error items should be skipped
    expected := []int{2, 4, 8, 10} // 3 is missing due to error
    assert.Equal(t, expected, results)
}
```

## Testing Timing-Dependent Processors

### Batching Tests

Test processors that depend on time or size:

```go
func TestBatcherSizeTrigger(t *testing.T) {
    ctx := context.Background()
    
    batcher := streamz.NewBatcher[int](streamz.BatchConfig{
        MaxSize: 3, // Trigger on size
        // No MaxLatency for this test
    })
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5, 6, 7})
    output := batcher.Process(ctx, input)
    results := collectAll(output)
    
    // Should produce: [1,2,3], [4,5,6], [7]
    assert.Len(t, results, 3)
    assert.Equal(t, []int{1, 2, 3}, results[0])
    assert.Equal(t, []int{4, 5, 6}, results[1])
    assert.Equal(t, []int{7}, results[2]) // Partial batch
}

func TestBatcherTimeTrigger(t *testing.T) {
    ctx := context.Background()
    
    batcher := streamz.NewBatcher[int](streamz.BatchConfig{
        MaxSize:    100,              // Won't be reached
        MaxLatency: 50 * time.Millisecond, // Time trigger
    })
    
    input := make(chan int)
    output := batcher.Process(ctx, input)
    
    // Send data slowly
    go func() {
        defer close(input)
        input <- 1
        time.Sleep(20 * time.Millisecond)
        input <- 2
        time.Sleep(60 * time.Millisecond) // Trigger batch emission
        input <- 3
    }()
    
    results, err := collectWithTimeout(output, time.Second)
    require.NoError(t, err)
    
    // Should produce two batches due to time trigger
    assert.Len(t, results, 2)
    assert.Equal(t, []int{1, 2}, results[0]) // Time-triggered batch
    assert.Equal(t, []int{3}, results[1])    // Final batch
}
```

### Debouncing Tests

Test processors with timing behavior:

```go
func TestDebounce(t *testing.T) {
    ctx := context.Background()
    
    debouncer := streamz.NewDebounce[string](100 * time.Millisecond)
    
    input := make(chan string)
    output := debouncer.Process(ctx, input)
    
    // Send burst of data
    go func() {
        defer close(input)
        input <- "first"
        time.Sleep(10 * time.Millisecond)
        input <- "second"  // Should be debounced
        time.Sleep(10 * time.Millisecond)
        input <- "third"   // Should be debounced
        time.Sleep(200 * time.Millisecond) // Wait for debounce
        input <- "fourth"  // Should pass through
    }()
    
    results, err := collectWithTimeout(output, 500*time.Millisecond)
    require.NoError(t, err)
    
    // Should only see "third" (last in burst) and "fourth"
    expected := []string{"third", "fourth"}
    assert.Equal(t, expected, results)
}
```

## Integration Testing

### Testing Complete Pipelines

Test entire pipeline behavior:

```go
func TestOrderProcessingPipeline(t *testing.T) {
    ctx := context.Background()
    
    // Define test pipeline
    validator := streamz.NewFilter(func(order Order) bool {
        return order.Amount > 0 && order.CustomerID != ""
    }).WithName("valid")
    
    enricher := streamz.NewMapper(func(order Order) Order {
        order.ProcessedAt = time.Now()
        return order
    }).WithName("enrich")
    
    batcher := streamz.NewBatcher[Order](streamz.BatchConfig{
        MaxSize: 2,
    })
    
    // Test data
    orders := []Order{
        {ID: "1", CustomerID: "cust1", Amount: 100},
        {ID: "2", CustomerID: "", Amount: 50},      // Invalid - no customer
        {ID: "3", CustomerID: "cust2", Amount: 0},  // Invalid - no amount
        {ID: "4", CustomerID: "cust3", Amount: 200},
        {ID: "5", CustomerID: "cust4", Amount: 150},
    }
    
    // Run pipeline
    input := sendData(ctx, orders)
    validated := validator.Process(ctx, input)
    enriched := enricher.Process(ctx, validated)
    batched := batcher.Process(ctx, enriched)
    
    results := collectAll(batched)
    
    // Verify: Only valid orders, properly batched
    assert.Len(t, results, 2) // Two batches
    
    // First batch: orders 1 and 4
    assert.Len(t, results[0], 2)
    assert.Equal(t, "1", results[0][0].ID)
    assert.Equal(t, "4", results[0][1].ID)
    
    // Second batch: order 5
    assert.Len(t, results[1], 1)
    assert.Equal(t, "5", results[1][0].ID)
    
    // Verify enrichment happened
    for _, batch := range results {
        for _, order := range batch {
            assert.False(t, order.ProcessedAt.IsZero())
        }
    }
}
```

### Testing Pipeline Composition

Test how processors work together:

```go
func TestPipelineComposition(t *testing.T) {
    ctx := context.Background()
    
    // Create a reusable pipeline function
    pipeline := func(input <-chan int) <-chan []int {
        filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
        mapper := streamz.NewMapper(func(n int) int { return n * n }).WithName("square")
        batcher := streamz.NewBatcher[int](streamz.BatchConfig{MaxSize: 3})
        
        filtered := filter.Process(ctx, input)
        squared := mapper.Process(ctx, filtered)
        return batcher.Process(ctx, squared)
    }
    
    // Test with various inputs
    testCases := []struct {
        name     string
        input    []int
        expected [][]int
    }{
        {
            name:     "mixed positive and negative",
            input:    []int{-1, 2, -3, 4, 5, -6, 7},
            expected: [][]int{{4, 16, 25}, {49}}, // Only positive, squared, batched
        },
        {
            name:     "all negative",
            input:    []int{-1, -2, -3},
            expected: [][]int{}, // All filtered out
        },
        {
            name:     "exact batch size",
            input:    []int{1, 2, 3},
            expected: [][]int{{1, 4, 9}},
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            input := sendData(ctx, tc.input)
            output := pipeline(input)
            results := collectAll(output)
            assert.Equal(t, tc.expected, results)
        })
    }
}
```

## Concurrency Testing

### Race Condition Detection

Always run tests with race detector:

```bash
go test -race ./...
```

Test concurrent access patterns:

```go
func TestConcurrentProcessing(t *testing.T) {
    ctx := context.Background()
    
    // Processor that might have race conditions
    counter := int64(0)
    processor := streamz.NewMapper(func(n int) int {
        // This would be a race condition without proper synchronization
        atomic.AddInt64(&counter, 1)
        return n
    }).WithName("counter")
    
    // Send data from multiple goroutines
    input := make(chan int, 100)
    
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(start int) {
            defer wg.Done()
            for j := 0; j < 10; j++ {
                input <- start*10 + j
            }
        }(i)
    }
    
    go func() {
        wg.Wait()
        close(input)
    }()
    
    output := processor.Process(ctx, input)
    results := collectAll(output)
    
    // Verify all items processed
    assert.Len(t, results, 100)
    assert.Equal(t, int64(100), atomic.LoadInt64(&counter))
}
```

### Testing AsyncMapper Ordering

Verify order preservation in concurrent processing:

```go
func TestAsyncMapperOrderPreservation(t *testing.T) {
    ctx := context.Background()
    
    // Processor with variable processing time
    processor := streamz.NewAsyncMapper(func(ctx context.Context, n int) (int, error) {
        // Add random delay to test ordering
        delay := time.Duration(rand.Intn(10)) * time.Millisecond
        time.Sleep(delay)
        return n * 2, nil
    }).WithWorkers(5)
    
    // Send ordered input
    input := make(chan int)
    go func() {
        defer close(input)
        for i := 1; i <= 20; i++ {
            input <- i
        }
    }()
    
    output := processor.Process(ctx, input)
    results := collectAll(output)
    
    // Verify order is preserved despite concurrent processing
    expected := make([]int, 20)
    for i := 0; i < 20; i++ {
        expected[i] = (i + 1) * 2
    }
    
    assert.Equal(t, expected, results)
}
```

## Performance Testing

### Benchmarking Processors

```go
func BenchmarkFilterThroughput(b *testing.B) {
    ctx := context.Background()
    filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
    
    // Prepare test data
    data := make([]int, 1000)
    for i := range data {
        data[i] = i - 500 // Mix of positive and negative
    }
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        input := sendData(ctx, data)
        output := filter.Process(ctx, input)
        
        // Consume all output
        count := 0
        for range output {
            count++
        }
    }
}

func BenchmarkPipelineLatency(b *testing.B) {
    ctx := context.Background()
    
    filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
    mapper := streamz.NewMapper(func(n int) int { return n * 2 }).WithName("double")
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        input := make(chan int, 1)
        
        start := time.Now()
        
        filtered := filter.Process(ctx, input)
        doubled := mapper.Process(ctx, filtered)
        
        input <- 42
        close(input)
        
        result := <-doubled
        
        latency := time.Since(start)
        
        b.ReportMetric(float64(latency.Nanoseconds()), "ns/item")
        
        if result != 84 {
            b.Fatalf("Expected 84, got %d", result)
        }
    }
}
```

### Load Testing

Test pipeline behavior under high load:

```go
func TestHighVolumeProcessing(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    ctx := context.Background()
    
    processor := streamz.NewAsyncMapper(func(ctx context.Context, n int) (int, error) {
        // Simulate some work
        time.Sleep(time.Microsecond)
        return n * 2, nil
    }).WithWorkers(10)
    
    itemCount := 100000
    input := make(chan int, 1000) // Buffered for high throughput
    
    // Send high volume of data
    go func() {
        defer close(input)
        for i := 0; i < itemCount; i++ {
            input <- i
        }
    }()
    
    start := time.Now()
    output := processor.Process(ctx, input)
    
    count := 0
    for range output {
        count++
    }
    
    duration := time.Since(start)
    throughput := float64(count) / duration.Seconds()
    
    t.Logf("Processed %d items in %v (%.0f items/sec)", count, duration, throughput)
    
    assert.Equal(t, itemCount, count)
    assert.Greater(t, throughput, 10000.0) // Expect at least 10k items/sec
}
```

## Testing Error Scenarios

### Context Cancellation

Test graceful shutdown:

```go
func TestContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    
    processor := streamz.NewMapper(func(n int) int {
        time.Sleep(10 * time.Millisecond) // Slow processing
        return n * 2
    }).WithName("slow")
    
    input := make(chan int)
    output := processor.Process(ctx, input)
    
    // Send some data
    go func() {
        defer close(input)
        for i := 0; i < 100; i++ {
            select {
            case input <- i:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Cancel context after short time
    go func() {
        time.Sleep(50 * time.Millisecond)
        cancel()
    }()
    
    // Collect results until channel closes
    var results []int
    for result := range output {
        results = append(results, result)
    }
    
    // Should have processed some items before cancellation
    assert.Greater(t, len(results), 0)
    assert.Less(t, len(results), 100) // But not all
}
```

### Resource Cleanup

Test that resources are properly cleaned up:

```go
func TestResourceCleanup(t *testing.T) {
    ctx := context.Background()
    
    // Count active goroutines before
    initialGoroutines := runtime.NumGoroutine()
    
    // Create and run multiple processors
    for i := 0; i < 10; i++ {
        processor := streamz.NewAsyncMapper(func(ctx context.Context, n int) (int, error) {
            return n * 2, nil
        }).WithWorkers(5)
        
        input := sendData(ctx, []int{1, 2, 3, 4, 5})
        output := processor.Process(ctx, input)
        
        // Consume all output
        for range output {
        }
    }
    
    // Allow time for cleanup
    time.Sleep(100 * time.Millisecond)
    runtime.GC()
    
    // Check that goroutines were cleaned up
    finalGoroutines := runtime.NumGoroutine()
    
    // Should be back to initial count (or very close)
    assert.InDelta(t, initialGoroutines, finalGoroutines, 5) // Allow some variance
}
```

## Testing Best Practices

### 1. **Test Structure**

```go
func TestProcessorName(t *testing.T) {
    // Arrange
    ctx := context.Background()
    processor := streamz.NewSomeProcessor(config)
    input := sendData(ctx, testData)
    
    // Act
    output := processor.Process(ctx, input)
    results := collectAll(output)
    
    // Assert
    assert.Equal(t, expected, results)
}
```

### 2. **Use Table-Driven Tests**

```go
func TestFilterVariousCases(t *testing.T) {
    tests := []struct {
        name     string
        predicate func(int) bool
        input    []int
        expected []int
    }{
        {
            name:     "positive numbers",
            predicate: func(n int) bool { return n > 0 },
            input:    []int{-1, 0, 1, 2, -3},
            expected: []int{1, 2},
        },
        {
            name:     "even numbers",
            predicate: func(n int) bool { return n%2 == 0 },
            input:    []int{1, 2, 3, 4, 5},
            expected: []int{2, 4},
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := context.Background()
            filter := streamz.NewFilter(tt.predicate).WithName("test")
            
            input := sendData(ctx, tt.input)
            output := filter.Process(ctx, input)
            results := collectAll(output)
            
            assert.Equal(t, tt.expected, results)
        })
    }
}
```

### 3. **Test Edge Cases**

```go
func TestEdgeCases(t *testing.T) {
    ctx := context.Background()
    processor := streamz.NewMapper(func(n int) int { return n * 2 }).WithName("test")
    
    t.Run("empty input", func(t *testing.T) {
        input := sendData(ctx, []int{})
        output := processor.Process(ctx, input)
        results := collectAll(output)
        assert.Empty(t, results)
    })
    
    t.Run("single item", func(t *testing.T) {
        input := sendData(ctx, []int{42})
        output := processor.Process(ctx, input)
        results := collectAll(output)
        assert.Equal(t, []int{84}, results)
    })
    
    t.Run("closed input immediately", func(t *testing.T) {
        input := make(chan int)
        close(input) // Close immediately
        
        output := processor.Process(ctx, input)
        results := collectAll(output)
        assert.Empty(t, results)
    })
}
```

### 4. **Mock External Dependencies**

```go
// For processors that call external services
type MockExternalService struct {
    responses map[string]string
    errors    map[string]error
}

func (m *MockExternalService) Enrich(ctx context.Context, id string) (string, error) {
    if err, ok := m.errors[id]; ok {
        return "", err
    }
    if resp, ok := m.responses[id]; ok {
        return resp, nil
    }
    return "", fmt.Errorf("not found: %s", id)
}

func TestWithMockedService(t *testing.T) {
    mock := &MockExternalService{
        responses: map[string]string{
            "valid": "enriched-data",
        },
        errors: map[string]error{
            "error": errors.New("service error"),
        },
    }
    
    enricher := streamz.NewAsyncMapper(func(ctx context.Context, id string) (string, error) {
        return mock.Enrich(ctx, id)
    }).WithWorkers(2)
    
    // Test with mock
    input := sendData(context.Background(), []string{"valid", "error", "unknown"})
    output := enricher.Process(context.Background(), input)
    results := collectAll(output)
    
    // Only "valid" should succeed
    assert.Equal(t, []string{"enriched-data"}, results)
}
```

### 5. **Integration with Testing Frameworks**

```go
// Using testify suite for setup/teardown
type ProcessorTestSuite struct {
    suite.Suite
    ctx context.Context
}

func (s *ProcessorTestSuite) SetupTest() {
    s.ctx = context.Background()
}

func (s *ProcessorTestSuite) TestFilter() {
    filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
    
    input := sendData(s.ctx, []int{-1, 1, -2, 2})
    output := filter.Process(s.ctx, input)
    results := collectAll(output)
    
    s.Equal([]int{1, 2}, results)
}

func TestProcessorSuite(t *testing.T) {
    suite.Run(t, new(ProcessorTestSuite))
}
```

## Common Testing Patterns

### Testing Custom Processors

```go
// Test a custom processor implementation
type CustomProcessor struct {
    name string
}

func (c *CustomProcessor) Name() string {
    return c.name
}

func (c *CustomProcessor) Process(ctx context.Context, in <-chan int) <-chan int {
    out := make(chan int)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                // Custom logic
                if item%3 == 0 {
                    select {
                    case out <- item * 10:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

func TestCustomProcessor(t *testing.T) {
    ctx := context.Background()
    processor := &CustomProcessor{name: "triple-multiplier"}
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8, 9})
    output := processor.Process(ctx, input)
    results := collectAll(output)
    
    // Only multiples of 3, multiplied by 10
    expected := []int{30, 60, 90}
    assert.Equal(t, expected, results)
}
```

## Test Organization

### Directory Structure

```
your-project/
├── processors/
│   ├── filter.go
│   ├── filter_test.go
│   ├── mapper.go
│   └── mapper_test.go
├── pipelines/
│   ├── order_pipeline.go
│   ├── order_pipeline_test.go
│   └── integration_test.go
└── testutil/
    ├── helpers.go
    └── fixtures.go
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Run with coverage
go test -cover ./...

# Run specific test
go test -run TestFilterPositive

# Run benchmarks
go test -bench=.

# Verbose output
go test -v ./...
```

## What's Next?

- **[Best Practices](./best-practices.md)**: Production testing strategies
- **[Performance Guide](./performance.md)**: Benchmark your tests
- **[API Reference](../api/)**: Test-friendly processor documentation
- **[Concepts: Error Handling](../concepts/error-handling.md)**: Test error scenarios