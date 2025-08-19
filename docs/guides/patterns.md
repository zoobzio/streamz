# Common Streaming Patterns

Real-world patterns and best practices for building production streaming systems with streamz.

## Data Processing Patterns

### 1. ETL Pipeline

**Extract → Transform → Load** with error handling and monitoring:

```go
func buildETLPipeline(ctx context.Context, rawData <-chan RawRecord) <-chan ProcessedRecord {
    // Extract: Parse and validate raw data
    parser := streamz.NewMapper(func(raw RawRecord) ParsedRecord {
        return parseRecord(raw) // Skip invalid records
    }).WithName("parse")
    
    // Transform: Clean and enrich data
    cleaner := streamz.NewFilter(isValidRecord).WithName("clean")
    enricher := streamz.NewAsyncMapper(func(ctx context.Context, record ParsedRecord) (EnrichedRecord, error) {
        return enrichWithExternalData(ctx, record)
    }).WithWorkers(10)
    
    // Load: Batch for efficient storage
    batcher := streamz.NewBatcher[EnrichedRecord](streamz.BatchConfig{
        MaxSize:    1000,
        MaxLatency: 30 * time.Second,
    })
    
    // Monitor the pipeline
    monitor := streamz.NewMonitor[EnrichedRecord](time.Minute).OnStats(func(stats streamz.StreamStats) {
        log.Info("ETL throughput", "records_per_sec", stats.Rate)
    })
    
    // Compose pipeline
    parsed := parser.Process(ctx, rawData)
    cleaned := cleaner.Process(ctx, parsed)
    monitored := monitor.Process(ctx, cleaned)
    enriched := enricher.Process(ctx, monitored)
    return batcher.Process(ctx, enriched)
}
```

### 2. Real-Time Analytics

**Stream aggregation** with windowing and metrics:

```go
func buildAnalyticsPipeline(ctx context.Context, events <-chan Event) <-chan Analytics {
    // Sample for manageable volume
    sampler := streamz.NewSample[Event](0.1) // 10% sample
    
    // Window events by time
    windower := streamz.NewTumblingWindow[Event](streamz.WindowConfig{
        Size: 5 * time.Minute,
    })
    
    // Aggregate within each window
    aggregator := streamz.NewMapper(func(window streamz.Window[Event]) Analytics {
        return calculateAnalytics(window.Items)
    }).WithName("aggregate")
    
    // Deduplicate to handle duplicates
    deduper := streamz.NewDedupe(func(a Analytics) string {
        return fmt.Sprintf("%s-%d", a.MetricName, a.WindowStart.Unix())
    }).WithTTL(10*time.Minute)
    
    sampled := sampler.Process(ctx, events)
    windowed := windower.Process(ctx, sampled)
    analytics := aggregator.Process(ctx, windowed)
    return deduper.Process(ctx, analytics)
}
```

### 3. Request/Response Processing

**API gateway pattern** with rate limiting and circuit breaking:

```go
func buildAPIGateway(ctx context.Context, requests <-chan APIRequest) <-chan APIResponse {
    // Rate limiting
    rateLimiter := streamz.NewThrottle[APIRequest](1000) // 1000 req/s
    
    // Request validation
    validator := streamz.NewFilter(func(req APIRequest) bool {
        return req.APIKey != "" && req.Endpoint != ""
    }).WithName("valid-requests")
    
    // Batch requests for efficiency  
    batcher := streamz.NewBatcher[APIRequest](streamz.BatchConfig{
        MaxSize:    10,
        MaxLatency: 50 * time.Millisecond,
    })
    
    // Process batches concurrently
    processor := streamz.NewAsyncMapper(func(ctx context.Context, batch []APIRequest) ([]APIResponse, error) {
        return processBatch(ctx, batch)
    }).WithWorkers(20)
    
    // Flatten back to individual responses
    flattener := streamz.NewFlatten[APIResponse]()
    
    // Add response monitoring
    monitor := streamz.NewMonitor[APIResponse](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.RecordAPIThroughput(stats.Rate)
    })
    
    limited := rateLimiter.Process(ctx, requests)
    validated := validator.Process(ctx, limited)
    batched := batcher.Process(ctx, validated)
    processed := processor.Process(ctx, batched)
    flattened := flattener.Process(ctx, processed)
    return monitor.Process(ctx, flattened)
}
```

## Flow Control Patterns

### 4. Backpressure Management

**Multi-tier buffering** for handling varying load:

```go
func buildResilientPipeline(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    // Tier 1: Small buffer for normal variations
    buffer1 := streamz.NewBuffer[Order](100)
    
    // Tier 2: Larger buffer for burst handling
    buffer2 := streamz.NewBuffer[Order](10000)
    
    // Tier 3: Dropping buffer for overload protection
    dropper := streamz.NewDroppingBuffer[Order](50000)
    
    // Fast preprocessing
    validator := streamz.NewFilter(isValidOrder).WithName("valid")
    
    // Slow processing with concurrency
    processor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (ProcessedOrder, error) {
        return expensiveProcessing(ctx, order)
    }).WithWorkers(50)
    
    // Monitor for alerting
    monitor := streamz.NewMonitor[Order](time.Minute).OnStats(func(stats streamz.StreamStats) {
        if stats.Rate < expectedMinRate {
            alerting.SendAlert("Pipeline overloaded", stats)
        }
    })
    
    buffered1 := buffer1.Process(ctx, input)
    buffered2 := buffer2.Process(ctx, buffered1)
    protected := dropper.Process(ctx, buffered2)
    monitored := monitor.Process(ctx, protected)
    validated := validator.Process(ctx, monitored)
    return processor.Process(ctx, validated)
}
```

### 5. Resilience Patterns

**Built-in retry and circuit breaker** for robust stream processing:

```go
func buildResilientPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // External API processor that might fail
    apiProcessor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (ProcessedOrder, error) {
        return callPaymentAPI(ctx, order)
    }).WithWorkers(10)
    
    // Add retry logic with exponential backoff
    retryProcessor := streamz.NewRetry(apiProcessor).
        MaxAttempts(3).
        BaseDelay(100 * time.Millisecond).
        MaxDelay(5 * time.Second).
        WithJitter(true).
        OnError(func(err error) bool {
            // Only retry on transient errors
            return isRetryableError(err)
        })
    
    // Wrap with circuit breaker for protection
    protected := streamz.NewCircuitBreaker(retryProcessor).
        FailureThreshold(0.5).
        MinRequests(10).
        RecoveryTimeout(30 * time.Second).
        OnOpen(func(stats streamz.CircuitStats) {
            log.Error("Circuit opened", "failures", stats.Failures, "rate", 
                float64(stats.Failures)/float64(stats.Requests))
        }).
        WithName("payment-circuit")
    
    return protected.Process(ctx, orders)
}
```

### 6. Content-Based Routing

**Route items to different processors** based on content:

```go
func buildRoutingPipeline(ctx context.Context, events <-chan Event) map[string]<-chan Event {
    // Create specialized processors
    criticalProcessor := streamz.NewAsyncMapper(handleCritical).WithWorkers(20)
    normalProcessor := streamz.NewAsyncMapper(handleNormal).WithWorkers(10)
    analyticsProcessor := streamz.NewBatcher[Event](streamz.BatchConfig{
        MaxSize:    1000,
        MaxLatency: 5 * time.Second,
    })
    
    // Route events based on type and priority
    router := streamz.NewRouter[Event]().
        AllMatches(). // Events can match multiple routes
        AddRoute("critical", func(e Event) bool {
            return e.Priority == "critical" || e.Type == "error"
        }, criticalProcessor).
        AddRoute("analytics", func(e Event) bool {
            return e.CollectMetrics
        }, analyticsProcessor).
        WithDefault(normalProcessor).
        WithBufferSize(100) // Buffer to handle processing speed differences
    
    outputs := router.Process(ctx, events)
    return outputs.Routes
}

// Advanced routing with load distribution
func buildLoadBalancedPipeline(ctx context.Context, requests <-chan Request) <-chan Response {
    // Create worker pools
    var workers []streamz.Processor[Request, Response]
    for i := 0; i < 5; i++ {
        worker := streamz.NewAsyncMapper(processRequest).
            WithWorkers(4).
            WithName(fmt.Sprintf("worker-%d", i))
        workers = append(workers, worker)
    }
    
    // Route requests to workers using consistent hashing
    router := streamz.NewRouter[Request]()
    for i, worker := range workers {
        workerIdx := i // Capture loop variable
        router.AddRoute(fmt.Sprintf("worker-%d", i), 
            func(r Request) bool {
                // Consistent hash routing
                hash := fnv.New32a()
                hash.Write([]byte(r.SessionID))
                return int(hash.Sum32()%5) == workerIdx
            }, worker)
    }
    
    // Merge all worker outputs
    return router.ProcessToSingle(ctx, requests)
}
```

## Monitoring and Observability Patterns

### 6. Multi-Level Monitoring

**Comprehensive observability** without affecting performance:

```go
func buildObservablePipeline(ctx context.Context, events <-chan Event) <-chan ProcessedEvent {
    // Input monitoring
    inputMonitor := streamz.NewMonitor[Event](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.InputRate.Set(stats.Rate)
        metrics.InputLatency.Observe(stats.AvgLatency.Seconds())
    })
    
    // Processing stage
    processor := streamz.NewAsyncMapper(processEvent).WithWorkers(10)
    
    // Output monitoring  
    outputMonitor := streamz.NewMonitor[ProcessedEvent](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.OutputRate.Set(stats.Rate)
        metrics.ProcessingLatency.Observe(stats.AvgLatency.Seconds())
    })
    
    // Error rate monitoring through sampling
    errorSampler := streamz.NewSample[ProcessedEvent](0.01) // 1% sample
    errorMonitor := streamz.NewTap[ProcessedEvent](func(event ProcessedEvent) {
        if event.Error != nil {
            metrics.ErrorRate.Inc()
            log.Error("Processing error", "error", event.Error, "event", event.ID)
        }
    })
    
    monitored := inputMonitor.Process(ctx, events)
    processed := processor.Process(ctx, monitored)
    outputMonitored := outputMonitor.Process(ctx, processed)
    sampled := errorSampler.Process(ctx, outputMonitored)
    errorTracked := errorMonitor.Process(ctx, sampled)
    
    return outputMonitored // Return full stream, not sampled
}
```

### 7. A/B Testing Pattern

**Split traffic** for experimentation using Router:

```go
func buildABTestPipeline(ctx context.Context, requests <-chan Request) <-chan Response {
    // Create variant processors
    variantA := streamz.NewAsyncMapper(func(ctx context.Context, req Request) (Response, error) {
        return processVariantA(ctx, req)
    }).WithWorkers(15).WithName("variant-a")
    
    variantB := streamz.NewAsyncMapper(func(ctx context.Context, req Request) (Response, error) {
        return processVariantB(ctx, req)
    }).WithWorkers(5).WithName("variant-b")
    
    // Route traffic based on user ID hash
    abRouter := streamz.NewRouter[Request]().
        AddRoute("variant-b", func(req Request) bool {
            // 10% to variant B
            h := fnv.New32a()
            h.Write([]byte(req.UserID))
            return h.Sum32()%100 < 10
        }, variantB).
        WithDefault(variantA).
        WithName("ab-test-router")
    
    // Process and merge results
    outputs := abRouter.Process(ctx, requests)
    
    // Monitor each variant separately
    var wg sync.WaitGroup
    responses := make(chan Response)
    
    for variant, output := range outputs.Routes {
        wg.Add(1)
        go func(v string, ch <-chan Response) {
            defer wg.Done()
            monitor := streamz.NewTap[Response](func(resp Response) {
                metrics.ABTestMetrics.WithLabelValues(v).Inc()
                metrics.ResponseTime.WithLabelValues(v).Observe(resp.Duration.Seconds())
            })
            
            monitored := monitor.Process(ctx, ch)
            for resp := range monitored {
                responses <- resp
            }
        }(variant, output)
    }
    
    go func() {
        wg.Wait()
        close(responses)
    }()
    
    return responses
}
```

## Error Handling Patterns

### 8. Built-in Retry with Exponential Backoff

**Resilient processing** using streamz's built-in retry processor:

```go
func buildResilientPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Processor that might fail (API calls, database operations, etc.)
    processor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (ProcessedOrder, error) {
        return processOrderWithExternalAPI(ctx, order)
    }).WithWorkers(10)
    
    // Wrap with retry logic
    resilient := streamz.NewRetry(processor).
        MaxAttempts(5).
        BaseDelay(100 * time.Millisecond).
        MaxDelay(30 * time.Second).
        WithJitter(true).
        WithName("order-processor-retry")
    
    return resilient.Process(ctx, orders)
}

// Custom error classification for specific retry logic
func buildAPIRetryPipeline(ctx context.Context, requests <-chan APIRequest) <-chan APIResponse {
    apiProcessor := streamz.NewAsyncMapper(callExternalAPI).WithWorkers(5)
    
    retry := streamz.NewRetry(apiProcessor).
        MaxAttempts(5).
        BaseDelay(200 * time.Millisecond).
        OnError(func(err error, attempt int) bool {
            errStr := strings.ToLower(err.Error())
            
            // Don't retry authentication errors
            if strings.Contains(errStr, "unauthorized") {
                return false
            }
            
            // Limited retries for rate limits
            if strings.Contains(errStr, "rate limit") {
                return attempt <= 2
            }
            
            // Retry timeouts and connection issues
            return strings.Contains(errStr, "timeout") ||
                   strings.Contains(errStr, "connection")
        })
    
    return retry.Process(ctx, requests)
}
```

### 9. Dead Letter Queue Pattern

**Capture failed items** for later analysis:

```go
func buildDLQPipeline(ctx context.Context, orders <-chan Order) (<-chan ProcessedOrder, <-chan FailedOrder) {
    processed := make(chan ProcessedOrder)
    failed := make(chan FailedOrder)
    
    go func() {
        defer close(processed)
        defer close(failed)
        
        for {
            select {
            case order, ok := <-orders:
                if !ok {
                    return
                }
                
                result, err := processOrder(ctx, order)
                if err != nil {
                    // Send to DLQ
                    select {
                    case failed <- FailedOrder{Order: order, Error: err, Timestamp: time.Now()}:
                    case <-ctx.Done():
                        return
                    }
                } else {
                    // Send successful result
                    select {
                    case processed <- result:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return processed, failed
}
```

### 10. Multi-Stage Routing Pattern

**Complex routing scenarios** with hierarchical processing:

```go
func buildMultiStageRoutingPipeline(ctx context.Context, events <-chan Event) <-chan ProcessedEvent {
    // Stage 1: Route by event type
    typeRouter := streamz.NewRouter[Event]().
        AddRoute("user-events", func(e Event) bool {
            return strings.HasPrefix(e.Type, "user.")
        }, buildUserEventPipeline()).
        AddRoute("system-events", func(e Event) bool {
            return strings.HasPrefix(e.Type, "system.")
        }, buildSystemEventPipeline()).
        AddRoute("business-events", func(e Event) bool {
            return strings.HasPrefix(e.Type, "business.")
        }, buildBusinessEventPipeline()).
        WithName("type-router")
    
    // Process through type-specific pipelines
    typeOutputs := typeRouter.Process(ctx, events)
    
    // Stage 2: Route outputs by priority for final processing
    priorityInputs := make(chan ProcessedEvent)
    
    // Merge type-specific outputs
    go func() {
        defer close(priorityInputs)
        var wg sync.WaitGroup
        
        for _, output := range typeOutputs.Routes {
            wg.Add(1)
            go func(ch <-chan ProcessedEvent) {
                defer wg.Done()
                for event := range ch {
                    priorityInputs <- event
                }
            }(output)
        }
        wg.Wait()
    }()
    
    // Stage 2: Priority routing
    priorityRouter := streamz.NewRouter[ProcessedEvent]().
        AddRoute("critical", func(e ProcessedEvent) bool {
            return e.Priority >= PriorityCritical
        }, streamz.NewAsyncMapper(handleCritical).WithWorkers(20)).
        AddRoute("high", func(e ProcessedEvent) bool {
            return e.Priority >= PriorityHigh
        }, streamz.NewAsyncMapper(handleHigh).WithWorkers(10)).
        WithDefault(streamz.NewAsyncMapper(handleNormal).WithWorkers(5))
    
    // Return merged output from all priority handlers
    return priorityRouter.ProcessToSingle(ctx, priorityInputs)
}

// Microservice routing pattern
func buildMicroserviceRouter(ctx context.Context, requests <-chan ServiceRequest) map[string]<-chan ServiceResponse {
    // Create service-specific processors with different configurations
    authService := streamz.NewCircuitBreaker(
        streamz.NewRetry(
            streamz.NewAsyncMapper(callAuthService).WithWorkers(5),
        ).MaxAttempts(3),
    ).FailureThreshold(0.3)
    
    userService := streamz.NewCircuitBreaker(
        streamz.NewAsyncMapper(callUserService).WithWorkers(10),
    ).FailureThreshold(0.5)
    
    orderService := streamz.NewAsyncMapper(callOrderService).WithWorkers(20)
    
    // Route requests to appropriate services
    serviceRouter := streamz.NewRouter[ServiceRequest]().
        AllMatches(). // Some requests need multiple services
        AddRoute("auth", func(r ServiceRequest) bool {
            return r.RequiresAuth
        }, authService).
        AddRoute("user", func(r ServiceRequest) bool {
            return r.Service == "user" || r.IncludeUserData
        }, userService).
        AddRoute("order", func(r ServiceRequest) bool {
            return r.Service == "order"
        }, orderService).
        WithBufferSize(100) // Handle burst traffic
    
    outputs := serviceRouter.Process(ctx, requests)
    
    // Transform output map to response map
    responseMap := make(map[string]<-chan ServiceResponse)
    for service, output := range outputs.Routes {
        responseMap[service] = transformToResponse(output)
    }
    
    return responseMap
}
```

## Performance Patterns

### 11. Batching for Efficiency

**Optimize database/API calls** with intelligent batching:

```go
func buildBatchingPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Batch similar orders together
    grouper := streamz.NewMapper(func(order Order) GroupedOrder {
        return GroupedOrder{
            Order: order,
            Group: order.CustomerID, // Group by customer
        }
    }).WithName("group")
    
    // Batch within groups
    batcher := streamz.NewBatcher[GroupedOrder](streamz.BatchConfig{
        MaxSize:    50,                    // Max 50 orders per batch
        MaxLatency: 100 * time.Millisecond, // Or every 100ms
    })
    
    // Process batches efficiently
    processor := streamz.NewAsyncMapper(func(ctx context.Context, batch []GroupedOrder) ([]ProcessedOrder, error) {
        // Group batch by customer for efficient processing
        customerBatches := groupByCustomer(batch)
        
        var results []ProcessedOrder
        for customerID, orders := range customerBatches {
            customerResults, err := processBatchForCustomer(ctx, customerID, orders)
            if err != nil {
                return nil, err
            }
            results = append(results, customerResults...)
        }
        
        return results, nil
    }).WithWorkers(10)
    
    // Flatten back to individual orders
    flattener := streamz.NewFlatten[ProcessedOrder]()
    
    grouped := grouper.Process(ctx, orders)
    batched := batcher.Process(ctx, grouped)
    processed := processor.Process(ctx, batched)
    return flattener.Process(ctx, processed)
}
```

### 12. Conditional Processing with Switch

**Route items to different processors** based on conditions:

```go
func buildConditionalPipeline(ctx context.Context, logs <-chan LogEntry) <-chan ProcessedLog {
    // Create specialized processors
    criticalProcessor := streamz.NewAsyncMapper(func(ctx context.Context, log LogEntry) (ProcessedLog, error) {
        // Send alerts, create incidents
        alerting.SendCritical(log)
        return ProcessedLog{Log: log, Action: "alerted"}, nil
    }).WithWorkers(10)
    
    securityProcessor := streamz.NewAsyncMapper(func(ctx context.Context, log LogEntry) (ProcessedLog, error) {
        // Forward to SIEM, analyze patterns
        siem.Analyze(log)
        return ProcessedLog{Log: log, Action: "security-analyzed"}, nil
    }).WithWorkers(5)
    
    standardProcessor := streamz.NewMapper(func(log LogEntry) ProcessedLog {
        return ProcessedLog{Log: log, Action: "logged"}
    })
    
    // Route based on log characteristics
    logSwitch := streamz.NewSwitch[LogEntry]().
        Case("critical", func(log LogEntry) bool {
            return log.Level == "FATAL" || log.Level == "ERROR"
        }, criticalProcessor).
        Case("security", func(log LogEntry) bool {
            return strings.Contains(log.Message, "unauthorized") ||
                   strings.Contains(log.Message, "authentication failed")
        }, securityProcessor).
        Default(standardProcessor).
        WithName("log-router")
    
    return logSwitch.Process(ctx, logs)
}
```

### 13. Multi-Tier Processing with Switch

**Different processing tiers** based on data characteristics:

```go
func buildTieredProcessing(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Premium tier - fast processing
    premiumProcessor := streamz.NewAsyncMapper(processPremiumOrder).
        WithWorkers(50).
        WithQueueSize(1000)
    
    // Standard tier - normal processing
    standardProcessor := streamz.NewAsyncMapper(processStandardOrder).
        WithWorkers(20).
        WithQueueSize(500)
    
    // Economy tier - batch processing
    economyBatcher := streamz.NewBatcher[Order](streamz.BatchConfig{
        MaxSize:    100,
        MaxLatency: 30 * time.Second,
    })
    economyProcessor := streamz.NewAsyncMapper(func(ctx context.Context, batch []Order) ([]ProcessedOrder, error) {
        return processBatch(ctx, batch)
    }).WithWorkers(5)
    
    // Create economy pipeline
    economyPipeline := streamz.ProcessorFunc[Order, ProcessedOrder](func(ctx context.Context, in <-chan Order) <-chan ProcessedOrder {
        batched := economyBatcher.Process(ctx, in)
        processed := economyProcessor.Process(ctx, batched)
        return streamz.NewFlatten[ProcessedOrder]().Process(ctx, processed)
    })
    
    // Route orders by tier
    tierSwitch := streamz.NewSwitch[Order]().
        Case("premium", func(o Order) bool {
            return o.CustomerTier == "premium" || o.Total > 10000
        }, premiumProcessor).
        Case("standard", func(o Order) bool {
            return o.CustomerTier == "standard" || o.Total > 1000
        }, standardProcessor).
        Default(economyPipeline).
        WithName("tier-router")
    
    return tierSwitch.Process(ctx, orders)
}
```

## Best Practices Summary

### 1. **Start Simple, Add Complexity Gradually**
```go
// Start with basic pipeline
pipeline := validator.Process(ctx, processor.Process(ctx, input))

// Add monitoring when needed
monitored := monitor.Process(ctx, input)
pipeline := validator.Process(ctx, processor.Process(ctx, monitored))

// Add buffering when bottlenecks appear
buffered := buffer.Process(ctx, monitored)
pipeline := validator.Process(ctx, processor.Process(ctx, buffered))
```

### 2. **Monitor at Multiple Levels**
- Input rate and patterns
- Processing latency and errors  
- Output rate and quality
- Resource utilization

### 3. **Design for Failure**
- Use circuit breakers for external dependencies
- Implement retry with exponential backoff
- Have dead letter queues for failed items
- Monitor and alert on anomalies

### 4. **Optimize for Your Workload**
- Profile before optimizing
- Use appropriate buffer sizes
- Batch when it makes sense
- Sample for monitoring heavy workloads

### 5. **Test Edge Cases**
- Context cancellation
- Channel closure
- Backpressure scenarios
- Error conditions

### 14. Key-Based Stream Partitioning

**Parallel processing** with order preservation per key:

```go
func buildPartitionedPipeline(ctx context.Context, events <-chan UserEvent) map[string]<-chan ProcessedEvent {
    // Partition by user ID for parallel processing
    partitioner := streamz.NewPartition[UserEvent](func(e UserEvent) string {
        return e.UserID
    }).WithPartitions(10).WithBufferSize(100)
    
    output := partitioner.Process(ctx, events)
    
    // Process each partition independently
    results := make(map[string]<-chan ProcessedEvent)
    
    for i, partition := range output.Partitions {
        // Create per-partition processor
        processor := streamz.NewAsyncMapper(func(ctx context.Context, event UserEvent) (ProcessedEvent, error) {
            // User events are processed in order within partition
            return processUserEvent(ctx, event)
        }).WithWorkers(2).WithName(fmt.Sprintf("partition-%d", i))
        
        results[fmt.Sprintf("partition-%d", i)] = processor.Process(ctx, partition)
    }
    
    return results
}

// Session-based partitioning for stateful processing
func buildSessionPipeline(ctx context.Context, activities <-chan Activity) <-chan SessionUpdate {
    // Partition by session to maintain state
    partitioner := streamz.NewPartition[Activity](func(a Activity) string {
        return a.SessionID
    }).WithPartitions(20).WithName("session-partitioner")
    
    output := partitioner.Process(ctx, activities)
    updates := make(chan SessionUpdate)
    
    // Process each partition with session state
    var wg sync.WaitGroup
    for i, partition := range output.Partitions {
        wg.Add(1)
        go func(partIdx int, activities <-chan Activity) {
            defer wg.Done()
            
            // Maintain session state per partition
            sessions := make(map[string]*SessionState)
            
            for activity := range activities {
                state := sessions[activity.SessionID]
                if state == nil {
                    state = NewSessionState()
                    sessions[activity.SessionID] = state
                }
                
                update := state.ProcessActivity(activity)
                
                select {
                case updates <- update:
                case <-ctx.Done():
                    return
                }
                
                // Clean up completed sessions
                if state.IsComplete() {
                    delete(sessions, activity.SessionID)
                }
            }
        }(i, partition)
    }
    
    go func() {
        wg.Wait()
        close(updates)
    }()
    
    return updates
}
```

### 15. Geographic Distribution Pattern

**Partition by location** for regional processing:

```go
func buildGeoDistributedPipeline(ctx context.Context, requests <-chan Request) map[string]<-chan Response {
    // Custom partitioner for geographic distribution
    geoPartitioner := func(key string, numPartitions int) int {
        // Extract region from request ID (e.g., "us-west-12345")
        parts := strings.Split(key, "-")
        if len(parts) < 2 {
            return 0 // Default partition
        }
        
        switch parts[0] {
        case "us":
            if parts[1] == "west" {
                return 0
            }
            return 1 // us-east
        case "eu":
            return 2
        case "asia":
            return 3
        default:
            return 4 // other regions
        }
    }
    
    // Create region-specific processors
    regionProcessors := map[int]streamz.Processor[Request, Response]{
        0: createUSWestProcessor(),
        1: createUSEastProcessor(),
        2: createEUProcessor(),
        3: createAsiaProcessor(),
        4: createDefaultProcessor(),
    }
    
    // Partition requests by region
    partitioner := streamz.NewPartition[Request](func(r Request) string {
        return r.ID // ID contains region prefix
    }).WithPartitions(5).
        WithPartitioner(geoPartitioner).
        WithName("geo-partitioner")
    
    output := partitioner.Process(ctx, requests)
    
    // Process each region with appropriate processor
    results := make(map[string]<-chan Response)
    regions := []string{"us-west", "us-east", "eu", "asia", "other"}
    
    for i, partition := range output.Partitions {
        processor := regionProcessors[i]
        region := regions[i]
        results[region] = processor.Process(ctx, partition)
    }
    
    return results
}
```

### 16. Load-Balanced Partitioning

**Even distribution** with monitoring:

```go
func buildLoadBalancedPipeline(ctx context.Context, tasks <-chan Task) <-chan Result {
    // Partition tasks evenly across workers
    partitioner := streamz.NewPartition[Task](func(t Task) string {
        return t.ID // Unique IDs ensure even distribution
    }).WithPartitions(runtime.NumCPU()).
        WithBufferSize(100).
        WithName("load-balancer")
    
    output := partitioner.Process(ctx, tasks)
    
    // Monitor partition distribution
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            stats := partitioner.GetStats()
            log.Printf("Partition distribution - Total: %d, Balance: %.2f, Per partition: %v",
                stats.TotalItems, stats.DistributionBalance(), stats.ItemsPerPartition)
            
            if stats.DistributionBalance() > 0.5 {
                log.Warn("Uneven partition distribution detected")
            }
        }
    }()
    
    // Create workers for each partition
    results := make(chan Result)
    var wg sync.WaitGroup
    
    for i, partition := range output.Partitions {
        wg.Add(1)
        go func(workerID int, tasks <-chan Task) {
            defer wg.Done()
            
            worker := NewWorker(workerID)
            for task := range tasks {
                result := worker.Process(task)
                
                select {
                case results <- result:
                case <-ctx.Done():
                    return
                }
            }
        }(i, partition)
    }
    
    go func() {
        wg.Wait()
        close(results)
    }()
    
    return results
}
```

### 17. Binary Classification with Split

**Separate streams** for different processing paths:

```go
func buildValidationPipeline(ctx context.Context, records <-chan Record) (<-chan Record, <-chan FailedRecord) {
    // Split valid and invalid records
    validator := streamz.NewSplit[Record](func(r Record) bool {
        return r.Validate() == nil
    }).WithBufferSize(100).WithName("validator")
    
    outputs := validator.Process(ctx, records)
    
    // Process valid records normally
    processed := make(chan Record)
    go func() {
        defer close(processed)
        
        processor := streamz.NewAsyncMapper(enrichRecord).WithWorkers(10)
        enriched := processor.Process(ctx, outputs.True)
        
        for record := range enriched {
            processed <- record
        }
    }()
    
    // Handle invalid records
    failures := make(chan FailedRecord)
    go func() {
        defer close(failures)
        
        for invalid := range outputs.False {
            failures <- FailedRecord{
                Record: invalid,
                Error:  invalid.Validate(),
                Time:   time.Now(),
            }
        }
    }()
    
    return processed, failures
}
```

### 18. Real-Time Analytics with Aggregation

**Compute statistics** over streaming data:

```go
func buildAnalyticsPipeline(ctx context.Context, events <-chan Event) <-chan Analytics {
    // Multiple aggregations in parallel
    
    // Count events by type
    typeCounter := streamz.NewAggregate(
        make(map[string]int),
        func(counts map[string]int, e Event) map[string]int {
            counts[e.Type]++
            return counts
        },
    ).WithTimeWindow(1 * time.Minute).WithName("type-counter")
    
    // Calculate revenue
    revenueSummer := streamz.NewAggregate(
        0.0,
        func(sum float64, e Event) float64 {
            if e.Type == "purchase" {
                sum += e.Value
            }
            return sum
        },
    ).WithTimeWindow(1 * time.Minute).WithName("revenue-tracker")
    
    // Track unique users
    uniqueUsers := streamz.NewAggregate(
        make(map[string]struct{}),
        func(users map[string]struct{}, e Event) map[string]struct{} {
            users[e.UserID] = struct{}{}
            return users
        },
    ).WithTimeWindow(1 * time.Minute).WithName("unique-users")
    
    // Fan out to all aggregators
    fanout := streamz.NewFanOut[Event](3)
    outputs := fanout.Process(ctx, events)
    
    // Process aggregations
    typeCounts := typeCounter.Process(ctx, outputs[0])
    revenue := revenueSummer.Process(ctx, outputs[1])
    users := uniqueUsers.Process(ctx, outputs[2])
    
    // Combine results
    analytics := make(chan Analytics)
    go func() {
        defer close(analytics)
        
        for {
            select {
            case window, ok := <-typeCounts:
                if !ok {
                    return
                }
                analytics <- Analytics{
                    Type:       "event_counts",
                    Window:     window.Start,
                    EventTypes: window.Result,
                }
                
            case window, ok := <-revenue:
                if !ok {
                    return
                }
                analytics <- Analytics{
                    Type:    "revenue",
                    Window:  window.Start,
                    Revenue: window.Result,
                }
                
            case window, ok := <-users:
                if !ok {
                    return
                }
                analytics <- Analytics{
                    Type:        "unique_users",
                    Window:      window.Start,
                    UniqueUsers: len(window.Result),
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return analytics
}
```

### 19. Quality Control Pipeline

**Split and aggregate** for quality monitoring:

```go
func buildQualityControlPipeline(ctx context.Context, products <-chan Product) <-chan QualityReport {
    // Split products by quality
    qualitySplitter := streamz.NewSplit[Product](func(p Product) bool {
        return p.QualityScore >= 0.95
    }).WithName("quality-splitter")
    
    outputs := qualitySplitter.Process(ctx, products)
    
    // Aggregate quality metrics
    type QualityStats struct {
        TotalCount   int
        PassCount    int
        FailCount    int
        ScoreSum     float64
        DefectTypes  map[string]int
    }
    
    // Track high quality products
    highQualityStats := streamz.NewAggregate(
        QualityStats{DefectTypes: make(map[string]int)},
        func(stats QualityStats, p Product) QualityStats {
            stats.TotalCount++
            stats.PassCount++
            stats.ScoreSum += p.QualityScore
            return stats
        },
    ).WithCountWindow(100).WithName("high-quality-stats")
    
    // Track defects
    defectStats := streamz.NewAggregate(
        QualityStats{DefectTypes: make(map[string]int)},
        func(stats QualityStats, p Product) QualityStats {
            stats.TotalCount++
            stats.FailCount++
            stats.ScoreSum += p.QualityScore
            for _, defect := range p.Defects {
                stats.DefectTypes[defect]++
            }
            return stats
        },
    ).WithCountWindow(100).WithName("defect-stats")
    
    // Process both streams
    highWindows := highQualityStats.Process(ctx, outputs.True)
    defectWindows := defectStats.Process(ctx, outputs.False)
    
    // Generate reports
    reports := make(chan QualityReport)
    go func() {
        defer close(reports)
        
        for {
            select {
            case window, ok := <-highWindows:
                if !ok {
                    return
                }
                
                reports <- QualityReport{
                    Type:         "high_quality_batch",
                    Time:         window.End,
                    Count:        window.Result.TotalCount,
                    PassRate:     1.0,
                    AverageScore: window.Result.ScoreSum / float64(window.Result.TotalCount),
                }
                
            case window, ok := <-defectWindows:
                if !ok {
                    return
                }
                
                reports <- QualityReport{
                    Type:         "defect_batch",
                    Time:         window.End,
                    Count:        window.Result.TotalCount,
                    PassRate:     0.0,
                    AverageScore: window.Result.ScoreSum / float64(window.Result.TotalCount),
                    DefectTypes:  window.Result.DefectTypes,
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Monitor split distribution
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()
        
        for range ticker.C {
            stats := qualitySplitter.GetStats()
            if stats.TrueRatio < 0.9 {
                alert("Quality degradation detected: %.1f%% pass rate", 
                    stats.TrueRatio * 100)
            }
        }
    }()
    
    return reports
}
```

## What's Next?

- **[Performance Guide](./performance.md)**: Benchmark and optimize your pipelines
- **[Testing Guide](./testing.md)**: Test streaming systems effectively
- **[Best Practices](./best-practices.md)**: Production-ready guidelines