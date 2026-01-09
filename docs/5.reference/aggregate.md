---
title: Aggregate
description: Reduce items to a single accumulated value
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - aggregation
---

# Aggregate

The Aggregate processor performs stateful aggregation over items in a stream. It maintains an aggregate state that is updated with each new item and emits results based on configured triggers (count, time, or both).

## Overview

Aggregate is essential for:
- Computing running statistics (sum, average, min, max)
- Time-based aggregations (hourly totals, daily counts)
- Custom aggregations (percentiles, unique counts)
- Incremental computation over streams

## Basic Usage

```go
import (
    "context"
    "time"
    "github.com/zoobzio/streamz"
)

// Sum values in 1-minute windows
summer := streamz.NewAggregate(
    0,                          // Initial state
    func(sum, n int) int {      // Aggregation function
        return sum + n
    },
).WithTimeWindow(time.Minute)

windows := summer.Process(ctx, numbers)
for window := range windows {
    fmt.Printf("Sum for %s: %d\n", window.Start.Format("15:04"), window.Result)
}
```

## Configuration Options

### WithCountWindow

Emit results after a specific number of items:

```go
aggregator := streamz.NewAggregate(initial, aggregateFunc).
    WithCountWindow(100) // Emit every 100 items
```

### WithTimeWindow

Emit results after a specific duration:

```go
aggregator := streamz.NewAggregate(initial, aggregateFunc).
    WithTimeWindow(5 * time.Minute) // Emit every 5 minutes
```

### WithEmptyWindows

Configure whether to emit windows with no data:

```go
aggregator := streamz.NewAggregate(initial, aggregateFunc).
    WithTimeWindow(1 * time.Hour).
    WithEmptyWindows(true) // Emit hourly even if no data
```

### WithName

Set a custom name for monitoring:

```go
aggregator := streamz.NewAggregate(initial, aggregateFunc).
    WithName("revenue-aggregator")
```

## Built-in Aggregation Functions

### Sum

Sum numeric values:

```go
summer := streamz.NewAggregate(0, streamz.Sum[int]()).
    WithCountWindow(1000)
```

### Count

Count items:

```go
counter := streamz.NewAggregate(0, streamz.Count[string]()).
    WithTimeWindow(1 * time.Minute)
```

### Average

Compute running average:

```go
averager := streamz.NewAggregate(streamz.Average{}, streamz.Avg[float64]()).
    WithCountWindow(100)

windows := averager.Process(ctx, measurements)
for window := range windows {
    fmt.Printf("Average: %.2f\n", window.Result.Value())
}
```

### Min/Max

Track minimum and maximum values:

```go
minMaxer := streamz.NewAggregate(streamz.MinMax[int]{}, streamz.MinMaxAgg[int]()).
    WithTimeWindow(10 * time.Second)

windows := minMaxer.Process(ctx, values)
for window := range windows {
    fmt.Printf("Range: %d to %d\n", window.Result.Min, window.Result.Max)
}
```

## Custom Aggregation Functions

### Unique Count

```go
// Track unique users
type UserSet map[string]struct{}

uniqueCounter := streamz.NewAggregate(
    make(UserSet),
    func(users UserSet, userID string) UserSet {
        users[userID] = struct{}{}
        return users
    },
).WithTimeWindow(1 * time.Hour)

windows := uniqueCounter.Process(ctx, userActivity)
for window := range windows {
    fmt.Printf("Hour %s: %d unique users\n", 
        window.Start.Format("15:04"), len(window.Result))
}
```

### String Concatenation

```go
// Collect messages
type Messages struct {
    Items []string
}

collector := streamz.NewAggregate(
    Messages{},
    func(msgs Messages, msg string) Messages {
        msgs.Items = append(msgs.Items, msg)
        return msgs
    },
).WithCountWindow(10)
```

### Statistical Aggregation

```go
// Running statistics
type Stats struct {
    Sum     float64
    Count   int
    Min     float64
    Max     float64
    SumSq   float64 // For variance calculation
}

stats := streamz.NewAggregate(
    Stats{Min: math.MaxFloat64, Max: -math.MaxFloat64},
    func(s Stats, value float64) Stats {
        s.Sum += value
        s.Count++
        s.SumSq += value * value
        if value < s.Min {
            s.Min = value
        }
        if value > s.Max {
            s.Max = value
        }
        return s
    },
).WithTimeWindow(5 * time.Minute)
```

## Advanced Patterns

### Multi-Trigger Aggregation

Use both count and time triggers:

```go
// Emit when we have 1000 items OR 1 minute passes
aggregator := streamz.NewAggregate(0, streamz.Sum[int]()).
    WithCountWindow(1000).
    WithTimeWindow(1 * time.Minute)

// Whichever condition is met first triggers emission
```

### Monitoring Current State

Inspect aggregate state without waiting for window:

```go
summer := streamz.NewAggregate(0, streamz.Sum[int]()).
    WithCountWindow(10000)

// In monitoring goroutine
state, count := summer.GetCurrentState()
fmt.Printf("Current sum: %d (from %d items)\n", state, count)
```

### Time-Series Analysis

```go
// Moving average with time windows
type MovingAvg struct {
    Values []float64
    Window int
}

movingAvg := streamz.NewAggregate(
    MovingAvg{Window: 20},
    func(ma MovingAvg, value float64) MovingAvg {
        ma.Values = append(ma.Values, value)
        if len(ma.Values) > ma.Window {
            ma.Values = ma.Values[1:] // Keep last N values
        }
        return ma
    },
).WithTimeWindow(1 * time.Second)

windows := movingAvg.Process(ctx, prices)
for window := range windows {
    avg := calculateAverage(window.Result.Values)
    fmt.Printf("20-period MA: %.2f\n", avg)
}
```

### Session-Based Aggregation

```go
// Aggregate user session data
type Session struct {
    UserID      string
    Actions     []string
    Duration    time.Duration
    LastAction  time.Time
}

sessionAgg := streamz.NewAggregate(
    Session{},
    func(session Session, action UserAction) Session {
        if session.UserID == "" {
            session.UserID = action.UserID
            session.LastAction = action.Timestamp
        }
        session.Actions = append(session.Actions, action.Type)
        session.Duration = action.Timestamp.Sub(session.LastAction)
        session.LastAction = action.Timestamp
        return session
    },
).WithTimeWindow(30 * time.Minute) // Session timeout
```

## Real-World Examples

### Revenue Tracking

```go
// Track revenue by hour
type Revenue struct {
    Total    float64
    Count    int
    Products map[string]int
}

revenueTracker := streamz.NewAggregate(
    Revenue{Products: make(map[string]int)},
    func(rev Revenue, order Order) Revenue {
        rev.Total += order.Amount
        rev.Count++
        rev.Products[order.ProductID]++
        return rev
    },
).WithTimeWindow(1 * time.Hour).
    WithEmptyWindows(true). // Report even if no sales
    WithName("hourly-revenue")

windows := revenueTracker.Process(ctx, orders)
for window := range windows {
    fmt.Printf("[%s] Revenue: $%.2f from %d orders\n",
        window.Start.Format("15:04"),
        window.Result.Total,
        window.Result.Count)
}
```

### Performance Metrics

```go
// Track API performance
type PerfStats struct {
    TotalLatency  time.Duration
    Count         int
    Errors        int
    StatusCodes   map[int]int
}

perfTracker := streamz.NewAggregate(
    PerfStats{StatusCodes: make(map[int]int)},
    func(stats PerfStats, resp APIResponse) PerfStats {
        stats.TotalLatency += resp.Latency
        stats.Count++
        stats.StatusCodes[resp.StatusCode]++
        if resp.StatusCode >= 400 {
            stats.Errors++
        }
        return stats
    },
).WithTimeWindow(1 * time.Minute)

windows := perfTracker.Process(ctx, responses)
for window := range windows {
    avgLatency := window.Result.TotalLatency / time.Duration(window.Result.Count)
    errorRate := float64(window.Result.Errors) / float64(window.Result.Count) * 100
    
    fmt.Printf("Minute %s: Avg latency %v, Error rate %.1f%%\n",
        window.Start.Format("15:04"),
        avgLatency,
        errorRate)
}
```

## Window Information

Each emitted window contains:

```go
type AggregateWindow[A any] struct {
    Result A          // The aggregated value
    Start  time.Time  // Window start time
    End    time.Time  // Window end time
    Count  int        // Number of items in window
}
```

## Performance Considerations

1. **State Size**: Keep aggregate state small for better performance
2. **Window Size**: Balance between latency and throughput
3. **Memory Usage**: State is held in memory until window closes
4. **Concurrent Access**: State updates are thread-safe but add overhead

## Best Practices

1. **Choose Appropriate Windows**: Match window size to your use case
2. **Handle Partial Windows**: Final window may have fewer items
3. **Monitor Memory**: Large states or long windows increase memory
4. **Use Empty Windows**: For regular reporting even without data
5. **Combine Triggers**: Use both count and time for flexibility

## Common Pitfalls

1. **Large State Objects**: Accumulating unbounded data in state
2. **Missing Final Window**: Not handling partial final window
3. **Time Skew**: Assuming perfect timing for time windows
4. **State Mutation**: Modifying state outside aggregator function