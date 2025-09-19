# FromChainable Adapter - First Implementation Chunk

**Technical Architecture & Implementation Plan**  
**Author:** midgel (Chief Engineering Officer)  
**Date:** 2025-08-27  
**Version:** 1.0

---

## Engineering Assessment

After analyzing both codebases, I can confirm this is a well-architected integration challenge. The pipz library is solid engineering - clean interfaces, comprehensive test coverage (97.8%), and performance that doesn't suck. The streamz library follows consistent patterns with proper testing structure. This isn't a case of trying to integrate two poorly designed systems.

**Key Architectural Reality:**
- **pipz**: Synchronous single-item processing (`T -> (T, error)`)
- **streamz**: Asynchronous channel-based streaming (`<-chan T -> <-chan T`)
- **Bridge Pattern**: We need a clean adapter that preserves type safety and error context

The impedance mismatch is real but solvable. Here's how we do this right.

---

## 1. Architecture Design for FromChainable Adapter

### Core Interface Design

```go
// FromChainable converts a pipz.Chainable[T] to a streamz.Processor[T, T]
// This enables reuse of existing pipz business logic in streaming contexts
// while preserving type safety and error information.
func FromChainable[T any](chainable pipz.Chainable[T]) *ChainableAdapter[T]

type ChainableAdapter[T any] struct {
    chainable   pipz.Chainable[T]
    name        string
    errorMode   ErrorHandlingMode  // skip-on-error vs fail-fast
}

type ErrorHandlingMode int

const (
    SkipOnError ErrorHandlingMode = iota  // Default: skip failed items, continue processing
    FailFast                              // Stop entire stream on first error
)
```

### Integration with streamz.Processor Interface

The adapter implements `streamz.Processor[T, T]` exactly:

```go
func (ca *ChainableAdapter[T]) Process(ctx context.Context, in <-chan T) <-chan T
func (ca *ChainableAdapter[T]) Name() string
```

**Design Principles:**
1. **Type Safety**: No `interface{}`, no runtime type assertions
2. **Error Preservation**: pipz error context available through structured logging
3. **Context Propagation**: Cancellation and timeouts work correctly
4. **Zero-Overhead Goal**: < 100ns per item (realistic target given channel overhead)

### Error Handling Strategy

**Default Behavior (SkipOnError):**
- Failed items are skipped, processing continues
- pipz error information logged with structured logging
- Stream continues with remaining items
- Matches streamz patterns where individual failures don't stop the pipeline

**Optional Behavior (FailFast):**
- First error terminates the entire stream
- pipz error context preserved and returned
- Matches pipz patterns for critical validation

### Context Propagation Approach

```go
func (ca *ChainableAdapter[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for item := range in {
            // Context cancellation check
            select {
            case <-ctx.Done():
                return
            default:
            }
            
            // Process through pipz chainable
            result, err := ca.chainable.Process(ctx, item)
            if err != nil {
                // Handle based on error mode
                ca.handleError(ctx, item, err)
                if ca.errorMode == FailFast {
                    return
                }
                continue // Skip on error mode
            }
            
            // Send result
            select {
            case out <- result:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}
```

### Zero-Overhead Design Considerations

**Performance Constraints:**
- Single goroutine per adapter (no goroutine pool overhead)
- Unbuffered channels (no hidden memory allocation)
- Context checking only at item boundaries (not per operation)
- Error handling optimized for success path

**Memory Management:**
- No persistent error collection (log and discard)
- Channels closed properly to prevent goroutine leaks
- Context cancellation stops processing immediately

---

## 2. First Implementation Chunk

### Scope Definition

**EXACTLY what goes in this chunk:**
1. `FromChainable` adapter function
2. `ChainableAdapter[T]` implementation  
3. Migration of 3 simple processors: **Mapper**, **Filter**, **Tap**
4. Complete test coverage for the adapter and migrated processors
5. Performance benchmarks

**Why these processors first:**
- **Mapper**: Most fundamental transformation, simple 1:1 mapping
- **Filter**: Basic predicate filtering, demonstrates error-free processing
- **Tap**: Side effects without modification, simplest possible case

These represent different processing patterns while being simple enough to prove the concept works.

### File Structure and Naming Conventions

Following existing streamz patterns:

```
streamz/
├── from_chainable.go           # New: FromChainable adapter implementation
├── from_chainable_test.go      # New: Unit tests for adapter
├── mapper.go                   # Modify: Add FromChainable usage example
├── mapper_test.go              # Modify: Add tests comparing direct vs FromChainable
├── filter.go                   # Modify: Add FromChainable usage example  
├── filter_test.go              # Modify: Add comparison tests
├── tap.go                      # Modify: Add FromChainable usage example
├── tap_test.go                 # Modify: Add comparison tests
└── testing/
    ├── integration/
    │   └── from_chainable_test.go    # New: Integration tests with pipelines
    └── benchmarks/
        └── from_chainable_bench_test.go  # New: Performance benchmarks
```

### Dependencies and Imports Required

```go
// from_chainable.go
import (
    "context"
    "log/slog"  // For structured error logging
    "github.com/zoobzio/pipz"
)

// go.mod additions
require (
    github.com/zoobzio/pipz v1.0.0  // Exact version TBD
)
```

**Dependency Analysis:**
- pipz has zero external dependencies (only stdlib + golang.org/x/time)
- No dependency bloat or transitive hell
- Clean module boundary

---

## 3. Comprehensive Test Strategy

### Unit Tests for FromChainable Adapter

**File: `from_chainable_test.go`**

```go
func TestFromChainable_BasicFunctionality(t *testing.T)
func TestFromChainable_ErrorHandling_SkipMode(t *testing.T)  
func TestFromChainable_ErrorHandling_FailFastMode(t *testing.T)
func TestFromChainable_ContextCancellation(t *testing.T)
func TestFromChainable_ContextTimeout(t *testing.T)
func TestFromChainable_TypeSafety(t *testing.T)
func TestFromChainable_EmptyStream(t *testing.T)
func TestFromChainable_ResourceCleanup(t *testing.T)
```

**Test Patterns Following streamz Conventions:**
- Table-driven tests for different pipz processor types
- `helpers.CollectAll()` for result collection
- `helpers.AssertStreamEqual()` for comparison
- Context cancellation testing with timeouts
- Goroutine leak detection

### Tests for Each Migrated Processor

**Files: `mapper_test.go`, `filter_test.go`, `tap_test.go`**

New test functions added to each:
```go
func TestMapper_FromChainable_Equivalence(t *testing.T)
func TestMapper_FromChainable_Performance(t *testing.T)
func TestMapper_FromChainable_ErrorHandling(t *testing.T)
```

**Test Strategy:**
1. **Equivalence Testing**: Same inputs produce identical outputs
2. **Performance Comparison**: Measure overhead vs direct pipz usage  
3. **Error Path Testing**: Verify error handling behavior matches expectations

### Performance Benchmarks

**File: `testing/benchmarks/from_chainable_bench_test.go`**

```go
func BenchmarkFromChainable_vs_DirectPipz(b *testing.B)
func BenchmarkFromChainable_vs_NativeStreamz(b *testing.B)  
func BenchmarkFromChainable_Mapper(b *testing.B)
func BenchmarkFromChainable_Filter(b *testing.B)
func BenchmarkFromChainable_Tap(b *testing.B)
func BenchmarkFromChainable_ErrorPath(b *testing.B)
```

**Performance Targets:**
- FromChainable overhead: < 100ns per item
- Memory allocations: Predictable and minimal
- Throughput: ≥ 90% of direct pipz performance

### Error Scenario Coverage

**Error Test Cases:**
1. **pipz processor returns error**: Skip vs fail-fast behavior
2. **Context cancellation during processing**: Clean termination
3. **Context timeout**: Proper timeout behavior  
4. **Downstream channel blocking**: Backpressure handling
5. **Input channel closed**: Proper cleanup
6. **Panic in pipz processor**: Recovery and error conversion

### Integration Tests

**File: `testing/integration/from_chainable_test.go`**

```go
func TestFromChainable_PipelineComposition(t *testing.T)
func TestFromChainable_MixedProcessors(t *testing.T)
func TestFromChainable_ComplexWorkflow(t *testing.T)
func TestFromChainable_BatchingIntegration(t *testing.T)
```

**Integration Scenarios:**
- FromChainable processors mixed with native streamz processors
- Complex pipelines with multiple FromChainable adapters
- Integration with Batcher, Window, and other streamz components
- Error propagation through mixed pipelines

---

## 4. Implementation Order (Specific Steps)

### Step 1: Core Adapter Implementation (Day 1-2)
1. **Create `from_chainable.go`**
   - Implement `FromChainable[T]()` function
   - Implement `ChainableAdapter[T]` struct
   - Implement `Process()` method with SkipOnError mode only
   - Implement `Name()` method
   - Add structured error logging

2. **Create `from_chainable_test.go`**
   - Basic functionality tests
   - Context cancellation tests  
   - Type safety tests
   - Empty stream handling

3. **Verification:**
   ```bash
   go test ./from_chainable_test.go -v
   go test -race ./from_chainable_test.go
   ```

### Step 2: First Processor Migration - Mapper (Day 2-3)
1. **Create pipz equivalent processor**
   ```go
   // In mapper_test.go
   pipzMapper := pipz.Transform("test-mapper", func(_ context.Context, n int) int {
       return n * 2
   })
   ```

2. **Add comparison tests to `mapper_test.go`**
   - Equivalence testing between direct pipz and FromChainable
   - Performance comparison
   - Error handling (though Transform can't error)

3. **Verification:**
   ```bash
   go test ./mapper_test.go -v -run="FromChainable"
   ```

### Step 3: Second Processor Migration - Filter (Day 3-4)
1. **Create pipz equivalent using pipz.Mutate** (since pipz doesn't have direct filter)
   ```go
   pipzFilter := pipz.Mutate("test-filter",
       predicate,  // condition function
       identity,   // pass-through function
   )
   ```

2. **Add comparison tests to `filter_test.go`**
   - Test filtering behavior equivalence
   - Edge cases (empty streams, all-fail filters)

3. **Verification:**
   ```bash
   go test ./filter_test.go -v -run="FromChainable"  
   ```

### Step 4: Third Processor Migration - Tap (Day 4-5)
1. **Create pipz equivalent using pipz.Effect**
   ```go
   pipzTap := pipz.Effect("test-tap", sideEffectFunc)
   ```

2. **Add comparison tests to `tap_test.go`**
   - Verify side effects execute correctly
   - Ensure data passes through unchanged

3. **Verification:**
   ```bash
   go test ./tap_test.go -v -run="FromChainable"
   ```

### Step 5: Integration Testing (Day 5-6)
1. **Create `testing/integration/from_chainable_test.go`**
   - Complex pipeline compositions
   - Mixed processor types
   - Error propagation scenarios

2. **Verification:**
   ```bash
   go test ./testing/integration/from_chainable_test.go -v
   go test -race ./testing/integration/from_chainable_test.go
   ```

### Step 6: Performance Benchmarking (Day 6-7)
1. **Create `testing/benchmarks/from_chainable_bench_test.go`**
   - Overhead measurements
   - Throughput comparisons
   - Memory allocation analysis

2. **Verification:**
   ```bash
   go test ./testing/benchmarks/from_chainable_bench_test.go -bench=. -benchmem
   ```

### Step 7: Final Testing and Documentation (Day 7)
1. **Run complete test suite**
   ```bash
   go test ./... -v
   go test ./... -race
   golangci-lint run
   ```

2. **Add usage examples to existing processor files**
   - Comment examples showing FromChainable usage
   - Update godoc comments

3. **Update module dependencies**
   ```bash
   go get github.com/zoobzio/pipz@latest
   go mod tidy
   ```

### Verification Checklist for Each Step

Before moving to the next step, verify:
- [ ] All tests pass: `go test -v`
- [ ] Race detector clean: `go test -race` 
- [ ] Linter clean: `golangci-lint run`
- [ ] No goroutine leaks (check with testing/reliability tools)
- [ ] Memory usage reasonable (profile with `go test -memprofile`)
- [ ] Performance within acceptable bounds

---

## 5. Next Steps (Chunk 2 Preview)

**What would come next (don't implement yet):**

1. **Error Handling Modes**
   - Add FailFast mode implementation
   - Configuration API for error handling strategies
   - Error collection and reporting modes

2. **Advanced Processors Migration**
   - AsyncMapper (more complex concurrency patterns)
   - Retry/Circuit breaker patterns
   - Batcher integration optimizations

3. **Performance Optimizations**
   - Batch processing hints for compatible processors
   - Zero-allocation paths for simple transformations
   - Memory pooling for high-throughput scenarios

4. **Monitoring and Observability**
   - Metrics integration
   - Tracing support
   - Debug logging configuration

**Success Criteria for Chunk 1:**
- FromChainable adapter works with 3 processor types
- Performance overhead < 200ns per item (realistic for first implementation)
- Zero test failures or race conditions
- Clean integration with existing streamz patterns
- Complete documentation and examples

---

## Engineering Quality Standards

### Code Quality Requirements

**Every file must pass:**
```bash
go fmt ./...
golangci-lint run
go test -race ./...
go test -coverprofile=coverage.out ./...
```

**Coverage Requirements:**
- `from_chainable.go`: >95% coverage (it's the core component)
- Modified processor files: Maintain existing coverage + new tests
- Integration tests: >90% coverage of integration scenarios

### Testing Requirements

**Test File Organization:**
- Unit tests in same directory as source (`from_chainable_test.go`)
- Integration tests in `testing/integration/`
- Benchmarks in `testing/benchmarks/`
- Each test file has corresponding README explaining test strategy

**Test Naming Conventions:**
```go
func TestFromChainable_SpecificBehavior(t *testing.T)       // Unit tests
func TestFromChainable_Integration_Scenario(t *testing.T)    // Integration tests  
func BenchmarkFromChainable_Operation(b *testing.B)         // Benchmarks
```

### Error Handling Standards

**Structured Error Logging:**
```go
slog.Error("FromChainable processing failed",
    "processor_name", ca.chainable.Name(),
    "error", err,
    "input_type", fmt.Sprintf("%T", item),
    "error_mode", ca.errorMode,
)
```

**Error Wrapping:**
- Preserve original pipz error information
- Add adapter-specific context
- Enable error unwrapping for debugging

### Documentation Requirements

**Godoc Comments:**
- Every exported function has complete godoc
- Usage examples in comments
- Performance characteristics documented
- Error behavior clearly explained

**README Updates:**
- Add FromChainable section to main README
- Integration examples
- Performance benchmarks results
- Migration guide for existing pipz users

---

## Risk Mitigation

### Technical Risks

**Risk: Performance overhead unacceptable**
- **Mitigation**: Benchmark early and often, optimize hot paths
- **Fallback**: Document overhead, let users decide

**Risk: Error handling mismatch causes production issues**  
- **Mitigation**: Comprehensive error testing, clear documentation
- **Fallback**: Conservative defaults (skip-on-error)

**Risk: Resource leaks in streaming applications**
- **Mitigation**: Extensive leak testing, proper cleanup patterns
- **Fallback**: Clear resource management documentation

### Integration Risks

**Risk: Breaking existing streamz patterns**
- **Mitigation**: Zero changes to existing APIs, comprehensive regression testing
- **Fallback**: Feature flag for FromChainable (if needed)

**Risk: pipz dependency introduces bloat**
- **Mitigation**: pipz is zero-dependency, clean module boundary
- **Fallback**: Vendor pipz interfaces only (extreme case)

---

## Success Definition

This chunk is successful when:

1. **Functional Success**: 3 processors successfully bridge pipz to streamz
2. **Performance Success**: Overhead measured and documented (< 200ns target)
3. **Quality Success**: 100% test pass rate, zero race conditions
4. **Integration Success**: Clean composition with existing streamz processors
5. **Developer Success**: Clear examples and documentation enable adoption

**Acceptance Criteria:**
```go
// This code compiles and works correctly
pipzValidator := pipz.Apply("validate", validateOrder)
streamzValidator := FromChainable(pipzValidator)

orders := make(chan Order)
validated := streamzValidator.Process(ctx, orders)
batched := NewBatcher[Order](config).Process(ctx, validated)
// ... rest of streaming pipeline
```

---

*This is engineering architecture that solves the real problem: enabling reuse of battle-tested pipz processors in streaming contexts. No overengineering, no speculation, just solid implementation that follows established patterns in both codebases.*

**Next Actions:**
1. **Implement** following this exact plan
2. **Test** at each step before proceeding  
3. **Benchmark** to verify performance assumptions
4. **Document** with real usage examples

*The ship won't fall apart if we follow this blueprint.*