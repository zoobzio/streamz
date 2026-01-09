---
title: Chunk
description: Group items into fixed-size chunks
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - batching
---

# Chunk

The Chunk processor groups items into fixed-size arrays, creating uniform batches for downstream processing.

## Overview

Chunk collects items into arrays of a specified size. When the chunk size is reached, the array is emitted. This is simpler than Batcher which supports both size and time-based batching. Use Chunk when you need consistent, fixed-size groups.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Group items into chunks of 10
chunker := streamz.NewChunk[Order](10)

// Process orders in groups of 10
chunks := chunker.Process(ctx, orders)
for chunk := range chunks {
    // chunk is []Order with exactly 10 items (except possibly the last)
    processBatch(chunk)
}
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `size` | `int` | Yes | Number of items per chunk |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |

## Usage Examples

### Bulk Database Inserts

```go
// Insert records in chunks for efficiency
insertChunker := streamz.NewChunk[Record](100).
    WithName("bulk-inserter")

chunks := insertChunker.Process(ctx, records)

for chunk := range chunks {
    // Bulk insert exactly 100 records at a time
    err := db.BulkInsert(chunk)
    if err != nil {
        log.Printf("Failed to insert chunk: %v", err)
    }
}
```

### API Request Batching

```go
// Group API requests to respect rate limits
requestChunker := streamz.NewChunk[APIRequest](25)

chunks := requestChunker.Process(ctx, requests)

// Process 25 requests at a time
for chunk := range chunks {
    responses := make([]APIResponse, len(chunk))
    
    // Send batch request
    err := api.BatchRequest(chunk, responses)
    if err != nil {
        handleError(err)
        continue
    }
    
    // Process responses
    for i, resp := range responses {
        processResponse(chunk[i], resp)
    }
}
```

### File Processing

```go
// Process file lines in chunks
lineChunker := streamz.NewChunk[string](1000)

lines := readFileLines("large.csv")
chunks := lineChunker.Process(ctx, lines)

// Process 1000 lines at a time
for chunk := range chunks {
    // Parse chunk
    records := make([]CSVRecord, 0, len(chunk))
    for _, line := range chunk {
        record, err := parseCSV(line)
        if err == nil {
            records = append(records, record)
        }
    }
    
    // Process batch of records
    processRecords(records)
}
```

### Pagination Helper

```go
// Convert stream to pages
type Page[T any] struct {
    Items      []T
    PageNumber int
    TotalItems int
}

func Paginate[T any](ctx context.Context, items <-chan T, pageSize int) <-chan Page[T] {
    chunker := streamz.NewChunk[T](pageSize)
    chunks := chunker.Process(ctx, items)
    
    pages := make(chan Page[T])
    go func() {
        defer close(pages)
        
        pageNum := 1
        totalItems := 0
        
        for chunk := range chunks {
            totalItems += len(chunk)
            
            page := Page[T]{
                Items:      chunk,
                PageNumber: pageNum,
                TotalItems: totalItems,
            }
            
            select {
            case pages <- page:
            case <-ctx.Done():
                return
            }
            
            pageNum++
        }
    }()
    
    return pages
}
```

## Performance Notes

- **Time Complexity**: O(1) per item
- **Space Complexity**: O(n) where n is chunk size
- **Characteristics**:
  - Fixed-size output (except last chunk)
  - No time-based triggers
  - Minimal overhead
  - Memory allocation per chunk