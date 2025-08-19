# Skip

The Skip processor discards the first N items from a stream and then passes through all subsequent items unchanged.

## Overview

Skip is useful when you need to ignore initial items in a stream, such as headers in data files, warm-up periods in measurements, or initial invalid data from sensors.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Skip first 5 items
skipper := streamz.NewSkip[string](5)

// Process stream without first 5 items
skipped := skipper.Process(ctx, lines)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `count` | `int` | Yes | Number of items to skip |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |

## Usage Examples

### CSV Header Skipping

```go
// Skip CSV header row
csvSkipper := streamz.NewSkip[string](1).
    WithName("csv-header-skip")

lines := readFileLines("data.csv")
dataLines := csvSkipper.Process(ctx, lines)

for line := range dataLines {
    // Process data rows only
    record, err := parseCSVLine(line)
    if err != nil {
        log.Printf("Parse error: %v", err)
        continue
    }
    processRecord(record)
}
```

### Sensor Warm-up Period

```go
// Skip initial unstable readings
sensorSkipper := streamz.NewSkip[SensorReading](100).
    WithName("warmup-skip")

readings := sensorSkipper.Process(ctx, sensorStream)

// Process only stable readings
for reading := range readings {
    if reading.Temperature > maxTemp {
        triggerHighTempAlert(reading)
    }
    recordReading(reading)
}
```

### Log Processing

```go
// Skip startup logs
logSkipper := streamz.NewSkip[LogEntry](50)

logs := logSkipper.Process(ctx, applicationLogs)

// Analyze logs after startup
errorDetector := streamz.NewFilter[LogEntry](func(log LogEntry) bool {
    return log.Level == "ERROR"
})

errors := errorDetector.Process(ctx, logs)

for err := range errors {
    handleError(err)
}
```

### Pagination Implementation

```go
// Skip items for pagination
func GetPage[T any](ctx context.Context, items <-chan T, pageSize, pageNum int) <-chan T {
    // Skip items from previous pages
    skipCount := pageSize * (pageNum - 1)
    skipper := streamz.NewSkip[T](skipCount)
    
    // Take items for current page
    taker := streamz.NewTake[T](pageSize)
    
    // Chain skip and take
    skipped := skipper.Process(ctx, items)
    page := taker.Process(ctx, skipped)
    
    return page
}

// Usage
page3 := GetPage(ctx, products, 20, 3) // Get page 3, 20 items per page
```

### Combined with Other Processors

```go
// Skip initial data then aggregate
skipper := streamz.NewSkip[Metric](1000) // Skip first 1000 metrics
aggregator := streamz.NewAggregate(
    MetricStats{},
    updateStats,
).WithCountWindow(100)

// Skip warm-up period then aggregate
stable := skipper.Process(ctx, metrics)
windows := aggregator.Process(ctx, stable)

for window := range windows {
    // Statistics exclude initial unstable period
    publishStats(window.Result)
}
```

## Performance Notes

- **Time Complexity**: O(1) per item
- **Space Complexity**: O(1)
- **Characteristics**:
  - Simple counter-based logic
  - No buffering required
  - Zero overhead after skip count reached
  - Maintains item order