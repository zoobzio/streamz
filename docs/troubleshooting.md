# Troubleshooting Guide

This guide helps you diagnose and fix common issues when working with streamz. Issues are organized by symptoms for quick navigation.

## Table of Contents

- [Quick Diagnostic Checklist](#quick-diagnostic-checklist)
- [Common Issues](#common-issues)
  - [Goroutine Leaks and Channel Deadlocks](#goroutine-leaks-and-channel-deadlocks)
  - [Memory Leaks and Unbounded Growth](#memory-leaks-and-unbounded-growth)
  - [Performance Degradation](#performance-degradation)
  - [Error Propagation Issues](#error-propagation-issues)
  - [Backpressure and Flow Control](#backpressure-and-flow-control)
  - [Race Conditions and Data Races](#race-conditions-and-data-races)
  - [Pipeline Composition Errors](#pipeline-composition-errors)
  - [Context Cancellation Issues](#context-cancellation-issues)
- [Diagnostic Techniques](#diagnostic-techniques)
  - [Using pprof for Performance Profiling](#using-pprof-for-performance-profiling)
  - [Detecting Goroutine Leaks](#detecting-goroutine-leaks)
  - [Monitoring Channel Buffer Usage](#monitoring-channel-buffer-usage)
  - [Tracing Data Flow](#tracing-data-flow)
  - [Using Race Detector](#using-race-detector)
- [Error Patterns and Solutions](#error-patterns-and-solutions)
- [Best Practices](#best-practices)
- [Debug Helpers](#debug-helpers)
- [Real-World Scenarios](#real-world-scenarios)

## Quick Diagnostic Checklist

When encountering issues with streamz pipelines, follow this checklist:

1. **Pipeline not processing data?**
   - [ ] Check if context is cancelled
   - [ ] Verify input channel is not nil
   - [ ] Ensure goroutines are running (check with pprof)
   - [ ] Look for blocking channel operations

2. **Memory usage growing?**
   - [ ] Check for unbounded buffers
   - [ ] Verify channels are being drained
   - [ ] Look for goroutine leaks
   - [ ] Check batch/window size configurations

3. **Performance issues?**
   - [ ] Profile with pprof
   - [ ] Check for synchronous operations that should be async
   - [ ] Verify buffer sizes are appropriate
   - [ ] Look for unnecessary allocations

4. **Data loss or corruption?**
   - [ ] Run with race detector
   - [ ] Check error handling
   - [ ] Verify context propagation
   - [ ] Ensure proper channel closure

## Common Issues

### Goroutine Leaks and Channel Deadlocks

#### Symptom: "All goroutines are asleep - deadlock!"

This panic occurs when all goroutines are blocked waiting for channel operations.

❌ **WRONG: Not closing input channels**
```go
func processData(ctx context.Context) {
    input := make(chan int)
    mapper := streamz.NewMapper(func(n int) int { return n * 2 })
    
    output := mapper.Process(ctx, input)
    
    // Send data
    go func() {
        for i := 0; i < 10; i++ {
            input <- i
        }
        // MISSING: close(input)
    }()
    
    // This will deadlock after 10 items
    for result := range output {
        fmt.Println(result)
    }
}
```

✅ **RIGHT: Always close input channels**
```go
func processData(ctx context.Context) {
    input := make(chan int)
    mapper := streamz.NewMapper(func(n int) int { return n * 2 })
    
    output := mapper.Process(ctx, input)
    
    // Send data
    go func() {
        defer close(input) // Always close when done
        for i := 0; i < 10; i++ {
            input <- i
        }
    }()
    
    // Now this will complete properly
    for result := range output {
        fmt.Println(result)
    }
}
```

#### Symptom: Goroutines accumulating over time

❌ **WRONG: Creating processors in hot paths**
```go
func handleRequest(ctx context.Context, data []int) []int {
    // Creates new goroutines on every request!
    batcher := streamz.NewBatcher[int](streamz.BatchConfig{
        MaxSize:    10,
        MaxLatency: time.Second,
    }, streamz.RealClock)
    
    input := make(chan int)
    output := batcher.Process(ctx, input)
    
    // Process data...
    // Goroutine from batcher may leak if not properly drained
}
```

✅ **RIGHT: Reuse processors or ensure cleanup**
```go
// Create once at package level
var batcher = streamz.NewBatcher[int](streamz.BatchConfig{
    MaxSize:    10,
    MaxLatency: time.Second,
}, streamz.RealClock)

func handleRequest(ctx context.Context, data []int) []int {
    // Or use context with timeout to ensure cleanup
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    input := make(chan int)
    output := batcher.Process(ctx, input)
    
    // Send data
    go func() {
        defer close(input)
        for _, v := range data {
            select {
            case input <- v:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Collect results
    var results []int
    for batch := range output {
        results = append(results, batch...)
    }
    return results
}
```

### Memory Leaks and Unbounded Growth

#### Symptom: Memory usage continuously increasing

❌ **WRONG: Unbounded channel buffers**
```go
func createPipeline(ctx context.Context) <-chan Event {
    // Unbounded buffer can consume all memory if consumer is slow
    events := make(chan Event, 1000000)
    
    // Fast producer
    go func() {
        for {
            events <- generateEvent()
        }
    }()
    
    // Slow consumer
    processor := streamz.NewAsyncMapper(func(ctx context.Context, e Event) (Event, error) {
        time.Sleep(100 * time.Millisecond) // Slow processing
        return e, nil
    }).WithWorkers(1)
    
    return processor.Process(ctx, events)
}
```

✅ **RIGHT: Use appropriate buffering strategies**
```go
func createPipeline(ctx context.Context) <-chan Event {
    // Use dropping buffer for real-time systems
    events := make(chan Event, 100)
    
    // Create dropping buffer to prevent memory growth
    buffer := streamz.NewDroppingBuffer[Event](1000)
    
    // Fast producer
    go func() {
        defer close(events)
        for {
            select {
            case events <- generateEvent():
            case <-ctx.Done():
                return
            default:
                // Drop event if buffer is full
                metrics.IncrementDropped()
            }
        }
    }()
    
    // Apply buffer before slow processor
    buffered := buffer.Process(ctx, events)
    
    processor := streamz.NewAsyncMapper(func(ctx context.Context, e Event) (Event, error) {
        time.Sleep(100 * time.Millisecond)
        return e, nil
    }).WithWorkers(10) // Increase workers for better throughput
    
    return processor.Process(ctx, buffered)
}
```

#### Symptom: Batches growing unbounded

❌ **WRONG: No timeout on batch collection**
```go
// This can cause memory issues if data flow stops
func processBatches(ctx context.Context, items <-chan Item) {
    var batch []Item
    for item := range items {
        batch = append(batch, item)
        if len(batch) >= 1000 {
            processBatch(batch)
            batch = nil
        }
    }
    // Last batch might never be processed!
}
```

✅ **RIGHT: Use Batcher with proper configuration**
```go
func processBatches(ctx context.Context, items <-chan Item) {
    batcher := streamz.NewBatcher[Item](streamz.BatchConfig{
        MaxSize:    1000,
        MaxLatency: 5 * time.Second, // Force emit after 5 seconds
    }, streamz.RealClock)
    
    batches := batcher.Process(ctx, items)
    for batch := range batches {
        processBatch(batch)
    }
}
```

### Performance Degradation

#### Symptom: Pipeline throughput decreasing over time

❌ **WRONG: Synchronous processing in pipeline**
```go
func slowPipeline(ctx context.Context, requests <-chan Request) <-chan Response {
    mapper := streamz.NewMapper(func(req Request) Response {
        // Synchronous external call blocks entire pipeline
        resp, _ := http.Get(req.URL)
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)
        return Response{Data: body}
    })
    
    return mapper.Process(ctx, requests)
}
```

✅ **RIGHT: Use AsyncMapper for concurrent processing**
```go
func fastPipeline(ctx context.Context, requests <-chan Request) <-chan Response {
    mapper := streamz.NewAsyncMapper(func(ctx context.Context, req Request) (Response, error) {
        // Concurrent processing with timeout
        ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
        defer cancel()
        
        httpReq, _ := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
        resp, err := http.DefaultClient.Do(httpReq)
        if err != nil {
            return Response{}, err
        }
        defer resp.Body.Close()
        
        body, _ := io.ReadAll(resp.Body)
        return Response{Data: body}, nil
    }).WithWorkers(20) // Process 20 requests concurrently
    
    return mapper.Process(ctx, requests)
}
```

#### Symptom: High CPU usage with little work done

❌ **WRONG: Tight polling loops**
```go
func pollForData(ctx context.Context) <-chan Data {
    out := make(chan Data)
    go func() {
        defer close(out)
        for {
            if data := checkForData(); data != nil {
                out <- *data
            }
            // Tight loop consuming CPU!
        }
    }()
    return out
}
```

✅ **RIGHT: Use proper timing and throttling**
```go
func pollForData(ctx context.Context) <-chan Data {
    out := make(chan Data)
    go func() {
        defer close(out)
        ticker := time.NewTicker(100 * time.Millisecond)
        defer ticker.Stop()
        
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                if data := checkForData(); data != nil {
                    select {
                    case out <- *data:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }
    }()
    return out
}
```

### Error Propagation Issues

#### Symptom: Errors silently dropped

❌ **WRONG: Ignoring errors in pipeline**
```go
func processPipeline(ctx context.Context, items <-chan Item) <-chan Result {
    mapper := streamz.NewMapper(func(item Item) Result {
        result, err := processItem(item)
        if err != nil {
            // Error is lost!
            return Result{}
        }
        return result
    })
    
    return mapper.Process(ctx, items)
}
```

✅ **RIGHT: Use proper error handling patterns**
```go
type ItemResult struct {
    Item   Item
    Result Result
    Error  error
}

func processPipeline(ctx context.Context, items <-chan Item) <-chan ItemResult {
    mapper := streamz.NewMapper(func(item Item) ItemResult {
        result, err := processItem(item)
        return ItemResult{
            Item:   item,
            Result: result,
            Error:  err,
        }
    })
    
    // Process with DLQ for errors
    output := mapper.Process(ctx, items)
    
    // Split errors to DLQ
    dlq := streamz.NewDLQ[ItemResult](func(ir ItemResult) bool {
        return ir.Error != nil
    })
    
    return dlq.Process(ctx, output)
}
```

### Backpressure and Flow Control

#### Symptom: Fast producer overwhelming slow consumer

❌ **WRONG: No backpressure handling**
```go
func noPressureControl(ctx context.Context) {
    data := make(chan Data)
    
    // Fast producer
    go func() {
        defer close(data)
        for i := 0; i < 1000000; i++ {
            data <- generateData(i) // Will block if consumer is slow
        }
    }()
    
    // Slow consumer
    for d := range data {
        time.Sleep(100 * time.Millisecond)
        process(d)
    }
}
```

✅ **RIGHT: Implement proper backpressure**
```go
func withPressureControl(ctx context.Context) {
    // Option 1: Use buffering
    buffer := streamz.NewBuffer[Data](1000)
    
    // Option 2: Use dropping buffer for real-time
    dropper := streamz.NewDroppingBuffer[Data](1000)
    
    // Option 3: Use sliding buffer to keep latest
    slider := streamz.NewSlidingBuffer[Data](1000)
    
    // Option 4: Use sampling to reduce load
    sampler := streamz.NewSample[Data](0.1) // Process 10% of data
    
    data := make(chan Data, 100) // Small buffer
    
    // Producer with backpressure awareness
    go func() {
        defer close(data)
        for i := 0; i < 1000000; i++ {
            select {
            case data <- generateData(i):
                // Sent successfully
            case <-time.After(10 * time.Millisecond):
                // Timeout - consumer is too slow
                metrics.IncrementDropped()
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Apply backpressure handling
    buffered := buffer.Process(ctx, data)
    
    // Slow consumer now won't block producer
    for d := range buffered {
        time.Sleep(100 * time.Millisecond)
        process(d)
    }
}
```

### Race Conditions and Data Races

#### Symptom: Inconsistent results or data corruption

❌ **WRONG: Shared state without synchronization**
```go
type Counter struct {
    count int // Not thread-safe!
}

func (c *Counter) Process(ctx context.Context, items <-chan Item) <-chan Item {
    fanout := streamz.NewFanOut[Item](3)
    streams := fanout.Process(ctx, items)
    
    // Multiple goroutines accessing shared state
    for _, stream := range streams {
        go func(s <-chan Item) {
            for item := range s {
                c.count++ // DATA RACE!
                processItem(item)
            }
        }(stream)
    }
    
    return items
}
```

✅ **RIGHT: Use proper synchronization**
```go
type Counter struct {
    count atomic.Int64 // Thread-safe
}

func (c *Counter) Process(ctx context.Context, items <-chan Item) <-chan Item {
    // Option 1: Use atomic operations
    mapper := streamz.NewMapper(func(item Item) Item {
        c.count.Add(1)
        return item
    })
    
    // Option 2: Use channels for synchronization
    countChan := make(chan int64)
    go func() {
        var count int64
        for delta := range countChan {
            count += delta
            c.count.Store(count)
        }
    }()
    
    // Option 3: Use mutex for complex operations
    var mu sync.Mutex
    processor := streamz.NewTap(func(item Item) {
        mu.Lock()
        defer mu.Unlock()
        // Complex stateful operation
        updateComplexState(item)
    })
    
    return processor.Process(ctx, items)
}
```

### Pipeline Composition Errors

#### Symptom: Type mismatches in pipeline

❌ **WRONG: Incompatible processor chaining**
```go
func brokenPipeline(ctx context.Context, numbers <-chan int) {
    // Batcher outputs []int, not int
    batcher := streamz.NewBatcher[int](streamz.BatchConfig{
        MaxSize: 10,
    }, streamz.RealClock)
    
    // This expects chan int, not chan []int!
    mapper := streamz.NewMapper(func(n int) int { return n * 2 })
    
    batches := batcher.Process(ctx, numbers)
    // mapped := mapper.Process(ctx, batches) // Compile error!
}
```

✅ **RIGHT: Match types correctly**
```go
func workingPipeline(ctx context.Context, numbers <-chan int) <-chan int {
    // Batch numbers
    batcher := streamz.NewBatcher[int](streamz.BatchConfig{
        MaxSize:    10,
        MaxLatency: time.Second,
    }, streamz.RealClock)
    
    // Process batches
    batchProcessor := streamz.NewMapper(func(batch []int) []int {
        // Process entire batch
        result := make([]int, len(batch))
        for i, n := range batch {
            result[i] = n * 2
        }
        return result
    })
    
    // Unbatch back to individual items
    unbatcher := streamz.NewUnbatcher[int]()
    
    // Compose pipeline with matching types
    batches := batcher.Process(ctx, numbers)
    processed := batchProcessor.Process(ctx, batches)
    return unbatcher.Process(ctx, processed)
}
```

### Context Cancellation Issues

#### Symptom: Goroutines not stopping when context cancelled

❌ **WRONG: Ignoring context in goroutines**
```go
func leakyProcessor(ctx context.Context, input <-chan Data) <-chan Result {
    output := make(chan Result)
    
    go func() {
        defer close(output)
        for data := range input { // Doesn't check context!
            result := expensiveOperation(data)
            output <- result
        }
    }()
    
    return output
}
```

✅ **RIGHT: Always respect context cancellation**
```go
func properProcessor(ctx context.Context, input <-chan Data) <-chan Result {
    output := make(chan Result)
    
    go func() {
        defer close(output)
        for {
            select {
            case <-ctx.Done():
                return // Stop immediately on cancellation
            case data, ok := <-input:
                if !ok {
                    return // Input closed
                }
                
                // Check context during long operations
                result, err := expensiveOperationWithContext(ctx, data)
                if err != nil {
                    if ctx.Err() != nil {
                        return // Context cancelled during operation
                    }
                    // Handle other errors
                    continue
                }
                
                // Send with context check
                select {
                case output <- result:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()
    
    return output
}
```

## Diagnostic Techniques

### Using pprof for Performance Profiling

Add profiling endpoints to your application:

```go
import (
    _ "net/http/pprof"
    "net/http"
)

func main() {
    // Start pprof server
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    
    // Your application code
    runPipeline()
}
```

Profile CPU usage:
```bash
# Record 30 second CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# In pprof interactive mode:
(pprof) top10
(pprof) list FunctionName
(pprof) web
```

Profile memory usage:
```bash
# Get heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# In pprof interactive mode:
(pprof) top10
(pprof) list FunctionName
(pprof) alloc_objects  # Show allocation counts
(pprof) inuse_objects  # Show live objects
```

### Detecting Goroutine Leaks

```go
func TestForGoroutineLeaks(t *testing.T) {
    // Record initial goroutine count
    initialGoroutines := runtime.NumGoroutine()
    
    // Run your pipeline
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    runPipeline(ctx)
    
    // Wait for goroutines to cleanup
    time.Sleep(100 * time.Millisecond)
    
    // Check final count
    finalGoroutines := runtime.NumGoroutine()
    if finalGoroutines > initialGoroutines {
        // Get goroutine dump
        buf := make([]byte, 1<<20)
        stackLen := runtime.Stack(buf, true)
        t.Fatalf("Goroutine leak detected: %d -> %d\n%s", 
            initialGoroutines, finalGoroutines, buf[:stackLen])
    }
}
```

Monitor goroutines at runtime:
```bash
# Check goroutine count and stacks
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

### Monitoring Channel Buffer Usage

```go
func monitorChannel[T any](name string, ch chan T) {
    ticker := time.NewTicker(time.Second)
    go func() {
        for range ticker.C {
            log.Printf("Channel %s: %d/%d items", 
                name, len(ch), cap(ch))
        }
    }()
}

// Usage
dataChan := make(chan Data, 1000)
monitorChannel("data", dataChan)
```

### Tracing Data Flow

Add tracing to your pipeline:

```go
type TracedItem struct {
    ID        string
    Data      interface{}
    Timestamp time.Time
    Stage     string
}

func traceStage[T any](stage string) streamz.Processor[T, T] {
    return streamz.NewTap(func(item T) {
        log.Printf("[%s] Processing: %+v", stage, item)
    }).WithName(stage)
}

// Build traced pipeline
func buildTracedPipeline(ctx context.Context, input <-chan Order) <-chan Order {
    // Add tracing at each stage
    stage1 := traceStage[Order]("validation")
    stage2 := traceStage[Order]("enrichment")
    stage3 := traceStage[Order]("storage")
    
    validated := stage1.Process(ctx, input)
    enriched := stage2.Process(ctx, validated)
    return stage3.Process(ctx, enriched)
}
```

### Using Race Detector

Always test with race detector during development:

```bash
# Run tests with race detector
go test -race ./...

# Run application with race detector
go run -race main.go

# Build with race detector
go build -race -o myapp
./myapp
```

## Error Patterns and Solutions

### Pattern: "Send on closed channel" panic

❌ **WRONG: Closing channel while senders are active**
```go
func broken() {
    ch := make(chan int)
    
    // Start sender
    go func() {
        for i := 0; i < 100; i++ {
            ch <- i // Might panic!
            time.Sleep(10 * time.Millisecond)
        }
    }()
    
    // Receiver closes channel prematurely
    go func() {
        count := 0
        for v := range ch {
            count++
            if count == 10 {
                close(ch) // PANIC: send on closed channel
                return
            }
        }
    }()
}
```

✅ **RIGHT: Only sender should close channel**
```go
func correct() {
    ch := make(chan int)
    done := make(chan struct{})
    
    // Sender controls channel lifetime
    go func() {
        defer close(ch)
        for i := 0; i < 100; i++ {
            select {
            case ch <- i:
            case <-done:
                return // Stop sending on signal
            }
            time.Sleep(10 * time.Millisecond)
        }
    }()
    
    // Receiver signals completion
    go func() {
        count := 0
        for v := range ch {
            count++
            if count == 10 {
                close(done) // Signal to stop
                return
            }
        }
    }()
}
```

### Pattern: Context deadline exceeded

```go
func handleDeadline(ctx context.Context, items <-chan Item) error {
    // Add monitoring for deadline
    deadline, ok := ctx.Deadline()
    if ok {
        remaining := time.Until(deadline)
        log.Printf("Processing with deadline in %v", remaining)
    }
    
    processor := streamz.NewAsyncMapper(func(ctx context.Context, item Item) (Result, error) {
        // Check context before expensive operations
        if ctx.Err() != nil {
            return Result{}, ctx.Err()
        }
        
        // Use context for external calls
        return callServiceWithContext(ctx, item)
    }).WithWorkers(10)
    
    results := processor.Process(ctx, items)
    
    for result := range results {
        if err := processResult(result); err != nil {
            if ctx.Err() != nil {
                return fmt.Errorf("context cancelled: %w", ctx.Err())
            }
            return fmt.Errorf("processing failed: %w", err)
        }
    }
    
    return nil
}
```

## Best Practices

### Proper Context Usage

```go
// Always propagate context through pipelines
func buildPipeline(ctx context.Context) {
    // Create derived context with timeout
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    // Pass context to all processors
    filter := streamz.NewFilter(predicate)
    batcher := streamz.NewBatcher[Item](config, streamz.RealClock)
    
    filtered := filter.Process(ctx, input)
    batched := batcher.Process(ctx, filtered)
}
```

### Channel Lifecycle Management

```go
// Clear ownership and lifecycle
type Pipeline struct {
    input  chan Data // Pipeline owns this
    output chan Result
    done   chan struct{}
}

func (p *Pipeline) Start(ctx context.Context) {
    go func() {
        defer close(p.output) // Pipeline closes what it creates
        // Process data
    }()
}

func (p *Pipeline) Stop() {
    close(p.done)   // Signal shutdown
    close(p.input)  // Close input to stop pipeline
}
```

### Error Handling Strategies

```go
// Use error channels for critical errors
type ProcessResult struct {
    Data  Data
    Error error
}

func processWithErrors(ctx context.Context, input <-chan Data) (<-chan Data, <-chan error) {
    output := make(chan Data)
    errors := make(chan error, 1) // Buffer to prevent blocking
    
    go func() {
        defer close(output)
        defer close(errors)
        
        for data := range input {
            result, err := process(data)
            if err != nil {
                select {
                case errors <- err:
                default:
                    // Error channel full, log and continue
                    log.Printf("Error processing: %v", err)
                }
                continue
            }
            
            select {
            case output <- result:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return output, errors
}
```

### Resource Cleanup Patterns

```go
func runPipelineWithCleanup(ctx context.Context) error {
    // Setup resources
    resources := setupResources()
    
    // Ensure cleanup
    defer func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        
        if err := resources.Cleanup(ctx); err != nil {
            log.Printf("Cleanup failed: %v", err)
        }
    }()
    
    // Run pipeline
    return runPipeline(ctx, resources)
}
```

### Testing Strategies

```go
func TestPipelineWithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    input := make(chan int)
    
    // Setup pipeline
    pipeline := buildPipeline()
    output := pipeline.Process(ctx, input)
    
    // Send test data
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
    
    // Collect with timeout
    var results []int
    for {
        select {
        case result, ok := <-output:
            if !ok {
                // Pipeline completed
                if len(results) != 100 {
                    t.Errorf("Expected 100 results, got %d", len(results))
                }
                return
            }
            results = append(results, result)
        case <-ctx.Done():
            t.Fatal("Test timeout")
        }
    }
}
```

## Debug Helpers

### Pipeline Inspector

```go
type PipelineInspector struct {
    name      string
    processed atomic.Int64
    errors    atomic.Int64
    latency   atomic.Int64 // nanoseconds
}

func (pi *PipelineInspector) Wrap[T any](processor streamz.Processor[T, T]) streamz.Processor[T, T] {
    return streamz.NewMapper(func(item T) T {
        start := time.Now()
        defer func() {
            pi.processed.Add(1)
            pi.latency.Store(int64(time.Since(start)))
        }()
        
        return item
    }).WithName(fmt.Sprintf("%s-inspector", pi.name))
}

func (pi *PipelineInspector) Stats() string {
    return fmt.Sprintf("[%s] Processed: %d, Errors: %d, Latency: %dms",
        pi.name,
        pi.processed.Load(),
        pi.errors.Load(),
        pi.latency.Load()/1e6,
    )
}
```

### Debug Logger

```go
func debugPipeline[T any](name string, ch <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        count := 0
        for item := range ch {
            count++
            log.Printf("[%s] Item %d: %+v", name, count, item)
            out <- item
        }
        log.Printf("[%s] Completed: %d items", name, count)
    }()
    
    return out
}

// Usage
pipeline := debugPipeline("after-filter", 
    filter.Process(ctx, 
        debugPipeline("before-filter", input)))
```

### Metrics Collector

```go
type Metrics struct {
    mu         sync.RWMutex
    counters   map[string]int64
    histograms map[string][]float64
    gauges     map[string]float64
}

func (m *Metrics) IncrementCounter(name string, delta int64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.counters[name] += delta
}

func (m *Metrics) RecordDuration(name string, d time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.histograms[name] = append(m.histograms[name], d.Seconds())
}

func (m *Metrics) SetGauge(name string, value float64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.gauges[name] = value
}

// Wrap processor with metrics
func withMetrics[T any](name string, metrics *Metrics, processor streamz.Processor[T, T]) streamz.Processor[T, T] {
    return streamz.NewMapper(func(item T) T {
        start := time.Now()
        defer func() {
            metrics.RecordDuration(name+".duration", time.Since(start))
            metrics.IncrementCounter(name+".processed", 1)
        }()
        return item
    })
}
```

## Real-World Scenarios

### HTTP Server with Streaming Responses

```go
func streamingHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Setup streaming response
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
        return
    }
    
    // Create pipeline
    source := getDataSource(ctx)
    
    // Add buffering to handle backpressure
    buffer := streamz.NewBuffer[Event](100)
    buffered := buffer.Process(ctx, source)
    
    // Process events
    processor := streamz.NewAsyncMapper(func(ctx context.Context, e Event) (string, error) {
        return formatSSE(e), nil
    }).WithWorkers(5)
    
    formatted := processor.Process(ctx, buffered)
    
    // Stream to client
    for {
        select {
        case event, ok := <-formatted:
            if !ok {
                return // Stream ended
            }
            
            fmt.Fprintf(w, "%s\n\n", event)
            flusher.Flush()
            
        case <-ctx.Done():
            // Client disconnected
            return
        }
    }
}
```

### File Processing Pipeline

```go
func processLargeFile(ctx context.Context, filename string) error {
    file, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()
    
    // Create line reader
    lines := make(chan string, 100)
    
    go func() {
        defer close(lines)
        scanner := bufio.NewScanner(file)
        for scanner.Scan() {
            select {
            case lines <- scanner.Text():
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Parse lines to records
    parser := streamz.NewMapper(func(line string) (Record, error) {
        return parseRecord(line)
    })
    
    // Batch for efficient processing
    batcher := streamz.NewBatcher[Record](streamz.BatchConfig{
        MaxSize:    1000,
        MaxLatency: time.Second,
    }, streamz.RealClock)
    
    // Process batches in parallel
    processor := streamz.NewAsyncMapper(func(ctx context.Context, batch []Record) ([]Result, error) {
        return processBatch(ctx, batch)
    }).WithWorkers(runtime.NumCPU())
    
    // Build pipeline
    records := parser.Process(ctx, lines)
    batches := batcher.Process(ctx, records)
    results := processor.Process(ctx, batches)
    
    // Unbatch and save results
    unbatcher := streamz.NewUnbatcher[Result]()
    individual := unbatcher.Process(ctx, results)
    
    // Save with error handling
    errorCount := 0
    successCount := 0
    
    for result := range individual {
        if err := saveResult(result); err != nil {
            errorCount++
            if errorCount > 100 {
                return fmt.Errorf("too many errors: %d", errorCount)
            }
        } else {
            successCount++
        }
        
        if (successCount+errorCount)%10000 == 0 {
            log.Printf("Progress: %d processed, %d errors", 
                successCount, errorCount)
        }
    }
    
    log.Printf("Completed: %d successful, %d errors", 
        successCount, errorCount)
    return nil
}
```

### Real-time Data Transformation

```go
func realtimePipeline(ctx context.Context) error {
    // Source: real-time event stream
    events := subscribeToEvents(ctx)
    
    // Deduplicate events within 5 minute window
    dedupe := streamz.NewDedupe(func(e Event) string {
        return e.ID
    }).WithTTL(5 * time.Minute)
    
    // Filter out test events
    filter := streamz.NewFilter(func(e Event) bool {
        return !e.IsTest
    })
    
    // Enrich with additional data
    enricher := streamz.NewAsyncMapper(func(ctx context.Context, e Event) (EnrichedEvent, error) {
        // Parallel enrichment from multiple sources
        return enrichEvent(ctx, e)
    }).WithWorkers(20)
    
    // Add monitoring
    monitor := streamz.NewMonitor[EnrichedEvent](time.Second).
        OnStats(func(stats streamz.StreamStats) {
            metrics.SetGauge("events.rate", stats.Rate)
            metrics.SetGauge("events.total", float64(stats.Count))
        })
    
    // Build pipeline
    deduped := dedupe.Process(ctx, events)
    filtered := filter.Process(ctx, deduped)
    enriched := enricher.Process(ctx, filtered)
    monitored := monitor.Process(ctx, enriched)
    
    // Process results
    for event := range monitored {
        if err := publishEvent(event); err != nil {
            log.Printf("Failed to publish: %v", err)
        }
    }
    
    return nil
}
```

### Fan-out/Fan-in Pattern

```go
func fanOutFanIn(ctx context.Context, jobs <-chan Job) <-chan Result {
    // Fan-out to multiple workers
    fanout := streamz.NewFanOut[Job](5)
    workers := fanout.Process(ctx, jobs)
    
    // Process each stream independently
    results := make([]<-chan Result, len(workers))
    for i, worker := range workers {
        processor := streamz.NewAsyncMapper(func(ctx context.Context, job Job) (Result, error) {
            return processJob(ctx, job)
        }).WithWorkers(3)
        
        results[i] = processor.Process(ctx, worker)
    }
    
    // Fan-in results
    fanin := streamz.NewFanIn[Result]()
    return fanin.Process(ctx, results...)
}
```

### Rate-limited API Consumption

```go
func consumeAPI(ctx context.Context, requests <-chan APIRequest) (<-chan APIResponse, <-chan error) {
    // Rate limit to 100 requests per second
    throttle := streamz.NewThrottle[APIRequest](100)
    
    // Add circuit breaker for protection
    breaker := streamz.NewCircuitBreaker(
        streamz.NewAsyncMapper(func(ctx context.Context, req APIRequest) (APIResponse, error) {
            return callAPI(ctx, req)
        }).WithWorkers(10),
        streamz.RealClock,
    ).
    FailureThreshold(0.5).
    MinRequests(10).
    RecoveryTimeout(30 * time.Second)
    
    // Add retry for transient failures
    retry := streamz.NewRetry(breaker, 3).
        WithBackoff(time.Second, 2.0).
        WithMaxDelay(30 * time.Second)
    
    // Build pipeline
    limited := throttle.Process(ctx, requests)
    responses := retry.Process(ctx, limited)
    
    // Split successes and errors
    successChan := make(chan APIResponse)
    errorChan := make(chan error, 10)
    
    go func() {
        defer close(successChan)
        defer close(errorChan)
        
        for resp := range responses {
            if resp.Error != nil {
                select {
                case errorChan <- resp.Error:
                default:
                    log.Printf("Error channel full: %v", resp.Error)
                }
            } else {
                select {
                case successChan <- resp:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()
    
    return successChan, errorChan
}
```

## Performance Optimization Checklist

When optimizing pipeline performance:

1. **Profile First**
   - [ ] Run CPU profile to identify hot spots
   - [ ] Run memory profile to find allocations
   - [ ] Check goroutine count for leaks
   - [ ] Monitor channel buffer utilization

2. **Optimize Processing**
   - [ ] Use AsyncMapper for I/O operations
   - [ ] Batch operations where possible
   - [ ] Add appropriate buffering
   - [ ] Consider sampling for high-volume streams

3. **Reduce Allocations**
   - [ ] Reuse slices and buffers
   - [ ] Use sync.Pool for temporary objects
   - [ ] Avoid unnecessary copying
   - [ ] Pre-allocate slices with known capacity

4. **Tune Concurrency**
   - [ ] Set appropriate worker counts
   - [ ] Balance between parallelism and contention
   - [ ] Use GOMAXPROCS effectively
   - [ ] Consider CPU vs I/O bound operations

5. **Monitor Production**
   - [ ] Add metrics collection
   - [ ] Set up alerting for anomalies
   - [ ] Track pipeline latency
   - [ ] Monitor error rates

## Getting Help

If you encounter issues not covered in this guide:

1. Check the [API documentation](./api/) for processor-specific details
2. Run tests with `-race` flag to detect race conditions
3. Use the debug helpers to add visibility to your pipeline
4. Profile your application to identify bottlenecks

Remember: Most streaming issues come from:
- Not closing channels properly
- Ignoring context cancellation
- Missing error handling
- Inadequate buffering/backpressure
- Race conditions in shared state

Always test with the race detector and profile your pipelines under realistic load conditions.