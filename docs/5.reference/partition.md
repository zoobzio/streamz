---
title: Partition
description: Split stream by hash key or round-robin
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - routing
---

# Partition

The Partition processor splits a stream into multiple sub-streams based on a partition key. Items with the same key are guaranteed to go to the same partition, preserving order within each partition while enabling parallel processing.

## Overview

Partitioning is essential for:
- Parallel processing by key (e.g., by customer, by region)
- Load distribution across workers
- Maintaining order within logical groups
- Horizontal scaling of stream processing

## Basic Usage

```go
import (
    "context"
    "sync"
    "github.com/zoobzio/streamz"
)

// Partition orders by customer ID
partitioner := streamz.NewPartition[Order](func(o Order) string {
    return o.CustomerID
}).WithPartitions(10)

// Process input stream
output := partitioner.Process(ctx, orders)

// Process each partition independently
var wg sync.WaitGroup
for i, partition := range output.Partitions {
    wg.Add(1)
    go func(idx int, p <-chan Order) {
        defer wg.Done()
        processPartition(ctx, idx, p)
    }(i, partition)
}
wg.Wait()
```

## Configuration Options

### WithPartitions

Sets the number of partitions to create:

```go
partitioner := streamz.NewPartition[Event](keyFunc).
    WithPartitions(8) // Create 8 partitions
```

### WithPartitioner

Uses a custom partitioning function:

```go
// Round-robin partitioner
roundRobin := func(key string, numPartitions int) int {
    hash := 0
    for _, ch := range key {
        hash += int(ch)
    }
    return hash % numPartitions
}

partitioner := streamz.NewPartition[Task](keyFunc).
    WithPartitioner(roundRobin)
```

### WithBufferSize

Sets the buffer size for each partition channel:

```go
partitioner := streamz.NewPartition[Message](keyFunc).
    WithBufferSize(100) // Buffer up to 100 items per partition
```

### WithName

Sets a custom name for monitoring:

```go
partitioner := streamz.NewPartition[Request](keyFunc).
    WithName("user-partitioner")
```

## Partitioning Strategies

### Default Partitioner

Uses consistent hashing (FNV-1a) for even distribution:

```go
// Default behavior - consistent hashing
partitioner := streamz.NewPartition[Item](func(i Item) string {
    return i.Key
})
```

### Custom Partitioners

#### Geographic Partitioning

```go
geoPartitioner := func(key string, numPartitions int) int {
    // Extract region from key (e.g., "us-west-123")
    parts := strings.Split(key, "-")
    if len(parts) > 0 {
        switch parts[0] {
        case "us":
            return 0
        case "eu":
            return 1
        case "asia":
            return 2
        default:
            return 3
        }
    }
    return 0
}

partitioner := streamz.NewPartition[Request](func(r Request) string {
    return r.Region + "-" + r.ID
}).WithPartitioner(geoPartitioner).WithPartitions(4)
```

#### Hash-Based Partitioning

```go
hashPartitioner := func(key string, numPartitions int) int {
    h := sha256.Sum256([]byte(key))
    return int(binary.BigEndian.Uint32(h[:4])) % numPartitions
}

partitioner := streamz.NewPartition[Document](func(d Document) string {
    return d.ID
}).WithPartitioner(hashPartitioner)
```

## Processing Patterns

### Parallel Processing

```go
// Partition by user for parallel processing
partitioner := streamz.NewPartition[Activity](func(a Activity) string {
    return a.UserID
}).WithPartitions(runtime.NumCPU())

output := partitioner.Process(ctx, activities)

// Process each partition with a worker pool
for i, partition := range output.Partitions {
    go func(idx int, p <-chan Activity) {
        worker := NewActivityWorker(idx)
        for activity := range p {
            worker.Process(activity)
        }
    }(i, partition)
}
```

### Ordered Processing

```go
// Maintain order within customer transactions
partitioner := streamz.NewPartition[Transaction](func(t Transaction) string {
    return t.AccountID
}).WithPartitions(10)

output := partitioner.Process(ctx, transactions)

// Each partition processes transactions in order
for i, partition := range output.Partitions {
    go func(p <-chan Transaction) {
        var balance float64
        for tx := range p {
            balance = processInOrder(tx, balance)
        }
    }(partition)
}
```

### Load Balancing

```go
// Distribute work evenly across workers
partitioner := streamz.NewPartition[Job](func(j Job) string {
    return j.ID // Unique IDs for even distribution
}).WithPartitions(numWorkers)

output := partitioner.Process(ctx, jobs)

// Create worker pool
workers := make([]*Worker, numWorkers)
for i := range workers {
    workers[i] = NewWorker(i)
    go workers[i].ProcessJobs(output.Partitions[i])
}
```

## Statistics and Monitoring

### Getting Statistics

```go
stats := partitioner.GetStats()
fmt.Printf("Total items: %d\n", stats.TotalItems)
fmt.Printf("Items per partition: %v\n", stats.ItemsPerPartition)
fmt.Printf("Distribution balance: %.2f\n", stats.DistributionBalance())
```

### Monitoring Distribution

```go
// Monitor partition distribution
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := partitioner.GetStats()
        if stats.DistributionBalance() > 0.5 {
            log.Printf("Warning: Uneven distribution detected: %.2f", 
                stats.DistributionBalance())
        }
    }
}()
```

## Advanced Examples

### Multi-Level Partitioning

```go
// First level: partition by region
regionPartitioner := streamz.NewPartition[Request](func(r Request) string {
    return r.Region
}).WithPartitions(3)

regionOutput := regionPartitioner.Process(ctx, requests)

// Second level: partition by customer within each region
for i, regionStream := range regionOutput.Partitions {
    customerPartitioner := streamz.NewPartition[Request](func(r Request) string {
        return r.CustomerID
    }).WithPartitions(10).WithName(fmt.Sprintf("region-%d-customers", i))
    
    customerOutput := customerPartitioner.Process(ctx, regionStream)
    
    // Process each customer partition
    for j, customerStream := range customerOutput.Partitions {
        go processCustomerRequests(i, j, customerStream)
    }
}
```

### Dynamic Partition Selection

```go
// Access specific partitions
partitioner := streamz.NewPartition[Event](func(e Event) string {
    return e.Type
}).WithPartitions(5)

output := partitioner.Process(ctx, events)

// Process high-priority partition with more resources
highPriorityPartition := output.GetPartition(0)
go processWithHighPriority(highPriorityPartition)

// Process other partitions normally
for i := 1; i < 5; i++ {
    partition := output.GetPartition(i)
    go processNormally(partition)
}
```

### Session-Based Partitioning

```go
// Partition by session to maintain state
partitioner := streamz.NewPartition[SessionEvent](func(e SessionEvent) string {
    return e.SessionID
}).WithPartitions(20)

output := partitioner.Process(ctx, sessionEvents)

// Each partition maintains session state
for i, partition := range output.Partitions {
    go func(idx int, p <-chan SessionEvent) {
        sessions := make(map[string]*SessionState)
        
        for event := range p {
            state, exists := sessions[event.SessionID]
            if !exists {
                state = NewSessionState()
                sessions[event.SessionID] = state
            }
            
            state.ProcessEvent(event)
            
            // Clean up expired sessions
            if state.IsExpired() {
                delete(sessions, event.SessionID)
            }
        }
    }(i, partition)
}
```

## Performance Considerations

1. **Number of Partitions**: More partitions enable more parallelism but increase memory usage
2. **Buffer Size**: Larger buffers reduce blocking but increase memory usage
3. **Key Distribution**: Ensure keys distribute evenly to avoid hot partitions
4. **Partitioner Function**: Keep partitioning logic simple and fast

## Best Practices

1. **Choose Keys Wisely**: Select partition keys that distribute data evenly
2. **Monitor Distribution**: Track partition statistics to detect imbalances
3. **Handle Shutdown**: Ensure all partition consumers handle graceful shutdown
4. **Test Key Distribution**: Verify your key function produces even distribution
5. **Consider Partition Count**: Balance between parallelism and resource usage

## Common Pitfalls

1. **Skewed Keys**: Some keys appearing much more frequently than others
2. **Too Many Partitions**: Creating more partitions than necessary
3. **Blocking Consumers**: Not consuming partitions fast enough
4. **Key Changes**: Changing partition keys can break ordering guarantees