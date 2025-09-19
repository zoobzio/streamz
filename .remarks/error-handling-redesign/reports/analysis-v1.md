# Streamz Error Handling Redesign: Intelligence Analysis

## Executive Summary

Streamz currently treats errors as second-class citizens, dropping them silently or wrapping processors in cumbersome retry/DLQ patterns. With ZERO users and NO backward compatibility constraints, we have a unique opportunity to implement first-class error handling through dual-channel architecture, making streamz the most error-aware streaming framework in Go.

The RIGHT approach: Every processor returns `(out <-chan T, errs <-chan *Error[T])`, creating parallel data and error streams that flow through the entire pipeline, preserving ALL context from pipz integration while enabling sophisticated error handling patterns impossible with current design.

## 1. Current Streamz Error Handling - Critical Failures

### How Streamz Mishandles Errors

My analysis reveals systematic error suppression throughout streamz:

```go
// CURRENT DISASTER: AsyncMapper silently drops errors
func (a *AsyncMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    // ...
    result, err := a.fn(ctx, seqItem.item)
    results <- sequencedItem[Out]{
        skip: err != nil,  // ERROR SILENTLY DROPPED!
    }
    // ...
}

// CURRENT WORKAROUND: DLQ processor is a band-aid
dlq := NewDeadLetterQueue(processor, clock)
// Requires wrapping every processor that might fail
// Creates separate error channel outside main flow
// Loses pipeline context and composition

// CURRENT PATTERN: Retry wraps processors individually
retry := NewRetry(processor, clock)
// Each retry is isolated, no pipeline-wide error strategy
// Can't correlate errors across pipeline stages
// No way to implement circuit breaking based on downstream errors
```

### Critical Information Being Lost

1. **Pipeline Path Context**: When AsyncMapper drops an error, we lose WHERE in the pipeline it occurred
2. **Timing Information**: No tracking of how long operations took before failing
3. **Data Correlation**: Can't correlate errors with the data that caused them
4. **Error Patterns**: No way to detect systemic failures across pipeline stages
5. **Retry Context**: Lost information about how many retries occurred and why they failed

## 2. Proposed Dual-Channel Architecture

### The New Processor Interface

```go
// IDEAL: Every processor MUST implement error-aware processing
type Processor[In, Out any] interface {
    // Process returns BOTH data and error channels
    Process(ctx context.Context, in <-chan In) (out <-chan Out, errs <-chan *Error[Out])
    Name() string
}

// Error type directly adopts pipz's rich error context
type Error[T any] struct {
    Timestamp time.Time
    InputData T          // Original input that caused error
    OutputData T         // Partial output if any
    Err       error      // The actual error
    Path      []string   // Complete pipeline path
    Duration  time.Duration
    Stage     string     // Which processor failed
    Retries   int        // How many retry attempts
    Metadata  map[string]any // Custom context
}
```

### Processor Composition with Dual Channels

```go
// ELEGANT: Composition maintains both channels
type Pipeline[In, Out any] struct {
    stages []interface{} // Type-erased processors
}

func (p *Pipeline[In, Out]) Process(ctx context.Context, in <-chan In) (<-chan Out, <-chan *Error[Out]) {
    var dataStream <-chan In = in
    var errorStream <-chan *Error[In]
    
    // Aggregate error channel
    allErrors := make(chan *Error[Out])
    errorAggregator := &sync.WaitGroup{}
    
    for i, stage := range p.stages {
        // Each stage processes data AND errors from previous stage
        nextData, nextErrors := stage.Process(ctx, dataStream)
        
        // Forward errors to aggregate channel with path context
        errorAggregator.Add(1)
        go func(stageNum int, errs <-chan *Error[any]) {
            defer errorAggregator.Done()
            for err := range errs {
                err.Path = append(err.Path, p.stages[stageNum].Name())
                allErrors <- err.Transform(Out) // Type conversion
            }
        }(i, nextErrors)
        
        dataStream = nextData
    }
    
    go func() {
        errorAggregator.Wait()
        close(allErrors)
    }()
    
    return dataStream, allErrors
}
```

## 3. Error Channel Mechanics - The Right Design

### Channel Buffering Strategy

```go
// UNBUFFERED for immediate backpressure (default)
// - Forces consumers to handle errors immediately
// - Prevents error accumulation and memory growth
// - Natural flow control

// BUFFERED for specific patterns
type ProcessorConfig struct {
    ErrorBufferSize int // 0 = unbuffered (default)
}

// Example: High-throughput processor needs buffer
func NewAsyncMapper[In, Out any](fn MapperFunc) *AsyncMapper[In, Out] {
    return &AsyncMapper[In, Out]{
        fn: fn,
        errorBuffer: 100, // Buffer to prevent worker blocking
    }
}
```

### Channel Closing Semantics

```go
// RULE: Error channel closes AFTER data channel
func (p *Processor) Process(ctx context.Context, in <-chan In) (<-chan Out, <-chan *Error[Out]) {
    out := make(chan Out)
    errs := make(chan *Error[Out])
    
    go func() {
        defer close(out)    // Close data first
        defer close(errs)   // Then close errors
        
        // Process items
        for item := range in {
            result, err := p.transform(ctx, item)
            if err != nil {
                select {
                case errs <- &Error[Out]{
                    InputData: item,
                    Err: err,
                    Timestamp: time.Now(),
                    Stage: p.Name(),
                }:
                case <-ctx.Done():
                    return
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

### Error Flow Patterns

```go
// PATTERN 1: Errors flow through (default)
// Errors continue downstream for aggregation
type FlowThrough struct{}

func (f *FlowThrough) Process(ctx context.Context, in <-chan Data) (<-chan Data, <-chan *Error[Data]) {
    out := make(chan Data)
    errs := make(chan *Error[Data])
    
    go func() {
        defer close(out)
        defer close(errs)
        
        for item := range in {
            if result, err := process(item); err != nil {
                errs <- wrapError(err, item)
                // Continue processing next item
            } else {
                out <- result
            }
        }
    }()
    
    return out, errs
}

// PATTERN 2: Circuit breaker on error rate
type CircuitBreaker struct {
    threshold float64
    window    time.Duration
}

func (cb *CircuitBreaker) Process(ctx context.Context, in <-chan Data) (<-chan Data, <-chan *Error[Data]) {
    out := make(chan Data)
    errs := make(chan *Error[Data])
    
    go func() {
        defer close(out)
        defer close(errs)
        
        var errorRate float64
        var errorWindow []time.Time
        
        for item := range in {
            // Clean old errors from window
            cutoff := time.Now().Add(-cb.window)
            errorWindow = filterAfter(errorWindow, cutoff)
            
            if result, err := process(item); err != nil {
                errorWindow = append(errorWindow, time.Now())
                errorRate = float64(len(errorWindow)) / float64(totalProcessed)
                
                if errorRate > cb.threshold {
                    // CIRCUIT OPEN - stop processing
                    errs <- &Error[Data]{
                        Err: fmt.Errorf("circuit open: error rate %.2f%% exceeds threshold", errorRate*100),
                        Metadata: map[string]any{
                            "error_rate": errorRate,
                            "threshold": cb.threshold,
                        },
                    }
                    return // Stop processing entirely
                }
                
                errs <- wrapError(err, item)
            } else {
                out <- result
            }
        }
    }()
    
    return out, errs
}
```

## 4. FromChainable Integration - Preserving pipz Context

### Surfacing pipz Errors with Full Context

```go
// FromChainable bridges pipz processors to streamz with FULL error context
type FromChainable[T any] struct {
    chainable pipz.Chainable[T]
    name      string
}

func (f *FromChainable[T]) Process(ctx context.Context, in <-chan T) (<-chan T, <-chan *Error[T]) {
    out := make(chan T)
    errs := make(chan *Error[T])
    
    go func() {
        defer close(out)
        defer close(errs)
        
        for item := range in {
            start := time.Now()
            result, err := f.chainable.Process(ctx, item)
            duration := time.Since(start)
            
            if err != nil {
                // Extract rich context from pipz.Error
                var pipzErr *pipz.Error[T]
                if errors.As(err, &pipzErr) {
                    // PRESERVE ALL PIPZ CONTEXT
                    errs <- &Error[T]{
                        Timestamp:  pipzErr.Timestamp,
                        InputData:  pipzErr.InputData,
                        OutputData: result, // Partial result if any
                        Err:        pipzErr.Err,
                        Path:       pipzErr.Path, // Complete pipz path
                        Duration:   pipzErr.Duration,
                        Stage:      f.name,
                        Metadata: map[string]any{
                            "timeout":  pipzErr.Timeout,
                            "canceled": pipzErr.Canceled,
                            "pipz_path": strings.Join(pipzErr.Path, "->"),
                        },
                    }
                } else {
                    // Wrap non-pipz errors
                    errs <- &Error[T]{
                        Timestamp:  time.Now(),
                        InputData:  item,
                        OutputData: result,
                        Err:        err,
                        Duration:   duration,
                        Stage:      f.name,
                    }
                }
            } else {
                out <- result
            }
        }
    }()
    
    return out, errs
}
```

### Preserving Error Context Through Pipeline

```go
// Errors accumulate context as they flow through pipeline
type ErrorEnricher[T any] struct {
    processor Processor[T, T]
    enrichFn  func(*Error[T]) // Add context
}

func (e *ErrorEnricher[T]) Process(ctx context.Context, in <-chan T) (<-chan T, <-chan *Error[T]) {
    data, errors := e.processor.Process(ctx, in)
    
    enrichedErrors := make(chan *Error[T])
    go func() {
        defer close(enrichedErrors)
        for err := range errors {
            e.enrichFn(err) // Add context
            enrichedErrors <- err
        }
    }()
    
    return data, enrichedErrors
}
```

## 5. Implementation Strategy - Breaking Everything Beautifully

### Phase 1: Core Interface Redesign (Do First)

```go
// NEW api.go - Complete replacement
package streamz

type Processor[In, Out any] interface {
    Process(ctx context.Context, in <-chan In) (out <-chan Out, errs <-chan *Error[Out])
    Name() string
}

type Error[T any] struct {
    Timestamp  time.Time
    InputData  any        // Input that caused error
    OutputData T          // Partial output if any
    Err        error      // Actual error
    Path       []string   // Pipeline path
    Duration   time.Duration
    Stage      string     // Current processor
    Retries    int        // Retry attempts
    Metadata   map[string]any
}

// Error implements error interface
func (e *Error[T]) Error() string {
    path := strings.Join(e.Path, " -> ")
    return fmt.Sprintf("[%s] %s failed after %v: %v", 
        path, e.Stage, e.Duration, e.Err)
}

func (e *Error[T]) Unwrap() error {
    return e.Err
}
```

### Phase 2: Convert Core Processors

```go
// Example: Mapper with error channel
func NewMapper[In, Out any](fn func(In) (Out, error)) *Mapper[In, Out] {
    return &Mapper[In, Out]{fn: fn, name: "mapper"}
}

func (m *Mapper[In, Out]) Process(ctx context.Context, in <-chan In) (<-chan Out, <-chan *Error[Out]) {
    out := make(chan Out)
    errs := make(chan *Error[Out])
    
    go func() {
        defer close(out)
        defer close(errs)
        
        for item := range in {
            start := time.Now()
            result, err := m.fn(item)
            
            if err != nil {
                select {
                case errs <- &Error[Out]{
                    Timestamp:  time.Now(),
                    InputData:  item,
                    Err:        err,
                    Duration:   time.Since(start),
                    Stage:      m.name,
                }:
                case <-ctx.Done():
                    return
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

### Phase 3: Implement FromChainable

Implement FromChainable AFTER core redesign to ensure proper error context preservation:

```go
func NewFromChainable[T any](chainable pipz.Chainable[T]) *FromChainable[T] {
    return &FromChainable[T]{
        chainable: chainable,
        name:      fmt.Sprintf("pipz:%s", chainable.Name()),
    }
}
```

## 6. Error Handling Patterns - What Becomes Possible

### Pattern 1: Error Aggregation

```go
type ErrorAggregator[T any] struct {
    window   time.Duration
    reporter func([]Error[T])
}

func (ea *ErrorAggregator[T]) Process(ctx context.Context, data <-chan T, errors <-chan *Error[T]) {
    ticker := time.NewTicker(ea.window)
    defer ticker.Stop()
    
    var errorBatch []Error[T]
    
    for {
        select {
        case err := <-errors:
            if err != nil {
                errorBatch = append(errorBatch, *err)
            }
        case <-ticker.C:
            if len(errorBatch) > 0 {
                ea.reporter(errorBatch)
                errorBatch = nil
            }
        case <-ctx.Done():
            return
        }
    }
}
```

### Pattern 2: Error-Based Routing

```go
type ErrorRouter[T any] struct {
    routes map[string]Processor[*Error[T], T]
}

func (er *ErrorRouter[T]) Process(ctx context.Context, errors <-chan *Error[T]) map[string]<-chan T {
    outputs := make(map[string]<-chan T)
    
    for name, processor := range er.routes {
        filtered := make(chan *Error[T])
        
        go func(n string, p Processor[*Error[T], T]) {
            defer close(filtered)
            for err := range errors {
                if shouldRoute(err, n) {
                    filtered <- err
                }
            }
        }(name, processor)
        
        recovered, _ := processor.Process(ctx, filtered)
        outputs[name] = recovered
    }
    
    return outputs
}
```

### Pattern 3: Circuit Breaking on Error Patterns

```go
type SmartCircuitBreaker[T any] struct {
    errorClassifier func(*Error[T]) string
    thresholds      map[string]float64
}

func (scb *SmartCircuitBreaker[T]) Process(ctx context.Context, in <-chan T) (<-chan T, <-chan *Error[T]) {
    out := make(chan T)
    errs := make(chan *Error[T])
    
    errorCounts := make(map[string]int)
    totalCounts := make(map[string]int)
    
    go func() {
        defer close(out)
        defer close(errs)
        
        for item := range in {
            result, err := scb.process(item)
            
            if err != nil {
                class := scb.errorClassifier(err)
                errorCounts[class]++
                totalCounts[class]++
                
                rate := float64(errorCounts[class]) / float64(totalCounts[class])
                if threshold, ok := scb.thresholds[class]; ok && rate > threshold {
                    // Circuit open for this error class
                    errs <- &Error[T]{
                        Err: fmt.Errorf("circuit open for %s: rate %.2f%%", class, rate*100),
                        Metadata: map[string]any{
                            "error_class": class,
                            "error_rate": rate,
                            "threshold": threshold,
                        },
                    }
                    continue // Skip processing
                }
                
                errs <- err
            } else {
                totalCounts["success"]++
                out <- result
            }
        }
    }()
    
    return out, errs
}
```

## The Right Design Philosophy

### Core Principles

1. **Errors are First-Class Data**: They flow through pipelines just like regular data
2. **Context Preservation**: Every error maintains full context of its journey
3. **Composability**: Error handling composes just like data processing
4. **Observability**: Error channels enable sophisticated monitoring and alerting
5. **Resilience**: Errors inform circuit breaking, retries, and recovery strategies

### What This Enables

- **Pipeline-Wide Error Strategies**: Instead of wrapping individual processors
- **Error Correlation**: Track error patterns across pipeline stages
- **Smart Recovery**: Route errors to specialized recovery pipelines
- **Real-Time Monitoring**: Error streams feed directly into monitoring systems
- **Testing**: Error injection and assertion becomes trivial

### Why This is Superior

Current streamz forces developers to:
- Wrap processors in DLQ/Retry individually
- Lose error context between stages
- Handle errors outside the main pipeline flow
- Implement error aggregation manually

The dual-channel architecture provides:
- Native error handling in every processor
- Automatic context accumulation
- Error streams that compose like data streams
- Built-in patterns for common error scenarios

## Recommendations

### Immediate Action: Break Everything

1. **Replace Processor Interface NOW**: This is the foundation - everything else follows
2. **Implement Error Type**: Adopt pipz.Error structure with streamz additions
3. **Convert Core Processors**: Start with Mapper, Filter, AsyncMapper as examples
4. **Build FromChainable**: Bridge to pipz with full error preservation
5. **Create Pipeline Builder**: Type-safe composition with automatic error aggregation

### Implementation Order

```
Week 1: Core Interface + Error Type
Week 2: Convert 10 core processors (Mapper, Filter, Batcher, etc.)
Week 3: FromChainable + Pipeline Builder
Week 4: Advanced processors (Circuit Breaker, Router, etc.)
Week 5: Error handling utilities (Aggregator, Reporter, Recovery)
```

### The Radical Simplification

Instead of:
```go
// Current: Error handling through wrapper hell
dlq := NewDeadLetterQueue(
    NewRetry(
        NewCircuitBreaker(processor),
        3,
    ),
)
```

We get:
```go
// New: Error handling is intrinsic
data, errors := processor.Process(ctx, input)
// Handle errors with same patterns as data
recovered := errorRecovery.Process(ctx, errors)
```

## Conclusion

With zero users and no compatibility constraints, streamz has a unique opportunity to implement the most sophisticated error handling in any Go streaming framework. The dual-channel architecture isn't just better - it's the RIGHT design that should have existed from day one.

This isn't incremental improvement. This is fixing the fundamental flaw in streamz's architecture. Every processor becomes error-aware, every pipeline becomes debuggable, and error handling becomes as composable as data processing itself.

The time to break everything is NOW, before we have users to disappoint.

---

*Intelligence Note: The analysis reveals that streamz's current error handling isn't just inadequate - it's architecturally wrong. The proposed dual-channel design aligns with functional programming principles while maintaining Go's pragmatic approach. This redesign would position streamz as the most error-aware streaming framework in the Go ecosystem.*