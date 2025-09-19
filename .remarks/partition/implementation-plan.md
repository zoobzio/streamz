# Partition Connector Implementation Plan

## Overview

Implement a Partition connector that splits a single input channel into N output channels using configurable routing strategies. Fixed number of partitions (N) established at creation time with two routing modes: hash-based partitioning via key extraction function and round-robin distribution via rotating counter.

## Core Requirements

- **Fixed partition count**: N channels created at initialization, never modified during operation
- **Hash-based partitioning**: Extract key via function, route using `hash(key) % N`
- **Round-robin partitioning**: Distribute using rotating counter `counter % N`
- **Configurable buffer size**: Apply to all output channels uniformly
- **Simple design**: No unnecessary complexity, following AEGIS patterns

## Architecture Analysis

### Reference Pattern: Switch Connector

Switch connector provides the multi-channel output pattern:
- Map-based channel management with mutex protection
- Channel creation at initialization with fixed buffer sizes
- Result[T] routing with metadata preservation
- Context-aware goroutine management
- Graceful channel closure on completion

### Key Differences from Switch

| Aspect | Switch | Partition |
|--------|--------|-----------|
| Output channels | Dynamic creation by key | Fixed N channels at init |
| Routing logic | Predicate evaluation | Hash or round-robin |
| Channel management | Lazy creation | Eager creation |
| Key requirements | Comparable type K | Any type for hash key |

## Implementation Design

### Core Types

```go
// Partition splits single input into N output channels using configurable strategy
type Partition[T any] struct {
    strategy    PartitionStrategy[T]
    channels    []chan Result[T]    // Fixed-size slice, N channels
    bufferSize  int
    mu          sync.RWMutex        // Protects during shutdown only
    name        string
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
func NewPartition[T any](config PartitionConfig[T]) *Partition[T]

// NewHashPartition creates hash-based partition
func NewHashPartition[T any, K comparable](
    partitionCount int,
    keyExtractor func(T) K,
    bufferSize int,
) *Partition[T]

// NewRoundRobinPartition creates round-robin partition
func NewRoundRobinPartition[T any](partitionCount int, bufferSize int) *Partition[T]
```

### Processing Logic

```go
// Process splits input across N output channels
func (p *Partition[T]) Process(ctx context.Context, in <-chan Result[T]) []<-chan Result[T]

// Core routing logic:
// 1. For errors: Route to partition 0 (consistent error handling)
// 2. For success: Use strategy.Route() to determine partition
// 3. Add partition metadata to Result[T]
// 4. Send to selected channel with context cancellation
```

### Strategy Implementations

**Hash Strategy:**
```go
func (h *HashPartition[T, K]) Route(value T, partitionCount int) int {
    key := h.keyExtractor(value)
    hash := h.hasher(key)
    return int(hash % uint64(partitionCount))
}
```

**Round-Robin Strategy:**
```go
func (r *RoundRobinPartition[T]) Route(value T, partitionCount int) int {
    current := atomic.AddUint64(&r.counter, 1) - 1
    return int(current % uint64(partitionCount))
}
```

## Channel Management

### Initialization
- Create exactly N channels during construction
- Apply uniform buffer size to all channels
- No lazy creation - all channels exist from start
- Store in slice for O(1) index-based access

### Lifecycle
- Channels created in constructor, never modified
- Process() returns read-only slice of channels
- All channels closed when processing completes
- No dynamic route addition/removal (unlike Switch)

## Error Handling Strategy

### Error Routing
- All errors go to partition 0 for centralized error handling
- Maintains consistent error processing location
- Errors bypass strategy evaluation (no key extraction on errors)

### Panic Recovery
- Wrap strategy.Route() calls in panic recovery
- Convert panics to StreamError with partition context
- Route panic-generated errors to partition 0

### Input Validation
- Validate partitionCount > 0 in constructors
- Return errors for invalid configuration
- Nil strategy checks in constructors

## Metadata Integration

### Partition Metadata
Add partition-specific metadata to routed Results:
```go
const (
    MetadataPartitionIndex = "partition_index"  // int - target partition [0, N)
    MetadataPartitionTotal = "partition_total"  // int - total partition count N
    MetadataPartitionStrategy = "partition_strategy" // string - "hash" or "round_robin"
)
```

### Metadata Preservation
- Preserve existing Result[T] metadata through routing
- Add partition metadata using Result[T].WithMetadata()
- Include standard processor metadata (processor, timestamp)

## Concurrency Design

### Thread Safety
- Strategy.Route() calls protected by atomic operations where needed
- No shared mutable state between goroutines
- Channel slice is read-only after initialization
- Mutex only for coordinated shutdown

### Performance Considerations
- Slice-based channel access: O(1) routing performance
- Atomic counter for round-robin: lock-free operation
- Hash function selection impacts performance (FNV vs crypto hashes)
- Fixed channel count eliminates dynamic allocation

## Testing Strategy

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
func TestRoundRobinPartition_EvenDistribution(t *testing.T)

// Edge cases
func TestPartition_SinglePartition(t *testing.T)
func TestPartition_StrategyPanic(t *testing.T)
func TestPartition_ConfigValidation(t *testing.T)
```

### Test Data Patterns
- Verify hash consistency: same key → same partition
- Verify round-robin distribution over many values
- Test error routing to partition 0
- Validate metadata attachment and preservation

## Implementation Files

### Core Implementation
- `partition.go`: Main Partition struct and Process method
- `partition_test.go`: Unit tests for partition functionality

### Dependencies
- `result.go`: Result[T] type for routing
- `error.go`: StreamError for error handling
- Standard library: `context`, `sync`, `hash/fnv`

## Complexity Evaluation

### Necessary Complexity
- **Fixed partition count**: Enables predictable resource allocation
- **Strategy pattern**: Supports both hash and round-robin without code duplication
- **Thread safety**: Required for concurrent producer/consumer scenarios
- **Error routing**: Centralizes error handling in predictable location
- **Metadata preservation**: Maintains tracing capabilities through partitioning

### Rejected Complexity
- Dynamic partition addition/removal (not in requirements)
- Custom hash function injection (use standard FNV)
- Sophisticated load balancing (round-robin sufficient)
- Partition rebalancing (fixed N eliminates need)
- Backpressure management (handled by channel buffer sizes)

## Quality Standards Compliance

- **Linter compliance**: Follow golangci-lint rules without exceptions
- **Test coverage**: >80% coverage on partition.go
- **File structure**: Standard AEGIS package layout
- **Godoc**: Document all public functions with decision rationale
- **Error handling**: Explicit error returns, no silent failures
- **Performance**: O(1) routing, lock-free where possible

## Implementation Notes

This design follows Mumbai lessons:
- Simple, predictable routing logic
- No abstract factories or strategy injection complexity
- Fixed resource allocation eliminates runtime scaling issues
- Clear separation between hash and round-robin strategies
- Consistent error handling patterns

The partition count N is fixed at creation to prevent operational complexity. Applications requiring dynamic partitioning should manage multiple Partition instances rather than adding runtime complexity to the connector itself.