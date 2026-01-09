---
title: Take
description: Take the first N items then close the stream
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - flow-control
---

# Take

The Take processor emits only the first N items from a stream and then closes the output channel, effectively limiting the stream to a fixed number of items.

## Overview

Take is useful when you need only a specific number of items from a potentially infinite or large stream, such as sampling data, implementing pagination, or limiting processing scope.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Take first 10 items
taker := streamz.NewTake[Event](10)

// Process only first 10 events
limited := taker.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `count` | `int` | Yes | Number of items to take |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |

## Usage Examples

### Data Sampling

```go
// Take first 1000 items for analysis
sampleTaker := streamz.NewTake[DataPoint](1000).
    WithName("sample-taker")

sample := sampleTaker.Process(ctx, largeDataset)

// Analyze sample
var values []float64
for point := range sample {
    values = append(values, point.Value)
}

stats := calculateStatistics(values)
fmt.Printf("Sample stats: mean=%.2f, stddev=%.2f\n", stats.Mean, stats.StdDev)
```

### Preview Implementation

```go
// Preview first N items from a stream
func PreviewStream[T any](ctx context.Context, stream <-chan T, count int) []T {
    taker := streamz.NewTake[T](count)
    limited := taker.Process(ctx, stream)
    
    var preview []T
    for item := range limited {
        preview = append(preview, item)
    }
    return preview
}

// Usage
logs := readLogStream()
preview := PreviewStream(ctx, logs, 20)
fmt.Printf("First 20 logs:\n")
for i, log := range preview {
    fmt.Printf("%d: %s\n", i+1, log)
}
```

### Limiting Expensive Operations

```go
// Process only first 100 images
imageTaker := streamz.NewTake[Image](100)

limited := imageTaker.Process(ctx, imageStream)

// Expensive processing on limited set
for img := range limited {
    // Perform expensive operations
    processed, err := applyFilters(img)
    if err != nil {
        log.Printf("Processing failed: %v", err)
        continue
    }
    
    thumbnail := generateThumbnail(processed)
    saveImage(thumbnail)
}
```

### Testing with Limited Data

```go
// Test pipeline with limited items
testTaker := streamz.NewTake[TestData](50)

// Create test pipeline
limited := testTaker.Process(ctx, testDataGenerator())
validated := validator.Process(ctx, limited)
transformed := transformer.Process(ctx, validated)

// Verify first 50 items
resultCount := 0
for result := range transformed {
    verifyResult(result)
    resultCount++
}

if resultCount != 50 {
    t.Errorf("Expected 50 results, got %d", resultCount)
}
```

### Combined with Skip for Windows

```go
// Take a window of items
func TakeWindow[T any](ctx context.Context, items <-chan T, start, size int) <-chan T {
    // Skip to start position
    skipper := streamz.NewSkip[T](start)
    // Take window size
    taker := streamz.NewTake[T](size)
    
    skipped := skipper.Process(ctx, items)
    window := taker.Process(ctx, skipped)
    
    return window
}

// Get items 100-150
window := TakeWindow(ctx, dataStream, 100, 50)

for item := range window {
    processWindowItem(item)
}
```

## Performance Notes

- **Time Complexity**: O(1) per item
- **Space Complexity**: O(1)
- **Characteristics**:
  - Closes output after count reached
  - Simple counter-based logic
  - No buffering required
  - Stops upstream processing early