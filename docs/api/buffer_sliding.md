# Sliding Buffer

The Sliding Buffer maintains a sliding window of the most recent N items, providing access to historical data while bounding memory usage.

## Overview

Sliding Buffer keeps a fixed-size window of the most recent items. When new items arrive and the buffer is full, the oldest item is removed to make room. Unlike Dropping Buffer which drops items before they enter the buffer, Sliding Buffer accepts all items but only retains the most recent ones.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Keep a sliding window of the last 100 items
buffer := streamz.NewSlidingBuffer[Event](100)

// All items pass through, but only recent ones are retained
windowed := buffer.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `size` | `int` | Yes | Size of the sliding window |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `GetBuffer()` | Returns a snapshot of current buffer contents |

## Usage Examples

### Moving Average Calculation

```go
// Calculate moving average over last 50 values
type ValueBuffer struct {
    buffer *streamz.SlidingBuffer[float64]
}

func (vb *ValueBuffer) Process(ctx context.Context, values <-chan float64) <-chan float64 {
    // Keep sliding window
    windowed := vb.buffer.Process(ctx, values)
    
    out := make(chan float64)
    go func() {
        defer close(out)
        
        for value := range windowed {
            // Get current window
            window := vb.buffer.GetBuffer()
            
            // Calculate average
            var sum float64
            for _, v := range window {
                sum += v
            }
            avg := sum / float64(len(window))
            
            select {
            case out <- avg:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

// Use it
avgCalc := &ValueBuffer{
    buffer: streamz.NewSlidingBuffer[float64](50),
}
movingAvg := avgCalc.Process(ctx, prices)
```

### Pattern Detection

```go
// Detect patterns in recent events
patternBuffer := streamz.NewSlidingBuffer[LogEntry](200).
    WithName("pattern-detector")

logs := patternBuffer.Process(ctx, logStream)

// Check for patterns periodically
go func() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        recent := patternBuffer.GetBuffer()
        
        // Analyze recent logs for patterns
        if detectErrorSpike(recent) {
            alert("Error spike detected in last 200 logs")
        }
        
        if detectSlowdown(recent) {
            alert("Performance degradation detected")
        }
    }
}()
```

### Context Window for ML

```go
// Maintain context window for predictions
contextBuffer := streamz.NewSlidingBuffer[Feature](128)

features := contextBuffer.Process(ctx, featureStream)

predictions := make(chan Prediction)
go func() {
    defer close(predictions)
    
    for feature := range features {
        // Get full context window
        context := contextBuffer.GetBuffer()
        
        // Make prediction using context
        pred := model.Predict(context)
        
        select {
        case predictions <- pred:
        case <-ctx.Done():
            return
        }
    }
}()
```

## Performance Notes

- **Time Complexity**: O(1) for append and remove operations
- **Space Complexity**: O(n) where n is the window size
- **Characteristics**:
  - All items pass through (no dropping at input)
  - Maintains FIFO order in buffer
  - Thread-safe buffer access
  - Efficient circular buffer implementation