# Pipeline Composition

Learn how to build complex streaming systems by composing simple, reusable processors into sophisticated pipelines.

## Composition Fundamentals

### The Power of Interfaces

streamz processors compose naturally because they all implement the same interface:

```go
type Processor[In, Out any] interface {
    Process(ctx context.Context, in <-chan In) <-chan Out
    Name() string
}
```

This enables **uniform composition** - any processor output can feed into any compatible processor input.

### Basic Composition

**Sequential Processing** - the most common pattern:

```go
// Type-safe pipeline: chan Order → chan Order → chan []Order → chan ProcessedOrder
orders := make(chan Order)

// Each step transforms the channel type
validated := validator.Process(ctx, orders)        // chan Order → chan Order
enriched := enricher.Process(ctx, validated)      // chan Order → chan EnrichedOrder
batched := batcher.Process(ctx, enriched)         // chan EnrichedOrder → chan []EnrichedOrder
processed := processor.Process(ctx, batched)      // chan []EnrichedOrder → chan ProcessedOrder
```

### Function Composition

Create reusable pipeline functions:

```go
// Define a reusable pipeline function
func OrderPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    validator := streamz.NewFilter(isValidOrder).WithName("valid-orders")
    enricher := streamz.NewMapper(enrichWithCustomerData).WithName("enrich")
    batcher := streamz.NewBatcher[EnrichedOrder](batchConfig)
    processor := streamz.NewMapper(processOrderBatch).WithName("process")
    
    validated := validator.Process(ctx, orders)
    enriched := enricher.Process(ctx, validated)
    batched := batcher.Process(ctx, enriched)
    return processor.Process(ctx, batched)
}

// Use the pipeline function
result := OrderPipeline(ctx, orders)
```

## Advanced Composition Patterns

### 1. Pipeline Builder Pattern

**Fluent API** for complex pipelines:

```go
type PipelineBuilder[T any] struct {
    ctx    context.Context
    stream <-chan T
}

func NewPipeline[T any](ctx context.Context, input <-chan T) *PipelineBuilder[T] {
    return &PipelineBuilder[T]{ctx: ctx, stream: input}
}

func (p *PipelineBuilder[T]) Filter(name string, predicate func(T) bool) *PipelineBuilder[T] {
    filter := streamz.NewFilter(predicate).WithName(name)
    return &PipelineBuilder[T]{
        ctx:    p.ctx,
        stream: filter.Process(p.ctx, p.stream),
    }
}

func (p *PipelineBuilder[T]) Map(name string, mapper func(T) T) *PipelineBuilder[T] {
    mapProcessor := streamz.NewMapper(mapper).WithName(name)
    return &PipelineBuilder[T]{
        ctx:    p.ctx,
        stream: mapProcessor.Process(p.ctx, p.stream),
    }
}

func (p *PipelineBuilder[T]) Build() <-chan T {
    return p.stream
}

// Usage: Fluent pipeline construction
result := NewPipeline(ctx, orders).
    Filter("valid", isValidOrder).
    Map("enrich", enrichOrder).
    Filter("premium", isPremiumOrder).
    Build()
```

### 2. Conditional Composition

**Dynamic pipeline construction** based on configuration:

```go
func BuildConfigurablePipeline(ctx context.Context, input <-chan Event, config PipelineConfig) <-chan ProcessedEvent {
    stream := input
    
    // Optional filtering
    if config.EnableFiltering {
        filter := streamz.NewFilter(config.FilterPredicate).WithName("filter")
        stream = filter.Process(ctx, stream)
    }
    
    // Optional sampling for high-volume streams
    if config.SampleRate > 0 && config.SampleRate < 1.0 {
        sampler := streamz.NewSample[Event](config.SampleRate)
        stream = sampler.Process(ctx, stream)
    }
    
    // Optional enrichment
    if config.EnableEnrichment {
        enricher := streamz.NewAsyncMapper(enrichEvent).WithWorkers(config.EnrichmentConcurrency)
        stream = enricher.Process(ctx, stream)
    }
    
    // Always process
    processor := streamz.NewMapper(processEvent).WithName("process")
    return processor.Process(ctx, stream)
}
```

### 3. Parallel Composition

**Fan-out and fan-in** for parallel processing:

```go
func BuildParallelPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Fan out to multiple processing paths
    fanout := streamz.NewFanOut[Order]()
    streams := fanout.Process(ctx, orders, 3) // 3 parallel streams
    
    // Process each stream differently
    processed1 := fastProcessor.Process(ctx, streams[0])
    processed2 := thoroughProcessor.Process(ctx, streams[1])
    processed3 := auditProcessor.Process(ctx, streams[2])
    
    // Fan in to combine results
    fanin := streamz.NewFanIn[ProcessedOrder]()
    return fanin.Process(ctx, processed1, processed2, processed3)
}
```

### 4. Hierarchical Composition

**Nested pipelines** for complex processing:

```go
// Sub-pipeline for validation
func ValidationPipeline(ctx context.Context, orders <-chan Order) <-chan ValidatedOrder {
    schemaValidator := streamz.NewFilter(validateSchema).WithName("schema")
    businessValidator := streamz.NewFilter(validateBusinessRules).WithName("business")
    enricher := streamz.NewMapper(addValidationMetadata).WithName("enrich")
    
    validated := schemaValidator.Process(ctx, orders)
    businessValidated := businessValidator.Process(ctx, validated)
    return enricher.Process(ctx, businessValidated)
}

// Sub-pipeline for processing
func ProcessingPipeline(ctx context.Context, orders <-chan ValidatedOrder) <-chan ProcessedOrder {
    batcher := streamz.NewBatcher[ValidatedOrder](batchConfig)
    processor := streamz.NewAsyncMapper(processBatch).WithWorkers(10)
    unbatcher := streamz.NewUnbatcher[ProcessedOrder]()
    
    batched := batcher.Process(ctx, orders)
    processed := processor.Process(ctx, batched)
    return unbatcher.Process(ctx, processed)
}

// Main pipeline composes sub-pipelines
func MainPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    validated := ValidationPipeline(ctx, orders)
    return ProcessingPipeline(ctx, validated)
}
```

## Type-Safe Composition

### Compile-Time Guarantees

streamz provides **compile-time type safety** throughout the pipeline:

```go
// This compiles - types match
orders := make(chan Order)                               // chan Order
validated := validator.Process(ctx, orders)             // chan Order
batched := batcher.Process(ctx, validated)              // chan []Order
processed := batchProcessor.Process(ctx, batched)       // chan []ProcessedOrder
flattened := unbatcher.Process(ctx, processed)          // chan ProcessedOrder

// This won't compile - type mismatch
wrongPipeline := unbatcher.Process(ctx, validated)      // ❌ Can't unbatch chan Order
```

### Generic Pipeline Functions

**Reusable typed pipelines**:

```go
// Generic validation pipeline
func ValidationPipeline[T any](
    ctx context.Context, 
    input <-chan T, 
    validators ...func(T) bool,
) <-chan T {
    stream := input
    
    for i, validator := range validators {
        filter := streamz.NewFilter(validator).WithName(fmt.Sprintf("validator-%d", i))
        stream = filter.Process(ctx, stream)
    }
    
    return stream
}

// Generic monitoring pipeline
func MonitoredPipeline[T any](
    ctx context.Context,
    input <-chan T,
    processor streamz.Processor[T, T],
    interval time.Duration,
) <-chan T {
    monitor := streamz.NewMonitor[T](interval).OnStats(func(stats streamz.StreamStats) {
        log.Info("Pipeline stats", "rate", stats.Rate, "latency", stats.AvgLatency)
    })
    
    monitored := monitor.Process(ctx, input)
    return processor.Process(ctx, monitored)
}

// Usage with type inference
validatedOrders := ValidationPipeline(ctx, orders, isValidOrder, hasValidPayment)
monitoredOrders := MonitoredPipeline(ctx, orders, enricher, time.Second)
```

## Error Handling in Composition

### Error Isolation

**Isolate failures** to prevent pipeline breakdown:

```go
func RobustPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Critical path - failures stop processing
    validator := streamz.NewFilter(isOrderValid).WithName("critical-validation")
    
    // Non-critical enrichment - failures are logged but don't stop pipeline
    enricher := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
        enriched, err := enrichOrder(ctx, order)
        if err != nil {
            log.Warn("Enrichment failed", "order", order.ID, "error", err)
            return order, nil // Return original order, don't fail pipeline
        }
        return enriched, nil
    }).WithWorkers(10)
    
    // Critical processing - failures stop processing
    processor := streamz.NewMapper(processOrder).WithName("critical-processing")
    
    validated := validator.Process(ctx, orders)
    enriched := enricher.Process(ctx, validated)
    return processor.Process(ctx, enriched)
}
```

### Dead Letter Queues

**Capture failed items** for later analysis:

```go
type PipelineWithDLQ[T any] struct {
    processor streamz.Processor[T, T]
    dlq       chan<- FailedItem[T]
}

func (p *PipelineWithDLQ[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        processed := p.processor.Process(ctx, in)
        for {
            select {
            case item, ok := <-processed:
                if !ok {
                    return
                }
                
                // Validate processed item
                if isValid(item) {
                    select {
                    case out <- item:
                    case <-ctx.Done():
                        return
                    }
                } else {
                    // Send to DLQ
                    select {
                    case p.dlq <- FailedItem[T]{Item: item, Reason: "validation failed"}:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}
```

## Performance Considerations

### Minimizing Goroutines

**Combine compatible processors** to reduce goroutine overhead:

```go
// ❌ Inefficient: Many small processors
func InefficientPipeline(ctx context.Context, orders <-chan Order) <-chan Order {
    p1 := streamz.NewFilter("filter1", filter1)
    p2 := streamz.NewFilter("filter2", filter2)
    p3 := streamz.NewFilter("filter3", filter3)
    
    step1 := p1.Process(ctx, orders)
    step2 := p2.Process(ctx, step1)
    return p3.Process(ctx, step2)
}

// ✅ Efficient: Combined processing
func EfficientPipeline(ctx context.Context, orders <-chan Order) <-chan Order {
    combinedFilter := streamz.NewFilter("combined", func(order Order) bool {
        return filter1(order) && filter2(order) && filter3(order)
    })
    
    return combinedFilter.Process(ctx, orders)
}
```

### Strategic Buffering

**Add buffers at bottlenecks**:

```go
func OptimizedPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Fast processing - no buffer needed
    validator := streamz.NewFilter("validate", isValid)
    
    // Potential bottleneck - add buffer
    buffer := streamz.NewBuffer[Order](10000)
    
    // Slow processing - benefits from buffering
    processor := streamz.NewAsyncMapper(5, expensiveProcessing)
    
    validated := validator.Process(ctx, orders)
    buffered := buffer.Process(ctx, validated)
    return processor.Process(ctx, buffered)
}
```

## Testing Composed Pipelines

### Unit Testing Components

**Test each processor independently**:

```go
func TestValidationStep(t *testing.T) {
    ctx := context.Background()
    validator := streamz.NewFilter("test-validator", isValidOrder)
    
    input := make(chan Order)
    output := validator.Process(ctx, input)
    
    // Test valid order passes through
    go func() {
        input <- Order{ID: "valid", Amount: 100}
        close(input)
    }()
    
    result := <-output
    assert.Equal(t, "valid", result.ID)
}
```

### Integration Testing

**Test the complete pipeline**:

```go
func TestCompletePipeline(t *testing.T) {
    ctx := context.Background()
    
    input := make(chan Order)
    output := OrderPipeline(ctx, input)
    
    // Send test data
    go func() {
        defer close(input)
        input <- Order{ID: "test1", Amount: 100}
        input <- Order{ID: "test2", Amount: 0} // Invalid
        input <- Order{ID: "test3", Amount: 200}
    }()
    
    // Collect results
    var results []ProcessedOrder
    for result := range output {
        results = append(results, result)
    }
    
    // Verify: Only valid orders processed
    assert.Len(t, results, 2)
    assert.Equal(t, "test1", results[0].ID)
    assert.Equal(t, "test3", results[1].ID)
}
```

## Best Practices

### 1. **Keep Processors Simple**
Each processor should have one clear responsibility:

```go
// ✅ Good: Single responsibility
emailValidator := streamz.NewFilter("email-validator", isValidEmail)
phoneValidator := streamz.NewFilter("phone-validator", isValidPhone)

// ❌ Avoid: Multiple responsibilities
complexValidator := streamz.NewFilter("complex", func(user User) bool {
    return isValidEmail(user.Email) && 
           isValidPhone(user.Phone) && 
           isValidAddress(user.Address)
})
```

### 2. **Use Descriptive Names**
Names help with debugging and monitoring:

```go
validator := streamz.NewFilter("order-business-rules-validator", validateBusinessRules)
enricher := streamz.NewMapper("customer-data-enricher", enrichWithCustomerData)
batcher := streamz.NewBatcher[Order](config) // "batcher" is the default name
```

### 3. **Plan for Observability**
Add monitoring at key points:

```go
func ObservablePipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Monitor input
    inputMonitor := streamz.NewMonitor[Order](time.Second, logInputStats)
    
    // Core processing
    validator := streamz.NewFilter("validator", isValid)
    processor := streamz.NewMapper("processor", processOrder)
    
    // Monitor output
    outputMonitor := streamz.NewMonitor[ProcessedOrder](time.Second, logOutputStats)
    
    monitored := inputMonitor.Process(ctx, orders)
    validated := validator.Process(ctx, monitored)
    processed := processor.Process(ctx, validated)
    return outputMonitor.Process(ctx, processed)
}
```

### 4. **Handle Context Properly**
Always pass context through the pipeline:

```go
// ✅ Good: Context flows through pipeline
func Pipeline(ctx context.Context, input <-chan T) <-chan T {
    step1 := processor1.Process(ctx, input)
    step2 := processor2.Process(ctx, step1)
    return step3.Process(ctx, step2)
}

// ❌ Avoid: Creating new contexts
func BadPipeline(ctx context.Context, input <-chan T) <-chan T {
    newCtx := context.Background() // Loses cancellation!
    return processor.Process(newCtx, input)
}
```

### 5. **Design for Reusability**
Create composable pipeline functions:

```go
// Reusable validation pipeline
func ValidationPipeline[T any](ctx context.Context, input <-chan T, validator func(T) bool) <-chan T {
    filter := streamz.NewFilter("validator", validator)
    return filter.Process(ctx, input)
}

// Reusable monitoring wrapper
func WithMonitoring[T any](ctx context.Context, input <-chan T, processor streamz.Processor[T, T]) <-chan T {
    monitor := streamz.NewMonitor[T](time.Second, logStats)
    monitored := monitor.Process(ctx, input)
    return processor.Process(ctx, monitored)
}
```

## What's Next?

- **[Backpressure](./backpressure.md)**: Handle slow consumers and flow control
- **[Error Handling](./error-handling.md)**: Advanced error strategies
- **[Performance Guide](../guides/performance.md)**: Optimize composed pipelines
- **[Testing Guide](../guides/testing.md)**: Test complex pipeline compositions