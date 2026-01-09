---
title: AtomicTime
description: Thread-safe time value for concurrent access
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - utilities
  - concurrency
---

# AtomicTime

AtomicTime provides thread-safe storage and retrieval of time.Time values without type assertions.

## Overview

AtomicTime is an internal utility type that enables lock-free atomic operations on time values. It achieves this by storing time as Unix nanoseconds internally, eliminating the need for type assertions that would otherwise be required with `atomic.Value`.

## Why It Exists

Go's `atomic.Value` can store any type, but retrieving values requires type assertions. This creates two problems:
1. Type assertions add runtime overhead
2. Type assertions can fail if the wrong type is stored

AtomicTime solves both issues by providing a type-safe API specifically for time values.

## Usage

```go
// Create a new AtomicTime (zero value is ready to use)
var lastUpdate AtomicTime

// Store a time value atomically
lastUpdate.Store(time.Now())

// Load the time value atomically
t := lastUpdate.Load()

// Check if time is zero
if lastUpdate.IsZero() {
    // No time has been stored yet
}
```

## Benefits

- **No type assertions**: Eliminates runtime type checks
- **Type safety**: Can only store time.Time values
- **Zero allocation**: Uses atomic.Int64 internally
- **Thread-safe**: Safe for concurrent use without locks

## Implementation Details

AtomicTime internally stores time as Unix nanoseconds in an `atomic.Int64`. This provides:
- 64-bit atomic operations on all platforms
- Nanosecond precision preservation
- Efficient conversion to/from time.Time

## Used By

- `CircuitBreaker`: Tracks state change times for timeout calculations
- `Monitor`: Tracks last report time for rate calculations