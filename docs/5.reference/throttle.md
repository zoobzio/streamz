---
title: Throttle
description: Rate limiting at the leading edge
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - flow-control
---

# Throttle

The Throttle processor implements rate limiting by controlling the maximum number of items that pass through per second.

## Overview

Throttle ensures that items flow through the stream at a controlled rate, preventing downstream systems from being overwhelmed. It uses a token bucket algorithm to maintain a steady flow rate while allowing short bursts.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Limit to 100 items per second
throttle := streamz.NewThrottle[Request](100)

// Process requests at controlled rate
limited := throttle.Process(ctx, requests)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ratePerSecond` | `int` | Yes | Maximum items allowed per second |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `WithBurst(int)` | Sets burst capacity (default: same as rate) |

## Usage Examples

### API Rate Limiting

```go
// Respect API rate limits
apiThrottle := streamz.NewThrottle[APICall](50). // 50 requests/second
    WithBurst(10). // Allow bursts up to 10
    WithName("api-limiter")

limited := apiThrottle.Process(ctx, apiCalls)

for call := range limited {
    // Calls are rate-limited to prevent 429 errors
    resp, err := makeAPICall(call)
    if err != nil {
        log.Printf("API call failed: %v", err)
    }
}
```

### Database Write Protection

```go
// Protect database from write storms
dbThrottle := streamz.NewThrottle[DBWrite](1000). // 1000 writes/second
    WithName("db-throttle")

// Chain with batcher for efficiency
batcher := streamz.NewBatcher[DBWrite](streamz.BatchConfig{
    MaxSize:    100,
    MaxLatency: 100 * time.Millisecond,
})

limited := dbThrottle.Process(ctx, writes)
batches := batcher.Process(ctx, limited)

for batch := range batches {
    // Writes are both rate-limited and batched
    err := db.BulkWrite(batch)
    if err != nil {
        handleError(err)
    }
}
```

### Message Queue Protection

```go
// Prevent overwhelming message queue
mqThrottle := streamz.NewThrottle[Message](500). // 500 msgs/second
    WithBurst(50) // Handle short bursts

messages := mqThrottle.Process(ctx, incomingMessages)

// Multiple consumers can process at controlled rate
for i := 0; i < 10; i++ {
    go func(workerID int) {
        for msg := range messages {
            // Each worker gets rate-limited messages
            processMessage(workerID, msg)
        }
    }(i)
}
```

### Multi-Tier Rate Limiting

```go
// Different rate limits for different priority levels
highPriority := streamz.NewThrottle[Task](100).WithName("high-pri")
normalPriority := streamz.NewThrottle[Task](50).WithName("normal-pri")
lowPriority := streamz.NewThrottle[Task](10).WithName("low-pri")

// Route tasks by priority
router := streamz.NewRouter[Task]().
    AddRoute("high", isHighPriority, highPriority).
    AddRoute("normal", isNormalPriority, normalPriority).
    AddRoute("low", isLowPriority, lowPriority)

outputs := router.Process(ctx, tasks)

// Merge rate-limited streams
merged := streamz.NewFanIn(outputs.Routes["high"], 
    outputs.Routes["normal"], 
    outputs.Routes["low"]).
    Process(ctx)
```

## Performance Notes

- **Time Complexity**: O(1) per item
- **Space Complexity**: O(1) - uses token bucket
- **Characteristics**:
  - Smooth rate limiting with burst handling
  - Non-blocking until rate limit reached
  - Accurate timing using token bucket
  - Minimal CPU overhead