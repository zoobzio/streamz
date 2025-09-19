# Sample Processor Implementation - v1

## Architecture Overview

Implemented the Sample processor for streamz - a simple probabilistic filter that randomly selects items from a stream based on a configurable rate.

### Core Design

**Type Definition:**
```go
type Sample[T any] struct {
    name string
    rate float64  // 0.0 to 1.0 probability
}
```

**Key Behavioral Contract:**
- Success items: Randomly selected based on rate using `rand.Float64() < rate`
- Error items: Always pass through unchanged (critical for error propagation)
- Empty streams: Handle gracefully (no items processed)
- Context cancellation: Immediate termination with proper cleanup

### Implementation Complexity Analysis

**Visible Complexity (Good):**
- Single probability comparison per item: `rand.Float64() < s.rate`
- Linear processing with O(1) per-item overhead
- Explicit error vs success handling paths
- Clear validation of rate bounds with panic for invalid inputs

**Hidden Complexity Avoided:**
- No statistical tracking or state accumulation
- No complex random number generation (uses Go's secure rand.Float64())
- No batching or windowing - each item independent
- No memory accumulation between items

**Maintenance Characteristics:**
- Zero state between items = no race conditions
- Deterministic behavior for edge cases (rate 0.0, 1.0)
- Testable with statistical validation for randomness
- Simple enough to verify all execution paths

### Unit Test Coverage Strategy

**Test Categories Implemented:**

1. **Constructor Tests:**
   - Valid rates (0.0, 0.1, 0.5, 0.9, 1.0)
   - Invalid rates (-0.1, 1.1, NaN, Inf) - all panic correctly
   - Default naming and custom naming

2. **Behavioral Tests:**
   - Rate 0.0: Zero items pass through (load shedding)
   - Rate 1.0: All items pass through (passthrough mode)
   - Error propagation: Always pass through regardless of rate

3. **Edge Case Tests:**
   - Empty streams
   - Context cancellation (immediate termination)
   - Mixed success/error scenarios

4. **Statistical Validation:**
   - 10,000 items at 30% rate within 5% variance
   - Verifies actual randomness (not deterministic)

5. **Real-World Scenarios:**
   - Load shedding (10% retention from 1000 items)
   - A/B testing (50/50 split)
   - Monitoring sampling (1% from 5000 items)

### Performance Profile

**Memory Usage:**
- Zero allocations in processing loop
- Fixed 24-byte struct (name string + rate float64)
- Channel operations only - no internal buffering

**CPU Usage:**
- Single rand.Float64() call per successful item
- No computation for error items (immediate passthrough)
- Context cancellation check per item (standard overhead)

**Concurrency Safety:**
- No shared mutable state
- Each processor instance independent
- Goroutine-per-processor model (standard streamz pattern)

### Dependencies Analysis

**Standard Library Only:**
- `context` - Required for cancellation (already in streamz)
- `math` - For NaN/Inf validation (minimal addition)
- `math/rand/v2` - Cryptographically secure randomness

**No External Dependencies:**
- No statistical libraries
- No complex random number generators
- Leverages Go's built-in secure randomness

### Integration Points

**Input Requirements:**
- `<-chan Result[T]` - Standard streamz input channel
- `context.Context` - Standard cancellation support

**Output Guarantees:**
- `<-chan Result[T]` - Standard streamz output channel
- Error items always forwarded
- Success items probabilistically forwarded
- Channel closes when input closes or context cancels

**Processor Interface Compliance:**
```go
Process(context.Context, <-chan Result[T]) <-chan Result[T]
Name() string
```

Plus sample-specific:
```go
Rate() float64  // For inspection/debugging
WithName(string) *Sample[T]  // For custom naming
```

### Error Handling Strategy

**Validation Errors (Panic):**
- Invalid rate ranges (< 0.0, > 1.0)
- Special float values (NaN, Inf)
- Immediate fail-fast during construction

**Runtime Errors (Pass-through):**
- All Result[T] errors forwarded unchanged
- No transformation or annotation of errors
- Maintains error context and metadata

**Context Cancellation:**
- Immediate goroutine termination
- Proper channel closing
- No resource leaks

### Testing Quality Gates

**All Tests Pass With:**
- Race detection (`-race` flag)
- Full coverage of public API
- Statistical validation of randomness
- Edge case coverage (empty, cancellation)
- Real-world usage scenarios

**Behavioral Verification:**
- Rate bounds enforced at construction
- Error propagation independent of rate
- Statistical behavior within expected variance
- Context cancellation immediate and clean

## Verification Complete

Sample processor implementation is production-ready:
- Simple, testable architecture
- Comprehensive unit test coverage
- Zero external dependencies beyond stdlib
- Performance optimized for high-volume streams
- VIPER-resistant (no hidden complexity or black boxes)

Ready for integration with other streamz processors.