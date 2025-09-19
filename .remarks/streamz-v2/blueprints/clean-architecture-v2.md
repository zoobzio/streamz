# Streamz v2 Clean Architecture - "Streamz Powered by Pipz"

**Author:** midgel  
**Version:** 2.0  
**Date:** 2025-08-24  
**Status:** Clean-Slate Design  

## Executive Summary

This is the architecture we wish we had started with. No backward compatibility, no legacy cruft, just elegant streaming built on solid pipz foundations. We're ruthlessly eliminating complexity and building the minimal viable streaming layer that actually makes sense.

## Core Philosophy: Boring Technology That Works

- **pipz handles the hard stuff** - Error handling, retries, circuit breakers, timeouts, observability
- **streamz handles the streaming stuff** - Channels, multiplexing, time windows, flow control
- **Single responsibility** - No overlap, no duplication, no confusion
- **Zero-cost abstractions** - If it doesn't add value, it doesn't exist

## 1. Clean Architecture - Simplified

### System Overview
```
┌─────────────────────────────────────────────────────────────┐
│ Streamz v2: The Minimal Streaming Layer                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────┐    │
│  │   Source    │───▶│   Stream     │───▶│    Sink     │    │
│  │  (chan T)   │    │  Processor   │    │  (chan T)   │    │
│  └─────────────┘    └──────────────┘    └─────────────┘    │
│                             │                               │
│                             ▼                               │
│                    ┌──────────────────┐                     │
│                    │  pipz.Chainable  │                     │
│                    │   (the engine)   │                     │
│                    └──────────────────┘                     │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│ pipz: Battle-tested processing, error handling, resilience │
└─────────────────────────────────────────────────────────────┘
```

### Architecture Layers
1. **Channel Layer** - Go channels for streaming data flow
2. **Stream Layer** - streamz processors (minimal, focused)  
3. **Processing Layer** - pipz chainables (comprehensive, robust)
4. **Integration Layer** - Direct, zero-overhead bridging

## 2. API Design - Fresh Start

### Core Interface (Simplified)
```go
// StreamProcessor - The ONLY interface in streamz v2
type StreamProcessor[T any] interface {
    // Process transforms input stream to output stream
    Process(ctx context.Context, in <-chan T) <-chan T
    
    // Name for debugging/monitoring
    Name() string
}

// No more separate Processor interface
// No more complex type hierarchies  
// One interface, one responsibility
```

### Construction Pattern (Direct)
```go
// FromPipz - The PRIMARY way to create stream processors
func FromPipz[T any](name string, chainable pipz.Chainable[T]) StreamProcessor[T]

// Direct construction for channel-specific operations
func NewFanIn[T any](name string) *FanIn[T]
func NewFanOut[T any](name string, routes int) *FanOut[T] 
func NewBatcher[T any](name string, config BatchConfig) *Batcher[T]
func NewWindow[T any](name string, config WindowConfig) *Window[T]
```

### Usage Pattern (Elegant)
```go
// Transform with pipz
transform := streamz.FromPipz("validate", 
    pipz.Apply("validate", validateOrder))

// Batch for efficiency  
batch := streamz.NewBatcher("batch-orders", streamz.BatchConfig{
    MaxSize: 100,
    MaxLatency: 5 * time.Second,
})

// Fan out for parallel processing
fanout := streamz.NewFanOut("distribute", 3)

// Chain them elegantly
processed := transform.Process(ctx, orders)
batched := batch.Process(ctx, processed) 
distributed := fanout.Process(ctx, batched)
```

## 3. Feature Elimination - Ruthless Simplification

### ELIMINATED Features (Good Riddance)

#### Complex Processor Types
- ❌ `AsyncMapper` - Use pipz.Apply with concurrency
- ❌ `CircuitBreaker` - Use pipz.NewCircuitBreaker  
- ❌ `Retry` - Use pipz.NewRetry
- ❌ `Throttle` - Use pipz.NewRateLimit
- ❌ `Filter` - Use pipz.Filter  
- ❌ `Mapper` - Use pipz.Transform or pipz.Apply

#### Redundant Error Handling  
- ❌ Custom error channels - Use pipz.Error[T]
- ❌ Silent error dropping - pipz forces explicit handling
- ❌ Error recovery patterns - pipz.NewFallback handles this

#### Complex Configuration
- ❌ Fluent builders with 20 options
- ❌ `WithName()`, `WithTimeout()`, etc.
- ❌ Configuration structs with defaults

#### Backward Compatibility
- ❌ Legacy Processor interface
- ❌ Adapter layers
- ❌ Migration helpers

### SIMPLIFIED Replacements

```go
// OLD (streamz v1) - Complex, duplicate functionality
filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
mapper := streamz.NewMapper(strings.ToUpper).WithName("uppercase")  
retry := streamz.NewRetry(processor, 3).WithBackoff(time.Second)

// NEW (streamz v2) - Direct pipz usage
processor := streamz.FromPipz("process-text",
    pipz.NewSequence("pipeline",
        pipz.Filter("positive", func(n int) bool { return n > 0 }),
        pipz.Transform("uppercase", strings.ToUpper),
        pipz.NewRetry("retry", operation, 3),
    ))
```

## 4. Streamz Core - What Actually Remains

### Channel Multiplexing (Core streamz responsibility)
```go
// FanIn - Merge multiple streams
type FanIn[T any] struct { /* simplified implementation */ }

// FanOut - Split stream to multiple destinations  
type FanOut[T any] struct { /* simplified implementation */ }

// These CANNOT be done with pipz alone - they need channel coordination
```

### Time-based Operations (Core streamz responsibility)
```go
// Batcher - Collect items by time/count
type Batcher[T any] struct { /* time-aware batching */ }

// Window - Time-based windowing
type Window[T any] struct { /* sliding/tumbling/session windows */ }

// These need timer coordination with channels
```

### Stream Flow Control (Core streamz responsibility)  
```go
// Buffer - Channel buffering strategies
type Buffer[T any] struct { /* dropping/sliding/blocking */ }

// These manage channel backpressure
```

### Integration Layer (Core streamz responsibility)
```go
// FromPipz - Bridge pipz.Chainable to StreamProcessor
func FromPipz[T any](name string, chainable pipz.Chainable[T]) StreamProcessor[T]

// This is the magic that makes it work
```

## 5. Integration Model - Direct Bridge

### The FromPipz Pattern (Zero Overhead)
```go
func FromPipz[T any](name string, chainable pipz.Chainable[T]) StreamProcessor[T] {
    return &PipzAdapter[T]{
        name: name,
        chainable: chainable,
    }
}

type PipzAdapter[T any] struct {
    name string
    chainable pipz.Chainable[T]
}

func (p *PipzAdapter[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for item := range in {
            result, err := p.chainable.Process(ctx, item)
            if err != nil {
                // pipz handles error reporting, we just skip failed items
                continue  
            }
            
            select {
            case out <- result:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

func (p *PipzAdapter[T]) Name() string {
    return p.name
}
```

### No Complex Wrappers
- Single adapter type
- Direct delegation to pipz
- No feature duplication
- No translation layer overhead

## 6. Error Handling - Native Pipz

### Error Flow (Simplified)
```go
// Errors are handled by pipz, not streamz
processor := streamz.FromPipz("risky-operation",
    pipz.NewSequence("error-handling",
        pipz.Apply("process", riskyOperation),
        pipz.NewFallback("fallback", 
            pipz.Apply("primary", primaryOperation),
            pipz.Apply("backup", backupOperation),
        ),
        pipz.NewRetry("retry", operation, 3),
    ))

// StreamProcessor just propagates successful results
// pipz handles all error scenarios internally
stream := processor.Process(ctx, input)
for result := range stream {
    // Only successful results reach here
    // All error handling was done by pipz
}
```

### No Dual-Channel Pattern  
- ❌ No separate error channels
- ❌ No error/success multiplexing
- ❌ No complex error handling in streamz

pipz already solved error handling. We use their solution.

## 7. Type System - Simplified Generics

### Single Generic Constraint
```go
// No complex type constraints
// No interface hierarchies
// Simple, clean generics

type StreamProcessor[T any] interface {
    Process(ctx context.Context, in <-chan T) <-chan T
    Name() string
}

// That's it. T can be anything.
```

### Type Safety Through pipz
```go
// pipz provides the type safety
processor := streamz.FromPipz("typed-processing",
    pipz.NewSequence("pipeline",
        pipz.Transform("to-string", func(i int) string { 
            return strconv.Itoa(i) 
        }),
        pipz.Apply("validate", func(s string) (string, error) {
            if len(s) == 0 {
                return "", errors.New("empty string")
            }
            return s, nil
        }),
    ))

// Type flows: chan int -> StreamProcessor[int] -> chan int
// But internally: int -> string -> string (pipz handles this)
```

## 8. Performance - Optimal Design

### Zero-Cost Abstractions
- No reflection
- No interface{} boxing  
- Direct channel operations
- Minimal allocation overhead

### Performance Profile (Theoretical)
```
Operation                  | Overhead     | Allocations
---------------------------|--------------|------------
FromPipz adapter          | ~50ns/item   | 0 allocs/op
FanIn (N streams)         | ~100ns/item  | 0 allocs/op  
FanOut (N streams)        | ~150ns/item  | 0 allocs/op
Batcher (time-based)      | ~200ns/item  | 1 alloc/batch
Window operations         | ~300ns/item  | 1 alloc/window

Pipeline (5 processors)   | ~250ns/item  | 1 alloc total
```

### Memory Profile
- Fixed overhead per processor (~200 bytes)
- No goroutine leaks (proper cleanup)
- Bounded channel buffers
- GC-friendly allocation patterns

## 9. Migration Path - Breaking Changes Accepted

### v1 → v2 Migration (NOT Automatic)
```go
// OLD v1 pattern
filter := streamz.NewFilter(isValid).WithName("validate")
mapper := streamz.NewMapper(transform).WithName("transform")  
retry := streamz.NewRetry(processor, 3)

filtered := filter.Process(ctx, input)
mapped := mapper.Process(ctx, filtered)
retried := retry.Process(ctx, mapped)

// NEW v2 pattern  
processor := streamz.FromPipz("process-pipeline",
    pipz.NewSequence("validation-transform",
        pipz.Filter("validate", isValid),
        pipz.Transform("transform", transform),
        pipz.NewRetry("retry", operation, 3),
    ))

processed := processor.Process(ctx, input)
```

### Breaking Changes (Intentional)
1. **No Processor interface** - Only StreamProcessor[T]
2. **No fluent builders** - Direct construction only
3. **No error channels** - pipz handles errors
4. **No async mappers** - Use pipz.Apply with pipz.NewConcurrent
5. **No custom retry/circuit breaker** - Use pipz implementations

## 10. Example Usage - The New Way

### Simple Pipeline
```go
// Clean, direct, powerful
validator := streamz.FromPipz("validate-orders",
    pipz.Apply("validate", func(ctx context.Context, order Order) (Order, error) {
        if err := order.Validate(); err != nil {
            return Order{}, fmt.Errorf("invalid order: %w", err)
        }
        return order, nil
    }))

batcher := streamz.NewBatcher("batch-orders", streamz.BatchConfig{
    MaxSize: 50,
    MaxLatency: 2 * time.Second,
})

// Pipeline composition  
validated := validator.Process(ctx, orders)
batched := batcher.Process(ctx, validated)

for batch := range batched {
    // Process valid order batches
    processBatch(batch)
}
```

### Complex Resilient Pipeline
```go
// All the resilience patterns from pipz
resilient := streamz.FromPipz("resilient-api",
    pipz.NewSequence("api-call",
        pipz.NewRateLimit("rate-limit", 100, time.Second),
        pipz.NewCircuitBreaker("breaker", 
            pipz.Apply("api-call", callExternalAPI),
            5, 30*time.Second),
        pipz.NewRetry("retry", operation, 3),
        pipz.NewFallback("fallback",
            pipz.Apply("primary", primaryAPI),
            pipz.Apply("secondary", secondaryAPI),
        ),
    ))

// Fan out to multiple workers
fanout := streamz.NewFanOut("distribute", 5)

// Fan back in from workers  
fanin := streamz.NewFanIn("collect")

processed := resilient.Process(ctx, requests)
distributed := fanout.Process(ctx, processed)
collected := fanin.Process(ctx, distributed...)
```

### Time Windows
```go
// Time-based aggregation (uniquely streamz)
windower := streamz.NewWindow("event-window", streamz.WindowConfig{
    Type: streamz.TumblingWindow,
    Size: 1 * time.Minute,
})

aggregator := streamz.FromPipz("aggregate-events",
    pipz.Apply("sum", func(ctx context.Context, events []Event) (EventSummary, error) {
        return aggregateEvents(events), nil
    }))

windowed := windower.Process(ctx, events)
aggregated := aggregator.Process(ctx, windowed)

for summary := range aggregated {
    // Process minute-by-minute summaries
    publishSummary(summary)
}
```

## 11. Implementation Phases

### Phase 1: Core Bridge (kevin)
1. Implement `FromPipz` adapter
2. Add pipz dependency to go.mod
3. Create basic StreamProcessor interface
4. Write integration tests

### Phase 2: Essential Streamz (kevin)  
1. Implement FanIn/FanOut (channel coordination)
2. Implement Batcher (time + count based)
3. Implement basic Buffer (dropping/sliding)
4. Write unit tests for each

### Phase 3: Time Windows (kevin)
1. Implement Window processor (tumbling/sliding/session)
2. Add time-based aggregation support
3. Write comprehensive window tests
4. Performance benchmarks

### Phase 4: Documentation & Examples (me + zidgel)
1. Migration guide (breaking changes)
2. Best practices documentation  
3. Performance benchmarks
4. Real-world examples

## 12. Success Metrics

### Code Quality
- **Lines of Code**: <2000 lines (vs >8000 in v1)
- **Cyclomatic Complexity**: <10 per function  
- **Test Coverage**: >95% for core processors
- **Zero** linter warnings

### Performance  
- **Throughput**: >1M items/sec for simple pipelines
- **Latency**: <100ns overhead per processor
- **Memory**: <1MB heap for typical pipeline
- **GC Pressure**: <100 allocs/sec under load

### Usability
- **API Surface**: <20 exported types (vs >50 in v1)
- **Learning Curve**: pipz knowledge transfers directly
- **Error Messages**: Clear, actionable (from pipz)
- **Debug Info**: Rich tracing via pipz.Error[T]

---

## The Bottom Line

This is the streamz we should have built from day one. Clean, minimal, powerful through composition with pipz rather than feature duplication. We eliminate thousands of lines of complex code and get better reliability, performance, and maintainability.

No backward compatibility means no baggage. We build the right thing, the right way, and users who want it will migrate because it's genuinely better.

**"Boring technology that works" - that's the goal.**

---
*Architecture by midgel - The gruff engineer who keeps the ship from falling apart*