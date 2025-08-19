# FanOut

The FanOut processor duplicates a stream to multiple output channels, enabling parallel processing of the same data.

## Overview

FanOut creates multiple identical copies of a stream, allowing you to process the same data in different ways simultaneously. Each output receives every item from the input stream.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Create 3 copies of the stream
fanOut := streamz.NewFanOut[Event](3)
outputs := fanOut.Process(ctx, events)

// outputs[0], outputs[1], outputs[2] each receive all events
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `count` | `int` | Yes | Number of output channels to create |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `WithBufferSize(int)` | Sets buffer size for output channels |

## Usage Examples

### Parallel Analysis

```go
// Analyze data in multiple ways simultaneously
fanOut := streamz.NewFanOut[Transaction](3).
    WithBufferSize(100).
    WithName("analysis-splitter")

outputs := fanOut.Process(ctx, transactions)

// Different analyses on same data
go fraudDetection(outputs[0])
go calculateStatistics(outputs[1])
go updateRealTimeDashboard(outputs[2])
```

### Multi-Destination Writing

```go
// Write to multiple destinations
fanOut := streamz.NewFanOut[LogEntry](3)
outputs := fanOut.Process(ctx, logs)

// Write to different stores
go writeToElasticsearch(outputs[0])
go writeToS3(outputs[1])
go writeToLocalFile(outputs[2])

// Monitor completion
var wg sync.WaitGroup
wg.Add(3)

go func() {
    for log := range outputs[0] {
        elasticsearch.Index(log)
    }
    wg.Done()
}()

go func() {
    for log := range outputs[1] {
        s3.Upload(log)
    }
    wg.Done()
}()

go func() {
    for log := range outputs[2] {
        fileWriter.Write(log)
    }
    wg.Done()
}()

wg.Wait()
```

### Real-time Monitoring

```go
// Monitor stream while processing
fanOut := streamz.NewFanOut[Metric](2)
outputs := fanOut.Process(ctx, metrics)

// Main processing
processed := processor.Process(ctx, outputs[0])

// Monitoring branch
go func() {
    monitor := streamz.NewMonitor[Metric](time.Second).
        OnStats(func(stats streamz.StreamStats) {
            log.Printf("Metrics rate: %.2f/sec", stats.Rate)
        })
    
    monitored := monitor.Process(ctx, outputs[1])
    
    // Sample for detailed analysis
    sampler := streamz.NewSample[Metric](0.01) // 1% sample
    sampled := sampler.Process(ctx, monitored)
    
    for metric := range sampled {
        analyzeMetric(metric)
    }
}()
```

### A/B Testing

```go
// Test different processing strategies
fanOut := streamz.NewFanOut[Request](2).
    WithBufferSize(50)

outputs := fanOut.Process(ctx, requests)

// Strategy A
resultA := make(chan Result)
go func() {
    defer close(resultA)
    for req := range outputs[0] {
        result := processStrategyA(req)
        resultA <- result
    }
}()

// Strategy B
resultB := make(chan Result)
go func() {
    defer close(resultB)
    for req := range outputs[1] {
        result := processStrategyB(req)
        resultB <- result
    }
}()

// Compare results
go compareStrategies(resultA, resultB)
```

### Backup Processing

```go
// Process with primary and backup pipelines
fanOut := streamz.NewFanOut[Order](2)
outputs := fanOut.Process(ctx, orders)

// Primary processing
primary := outputs[0]
go func() {
    for order := range primary {
        if err := primaryProcessor.Process(order); err != nil {
            log.Printf("Primary processing failed: %v", err)
            // Backup will handle it
        }
    }
}()

// Backup processing with delay
backup := outputs[1]
go func() {
    // Delay backup processing
    delayed := streamz.NewDebounce[Order](5 * time.Second).
        Process(ctx, backup)
    
    for order := range delayed {
        // Process only if primary didn't succeed
        if !checkPrimarySuccess(order.ID) {
            backupProcessor.Process(order)
        }
    }
}()
```

## Performance Notes

- **Time Complexity**: O(n) where n is number of outputs
- **Space Complexity**: O(n*b) where b is buffer size
- **Characteristics**:
  - All outputs must be consumed to avoid blocking
  - Synchronous distribution to all outputs
  - Closes all outputs when input closes
  - Buffer size affects memory usage