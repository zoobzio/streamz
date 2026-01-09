---
title: Dedupe
description: Remove duplicate items within a time window
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - deduplication
---

# Dedupe

The Dedupe processor removes duplicate items from a stream based on a key function, using a time-windowed cache to track seen items.

## Overview

Dedupe maintains a cache of recently seen item keys and filters out duplicates within a configurable time window. This is essential for handling duplicate events, idempotency, and ensuring unique processing.

## Basic Usage

```go
import (
    "context"
    "time"
    "github.com/zoobzio/streamz"
)

// Deduplicate by ID with 5-minute window
deduper := streamz.NewDedupe(func(event Event) string {
    return event.ID
}).WithTTL(5 * time.Minute)

unique := deduper.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `keyFunc` | `func(T) string` | Yes | Function to extract unique key from items |

### Methods

| Method | Description |
|--------|-------------|
| `WithTTL(duration)` | Sets how long to remember seen keys (default: 1 hour) |
| `WithName(string)` | Sets a custom name for monitoring |
| `Stats()` | Returns deduplication statistics |

## Usage Examples

### Event Deduplication

```go
// Remove duplicate events from unreliable sources
eventDeduper := streamz.NewDedupe(func(e Event) string {
    return e.EventID
}).WithTTL(10 * time.Minute).
    WithName("event-deduper")

unique := eventDeduper.Process(ctx, events)

// Process each event only once
for event := range unique {
    processEvent(event)
    
    // Check dedup stats periodically
    if processed%1000 == 0 {
        stats := eventDeduper.Stats()
        log.Printf("Dedup stats: %d unique, %d duplicates filtered", 
            stats.UniqueCount, stats.DuplicateCount)
    }
}
```

### API Request Deduplication

```go
// Prevent duplicate API calls
requestDeduper := streamz.NewDedupe(func(req APIRequest) string {
    // Composite key for request deduplication
    return fmt.Sprintf("%s:%s:%s", req.Method, req.Path, req.UserID)
}).WithTTL(30 * time.Second) // Short window for API calls

unique := requestDeduper.Process(ctx, requests)

// Process unique requests only
for req := range unique {
    response, err := handleAPIRequest(req)
    if err != nil {
        log.Printf("Request failed: %v", err)
    }
}
```

### Message Queue Deduplication

```go
// Handle at-least-once delivery guarantees
messageDeduper := streamz.NewDedupe(func(msg Message) string {
    return msg.MessageID
}).WithTTL(24 * time.Hour) // Long window for reliability

unique := messageDeduper.Process(ctx, messages)

// Process each message exactly once
for msg := range unique {
    if err := processMessage(msg); err != nil {
        // Safe to retry - deduper prevents double processing
        retryQueue.Add(msg)
    }
}
```

### User Action Deduplication

```go
// Prevent duplicate user actions
actionDeduper := streamz.NewDedupe(func(action UserAction) string {
    // Dedupe by user and action type within time window
    return fmt.Sprintf("%s:%s:%d", 
        action.UserID, 
        action.Type, 
        action.Timestamp.Unix()/300) // 5-minute buckets
}).WithTTL(10 * time.Minute)

unique := actionDeduper.Process(ctx, actions)

// Process unique actions
for action := range unique {
    switch action.Type {
    case "purchase":
        // Prevent duplicate charges
        processPurchase(action)
    case "signup":
        // Prevent duplicate accounts
        processSignup(action)
    }
}
```

### Combined with Other Processors

```go
// Dedupe before expensive operations
deduper := streamz.NewDedupe(func(img Image) string {
    return img.Hash // Use content hash
}).WithTTL(1 * time.Hour)

// Only process unique images
unique := deduper.Process(ctx, images)

// Expensive processing only on unique items
processor := streamz.NewAsyncMapper(func(ctx context.Context, img Image) (ProcessedImage, error) {
    return expensiveImageProcessing(ctx, img)
}).WithWorkers(10)

processed := processor.Process(ctx, unique)

// Monitor deduplication efficiency
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := deduper.Stats()
        efficiency := float64(stats.DuplicateCount) / 
            float64(stats.UniqueCount + stats.DuplicateCount) * 100
        
        log.Printf("Deduplication efficiency: %.1f%% duplicates filtered", efficiency)
    }
}()
```

## Performance Notes

- **Time Complexity**: O(1) average for lookup/insert
- **Space Complexity**: O(n) where n is unique items in TTL window
- **Characteristics**:
  - In-memory cache with TTL
  - Automatic cache cleanup
  - Thread-safe operations
  - Configurable memory usage via TTL