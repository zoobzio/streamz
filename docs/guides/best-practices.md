# Best Practices for Production Streaming

Guidelines for building robust, maintainable, and efficient streaming systems with streamz.

## Architecture Principles

### 1. Single Responsibility Principle

**Each processor should have one clear purpose:**

```go
// ✅ Good: Clear, single responsibility
emailValidator := streamz.NewFilter(isValidEmail).WithName("email-validator")
phoneValidator := streamz.NewFilter(isValidPhone).WithName("phone-validator")
customerEnricher := streamz.NewMapper(enrichWithCustomerData).WithName("customer-enricher")

// ❌ Avoid: Multiple responsibilities in one processor
megaValidator := streamz.NewFilter("mega-validator", func(user User) bool {
    return isValidEmail(user.Email) && 
           isValidPhone(user.Phone) && 
           isValidAddress(user.Address) &&
           hasValidPaymentMethod(user) &&
           isNotFraudulent(user) // Too much responsibility
})
```

### 2. Fail-Safe Defaults

**Design processors to handle errors gracefully:**

```go
// ✅ Good: Graceful error handling with defaults
enricher := streamz.NewMapper(func(order Order) Order {
    enriched, err := externalEnrichment(ctx, order)
    if err != nil {
        log.Warn("Enrichment failed, using defaults", "order", order.ID, "error", err)
        return order // Return original order if enrichment fails
    }
    return enriched
}).WithName("safe-enricher")

// ✅ Good: Timeout protection
timeoutEnricher := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    return externalEnrichment(ctx, order)
}).WithWorkers(10)

// ✅ Even better: Add retry for transient failures
resilientEnricher := streamz.NewRetry(timeoutEnricher).
    MaxAttempts(3).
    BaseDelay(100 * time.Millisecond).
    WithJitter(true).
    OnError(func(err error, attempt int) bool {
        // Only retry network-related errors
        errStr := strings.ToLower(err.Error())
        return strings.Contains(errStr, "timeout") ||
               strings.Contains(errStr, "connection") ||
               strings.Contains(errStr, "network")
    })
```

### 3. Explicit Resource Management

**Make resource limits and cleanup explicit:**

```go
// ✅ Good: Explicit resource configuration
type PipelineConfig struct {
    MaxConcurrency    int           `json:"max_concurrency"`
    BufferSize        int           `json:"buffer_size"`
    BatchSize         int           `json:"batch_size"`
    BatchTimeout      time.Duration `json:"batch_timeout"`
    ProcessingTimeout time.Duration `json:"processing_timeout"`
}

func buildConfiguredPipeline(ctx context.Context, config PipelineConfig, input <-chan Order) <-chan ProcessedOrder {
    buffer := streamz.NewBuffer[Order](config.BufferSize)
    validator := streamz.NewFilter(isValidOrder).WithName("validator")
    
    processor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
        ctx, cancel := context.WithTimeout(ctx, config.ProcessingTimeout)
        defer cancel()
        return processOrder(ctx, order)
    }).WithWorkers(config.MaxConcurrency)
    
    batcher := streamz.NewBatcher[Order](streamz.BatchConfig{
        MaxSize:    config.BatchSize,
        MaxLatency: config.BatchTimeout,
    })
    
    buffered := buffer.Process(ctx, input)
    validated := validator.Process(ctx, buffered)
    processed := processor.Process(ctx, validated)
    return batcher.Process(ctx, processed)
}
```

## Content-Based Processing

### 1. Use Router for Clear Separation of Concerns

**Route different item types to specialized processors:**

```go
// ✅ Good: Clear routing logic with named routes
orderRouter := streamz.NewRouter[Order]().
    AddRoute("high-priority", func(o Order) bool {
        return o.Priority == "urgent" || o.Total > 10000
    }, highPriorityProcessor).
    AddRoute("international", func(o Order) bool {
        return o.Country != "US"
    }, internationalProcessor).
    AddRoute("subscription", func(o Order) bool {
        return o.Type == "subscription"
    }, subscriptionProcessor).
    WithDefault(standardProcessor).
    WithBufferSize(100) // Handle speed differences

// ❌ Bad: Complex if-else chains in single processor
processor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (ProcessedOrder, error) {
    if order.Priority == "urgent" || order.Total > 10000 {
        return processHighPriority(ctx, order)
    } else if order.Country != "US" {
        return processInternational(ctx, order)
    } else if order.Type == "subscription" {
        return processSubscription(ctx, order)
    } else {
        return processStandard(ctx, order)
    }
})
```

### 2. Use Circuit Breaker for External Dependencies

**Protect against cascading failures:**

```go
// ✅ Good: Circuit breaker with monitoring
apiProcessor := streamz.NewAsyncMapper(callPaymentAPI).WithWorkers(10)

protected := streamz.NewCircuitBreaker(apiProcessor).
    FailureThreshold(0.5).
    MinRequests(10).
    RecoveryTimeout(30 * time.Second).
    OnOpen(func(stats streamz.CircuitStats) {
        log.Error("Payment API circuit opened",
            "failures", stats.Failures,
            "rate", float64(stats.Failures)/float64(stats.Requests))
        // Alert operations team
        alerting.Send("Payment API Down", "critical")
    }).
    OnStateChange(func(from, to streamz.State) {
        metrics.CircuitStateChanges.WithLabelValues(
            "payment-api", from.String(), to.String()).Inc()
    })

// ✅ Best: Combine with retry for maximum resilience
resilient := streamz.NewCircuitBreaker(
    streamz.NewRetry(apiProcessor).
        MaxAttempts(3).
        BaseDelay(100 * time.Millisecond),
).FailureThreshold(0.5)
```

## Error Handling Best Practices

### 1. Error Classification

**Classify errors by type and handling strategy:**

```go
type ErrorType int

const (
    ErrorTypeTransient ErrorType = iota // Retry
    ErrorTypePermanent                  // Skip/DLQ
    ErrorTypeThrottling                 // Backoff
    ErrorTypeFatal                      // Stop pipeline
)

func classifyError(err error) ErrorType {
    if err == nil {
        return ErrorTypePermanent // Shouldn't happen, but handle it
    }
    
    errStr := strings.ToLower(err.Error())
    
    // Transient errors - worth retrying
    for _, pattern := range []string{"timeout", "connection refused", "network", "temporary"} {
        if strings.Contains(errStr, pattern) {
            return ErrorTypeTransient
        }
    }
    
    // Throttling errors - need backoff
    for _, pattern := range []string{"rate limit", "too many requests", "quota exceeded"} {
        if strings.Contains(errStr, pattern) {
            return ErrorTypeThrottling
        }
    }
    
    // Fatal errors - stop everything
    for _, pattern := range []string{"authentication failed", "invalid credentials"} {
        if strings.Contains(errStr, pattern) {
            return ErrorTypeFatal
        }
    }
    
    // Default to permanent (skip item)
    return ErrorTypePermanent
}

// Use classification in error handling
func smartErrorHandler(ctx context.Context, order Order, err error) (Order, bool) {
    errorType := classifyError(err)
    
    switch errorType {
    case ErrorTypeTransient:
        // Implement retry logic
        return retryOperation(ctx, order)
    case ErrorTypeThrottling:
        // Implement backoff
        time.Sleep(calculateBackoff())
        return retryOperation(ctx, order)
    case ErrorTypeFatal:
        // Log and stop pipeline
        log.Error("Fatal error encountered", "error", err)
        panic(fmt.Sprintf("Fatal error: %v", err))
    default: // ErrorTypePermanent
        // Log and skip
        log.Warn("Skipping item due to permanent error", "order", order.ID, "error", err)
        return order, false // Skip this item
    }
}
```

### 2. Dead Letter Queues

**Implement comprehensive DLQ strategy:**

```go
type DLQItem[T any] struct {
    Item       T         `json:"item"`
    Error      string    `json:"error"`
    Timestamp  time.Time `json:"timestamp"`
    Attempts   int       `json:"attempts"`
    StackTrace string    `json:"stack_trace,omitempty"`
    Context    map[string]interface{} `json:"context,omitempty"`
}

type DLQHandler[T any] struct {
    storage    DLQStorage[T]
    maxRetries int
    retryDelay time.Duration
}

func (d *DLQHandler[T]) HandleFailure(item T, err error, context map[string]interface{}) {
    dlqItem := DLQItem[T]{
        Item:       item,
        Error:      err.Error(),
        Timestamp:  time.Now(),
        Attempts:   1,
        StackTrace: string(debug.Stack()),
        Context:    context,
    }
    
    if err := d.storage.Store(dlqItem); err != nil {
        log.Error("Failed to store item in DLQ", "error", err, "item", item)
    }
    
    // Emit metrics
    metrics.DLQItems.Inc()
    metrics.DLQErrors.WithLabelValues(classifyErrorString(err.Error())).Inc()
}

// Reprocessing from DLQ
func (d *DLQHandler[T]) Reprocess(ctx context.Context, processor func(context.Context, T) (T, error)) error {
    items, err := d.storage.Retrieve(100) // Batch reprocessing
    if err != nil {
        return fmt.Errorf("failed to retrieve DLQ items: %w", err)
    }
    
    for _, dlqItem := range items {
        if dlqItem.Attempts >= d.maxRetries {
            log.Warn("Item exceeded max retries, permanently failing", "item", dlqItem.Item)
            continue
        }
        
        time.Sleep(d.retryDelay * time.Duration(dlqItem.Attempts)) // Exponential backoff
        
        _, err := processor(ctx, dlqItem.Item)
        if err != nil {
            dlqItem.Attempts++
            dlqItem.Error = err.Error()
            dlqItem.Timestamp = time.Now()
            d.storage.Update(dlqItem)
        } else {
            d.storage.Remove(dlqItem)
            log.Info("Successfully reprocessed DLQ item", "item", dlqItem.Item)
        }
    }
    
    return nil
}
```

## Performance Best Practices

### 1. Right-Size Your Processors

**Choose processor configurations based on workload:**

```go
// Configuration based on workload characteristics
type WorkloadConfig struct {
    Name              string
    ExpectedThroughput int           // items/sec
    AvgProcessingTime time.Duration
    MemoryPerItem     int           // bytes
    IOIntensive       bool
    CPUIntensive      bool
}

func recommendConfiguration(workload WorkloadConfig) ProcessorConfig {
    config := ProcessorConfig{}
    
    // Calculate concurrency
    if workload.IOIntensive {
        // IO-bound: higher concurrency
        config.Concurrency = min(100, workload.ExpectedThroughput/10)
    } else if workload.CPUIntensive {
        // CPU-bound: match cores
        config.Concurrency = runtime.NumCPU()
    } else {
        // Mixed: moderate concurrency
        config.Concurrency = min(20, workload.ExpectedThroughput/50)
    }
    
    // Calculate buffer size
    maxMemoryMB := 100 // 100MB limit
    maxItems := (maxMemoryMB * 1024 * 1024) / workload.MemoryPerItem
    config.BufferSize = min(maxItems, workload.ExpectedThroughput*2) // 2 seconds of buffer
    
    // Calculate batch size
    if workload.ExpectedThroughput > 1000 {
        config.BatchSize = 100 // High throughput: larger batches
    } else {
        config.BatchSize = 10 // Low throughput: smaller batches for lower latency
    }
    
    return config
}
```

### 2. Memory Management

**Implement proper memory management patterns:**

```go
// Object pooling for frequently allocated objects
var orderPool = sync.Pool{
    New: func() interface{} {
        return &Order{}
    },
}

func getOrder() *Order {
    return orderPool.Get().(*Order)
}

func putOrder(order *Order) {
    // Reset all fields to prevent memory leaks
    *order = Order{}
    orderPool.Put(order)
}

// Use pooling in processors
func pooledProcessor(ctx context.Context, input <-chan OrderData) <-chan *Order {
    mapper := streamz.NewMapper(func(data OrderData) *Order {
        order := getOrder()
        order.ID = data.ID
        order.Amount = data.Amount
        order.CustomerID = data.CustomerID
        // ... populate order
        
        // Don't put back to pool here - consumer should do it
        return order
    }).WithName("pooled")
    
    return mapper.Process(ctx, input)
}

// Pre-allocate slices with known capacity
func efficientBatchProcessor(batch []Order) []ProcessedOrder {
    results := make([]ProcessedOrder, 0, len(batch)) // Pre-allocate capacity
    
    for _, order := range batch {
        if processed := processOrderEfficiently(order); processed != nil {
            results = append(results, *processed)
        }
    }
    
    return results
}
```

### 3. Monitoring and Observability

**Implement comprehensive monitoring:**

```go
type PipelineMetrics struct {
    InputRate       prometheus.Gauge
    OutputRate      prometheus.Gauge
    ErrorRate       prometheus.Counter
    ProcessingTime  prometheus.Histogram
    QueueDepth      prometheus.Gauge
    DLQSize         prometheus.Gauge
}

func NewPipelineMetrics(name string) *PipelineMetrics {
    return &PipelineMetrics{
        InputRate: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "pipeline_input_rate",
            Help: "Rate of items entering the pipeline",
            ConstLabels: prometheus.Labels{"pipeline": name},
        }),
        OutputRate: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "pipeline_output_rate", 
            Help: "Rate of items exiting the pipeline",
            ConstLabels: prometheus.Labels{"pipeline": name},
        }),
        ErrorRate: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "pipeline_errors_total",
            Help: "Total number of processing errors",
            ConstLabels: prometheus.Labels{"pipeline": name},
        }),
        ProcessingTime: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name: "pipeline_processing_duration_seconds",
            Help: "Time spent processing items",
            ConstLabels: prometheus.Labels{"pipeline": name},
            Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        }),
    }
}

// Monitor pipeline health
func monitoredPipeline(ctx context.Context, metrics *PipelineMetrics, input <-chan Order) <-chan ProcessedOrder {
    // Input monitoring
    inputMonitor := streamz.NewMonitor[Order](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.InputRate.Set(stats.Rate)
    })
    
    // Processing with timing
    timedProcessor := streamz.NewMapper(func(order Order) ProcessedOrder {
        start := time.Now()
        defer func() {
            metrics.ProcessingTime.Observe(time.Since(start).Seconds())
        }()
        
        processed, err := processOrder(ctx, order)
        if err != nil {
            metrics.ErrorRate.Inc()
            return ProcessedOrder{} // Return empty on error
        }
        
        return processed
    }).WithName("timed")
    
    // Output monitoring
    outputMonitor := streamz.NewMonitor[ProcessedOrder](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.OutputRate.Set(stats.Rate)
    })
    
    monitored := inputMonitor.Process(ctx, input)
    processed := timedProcessor.Process(ctx, monitored)
    return outputMonitor.Process(ctx, processed)
}
```

## Configuration Management

### 1. Environment-Based Configuration

**Separate configuration by environment:**

```go
type EnvironmentConfig struct {
    Development PipelineConfig `json:"development"`
    Staging     PipelineConfig `json:"staging"`
    Production  PipelineConfig `json:"production"`
}

type PipelineConfig struct {
    Concurrency     int           `json:"concurrency"`
    BufferSize      int           `json:"buffer_size"`
    BatchSize       int           `json:"batch_size"`
    BatchTimeout    time.Duration `json:"batch_timeout"`
    RetryAttempts   int           `json:"retry_attempts"`
    CircuitBreaker  CircuitBreakerConfig `json:"circuit_breaker"`
    Monitoring      MonitoringConfig     `json:"monitoring"`
}

type CircuitBreakerConfig struct {
    Enabled         bool          `json:"enabled"`
    FailureRate     float64       `json:"failure_rate"`
    MinRequests     int           `json:"min_requests"`
    ResetTimeout    time.Duration `json:"reset_timeout"`
}

func loadConfig(env string) (PipelineConfig, error) {
    envConfig := EnvironmentConfig{
        Development: PipelineConfig{
            Concurrency:   2,
            BufferSize:    100,
            BatchSize:     10,
            BatchTimeout:  time.Second,
            RetryAttempts: 1,
            CircuitBreaker: CircuitBreakerConfig{
                Enabled:      false, // Disabled in dev
                FailureRate:  0.5,
                MinRequests:  10,
                ResetTimeout: 30 * time.Second,
            },
        },
        Staging: PipelineConfig{
            Concurrency:   10,
            BufferSize:    1000,
            BatchSize:     50,
            BatchTimeout:  5 * time.Second,
            RetryAttempts: 3,
            CircuitBreaker: CircuitBreakerConfig{
                Enabled:      true,
                FailureRate:  0.5,
                MinRequests:  20,
                ResetTimeout: time.Minute,
            },
        },
        Production: PipelineConfig{
            Concurrency:   50,
            BufferSize:    10000,
            BatchSize:     100,
            BatchTimeout:  10 * time.Second,
            RetryAttempts: 5,
            CircuitBreaker: CircuitBreakerConfig{
                Enabled:      true,
                FailureRate:  0.3,
                MinRequests:  50,
                ResetTimeout: 5 * time.Minute,
            },
        },
    }
    
    switch env {
    case "development":
        return envConfig.Development, nil
    case "staging":
        return envConfig.Staging, nil
    case "production":
        return envConfig.Production, nil
    default:
        return PipelineConfig{}, fmt.Errorf("unknown environment: %s", env)
    }
}
```

### 2. Runtime Configuration Updates

**Support hot reloading of configuration:**

```go
type ConfigurablePipeline struct {
    config     atomic.Value // Stores PipelineConfig
    processor  atomic.Value // Stores current processor
    mutex      sync.RWMutex
    reload     chan struct{}
}

func NewConfigurablePipeline(initialConfig PipelineConfig) *ConfigurablePipeline {
    cp := &ConfigurablePipeline{
        reload: make(chan struct{}, 1),
    }
    
    cp.config.Store(initialConfig)
    cp.updateProcessor(initialConfig)
    
    return cp
}

func (cp *ConfigurablePipeline) UpdateConfig(newConfig PipelineConfig) {
    cp.config.Store(newConfig)
    cp.updateProcessor(newConfig)
    
    // Signal reload
    select {
    case cp.reload <- struct{}{}:
    default: // Don't block if reload is already queued
    }
}

func (cp *ConfigurablePipeline) Process(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    output := make(chan ProcessedOrder)
    
    go func() {
        defer close(output)
        
        currentProcessor := cp.processor.Load().(streamz.Processor[Order, ProcessedOrder])
        currentOutput := currentProcessor.Process(ctx, input)
        
        for {
            select {
            case item, ok := <-currentOutput:
                if !ok {
                    return
                }
                select {
                case output <- item:
                case <-ctx.Done():
                    return
                }
                
            case <-cp.reload:
                // Configuration changed, recreate processor
                // Note: This is simplified - real implementation would need
                // graceful transition between old and new processors
                newProcessor := cp.processor.Load().(streamz.Processor[Order, ProcessedOrder])
                currentOutput = newProcessor.Process(ctx, input)
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return output
}
```

## Testing in Production

### 1. Canary Deployments

**Test new pipeline versions with subset of traffic:**

```go
type CanaryPipeline struct {
    primary   streamz.Processor[Order, ProcessedOrder]
    canary    streamz.Processor[Order, ProcessedOrder]
    splitRate float64 // Percentage to send to canary (0.0-1.0)
}

func (c *CanaryPipeline) Process(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    output := make(chan ProcessedOrder)
    
    go func() {
        defer close(output)
        
        primaryCh := make(chan Order)
        canaryCh := make(chan Order)
        
        // Start both processors
        primaryOut := c.primary.Process(ctx, primaryCh)
        canaryOut := c.canary.Process(ctx, canaryCh)
        
        // Route input based on split rate
        go func() {
            defer close(primaryCh)
            defer close(canaryCh)
            
            for {
                select {
                case order, ok := <-input:
                    if !ok {
                        return
                    }
                    
                    if rand.Float64() < c.splitRate {
                        // Send to canary
                        select {
                        case canaryCh <- order:
                        case <-ctx.Done():
                            return
                        }
                    } else {
                        // Send to primary
                        select {
                        case primaryCh <- order:
                        case <-ctx.Done():
                            return
                        }
                    }
                    
                case <-ctx.Done():
                    return
                }
            }
        }()
        
        // Merge outputs
        for {
            select {
            case result, ok := <-primaryOut:
                if !ok {
                    primaryOut = nil
                } else {
                    select {
                    case output <- result:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case result, ok := <-canaryOut:
                if !ok {
                    canaryOut = nil
                } else {
                    // Mark as canary result for comparison
                    result.Metadata = map[string]interface{}{"canary": true}
                    select {
                    case output <- result:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
            
            if primaryOut == nil && canaryOut == nil {
                return
            }
        }
    }()
    
    return output
}
```

### 2. Shadow Traffic

**Test new implementations without affecting production:**

```go
type ShadowPipeline struct {
    primary streamz.Processor[Order, ProcessedOrder]
    shadow  streamz.Processor[Order, ProcessedOrder]
    compare func(primary, shadow ProcessedOrder) bool
}

func (s *ShadowPipeline) Process(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    output := make(chan ProcessedOrder)
    
    go func() {
        defer close(output)
        
        for {
            select {
            case order, ok := <-input:
                if !ok {
                    return
                }
                
                // Process with primary (production)
                primaryResult := s.processSingle(ctx, s.primary, order)
                
                // Process with shadow (testing) - don't block on this
                go func(o Order) {
                    shadowResult := s.processSingle(ctx, s.shadow, o)
                    
                    // Compare results and log differences
                    if !s.compare(primaryResult, shadowResult) {
                        log.Warn("Shadow pipeline produced different result",
                            "order", o.ID,
                            "primary", primaryResult,
                            "shadow", shadowResult)
                    }
                }(order)
                
                // Only return primary result
                select {
                case output <- primaryResult:
                case <-ctx.Done():
                    return
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return output
}
```

## Deployment Best Practices

### 1. Health Checks

**Implement comprehensive health checking:**

```go
type PipelineHealth struct {
    pipeline     streamz.Processor[Order, ProcessedOrder]
    healthStatus atomic.Value // Stores HealthStatus
    lastCheck    atomic.Value // Stores time.Time
}

type HealthStatus struct {
    Healthy   bool              `json:"healthy"`
    Timestamp time.Time         `json:"timestamp"`
    Errors    []string          `json:"errors,omitempty"`
    Metrics   map[string]float64 `json:"metrics"`
}

func (p *PipelineHealth) HealthCheck(ctx context.Context) HealthStatus {
    status := HealthStatus{
        Timestamp: time.Now(),
        Metrics:   make(map[string]float64),
    }
    
    // Test pipeline with synthetic data
    testOrder := Order{ID: "health-check", Amount: 100}
    
    testInput := make(chan Order, 1)
    testInput <- testOrder
    close(testInput)
    
    timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    output := p.pipeline.Process(timeoutCtx, testInput)
    
    start := time.Now()
    select {
    case result, ok := <-output:
        if !ok {
            status.Errors = append(status.Errors, "no output received")
        } else {
            processingTime := time.Since(start)
            status.Metrics["processing_time_ms"] = float64(processingTime.Milliseconds())
            
            if result.ID != testOrder.ID {
                status.Errors = append(status.Errors, "incorrect result received")
            }
        }
    case <-timeoutCtx.Done():
        status.Errors = append(status.Errors, "health check timeout")
    }
    
    status.Healthy = len(status.Errors) == 0
    p.healthStatus.Store(status)
    p.lastCheck.Store(time.Now())
    
    return status
}

func (p *PipelineHealth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    status := p.HealthCheck(r.Context())
    
    w.Header().Set("Content-Type", "application/json")
    if !status.Healthy {
        w.WriteHeader(http.StatusServiceUnavailable)
    }
    
    json.NewEncoder(w).Encode(status)
}
```

### 2. Graceful Shutdown

**Implement proper shutdown procedures:**

```go
type GracefulPipeline struct {
    processor  streamz.Processor[Order, ProcessedOrder]
    shutdown   chan struct{}
    done       chan struct{}
    processing int64 // Number of items currently processing
}

func (g *GracefulPipeline) Process(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    output := make(chan ProcessedOrder)
    
    go func() {
        defer close(output)
        defer close(g.done)
        
        processorOutput := g.processor.Process(ctx, input)
        
        for {
            select {
            case result, ok := <-processorOutput:
                if !ok {
                    return
                }
                
                atomic.AddInt64(&g.processing, 1)
                
                select {
                case output <- result:
                    atomic.AddInt64(&g.processing, -1)
                case <-g.shutdown:
                    atomic.AddInt64(&g.processing, -1)
                    return
                case <-ctx.Done():
                    atomic.AddInt64(&g.processing, -1)
                    return
                }
                
            case <-g.shutdown:
                return
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return output
}

func (g *GracefulPipeline) Shutdown(timeout time.Duration) error {
    close(g.shutdown)
    
    // Wait for processing to complete or timeout
    timer := time.NewTimer(timeout)
    defer timer.Stop()
    
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-g.done:
            return nil
        case <-timer.C:
            processing := atomic.LoadInt64(&g.processing)
            return fmt.Errorf("shutdown timeout: %d items still processing", processing)
        case <-ticker.C:
            processing := atomic.LoadInt64(&g.processing)
            if processing == 0 {
                return nil
            }
            log.Info("Waiting for processing to complete", "remaining", processing)
        }
    }
}
```

## Security Best Practices

### 1. Input Validation

**Validate all input data:**

```go
func secureOrderValidator(order Order) bool {
    // Basic field validation
    if order.ID == "" || len(order.ID) > 100 {
        return false
    }
    
    if order.Amount < 0 || order.Amount > 1000000 { // Reasonable limits
        return false
    }
    
    // Prevent injection attacks
    if containsSuspiciousContent(order.CustomerID) {
        return false
    }
    
    // Validate format
    if !isValidUUID(order.ID) {
        return false
    }
    
    return true
}

func containsSuspiciousContent(input string) bool {
    suspiciousPatterns := []string{
        "<script", "javascript:", "data:", "vbscript:",
        "SELECT", "DROP", "INSERT", "UPDATE", "DELETE",
        "../", "..\\", "/etc/", "C:\\",
    }
    
    lowerInput := strings.ToLower(input)
    for _, pattern := range suspiciousPatterns {
        if strings.Contains(lowerInput, strings.ToLower(pattern)) {
            return true
        }
    }
    
    return false
}
```

### 2. Secrets Management

**Never log or expose sensitive data:**

```go
type SecureOrder struct {
    ID         string  `json:"id"`
    Amount     float64 `json:"amount"`
    CustomerID string  `json:"customer_id"`
    
    // Sensitive fields - never logged
    CreditCard string `json:"-"` // Excluded from JSON
    SSN        string `json:"-"`
    Password   string `json:"-"`
}

func (o SecureOrder) String() string {
    // Safe string representation for logging
    return fmt.Sprintf("Order{ID: %s, Amount: %.2f, CustomerID: %s}", 
        o.ID, o.Amount, o.CustomerID)
}

// Secure processing that doesn't log sensitive data
func secureProcessor(ctx context.Context, order SecureOrder) SecureOrder {
    log.Info("Processing order", "order", order) // Safe - uses String() method
    
    // Process order...
    processed := processSecurely(order)
    
    // Clear sensitive data from memory when done
    defer func() {
        order.CreditCard = ""
        order.SSN = ""
        order.Password = ""
    }()
    
    return processed
}
```

## Documentation and Maintenance

### 1. Code Documentation

**Document complex processors thoroughly:**

```go
// OrderEnricher enriches orders with customer data from multiple sources.
//
// The processor performs the following operations:
// 1. Validates the order format and required fields
// 2. Fetches customer data from the customer service (with 5s timeout)
// 3. Enriches order with customer tier, preferences, and history
// 4. Falls back to cached data if the service is unavailable
//
// Error Handling:
// - Invalid orders are logged and skipped
// - Service timeouts fall back to cached data
// - Network errors are retried up to 3 times with exponential backoff
//
// Performance:
// - Processes up to 50 orders concurrently
// - Maintains order of processing despite concurrent operations
// - Includes circuit breaker for customer service protection
//
// Configuration:
// - Concurrency: 50 (configurable via ENRICHER_CONCURRENCY)
// - Timeout: 5s (configurable via ENRICHER_TIMEOUT)
// - Retry attempts: 3 (configurable via ENRICHER_RETRIES)
type OrderEnricher struct {
    customerService CustomerService
    cache          CustomerCache
    config         EnricherConfig
    circuitBreaker *CircuitBreaker
}
```

### 2. Operational Runbooks

**Create clear operational procedures:**

```markdown
# Order Processing Pipeline Runbook

## Pipeline Overview
The order processing pipeline handles e-commerce orders through validation, enrichment, and batching stages.

## Key Metrics
- **Input Rate**: `pipeline_input_rate` (target: 1000/sec)
- **Output Rate**: `pipeline_output_rate` (should match input within 5%)
- **Error Rate**: `pipeline_errors_total` (target: <1%)
- **Processing Latency**: `pipeline_processing_duration_seconds` (target: <100ms p99)

## Common Issues

### High Error Rate
**Symptoms**: Error rate > 5%
**Likely Causes**: 
- Downstream service issues
- Invalid input data
- Configuration problems

**Investigation**:
1. Check error logs: `kubectl logs -f deployment/order-processor | grep ERROR`
2. Check downstream service health
3. Verify input data format

**Resolution**:
- If downstream issue: Enable circuit breaker, use fallback
- If data issue: Update validation rules
- If config issue: Roll back to previous configuration

### Low Throughput
**Symptoms**: Output rate < 80% of input rate
**Likely Causes**:
- Backpressure from slow consumers
- Resource constraints
- Processing bottlenecks

**Investigation**:
1. Check resource utilization: CPU, memory, network
2. Check buffer utilization metrics
3. Check downstream consumer health

**Resolution**:
- Increase buffer sizes if memory allows
- Scale up processing concurrency
- Scale downstream consumers
```

## What's Next?

These best practices provide a foundation for building production-ready streaming systems. Continue with:

- **[Testing Guide](./testing.md)**: Implement comprehensive testing strategies
- **[Performance Guide](./performance.md)**: Optimize for your specific workload
- **[API Reference](../api/)**: Understand processor capabilities and configurations
- **[Concepts](../concepts/)**: Deep dive into streaming system design