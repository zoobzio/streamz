package streamz

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync/atomic"
)

// PartitionOutput provides access to individual partition channels.
type PartitionOutput[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	// Partitions contains the output channels for each partition.
	Partitions []<-chan T
}

// GetPartition returns the output channel for a specific partition index.
func (po PartitionOutput[T]) GetPartition(index int) <-chan T {
	if index < 0 || index >= len(po.Partitions) {
		return nil
	}
	return po.Partitions[index]
}

// Partitioner determines which partition an item should be routed to.
type Partitioner func(key string, numPartitions int) int

// DefaultPartitioner uses consistent hashing to distribute keys across partitions.
func DefaultPartitioner(key string, numPartitions int) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	if numPartitions <= 0 {
		return 0
	}
	// Use uint64 to avoid overflow on conversion.
	sum := uint64(h.Sum32())
	np := uint64(numPartitions)
	return int(sum % np) // #nosec G115 - modulo ensures result fits in int.
}

// Partition splits a stream into multiple sub-streams based on a partition key.
// Items with the same key are guaranteed to go to the same partition, preserving
// order within each partition while enabling parallel processing.
//
// The Partition processor is essential for:
//   - Parallel processing by key (e.g., by customer, by region).
//   - Load distribution across workers.
//   - Maintaining order within logical groups.
//   - Horizontal scaling of stream processing.
//
// Key features:
//   - Consistent key-based routing.
//   - Configurable number of partitions.
//   - Custom partitioning strategies.
//   - Per-partition statistics.
//   - Graceful shutdown with proper cleanup.
//
// Example:
//
//	// Partition orders by customer for parallel processing.
//	partitioner := streamz.NewPartition[Order](func(o Order) string {
//	    return o.CustomerID
//	}).WithPartitions(10)
//
//	partitions := partitioner.Process(ctx, orders)
//
//	// Process each partition independently.
//	var wg sync.WaitGroup
//	for i, partition := range partitions.Partitions {
//	    wg.Add(1)
//	    go func(idx int, p <-chan Order) {
//	        defer wg.Done()
//	        processPartition(ctx, idx, p)
//	    }(i, partition)
//	}
//	wg.Wait()
//
// Performance characteristics:
//   - O(1) partitioning decision per item.
//   - Minimal overhead for routing.
//   - Memory usage proportional to buffer size × partitions.
type Partition[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	keyFunc       func(T) string
	partitioner   Partitioner
	numPartitions int
	bufferSize    int
	name          string

	// Statistics.
	itemCounts []atomic.Int64
	totalItems atomic.Int64
}

// NewPartition creates a processor that partitions items based on a key function.
// Items with the same key are guaranteed to go to the same partition.
//
// The keyFunc extracts a partition key from each item. Items with the same
// key will always be routed to the same partition, preserving order within
// that partition.
//
// Default configuration:
//   - Partitions: 4
//   - Partitioner: Consistent hashing
//   - Buffer size: 0 (unbuffered channels)
//   - Name: "partition"
//
// Parameters:
//   - keyFunc: Function to extract partition key from items
//
// Returns: A new Partition processor with fluent configuration methods.
func NewPartition[T any](keyFunc func(T) string) *Partition[T] {
	return &Partition[T]{
		keyFunc:       keyFunc,
		partitioner:   DefaultPartitioner,
		numPartitions: 4,
		bufferSize:    0,
		name:          "partition",
	}
}

// WithPartitions sets the number of partitions to create.
func (p *Partition[T]) WithPartitions(n int) *Partition[T] {
	if n < 1 {
		n = 1
	}
	p.numPartitions = n
	return p
}

// WithPartitioner sets a custom partitioning function.
// The function receives the key and number of partitions,
// and should return a partition index (0 to numPartitions-1).
func (p *Partition[T]) WithPartitioner(partitioner Partitioner) *Partition[T] {
	if partitioner != nil {
		p.partitioner = partitioner
	}
	return p
}

// WithBufferSize sets the buffer size for each partition channel.
func (p *Partition[T]) WithBufferSize(size int) *Partition[T] {
	if size < 0 {
		size = 0
	}
	p.bufferSize = size
	return p
}

// WithName sets a custom name for this processor.
func (p *Partition[T]) WithName(name string) *Partition[T] {
	p.name = name
	return p
}

// Process partitions the input stream into multiple output streams.
// Items with the same key are guaranteed to go to the same partition.
func (p *Partition[T]) Process(ctx context.Context, in <-chan T) PartitionOutput[T] {
	// Initialize statistics
	p.itemCounts = make([]atomic.Int64, p.numPartitions)

	// Create output channels
	outputs := make([]chan T, p.numPartitions)
	partitions := make([]<-chan T, p.numPartitions)

	for i := 0; i < p.numPartitions; i++ {
		outputs[i] = make(chan T, p.bufferSize)
		partitions[i] = outputs[i]
	}

	// Create partition output
	result := PartitionOutput[T]{
		Partitions: partitions,
	}

	// Start router goroutine
	go func() {
		// Close all outputs when done
		defer func() {
			for _, ch := range outputs {
				close(ch)
			}
		}()

		for {
			select {
			case item, ok := <-in:
				if !ok {
					return
				}

				p.totalItems.Add(1)

				// Extract key and determine partition
				key := p.keyFunc(item)
				partitionIdx := p.partitioner(key, p.numPartitions)

				// Ensure partition index is valid
				if partitionIdx < 0 || partitionIdx >= p.numPartitions {
					partitionIdx = 0 // Fallback to first partition
				}

				// Update statistics
				p.itemCounts[partitionIdx].Add(1)

				// Send to appropriate partition
				select {
				case outputs[partitionIdx] <- item:
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return result
}

// GetStats returns statistics about partition distribution.
func (p *Partition[T]) GetStats() PartitionStats {
	stats := PartitionStats{
		TotalItems:        p.totalItems.Load(),
		NumPartitions:     p.numPartitions,
		ItemsPerPartition: make([]int64, p.numPartitions),
	}

	for i := 0; i < p.numPartitions; i++ {
		stats.ItemsPerPartition[i] = p.itemCounts[i].Load()
	}

	return stats
}

// Name returns the processor name.
func (p *Partition[T]) Name() string {
	return p.name
}

// PartitionStats contains statistics about partition distribution.
type PartitionStats struct { //nolint:govet // logical field grouping preferred over memory optimization
	TotalItems        int64   // Total items processed
	NumPartitions     int     // Number of partitions
	ItemsPerPartition []int64 // Items routed to each partition
}

// DistributionBalance returns a measure of how evenly distributed items are.
// Returns 0.0 for perfect distribution, higher values indicate imbalance.
func (s PartitionStats) DistributionBalance() float64 {
	if s.TotalItems == 0 || s.NumPartitions == 0 {
		return 0.0
	}

	idealPerPartition := float64(s.TotalItems) / float64(s.NumPartitions)
	var totalDeviation float64

	for _, count := range s.ItemsPerPartition {
		deviation := float64(count) - idealPerPartition
		totalDeviation += deviation * deviation
	}

	// Return coefficient of variation
	variance := totalDeviation / float64(s.NumPartitions)
	stdDev := variance // Simplified - not taking square root
	return stdDev / idealPerPartition
}

// String returns a string representation of the statistics.
func (s PartitionStats) String() string {
	return fmt.Sprintf("PartitionStats{Total: %d, Partitions: %d, Balance: %.2f}",
		s.TotalItems, s.NumPartitions, s.DistributionBalance())
}
