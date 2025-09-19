# Error Handling Feasibility Analysis for streamz

## Executive Summary

Adding proper error handling to streamz before the FromChainable refactor is **critical and feasible**, but requires careful design to avoid breaking changes. The current streamz architecture treats errors as second-class citizens (skip-on-error pattern), while pipz provides rich error context through `*Error[T]`. I recommend implementing a **dual-channel pattern** with backward compatibility as the minimal viable solution that unblocks the FromChainable refactor.

## 1. Current streamz Error Handling Patterns

### The Skip-on-Error Philosophy
Investigation reveals streamz follows a "graceful degradation" approach:

```go
// AsyncMapper - errors cause items to be skipped
func (a *AsyncMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    // ...worker code...
    result, err := a.fn(ctx, seqItem.item)
    results <- sequencedItem[Out]{
        seq:  seqItem.seq,
        item: result,
        skip: err != nil,  // ERROR: Item is silently skipped!
    }
    // Later...
    if !item.skip {
        out <- item.item  // Only successful items pass through
    }
}
```

**Key Observations:**
- Errors are swallowed silently - no way to observe failures
- Failed items disappear from the stream with no trace
- No error propagation mechanism to downstream processors
- DLQ and Retry processors exist but operate on the Processor[T,T] interface (can't see errors)

### Specialized Error Handlers
Only two processors handle errors explicitly:

1. **DLQ (Dead Letter Queue)**:
   - Wraps another processor and catches failures
   - Creates `DLQItem[T]` with error context
   - Has its own error channel: `failedItems chan DLQItem[T]`
   - Problem: Only works by wrapping processors, not native error support

2. **Retry**:
   - Wraps processors and retries on failure
   - Problem: Determines failure by channel closure, not error returns
   - No access to actual error for classification

## 2. Error Channel Pattern Feasibility

### Proposed Dual-Channel Design

```go
// New error-aware processor interface (backward compatible)
type ErrorProcessor[In, Out any] interface {
    Processor[In, Out]  // Embed existing interface
    ProcessWithErrors(ctx context.Context, in <-chan In) (<-chan Out, <-chan error)
}

// Concrete implementation example
type ErrorAwareMapper[In, Out any] struct {
    fn   func(context.Context, In) (Out, error)
    name string
}

func (m *ErrorAwareMapper[In, Out]) ProcessWithErrors(ctx context.Context, in <-chan In) (<-chan Out, <-chan error) {
    out := make(chan Out)
    errs := make(chan error, 100)  // Buffered to prevent blocking
    
    go func() {
        defer close(out)
        defer close(errs)
        
        for item := range in {
            result, err := m.fn(ctx, item)
            if err != nil {
                select {
                case errs <- err:  // Non-blocking send
                default:
                    // Error channel full, log and continue
                }
            } else {
                select {
                case out <- result:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()
    
    return out, errs
}

// Backward compatible Process method
func (m *ErrorAwareMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    out, errs := m.ProcessWithErrors(ctx, in)
    // Drain errors to maintain skip-on-error behavior
    go func() {
        for range errs {
            // Silently discard for backward compatibility
        }
    }()
    return out
}
```

### Composition Challenges

**Problem**: How do error channels compose in processor chains?

```go
// Challenge: Chaining error-aware processors
mapper1 := NewErrorAwareMapper(transform1)
mapper2 := NewErrorAwareMapper(transform2)

data1, errs1 := mapper1.ProcessWithErrors(ctx, input)
data2, errs2 := mapper2.ProcessWithErrors(ctx, data1)

// Now we have TWO error channels to monitor!
// Need error aggregation pattern
```

**Solution**: Error aggregator utility:

```go
type ErrorAggregator struct {
    errors chan error
    sources []<-chan error
}

func MergeErrors(ctx context.Context, errorChans ...<-chan error) <-chan error {
    merged := make(chan error)
    var wg sync.WaitGroup
    
    for _, errs := range errorChans {
        wg.Add(1)
        go func(errors <-chan error) {
            defer wg.Done()
            for err := range errors {
                select {
                case merged <- err:
                case <-ctx.Done():
                    return
                }
            }
        }(errs)
    }
    
    go func() {
        wg.Wait()
        close(merged)
    }()
    
    return merged
}
```

## 3. Alternative Patterns Considered

### Result Type Pattern
```go
type Result[T any] struct {
    Value T
    Error error
}

type ResultProcessor[In, Out any] interface {
    Process(ctx context.Context, in <-chan Result[In]) <-chan Result[Out]
}
```

**Pros:**
- Single channel maintains ordering
- Easier composition
- Error context travels with data

**Cons:**
- MASSIVE breaking change - changes all type signatures
- Forces all processors to handle Result type
- Increases memory overhead for successful items

### Callback Pattern
```go
type ProcessorWithCallback[In, Out any] interface {
    Process(ctx context.Context, in <-chan In) <-chan Out
    OnError(func(err error, item In))
}
```

**Pros:**
- Non-breaking addition
- Simple to implement

**Cons:**
- Callbacks don't compose well
- Hard to aggregate errors across pipeline
- Race conditions with concurrent processors

### Context-Based Error Collection
```go
type errorKey struct{}

func WithErrorCollector(ctx context.Context) (context.Context, *ErrorCollector) {
    ec := &ErrorCollector{errors: make([]error, 0)}
    return context.WithValue(ctx, errorKey{}, ec), ec
}
```

**Pros:**
- No API changes needed
- Errors collected centrally

**Cons:**
- Hidden side effects
- Not goroutine-safe without locks
- Easy to miss errors

## 4. Integration with pipz Errors

### pipz Error Structure
```go
type Error[T any] struct {
    Timestamp time.Time
    InputData T           // Original input that failed
    Err       error       // Underlying error
    Path      []string    // Processing path taken
    Duration  time.Duration
    Timeout   bool
    Canceled  bool
}
```

### FromChainable Error Mapping

```go
type FromChainable[T any] struct {
    chainable pipz.Chainable[T]
    name      string
}

func (f *FromChainable[T]) ProcessWithErrors(ctx context.Context, in <-chan T) (<-chan T, <-chan *pipz.Error[T]) {
    out := make(chan T)
    errs := make(chan *pipz.Error[T], 100)
    
    go func() {
        defer close(out)
        defer close(errs)
        
        for item := range in {
            result, err := f.chainable.Process(ctx, item)
            if err != nil {
                // Check if it's already a pipz.Error
                var pipzErr *pipz.Error[T]
                if errors.As(err, &pipzErr) {
                    select {
                    case errs <- pipzErr:
                    default:
                        // Error buffer full
                    }
                } else {
                    // Wrap in pipz.Error
                    select {
                    case errs <- &pipz.Error[T]{
                        Timestamp: time.Now(),
                        InputData: item,
                        Err:       err,
                        Path:      []string{f.chainable.Name()},
                        Duration:  0, // Would need timing
                    }:
                    default:
                    }
                }
            } else {
                select {
                case out <- result:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()
    
    return out, errs
}
```

## 5. Recommendations

### Immediate (Minimal Viable for FromChainable)

**Implement dual-channel pattern with backward compatibility:**

1. Create `ErrorProcessor[In,Out]` interface extending `Processor[In,Out]`
2. Add `ProcessWithErrors()` method returning `(<-chan Out, <-chan error)`
3. Implement for critical processors: Mapper, AsyncMapper, Filter
4. FromChainable implements ErrorProcessor to surface pipz errors
5. Provide error aggregation utilities

**Why this approach:**
- Non-breaking - existing code continues working
- Enables proper error handling for new code
- FromChainable can surface rich pipz errors
- Progressive migration path

### Short-term (After FromChainable)

1. **Migrate all processors** to ErrorProcessor interface
2. **Add error routing** processors:
   ```go
   type ErrorRouter[T any] struct {
       routes map[string]func(error) bool
       handlers map[string]Processor[T, T]
   }
   ```
3. **Create error transformation** utilities:
   ```go
   func WrapErrors[T any](processor Processor[T,T]) ErrorProcessor[T,T]
   func IgnoreErrors[T any](processor ErrorProcessor[T,T]) Processor[T,T]
   ```

### Long-term (v2.0)

Consider Result[T] pattern for v2.0 with breaking changes:
- Cleaner composition
- Guaranteed error visibility
- Better alignment with functional patterns
- Full type safety

## Implementation Complexity

### Dual-Channel Approach
- **Complexity**: Medium
- **Time Estimate**: 2-3 days
- **Risk**: Low (backward compatible)
- **Files to modify**: ~10-15 core processors

### Result Type Approach  
- **Complexity**: High
- **Time Estimate**: 1-2 weeks
- **Risk**: Very High (breaking change)
- **Files to modify**: ALL processors and tests

## Testing Considerations

```go
func TestErrorProcessor(t *testing.T) {
    processor := NewErrorAwareMapper(func(ctx context.Context, n int) (int, error) {
        if n < 0 {
            return 0, fmt.Errorf("negative number: %d", n)
        }
        return n * 2, nil
    })
    
    input := make(chan int)
    go func() {
        input <- 1
        input <- -1
        input <- 2
        close(input)
    }()
    
    out, errs := processor.ProcessWithErrors(context.Background(), input)
    
    // Collect results
    var results []int
    var errors []error
    
    var wg sync.WaitGroup
    wg.Add(2)
    
    go func() {
        defer wg.Done()
        for v := range out {
            results = append(results, v)
        }
    }()
    
    go func() {
        defer wg.Done()
        for err := range errs {
            errors = append(errors, err)
        }
    }()
    
    wg.Wait()
    
    assert.Equal(t, []int{2, 4}, results)
    assert.Len(t, errors, 1)
    assert.Contains(t, errors[0].Error(), "negative number: -1")
}
```

## Decision Point

**Should we add error handling BEFORE FromChainable?**

**YES** - But implement the minimal dual-channel pattern:

1. **Unblocks FromChainable** to properly surface pipz errors
2. **Non-breaking** - existing code continues working
3. **Progressive** - can migrate processors incrementally
4. **Testable** - clear error path for testing
5. **Composable** - error aggregation utilities handle multi-channel complexity

**Implementation Priority:**
1. ErrorProcessor interface definition
2. ErrorAwareMapper implementation
3. Error aggregation utilities
4. FromChainable with error support
5. Migration of other processors (can be gradual)

The dual-channel pattern provides the best balance of:
- Immediate value (unblocks FromChainable)
- Backward compatibility (non-breaking)
- Future flexibility (can evolve to Result[T] in v2)
- Implementation simplicity (2-3 days work)