# Debounce

The Debounce processor delays emitting items until a specified quiet period has passed with no new items, effectively filtering out rapid sequences.

## Overview

Debounce waits for a pause in the input stream before emitting the most recent item. This is useful for scenarios where you want to react to the final state after a series of rapid changes, such as user input or sensor readings that fluctuate before stabilizing.

## Basic Usage

```go
import (
    "context"
    "time"
    "github.com/zoobzio/streamz"
)

// Wait for 500ms of quiet before emitting
debounce := streamz.NewDebounce[SearchQuery](500 * time.Millisecond)

// Only emit search query after user stops typing
debounced := debounce.Process(ctx, searchQueries)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `delay` | `time.Duration` | Yes | How long to wait for quiet period |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |

## Usage Examples

### Search Input Handling

```go
// Debounce search queries to avoid excessive API calls
searchDebounce := streamz.NewDebounce[string](300 * time.Millisecond).
    WithName("search-debounce")

queries := searchDebounce.Process(ctx, userInput)

for query := range queries {
    // Only search after user stops typing for 300ms
    results, err := searchAPI(query)
    if err != nil {
        log.Printf("Search failed: %v", err)
        continue
    }
    displayResults(results)
}
```

### Sensor Stabilization

```go
// Wait for sensor readings to stabilize
sensorDebounce := streamz.NewDebounce[SensorReading](1 * time.Second)

readings := sensorDebounce.Process(ctx, rawSensorData)

for reading := range readings {
    // Only process stable readings
    if reading.Value > threshold {
        triggerAlert(reading)
    }
    
    // Update dashboard with stable value
    updateDashboard(reading)
}
```

### Configuration Changes

```go
// Debounce configuration updates to avoid thrashing
configDebounce := streamz.NewDebounce[Config](2 * time.Second).
    WithName("config-debounce")

configs := configDebounce.Process(ctx, configUpdates)

for config := range configs {
    // Only reload after configuration changes settle
    log.Printf("Applying configuration update")
    
    err := applyConfig(config)
    if err != nil {
        log.Printf("Failed to apply config: %v", err)
        continue
    }
    
    // Restart services with new config
    restartServices()
}
```

### UI State Updates

```go
// Debounce form field updates
type FormUpdate struct {
    FieldName string
    Value     string
}

formDebounce := streamz.NewDebounce[FormUpdate](1 * time.Second)

updates := formDebounce.Process(ctx, formChanges)

for update := range updates {
    // Save form state after user stops editing
    err := saveFormDraft(userID, update)
    if err != nil {
        log.Printf("Failed to save draft: %v", err)
    }
    
    // Validate complete form
    if err := validateForm(userID); err != nil {
        showValidationError(err)
    }
}
```

### Combined with Other Processors

```go
// Debounce + Aggregate for stable metrics
debounce := streamz.NewDebounce[Metric](500 * time.Millisecond)
aggregator := streamz.NewAggregate(
    MetricStats{},
    updateStats,
).WithTimeWindow(10 * time.Second)

// Only aggregate stable readings
stable := debounce.Process(ctx, metrics)
windows := aggregator.Process(ctx, stable)

for window := range windows {
    // Process aggregated stable metrics
    publishMetrics(window.Result)
}
```

## Performance Notes

- **Time Complexity**: O(1) per item
- **Space Complexity**: O(1) - only keeps latest item
- **Characteristics**:
  - Emits only the most recent item
  - Resets timer on each new item
  - May introduce latency equal to delay
  - Drops intermediate items