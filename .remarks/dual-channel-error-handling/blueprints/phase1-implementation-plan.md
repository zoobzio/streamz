# Dual-Channel Error Handling Implementation Plan - Phase 1

**Engineer:** midgel  
**Status:** Blueprint  
**Breaking Change:** YES - Complete API overhaul  
**Users:** ZERO - Breaking everything is acceptable  

---

## Executive Summary

We're implementing dual-channel error handling where every processor returns both success and error channels. This is a fundamental architectural change that will break EVERYTHING in the current codebase, but since we have no users, we can rebuild it properly.

**Core Change:** `Process(ctx, in) <-chan Out` becomes `Process(ctx, in) (<-chan Out, <-chan *Error[Out])`

**Phase 1 Scope:** Prove the dual-channel pattern works with 2 connectors only. Everything else will be broken.

---

## 1. New Interface Design

### 1.1 New Processor Interface

```go
// Processor is the core interface for stream processing components with dual-channel error handling.
// It transforms an input channel to separate output and error channels, enabling proper
// error flow through streaming pipelines without losing successful items.
type Processor[In, Out any] interface {
    // Process transforms the input channel to output and error channels.
    // - out: Successfully processed items
    // - errs: Processing errors with rich context
    // Both channels are closed when processing completes.
    Process(ctx context.Context, in <-chan In) (out <-chan Out, errs <-chan *Error[Out])
    
    // Name returns a descriptive name for the processor, useful for debugging.
    Name() string
}
```

**Key Design Decisions:**
- Both channels MUST be closed when input is exhausted or context canceled
- Error channel contains `*Error[Out]` (pointer for nil-ability and memory efficiency)
- Errors flow alongside successes, not instead of them
- Context cancellation affects both channels equally

### 1.2 Error Type Definition

```go
// Error provides rich context about streaming processing failures.
// Adapted from pipz.Error[T] but specialized for streaming use cases.
type Error[T any] struct {
    Timestamp  time.Time              // When the error occurred
    InputData  interface{}            // The original input that caused the error
    OutputData T                      // Attempted output (may be zero value)
    Err        error                  // The underlying error
    Processor  string                 // Name of the processor that failed
    StreamID   string                 // Optional stream identifier
    Metadata   map[string]interface{} // Additional context
    
    // Streaming-specific fields
    ItemIndex int64 // Position in stream (if trackable)
    Retryable bool  // Whether this error should be retried
}
```

**Error Methods:**
```go
func (e *Error[T]) Error() string
func (e *Error[T]) Unwrap() error
func (e *Error[T]) IsRetryable() bool
func (e *Error[T]) WithMetadata(key string, value interface{}) *Error[T]
```

### 1.3 Error Channel Flow Patterns

**Pattern 1: Error Propagation**
```go
// Errors flow through the pipeline alongside successes
mapper := NewMapper(transform)
filter := NewFilter(predicate)

// Chain processors - errors propagate automatically
mapped, mappedErrs := mapper.Process(ctx, input)
filtered, filteredErrs := filter.Process(ctx, mapped)

// Merge error streams
allErrors := NewFanIn[*Error[Output]]().Process(ctx, mappedErrs, filteredErrs)
```

**Pattern 2: Error Handling**
```go
// Handle errors at any point in the pipeline
results, errors := processor.Process(ctx, input)

go func() {
    for err := range errors {
        log.Error("Processing failed", "error", err, "processor", err.Processor)
        metrics.IncErrorCount(err.Processor)
        
        if err.IsRetryable() {
            retryQueue.Send(err.InputData)
        }
    }
}()
```

---

## 2. Code Organization Strategy

### 2.1 Directory Structure During Transition

```
streamz/
├── api.go                    # New dual-channel interface
├── error.go                  # New Error[T] type
├── error_test.go            # Error type tests
├── 
├── v2/                      # NEW: Phase 1 implementations
│   ├── fanin.go            # New FanIn with dual channels
│   ├── fanin_test.go       # Comprehensive tests
│   ├── fanout.go           # New FanOut with dual channels  
│   ├── fanout_test.go      # Comprehensive tests
│   └── README.md           # Usage guide for v2 processors
├── 
├── deprecated/              # OLD: Move broken processors here
│   ├── README.md           # "This code is broken, use v2/"
│   ├── fanin.go            # Old FanIn (broken)
│   ├── fanout.go           # Old FanOut (broken)  
│   ├── mapper.go           # All other old processors
│   ├── filter.go           
│   ├── batcher.go          
│   └── ... (all current processors)
│
├── testing/
│   ├── v2/                 # NEW: Tests for dual-channel processors
│   │   ├── integration/    # Integration tests for v2
│   │   └── helpers/        # Test utilities for dual-channel
│   └── deprecated/         # OLD: Move old tests here
│
└── examples/
    ├── v2/                 # NEW: Examples using dual-channel API
    │   └── dual-channel-demo/
    └── deprecated/         # OLD: Examples that no longer work
```

### 2.2 Import Strategy

**Phase 1 Usage:**
```go
import (
    "github.com/streamz"              // Core interfaces (Error[T], Processor)
    "github.com/streamz/v2"           // New dual-channel processors
)

// Only these work:
fanin := v2.NewFanIn[Data]()
fanout := v2.NewFanOut[Data](3)

// Everything else is broken:
// mapper := streamz.NewMapper(...) // COMPILATION ERROR
```

### 2.3 Migration Markers

**Clear Deprecation:**
```go
// deprecated/README.md
# DEPRECATED: Old Single-Channel Processors

⚠️ **ALL PROCESSORS IN THIS DIRECTORY ARE BROKEN** ⚠️

The streamz library has migrated to dual-channel error handling.
These processors use the old `Process(ctx, in) <-chan Out` signature
and will not compile with the new `Processor[In, Out]` interface.

## Use v2/ Instead

- Old: `streamz.NewMapper(...)` 
- New: `v2.NewMapper(...)`  (when implemented)

Only `v2.NewFanIn` and `v2.NewFanOut` are currently implemented.
```

---

## 3. Proof-of-Concept Connectors

### 3.1 Why FanIn and FanOut

**Selected Processors:** FanIn[T] and FanOut[T]

**Rationale:**
1. **Channel-centric operations** - These are pure channel manipulation, won't benefit from pipz patterns
2. **Different complexity levels** - FanIn (merge) vs FanOut (broadcast) test different scenarios
3. **Common patterns** - Most pipelines use fan-in/fan-out for parallelization
4. **Error aggregation challenge** - FanIn must merge error streams from multiple inputs
5. **Error broadcasting challenge** - FanOut must handle errors when some outputs block

**WON'T Choose:**
- Mapper, Filter - These will benefit from pipz.FromChainable later
- Batcher, Window - Complex stateful processors, better for later phases
- CircuitBreaker, DLQ - Already have error handling, would confuse the test

### 3.2 FanIn Implementation Challenges

**Error Aggregation Pattern:**
```go
func (f *FanIn[T]) Process(ctx context.Context, ins ...<-chan T) (<-chan T, <-chan *Error[T]) {
    out := make(chan T)
    errOut := make(chan *Error[T])
    
    // Challenge: Merge multiple error streams
    // Solution: Fan-in pattern for both data and errors
}
```

**Key Technical Challenges:**
- Multiple input channels, each can have errors
- Must merge error streams without losing context
- Goroutine management for N input channels
- Proper cleanup when context cancels

### 3.3 FanOut Implementation Challenges  

**Error Broadcasting Pattern:**
```go
func (f *FanOut[T]) Process(ctx context.Context, in <-chan T) ([]<-chan T, <-chan *Error[T]) {
    // Challenge: What if one output blocks but others are ready?
    // Challenge: How to handle errors when broadcasting?
}
```

**Key Technical Challenges:**
- One input, multiple outputs - different pattern than current
- Return type changes from `[]<-chan T` to `([]<-chan T, <-chan *Error[T])`  
- Error handling when individual outputs block or fail
- Context cancellation affects all outputs

---

## 4. Detailed Implementation Steps

### 4.1 Step 1: Core Infrastructure

**Files to Create:**
1. `/home/zoobzio/code/streamz/error.go` - Error[T] type
2. `/home/zoobzio/code/streamz/error_test.go` - Error type tests
3. **Update** `/home/zoobzio/code/streamz/api.go` - New Processor interface

**Tasks:**
- [ ] Define Error[T] struct with streaming-specific fields
- [ ] Implement Error methods (Error(), Unwrap(), IsRetryable())
- [ ] Update Processor[In, Out] interface to return dual channels
- [ ] Add constructor functions for creating errors
- [ ] Write comprehensive tests for Error[T] behavior

**Acceptance Criteria:**
- Error[T] can wrap any underlying error
- Error provides rich context (processor, timestamp, data)
- Processor interface compiles but breaks ALL existing processors
- Error tests cover wrapping, unwrapping, and metadata

### 4.2 Step 2: Directory Reorganization

**Tasks:**
- [ ] Create `v2/` directory for new implementations
- [ ] Create `deprecated/` directory  
- [ ] Move current fanin.go, fanout.go to deprecated/
- [ ] Create `deprecated/README.md` with clear warning
- [ ] Create `v2/README.md` with usage guide
- [ ] Update `testing/` structure

**Acceptance Criteria:**
- Clear separation between working (v2) and broken (deprecated) code
- Documentation explains the transition
- Directory structure supports parallel development

### 4.3 Step 3: V2 FanIn Implementation

**Files to Create:**
1. `/home/zoobzio/code/streamz/v2/fanin.go` - New dual-channel FanIn
2. `/home/zoobzio/code/streamz/v2/fanin_test.go` - Comprehensive tests

**Key Implementation Points:**
```go
// v2/fanin.go
func (f *FanIn[T]) Process(ctx context.Context, ins ...<-chan T) (<-chan T, <-chan *Error[T]) {
    out := make(chan T)
    errOut := make(chan *Error[T])
    
    var wg sync.WaitGroup
    
    // Start goroutine for each input
    for i, in := range ins {
        wg.Add(1)
        go func(input <-chan T, inputIndex int) {
            defer wg.Done()
            for {
                select {
                case item, ok := <-input:
                    if !ok {
                        return // Input closed
                    }
                    
                    select {
                    case out <- item:
                        // Success
                    case <-ctx.Done():
                        // Create cancellation error
                        err := &Error[T]{
                            Timestamp: time.Now(),
                            InputData: item,
                            Err:       ctx.Err(),
                            Processor: f.name,
                            Metadata: map[string]interface{}{
                                "input_index": inputIndex,
                            },
                        }
                        
                        select {
                        case errOut <- err:
                        case <-ctx.Done():
                        }
                        return
                    }
                    
                case <-ctx.Done():
                    return
                }
            }
        }(in, i)
    }
    
    // Cleanup goroutine
    go func() {
        wg.Wait()
        close(out)
        close(errOut)
    }()
    
    return out, errOut
}
```

**Testing Strategy:**
- [ ] Basic fan-in functionality (merge multiple inputs)
- [ ] Context cancellation handling
- [ ] Error generation scenarios
- [ ] Goroutine leak detection
- [ ] Race condition testing (`go test -race`)

### 4.4 Step 4: V2 FanOut Implementation

**Files to Create:**
1. `/home/zoobzio/code/streamz/v2/fanout.go` - New dual-channel FanOut
2. `/home/zoobzio/code/streamz/v2/fanout_test.go` - Comprehensive tests

**Key Implementation Points:**
```go
// v2/fanout.go - SIGNATURE CHANGE REQUIRED
func (f *FanOut[T]) Process(ctx context.Context, in <-chan T) ([]<-chan T, <-chan *Error[T]) {
    errOut := make(chan *Error[T])
    outputs := make([]chan T, f.count)
    publicOutputs := make([]<-chan T, f.count)
    
    // Initialize output channels
    for i := 0; i < f.count; i++ {
        outputs[i] = make(chan T)
        publicOutputs[i] = outputs[i]
    }
    
    go func() {
        defer func() {
            // Close all output channels
            for _, ch := range outputs {
                close(ch)
            }
            close(errOut)
        }()
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return // Input closed
                }
                
                // Broadcast to all outputs (with timeout to prevent blocking)
                for i, ch := range outputs {
                    select {
                    case ch <- item:
                        // Success
                    case <-time.After(5 * time.Second):
                        // Output blocked - create error
                        err := &Error[T]{
                            Timestamp: time.Now(),
                            InputData: item,
                            OutputData: item, // Same for fanout
                            Err:       fmt.Errorf("output %d blocked", i),
                            Processor: f.name,
                            Retryable: false, // Blocking is not retryable
                            Metadata: map[string]interface{}{
                                "output_index": i,
                                "blocked": true,
                            },
                        }
                        
                        select {
                        case errOut <- err:
                        case <-ctx.Done():
                            return
                        }
                        
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return publicOutputs, errOut
}
```

**Breaking Change Note:**
- Old: `Process(ctx, in) []<-chan T`
- New: `Process(ctx, in) ([]<-chan T, <-chan *Error[T])`

### 4.5 Step 5: Integration Testing

**Files to Create:**
1. `/home/zoobzio/code/streamz/testing/v2/integration/dual_channel_test.go`
2. `/home/zoobzio/code/streamz/testing/v2/helpers/test_utils.go`

**Integration Test Scenarios:**
```go
func TestDualChannelPipeline(t *testing.T) {
    // Test: FanOut -> Multiple Processing -> FanIn
    ctx := context.Background()
    
    // Setup pipeline
    fanout := v2.NewFanOut[int](3)
    fanin := v2.NewFanIn[int]()
    
    // Input data
    input := make(chan int)
    go func() {
        defer close(input)
        for i := 0; i < 100; i++ {
            input <- i
        }
    }()
    
    // Fan out
    branches, fanoutErrs := fanout.Process(ctx, input)
    
    // Process each branch (simulate work)
    processedBranches := make([]<-chan int, len(branches))
    for i, branch := range branches {
        processedBranches[i] = simulateProcessing(ctx, branch)
    }
    
    // Fan in
    output, faninErrs := fanin.Process(ctx, processedBranches...)
    
    // Collect results and errors
    results := collectResults(output)
    errors := collectErrors(fanoutErrs, faninErrs)
    
    // Verify
    assert.Len(t, results, 100) // All items processed
    assert.Empty(t, errors)     // No errors in happy path
}
```

**Error Scenario Tests:**
- [ ] Context cancellation propagates to error channels
- [ ] Blocked outputs generate appropriate errors
- [ ] Multiple error streams merge correctly
- [ ] Error metadata preserves context

### 4.6 Step 6: Documentation and Examples

**Files to Create:**
1. `/home/zoobzio/code/streamz/v2/README.md` - API guide
2. `/home/zoobzio/code/streamz/examples/v2/dual-channel-demo/main.go` - Working example

**Documentation Requirements:**
- Clear migration guide from old to new API
- Error handling best practices
- Performance considerations
- Common patterns and anti-patterns

---

## 5. Success Criteria

### 5.1 Technical Success Metrics

**Compilation:**
- [ ] New Error[T] type compiles and tests pass
- [ ] New Processor interface compiles  
- [ ] V2 FanIn and FanOut implement new interface
- [ ] All old processors fail compilation (expected)

**Functionality:**
- [ ] FanIn merges multiple inputs correctly
- [ ] FanOut broadcasts to multiple outputs correctly
- [ ] Error channels provide rich context
- [ ] Context cancellation works on both channels
- [ ] No goroutine leaks under normal or error conditions

**Testing:**
- [ ] Unit tests: >95% coverage for v2 processors
- [ ] Integration tests: End-to-end dual-channel pipeline
- [ ] Race testing: `go test -race` passes
- [ ] Stress testing: Handles high throughput
- [ ] Error testing: All error scenarios covered

### 5.2 Quality Metrics

**Performance:**
- [ ] No significant performance regression vs old processors
- [ ] Error channel overhead is minimal for happy path
- [ ] Memory usage is reasonable (no major leaks)

**Maintainability:**
- [ ] Clear separation between old and new code
- [ ] Documentation explains the transition
- [ ] Code follows established Go patterns
- [ ] Error handling is comprehensive

### 5.3 Validation Tests

**The Pipeline Test:**
```go
// This must work perfectly to prove the concept
func TestDualChannelConcept(t *testing.T) {
    ctx := context.Background()
    
    // Create a pipeline: Input -> FanOut -> Process -> FanIn -> Output
    fanout := v2.NewFanOut[string](3)
    fanin := v2.NewFanIn[string]()
    
    input := generateTestData(1000)
    
    // Fan out
    branches, outErrs := fanout.Process(ctx, input)
    
    // Process branches (simulate work with some failures)
    processed := make([]<-chan string, len(branches))
    for i, branch := range branches {
        processed[i] = addPrefix(branch, fmt.Sprintf("branch-%d-", i))
    }
    
    // Fan in
    output, inErrs := fanin.Process(ctx, processed...)
    
    // Collect everything
    results := drainChannel(output)
    errors := drainErrors(outErrs, inErrs)
    
    // Validate
    assert.Equal(t, 1000, len(results), "All items should be processed")
    assert.Empty(t, errors, "No errors in happy path")
    
    // Verify each item went through exactly one branch
    prefixes := map[string]int{}
    for _, result := range results {
        for i := 0; i < 3; i++ {
            prefix := fmt.Sprintf("branch-%d-", i)
            if strings.HasPrefix(result, prefix) {
                prefixes[prefix]++
                break
            }
        }
    }
    
    // Should be roughly evenly distributed
    for prefix, count := range prefixes {
        assert.Greater(t, count, 300, "Prefix %s should have reasonable distribution", prefix)
    }
}
```

---

## 6. Risk Mitigation

### 6.1 Deadlock Prevention

**Risk:** Error channel backpressure causes deadlock

**Mitigation:**
```go
// Always use select with context for error channel writes
select {
case errOut <- err:
    // Error sent
case <-ctx.Done():
    // Context canceled, abandon error
    return
}
```

**Testing:** Create tests that intentionally don't read error channel and verify no deadlock.

### 6.2 Resource Management

**Risk:** Goroutine leaks when error channels aren't read

**Mitigation:**
- Always close error channels in defer statements
- Use WaitGroup for proper cleanup coordination  
- Context cancellation terminates all goroutines

**Testing:** Use goroutine leak detection in tests.

### 6.3 Error Channel Overflow

**Risk:** Too many errors cause memory issues

**Mitigation:**
```go
// Use buffered channels with reasonable capacity
errOut := make(chan *Error[T], 1000)

// Or implement dropping behavior for error channel
select {
case errOut <- err:
    // Error sent
default:
    // Error channel full, drop error (or log overflow)
    overflowCounter.Add(1)
}
```

### 6.4 API Compatibility

**Risk:** Future changes break v2 API

**Mitigation:**
- Design Error[T] to be extensible (struct with metadata map)
- Use interface for Processor to allow future additions
- Version the API clearly (`v2/` package)

---

## 7. What Could Go Wrong

### 7.1 Dual-Channel Complexity

**Problem:** Developers forget to read error channel, causing deadlocks

**Solution:** 
- Comprehensive documentation with examples
- Helper functions for common patterns
- Lint rules (future) to detect unused error channels

### 7.2 Performance Overhead

**Problem:** Error channel overhead slows down happy path

**Measurement Plan:**
- Benchmark old vs new FanIn/FanOut
- Profile memory allocation
- Test high-throughput scenarios

**Acceptable Overhead:** <10% performance impact for happy path

### 7.3 Context Cancellation Complexity

**Problem:** Context cancellation affects both channels differently

**Testing Strategy:**
- Test cancellation at various pipeline stages
- Verify error messages indicate cancellation vs failure
- Ensure proper cleanup in all scenarios

---

## 8. Implementation Timeline

### Phase 1 Tasks (This Implementation)

| Task | Estimated Time | Dependencies |
|------|---------------|--------------|
| Core Error[T] type + tests | 2 hours | None |
| Interface update + directory reorg | 1 hour | Error type |
| V2 FanIn implementation + tests | 4 hours | Interface |
| V2 FanOut implementation + tests | 4 hours | Interface |
| Integration tests | 3 hours | Both processors |
| Documentation + examples | 2 hours | Everything |
| **Total** | **16 hours** | |

### Success Gate for Phase 2

Before proceeding to pipz integration:
- [ ] All Phase 1 acceptance criteria met
- [ ] Performance benchmarks within acceptable range
- [ ] No known deadlock or resource leak issues
- [ ] Clear documentation for migration

---

## 9. Post-Phase 1 Future Work

**Phase 2: pipz Integration**
- Implement `FromChainable` to wrap pipz processors
- Migrate complex processors (Mapper, Filter, etc.)
- Error aggregation patterns

**Phase 3: Complete Migration**  
- All processors use dual-channel pattern
- Remove deprecated/ directory
- Performance optimization
- Advanced error handling (DLQ with dual channels)

---

**Engineer's Note:** This is a massive breaking change, but it's the RIGHT architectural decision. Single-channel error handling was a design mistake. Dual channels separate concerns properly and enable true error-aware streaming. We're building this to last.

The complexity is front-loaded in the infrastructure (Error[T], interface changes), but the individual processor implementations become cleaner and more predictable. Error handling stops being an afterthought and becomes a first-class citizen in the streaming pipeline.

No shortcuts. No compromises. We build it right.