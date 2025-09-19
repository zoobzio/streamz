# Partition Connector Implementation Plan v2

## Overview

Implement a Partition connector that splits a single input channel into N output channels using configurable routing strategies. Fixed number of partitions (N) established at creation time with two routing modes: hash-based partitioning via key extraction function and round-robin distribution via rotating counter.

**Key Changes from v1:** Process method signature corrected, hash distribution improved, panic recovery added, input validation enhanced, default hash function specified, thread safety documented.

## Core Requirements

- **Fixed partition count**: N channels created in Process method, never modified during operation
- **Hash-based partitioning**: Extract key via function, route using improved hash distribution
- **Round-robin partitioning**: Distribute using rotating counter `counter % N`
- **Configurable buffer size**: Apply to all output channels uniformly
- **Simple design**: No unnecessary complexity, following AEGIS patterns
- **Process method pattern**: Channels created in Process, not constructor (matching Switch pattern)

## Architecture Analysis

### Reference Pattern: Switch Connector

Switch connector provides the multi-channel output pattern:
- Channels created in Process method, not constructor
- Result[T] routing with metadata preservation
- Context-aware goroutine management
- Graceful channel closure on completion
- Read-only channel slice return from Process

### Key Differences from Switch

| Aspect | Switch | Partition |
|--------|--------|-----------|
| Output channels | Dynamic creation by key | Fixed N channels in Process |
| Routing logic | Predicate evaluation | Hash or round-robin |
| Channel management | Lazy creation | Eager creation in Process |
| Key requirements | Comparable type K | Any type for hash key |

## Implementation Design

### Core Types

```go
// Partition splits single input into N output channels using configurable strategy
type Partition[T any] struct {
    strategy       PartitionStrategy[T]
    partitionCount int
    bufferSize     int
    name           string
}

// PartitionStrategy defines routing behavior
type PartitionStrategy[T any] interface {
    Route(value T, partitionCount int) int  // Returns partition index [0, N)
}

// HashPartition implements hash-based routing
type HashPartition[T any, K comparable] struct {
    keyExtractor func(T) K
    hasher       func(K) uint64  // Default: use Go's hash/fnv
}

// RoundRobinPartition implements counter-based routing
type RoundRobinPartition[T any] struct {
    counter uint64  // Atomic counter for thread safety
}

// PartitionConfig configures partition behavior
type PartitionConfig[T any] struct {
    PartitionCount int
    Strategy       PartitionStrategy[T]
    BufferSize     int  // Applied to all channels
}
```

### Constructor Functions

```go
// NewPartition creates a partition with custom strategy
func NewPartition[T any](config PartitionConfig[T]) (*Partition[T], error)

// NewHashPartition creates hash-based partition
func NewHashPartition[T any, K comparable](
    partitionCount int,
    keyExtractor func(T) K,
    bufferSize int,
) (*Partition[T], error)

// NewRoundRobinPartition creates round-robin partition
func NewRoundRobinPartition[T any](partitionCount int, bufferSize int) (*Partition[T], error)
```

### Process Method Signature (CORRECTED)

```go
// Process splits input across N output channels - channels created here, not in constructor
func (p *Partition[T]) Process(ctx context.Context, in <-chan Result[T]) []<-chan Result[T] {
    // Create output channels during Process call
    channels := make([]chan Result[T], p.partitionCount)
    for i := 0; i < p.partitionCount; i++ {
        channels[i] = make(chan Result[T], p.bufferSize)
    }

    // Convert to read-only slice for return
    out := make([]<-chan Result[T], p.partitionCount)
    for i, ch := range channels {
        out[i] = ch
    }

    go func() {
        defer func() {
            // Close all channels when processing completes
            for _, ch := range channels {
                close(ch)
            }
        }()

        // Main routing loop
        for {
            select {
            case <-ctx.Done():
                return
            case result, ok := <-in:
                if !ok {
                    return
                }
                p.routeResult(ctx, result, channels)
            }
        }
    }()

    return out
}
```

### Strategy Implementations (IMPROVED)

**Hash Strategy with Panic Recovery:**
```go
func (h *HashPartition[T, K]) Route(value T, partitionCount int) (idx int) {
    defer func() {
        if r := recover(); r != nil {
            idx = 0 // Route to partition 0 on panic
        }
    }()

    key := h.keyExtractor(value)  // Can panic - recovered above
    hash := h.hasher(key)         // Can panic - recovered above
    
    // Improved distribution - avoid modulo bias
    return int((hash * uint64(partitionCount)) >> 32)
}
```

**Round-Robin Strategy:**
```go
func (r *RoundRobinPartition[T]) Route(value T, partitionCount int) int {
    current := atomic.AddUint64(&r.counter, 1) - 1
    return int(current % uint64(partitionCount))
}
```

### Default Hash Function (NEW)

```go
import (
    "encoding/binary"
    "hash/fnv"
)

// defaultHasher provides FNV-1a hash for common comparable types
func defaultHasher[K comparable](key K) uint64 {
    h := fnv.New64a()
    
    switch v := any(key).(type) {
    case string:
        h.Write([]byte(v))
    case int:
        binary.Write(h, binary.LittleEndian, int64(v))
    case int64:
        binary.Write(h, binary.LittleEndian, v)
    case int32:
        binary.Write(h, binary.LittleEndian, int64(v))
    case uint64:
        binary.Write(h, binary.LittleEndian, v)
    case uint32:
        binary.Write(h, binary.LittleEndian, uint64(v))
    case float64:
        binary.Write(h, binary.LittleEndian, v)
    case float32:
        binary.Write(h, binary.LittleEndian, float64(v))
    default:
        // Fallback: convert to string and hash
        h.Write([]byte(fmt.Sprintf("%v", v)))
    }
    
    return h.Sum64()
}
```

## Channel Management (UPDATED)

### Initialization in Process Method
- Create exactly N channels during Process() call, not constructor
- Apply uniform buffer size to all channels
- No lazy creation - all channels exist from Process start
- Store in slice for O(1) index-based access
- Return read-only slice of channels

### Lifecycle
- Channels created in Process(), never in constructor
- Process() returns read-only slice of channels immediately
- All channels closed when processing completes or context cancels
- No dynamic route addition/removal (unlike Switch)

## Error Handling Strategy (ENHANCED)

### Error Routing
- All errors go to partition 0 for centralized error handling
- Maintains consistent error processing location
- Errors bypass strategy evaluation (no key extraction on errors)

### Panic Recovery (NEW)
- Wrap all user-provided functions in panic recovery:
  - keyExtractor function calls
  - hasher function calls
  - strategy.Route() calls
- Convert panics to route to partition 0 (error partition)
- Preserve stack traces in error metadata

### Input Validation (ENHANCED)
```go
func validateConfig[T any](config PartitionConfig[T]) error {
    if config.PartitionCount <= 0 {
        return fmt.Errorf("partition count must be > 0, got %d", config.PartitionCount)
    }
    if config.BufferSize < 0 {
        return fmt.Errorf("buffer size must be >= 0, got %d", config.BufferSize)
    }
    if config.Strategy == nil {
        return fmt.Errorf("strategy cannot be nil")
    }
    return nil
}

func validateHashConfig[T any, K comparable](
    partitionCount int,
    keyExtractor func(T) K,
    bufferSize int,
) error {
    if partitionCount <= 0 {
        return fmt.Errorf("partition count must be > 0, got %d", partitionCount)
    }
    if bufferSize < 0 {
        return fmt.Errorf("buffer size must be >= 0, got %d", bufferSize)
    }
    if keyExtractor == nil {
        return fmt.Errorf("key extractor cannot be nil")
    }
    return nil
}
```

## Metadata Integration

### Partition Metadata
Add partition-specific metadata to routed Results:
```go
const (
    MetadataPartitionIndex    = "partition_index"    // int - target partition [0, N)
    MetadataPartitionTotal    = "partition_total"    // int - total partition count N
    MetadataPartitionStrategy = "partition_strategy" // string - "hash" or "round_robin"
)
```

### Metadata Preservation
- Preserve existing Result[T] metadata through routing
- Add partition metadata using Result[T].WithMetadata()
- Include standard processor metadata (processor, timestamp)

## Concurrency Design (DOCUMENTED)

### Thread Safety Requirements
- **keyExtractor functions must be pure**: No side effects, no shared mutable state
- **hasher functions must be pure**: No side effects, no shared mutable state
- **Strategy.Route() implementations must be thread-safe**: Multiple goroutines may call concurrently
- Atomic operations used in RoundRobinPartition for lock-free operation
- Channel slice is read-only after Process() creates it

### Performance Considerations
- Slice-based channel access: O(1) routing performance
- Atomic counter for round-robin: lock-free operation
- FNV-1a hash function: fast with good distribution properties
- Fixed channel count eliminates dynamic allocation
- Improved hash distribution avoids modulo bias

## Testing Strategy (EXPANDED)

### Unit Test Coverage
```go
// partition_test.go - Basic functionality
func TestPartition_HashRouting(t *testing.T)
func TestPartition_RoundRobinRouting(t *testing.T)
func TestPartition_ErrorHandling(t *testing.T)
func TestPartition_ContextCancellation(t *testing.T)
func TestPartition_MetadataPreservation(t *testing.T)

// Strategy-specific tests
func TestHashPartition_ConsistentRouting(t *testing.T)
func TestHashPartition_DistributionQuality(t *testing.T)  // NEW
func TestRoundRobinPartition_EvenDistribution(t *testing.T)

// Edge cases
func TestPartition_SinglePartition(t *testing.T)
func TestPartition_StrategyPanic(t *testing.T)             // NEW
func TestPartition_KeyExtractorPanic(t *testing.T)         // NEW
func TestPartition_HasherPanic(t *testing.T)               // NEW
func TestPartition_ConfigValidation(t *testing.T)

// Concurrency tests (NEW)
func TestPartition_ConcurrentRouting(t *testing.T)
func TestPartition_BufferOverflow(t *testing.T)
func TestPartition_ContextDuringRoute(t *testing.T)

// Hash distribution tests (NEW)
func TestDefaultHasher_CommonTypes(t *testing.T)
func TestHashPartition_NoModuloBias(t *testing.T)
```

### Test Data Patterns
- Verify hash consistency: same key → same partition across multiple calls
- Verify improved hash distribution over large sample sizes
- Test error routing to partition 0 in all scenarios
- Validate metadata attachment and preservation
- Test panic recovery for all user-provided functions
- Verify concurrent access safety

## Implementation Files

### Core Implementation
- `partition.go`: Main Partition struct and Process method
- `partition_test.go`: Unit tests for partition functionality

### Dependencies
- `result.go`: Result[T] type for routing
- `error.go`: StreamError for error handling
- Standard library: `context`, `sync`, `hash/fnv`, `encoding/binary`

## Complexity Evaluation

### Necessary Complexity (PRESERVED)
- **Fixed partition count**: Enables predictable resource allocation
- **Strategy pattern**: Supports both hash and round-robin without code duplication
- **Thread safety**: Required for concurrent producer/consumer scenarios
- **Error routing**: Centralizes error handling in predictable location
- **Metadata preservation**: Maintains tracing capabilities through partitioning
- **Panic recovery**: Prevents user function panics from crashing processor
- **Input validation**: Prevents runtime errors from invalid configurations

### Rejected Complexity
- Dynamic partition addition/removal (not in requirements)
- Sophisticated load balancing (round-robin sufficient)
- Partition rebalancing (fixed N eliminates need)
- Backpressure management (handled by channel buffer sizes)
- Complex hash algorithms (FNV-1a sufficient for most use cases)

## Quality Standards Compliance

- **Linter compliance**: Follow golangci-lint rules without exceptions
- **Test coverage**: >80% coverage on partition.go
- **File structure**: Standard AEGIS package layout
- **Godoc**: Document all public functions with decision rationale
- **Error handling**: Explicit error returns, no silent failures
- **Performance**: O(1) routing, lock-free where possible
- **Thread safety**: Pure functions required, documented clearly

## Implementation Notes

This design incorporates RAINMAN's review findings:

### Mumbai Lessons Applied
- Simple, predictable routing logic without over-abstraction
- Fixed resource allocation eliminates runtime scaling issues
- Clear separation between hash and round-robin strategies
- Consistent error handling patterns
- Panic recovery prevents user code from crashing system

### RAINMAN Fixes Implemented
1. **Process signature corrected**: Channels created in Process method, not constructor
2. **Hash distribution improved**: Using `(hash * N) >> 32` to avoid modulo bias
3. **Panic recovery added**: All user functions wrapped in defer/recover
4. **Input validation enhanced**: All parameters validated with clear error messages
5. **Default hash function specified**: FNV-1a with common type optimizations
6. **Thread safety documented**: Clear requirements for user-provided functions

### Pattern Consistency
- Follows Switch connector pattern for Process method signature
- Maintains AEGIS metadata conventions
- Uses Result[T] error propagation consistently
- Implements standard processor lifecycle

The partition count N is fixed at creation to prevent operational complexity. Applications requiring dynamic partitioning should manage multiple Partition instances rather than adding runtime complexity to the connector itself.

**Thread Safety Note**: All user-provided functions (keyExtractor, hasher, custom strategies) MUST be pure functions with no side effects or shared mutable state. This is a requirement for correct concurrent operation and is documented in the API.