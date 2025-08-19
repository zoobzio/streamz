# Unbatcher

The Unbatcher processor converts batches back into individual items, essentially reversing the Batcher operation.

## Overview

Unbatcher takes a stream of slices/batches and emits each item individually. This is useful when you need to process items individually after batch operations, or when interfacing between batch-oriented and stream-oriented systems.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Convert batches back to individual items
unbatcher := streamz.NewUnbatcher[Order]()

// []Order -> Order
individual := unbatcher.Process(ctx, batches)
```

## Configuration Options

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |

## Usage Examples

### Post-Batch Processing

```go
// Batch for efficient processing, then unbatch for individual handling
batcher := streamz.NewBatcher[Request](streamz.BatchConfig{
    MaxSize:    50,
    MaxLatency: 100 * time.Millisecond,
})

// Batch process
batches := batcher.Process(ctx, requests)
batchResults := batchAPI.Process(ctx, batches) // Returns []Response

// Unbatch for individual response handling
unbatcher := streamz.NewUnbatcher[Response]().
    WithName("response-unbatcher")

responses := unbatcher.Process(ctx, batchResults)

// Handle each response individually
for response := range responses {
    if response.Error != nil {
        handleError(response)
    } else {
        notifySuccess(response.RequestID)
    }
}
```

### Database Read Optimization

```go
// Read from database in chunks
func StreamTableData(ctx context.Context, table string) <-chan Record {
    chunks := make(chan []Record)
    
    // Read in chunks for efficiency
    go func() {
        defer close(chunks)
        offset := 0
        batchSize := 1000
        
        for {
            records, err := db.Query(
                "SELECT * FROM ? LIMIT ? OFFSET ?",
                table, batchSize, offset,
            )
            
            if err != nil {
                log.Printf("Query error: %v", err)
                return
            }
            
            if len(records) == 0 {
                return // No more data
            }
            
            select {
            case chunks <- records:
            case <-ctx.Done():
                return
            }
            
            offset += batchSize
        }
    }()
    
    // Unbatch for streaming interface
    unbatcher := streamz.NewUnbatcher[Record]()
    return unbatcher.Process(ctx, chunks)
}

// Use the streaming interface
records := StreamTableData(ctx, "orders")
for record := range records {
    processRecord(record)
}
```

### Pipeline Integration

```go
// Integration between batch and stream systems
type BatchSystem interface {
    ProcessBatch([]Item) []Result
}

type StreamSystem interface {
    Process(<-chan Result)
}

// Bridge batch and stream systems
func BridgeSystems(ctx context.Context, items <-chan Item, batch BatchSystem, stream StreamSystem) {
    // Batch items
    batcher := streamz.NewBatcher[Item](streamz.BatchConfig{
        MaxSize:    100,
        MaxLatency: 1 * time.Second,
    })
    
    batches := batcher.Process(ctx, items)
    
    // Process in batch system
    results := make(chan []Result)
    go func() {
        defer close(results)
        for batch := range batches {
            batchResults := batch.ProcessBatch(batch)
            select {
            case results <- batchResults:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Unbatch for stream system
    unbatcher := streamz.NewUnbatcher[Result]()
    individual := unbatcher.Process(ctx, results)
    
    // Process in stream system
    stream.Process(individual)
}
```

### Parallel Batch Processing

```go
// Process batches in parallel, merge results
fanOut := streamz.NewFanOut[[]Task](3)
outputs := fanOut.Process(ctx, taskBatches)

// Parallel batch processors
processed := make([]<-chan []Result, 3)
for i := 0; i < 3; i++ {
    processed[i] = processBatchWorker(ctx, outputs[i])
}

// Merge batch results
merged := streamz.NewFanIn(processed...).Process(ctx)

// Unbatch merged results
unbatcher := streamz.NewUnbatcher[Result]()
allResults := unbatcher.Process(ctx, merged)

// Process individual results
for result := range allResults {
    recordResult(result)
}
```

### Error Recovery

```go
// Unbatch for individual error handling
type BatchResult struct {
    Results []ItemResult
    Error   error
}

batchResults := batchProcessor.Process(ctx, batches)

// Handle batch-level errors
errorFilter := streamz.NewFilter(func(br BatchResult) bool {
    if br.Error != nil {
        log.Printf("Batch error: %v", br.Error)
        handleBatchError(br)
        return false
    }
    return true
})

successfulBatches := errorFilter.Process(ctx, batchResults)

// Extract results from successful batches
resultMapper := streamz.NewMapper(func(br BatchResult) []ItemResult {
    return br.Results
})

resultBatches := resultMapper.Process(ctx, successfulBatches)

// Unbatch for individual processing
unbatcher := streamz.NewUnbatcher[ItemResult]()
results := unbatcher.Process(ctx, resultBatches)

// Process individual results
for result := range results {
    if result.Error != nil {
        retryItem(result)
    } else {
        saveResult(result)
    }
}
```

## Performance Notes

- **Time Complexity**: O(m) where m is total items across all batches
- **Space Complexity**: O(1) - no buffering
- **Characteristics**:
  - Simple iteration over batch items
  - Preserves order within batches
  - No memory overhead
  - Handles empty batches gracefully