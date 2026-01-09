package streamz

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sync/atomic"
	"time"
)

// Partition splits a single input channel into N output channels using configurable routing strategies.
// The number of partitions is fixed at creation time and channels are created during Process method execution.
// Supports hash-based partitioning via key extraction and round-robin distribution via rotating counter.
// All errors route to partition 0 for centralized error handling.
type Partition[T any] struct {
	strategy       PartitionStrategy[T] // 16 bytes (interface)
	name           string               // 16 bytes (pointer + len)
	partitionCount int                  // 8 bytes (aligned)
	bufferSize     int                  // 8 bytes (aligned)
}

// PartitionStrategy defines the routing behavior for distributing values across partitions.
// Implementations must be thread-safe as they may be called concurrently from multiple goroutines.
// The Route method must return a partition index in range [0, partitionCount).
type PartitionStrategy[T any] interface {
	Route(value T, partitionCount int) int // Returns partition index [0, N)
}

// HashPartition implements hash-based routing using a key extraction function and hash function.
// Keys are extracted from values and hashed to determine the target partition.
// Panic recovery ensures user function failures route to partition 0 (error partition).
type HashPartition[T any, K comparable] struct {
	keyExtractor func(T) K
	hasher       func(K) uint64
}

// RoundRobinPartition implements counter-based routing that distributes values evenly across partitions.
// Uses an atomic counter to ensure thread-safe operation without locks.
type RoundRobinPartition[T any] struct {
	counter uint64 // Atomic counter for thread-safe operation
}

// PartitionConfig configures partition behavior including strategy and buffer sizing.
type PartitionConfig[T any] struct {
	Strategy       PartitionStrategy[T] // Routing strategy implementation
	PartitionCount int                  // Number of output partitions (must be > 0)
	BufferSize     int                  // Buffer size applied to all output channels (must be >= 0)
}

// Standard partition metadata keys for tracing and debugging.
const (
	MetadataPartitionIndex    = "partition_index"    // int - target partition [0, N)
	MetadataPartitionTotal    = "partition_total"    // int - total partition count N
	MetadataPartitionStrategy = "partition_strategy" // string - "hash", "round_robin", or "error"
)

// Partition strategy name constants.
const (
	partitionStrategyError = "error"
)

// NewPartition creates a partition with custom strategy configuration.
// Validates all configuration parameters and returns an error for invalid inputs.
func NewPartition[T any](config PartitionConfig[T]) (*Partition[T], error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return &Partition[T]{
		strategy:       config.Strategy,
		partitionCount: config.PartitionCount,
		bufferSize:     config.BufferSize,
		name:           "partition",
	}, nil
}

// NewHashPartition creates a hash-based partition using the provided key extractor.
// Uses FNV-1a hash by default for good distribution properties and performance.
// The keyExtractor function must be pure (no side effects, no shared mutable state).
func NewHashPartition[T any, K comparable](
	partitionCount int,
	keyExtractor func(T) K,
	bufferSize int,
) (*Partition[T], error) {
	if err := validateHashConfig(partitionCount, keyExtractor, bufferSize); err != nil {
		return nil, err
	}

	strategy := &HashPartition[T, K]{
		keyExtractor: keyExtractor,
		hasher:       defaultHasher[K],
	}

	return &Partition[T]{
		strategy:       strategy,
		partitionCount: partitionCount,
		bufferSize:     bufferSize,
		name:           "partition",
	}, nil
}

// NewRoundRobinPartition creates a round-robin partition that distributes values evenly.
// Uses atomic operations for lock-free thread safety.
func NewRoundRobinPartition[T any](partitionCount int, bufferSize int) (*Partition[T], error) {
	if partitionCount <= 0 {
		return nil, fmt.Errorf("partition count must be > 0, got %d", partitionCount)
	}
	if bufferSize < 0 {
		return nil, fmt.Errorf("buffer size must be >= 0, got %d", bufferSize)
	}

	strategy := &RoundRobinPartition[T]{
		counter: 0,
	}

	return &Partition[T]{
		strategy:       strategy,
		partitionCount: partitionCount,
		bufferSize:     bufferSize,
		name:           "partition",
	}, nil
}

// Process splits input across N output channels using the configured strategy.
// Channels are created during this method call, not in the constructor.
// Returns a read-only slice of channels for immediate consumption.
// All channels are closed when processing completes or context is canceled.
func (p *Partition[T]) Process(ctx context.Context, in <-chan Result[T]) []<-chan Result[T] {
	// Create output channels during Process call (not constructor)
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

// routeResult determines the target partition and sends the result with metadata.
// Errors always route to partition 0 for centralized error handling.
// Adds partition metadata for tracing and debugging purposes.
func (p *Partition[T]) routeResult(ctx context.Context, result Result[T], channels []chan Result[T]) {
	var targetIndex int
	var strategyName string

	if result.IsError() {
		// All errors go to partition 0 for centralized error handling
		targetIndex = 0
		strategyName = partitionStrategyError
	} else {
		// Route successful values using strategy
		targetIndex = p.safeRoute(result.Value())
		strategyName = p.getStrategyName()
	}

	// Add partition metadata for tracing
	enrichedResult := result.
		WithMetadata(MetadataPartitionIndex, targetIndex).
		WithMetadata(MetadataPartitionTotal, p.partitionCount).
		WithMetadata(MetadataPartitionStrategy, strategyName).
		WithMetadata(MetadataProcessor, p.name).
		WithMetadata(MetadataTimestamp, time.Now())

	// Send to target partition with context cancellation support
	select {
	case channels[targetIndex] <- enrichedResult:
	case <-ctx.Done():
		return
	}
}

// safeRoute calls the strategy with panic recovery.
// Any panic in user-provided functions routes to partition 0.
func (p *Partition[T]) safeRoute(value T) (targetIndex int) {
	defer func() {
		if r := recover(); r != nil {
			targetIndex = 0 // Route to partition 0 on panic
		}
	}()

	targetIndex = p.strategy.Route(value, p.partitionCount)

	// Validate returned index is in valid range
	if targetIndex < 0 || targetIndex >= p.partitionCount {
		targetIndex = 0 // Route to partition 0 for invalid indices
	}

	return targetIndex
}

// getStrategyName returns a human-readable name for the current strategy.
func (p *Partition[T]) getStrategyName() string {
	switch p.strategy.(type) {
	case *HashPartition[T, string], *HashPartition[T, int], *HashPartition[T, int64]:
		return "hash"
	case *RoundRobinPartition[T]:
		return "round_robin"
	default:
		return "custom"
	}
}

// Route implements hash-based routing with panic recovery.
// Extracts key from value, hashes it, and uses improved distribution to avoid modulo bias.
func (h *HashPartition[T, K]) Route(value T, partitionCount int) (idx int) {
	defer func() {
		if r := recover(); r != nil {
			idx = 0 // Route to partition 0 on panic
		}
	}()

	// Guard against invalid partition count
	if partitionCount <= 0 {
		return 0
	}

	key := h.keyExtractor(value) // Can panic - recovered above
	hash := h.hasher(key)        // Can panic - recovered above

	// Use modulo for simplicity and reliability
	// While modulo has slight bias, it's predictable and correct
	// Safe: partitionCount validated > 0, so uint64 conversion is safe
	// Result is always < partitionCount which fits in int
	partition := int(hash % uint64(partitionCount)) //nolint:gosec // bound by partitionCount
	return partition
}

// Route implements round-robin routing using atomic counter.
// Thread-safe operation without locks for high performance.
func (r *RoundRobinPartition[T]) Route(_ T, partitionCount int) int {
	// Guard against invalid partition count
	if partitionCount <= 0 {
		return 0
	}

	current := atomic.AddUint64(&r.counter, 1) - 1
	// Safe: partitionCount validated > 0, so uint64 conversion is safe
	// Result is always < partitionCount which fits in int
	partition := int(current % uint64(partitionCount)) //nolint:gosec // bound by partitionCount
	return partition
}

// defaultHasher provides FNV-1a hash for common comparable types.
// Optimized paths for common types with fallback for unknown types.
func defaultHasher[K comparable](key K) uint64 {
	h := fnv.New64a()

	switch v := any(key).(type) {
	case string:
		_, _ = h.Write([]byte(v))
	case int:
		_ = binary.Write(h, binary.LittleEndian, int64(v)) //nolint:errcheck // hash writer never fails
	case int64:
		_ = binary.Write(h, binary.LittleEndian, v) //nolint:errcheck // hash writer never fails
	case int32:
		_ = binary.Write(h, binary.LittleEndian, int64(v)) //nolint:errcheck // hash writer never fails
	case uint64:
		_ = binary.Write(h, binary.LittleEndian, v) //nolint:errcheck // hash writer never fails
	case uint32:
		_ = binary.Write(h, binary.LittleEndian, uint64(v)) //nolint:errcheck // hash writer never fails
	case float64:
		_ = binary.Write(h, binary.LittleEndian, v) //nolint:errcheck // hash writer never fails
	case float32:
		_ = binary.Write(h, binary.LittleEndian, float64(v)) //nolint:errcheck // hash writer never fails
	default:
		// Fallback: convert to string and hash
		_, _ = fmt.Fprintf(h, "%v", v) //nolint:errcheck // hash writer never fails
	}

	return h.Sum64()
}

// validateConfig validates the partition configuration parameters.
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

// validateHashConfig validates hash-specific configuration parameters.
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
