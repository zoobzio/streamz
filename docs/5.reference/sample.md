---
title: Sample
description: Random sampling by probability
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - flow-control
---

# Sample

The Sample processor randomly samples items from a stream based on a specified percentage, useful for data reduction and statistical sampling.

## Overview

Sample uses probability-based selection to pass through a percentage of items from the input stream. Each item has an independent probability of being selected, ensuring statistically representative sampling over large datasets.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Sample 10% of items
sampler := streamz.NewSample[Event](0.1)

// Process only a sample of events
sampled := sampler.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `percentage` | `float64` | Yes | Sampling rate (0.0 to 1.0) |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `WithSeed(int64)` | Sets random seed for reproducible sampling |

## Usage Examples

### Metrics Sampling

```go
// Sample 5% of metrics for monitoring
metricsSampler := streamz.NewSample[Metric](0.05).
    WithName("metrics-sampler")

sampled := metricsSampler.Process(ctx, highVolumeMetrics)

// Process sampled metrics without overwhelming system
for metric := range sampled {
    // Detailed analysis on sample
    analyzeMetric(metric)
    publishToMonitoring(metric)
}
```

### A/B Testing

```go
// Sample 20% of users for experiment
experimentSampler := streamz.NewSample[User](0.2).
    WithSeed(42). // Reproducible sampling
    WithName("ab-test-sampler")

sampled := experimentSampler.Process(ctx, users)

// Apply experimental feature to sampled users
for user := range sampled {
    enableExperimentalFeature(user.ID)
    trackExperimentUser(user)
}
```

### Log Sampling

```go
// Sample logs based on level
debugSampler := streamz.NewSample[LogEntry](0.01)  // 1% of debug logs
infoSampler := streamz.NewSample[LogEntry](0.1)    // 10% of info logs
warnSampler := streamz.NewSample[LogEntry](1.0)    // 100% of warnings

// Route logs by level with sampling
router := streamz.NewRouter[LogEntry]().
    AddRoute("debug", isDebugLog, debugSampler).
    AddRoute("info", isInfoLog, infoSampler).
    AddRoute("warn", isWarnLog, warnSampler)

outputs := router.Process(ctx, logs)

// Merge sampled streams
sampled := streamz.NewFanIn(
    outputs.Routes["debug"],
    outputs.Routes["info"],
    outputs.Routes["warn"],
).Process(ctx)
```

### Performance Analysis

```go
// Sample requests for detailed tracing
tracingSampler := streamz.NewSample[Request](0.001). // 0.1% for tracing
    WithName("tracing-sampler")

sampled := tracingSampler.Process(ctx, requests)

// Detailed tracing for sampled requests
for request := range sampled {
    trace := startTrace(request)
    
    // Process with full instrumentation
    response := processWithTracing(request, trace)
    
    // Analyze performance
    trace.End()
    publishTrace(trace)
}
```

### Data Reduction Pipeline

```go
// Progressive sampling for data reduction
initialSample := streamz.NewSample[DataPoint](0.5)   // 50% first pass
secondSample := streamz.NewSample[DataPoint](0.2)    // 20% of the 50% = 10% total

// Chain samplers
firstPass := initialSample.Process(ctx, rawData)
finalSample := secondSample.Process(ctx, firstPass)

// Aggregate sampled data
aggregator := streamz.NewAggregate(
    DataStats{},
    updateDataStats,
).WithCountWindow(1000)

windows := aggregator.Process(ctx, finalSample)

for window := range windows {
    // Statistics on 10% sample
    publishStats(window.Result)
}
```

## Performance Notes

- **Time Complexity**: O(1) per item
- **Space Complexity**: O(1)
- **Characteristics**:
  - Statistically uniform sampling
  - No memory of previous selections
  - Minimal processing overhead
  - Configurable randomness seed