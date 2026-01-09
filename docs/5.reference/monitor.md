---
title: Monitor
description: Observe stream metrics and statistics
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - observability
---

# Monitor

Observes stream metrics like throughput, latency, and error rates without modifying the data flow. Essential for monitoring and alerting in production systems.

## Overview

The Monitor processor provides real-time observability into stream processing pipelines. It tracks key metrics and executes callback functions at regular intervals while passing all data through unchanged. This is crucial for understanding system behavior and detecting performance issues.

**Type Signature:** `chan T → chan T` (pass-through)  
**Side Effects:** Metrics collection and callback execution  
**Performance Impact:** Minimal overhead

## When to Use

- **Performance monitoring** - Track throughput and latency
- **Alerting** - Detect anomalies and performance degradation
- **Debugging** - Understand pipeline behavior during development
- **Capacity planning** - Measure system utilization and bottlenecks
- **SLA monitoring** - Ensure service level agreements are met
- **Business metrics** - Track business KPIs in real-time

## Constructor

```go
func NewMonitor[T any](interval time.Duration) *Monitor[T]
```

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `interval` | `time.Duration` | Yes | How often to report metrics |

## Fluent Methods

| Method | Description |
|--------|-------------|
| `OnStats(callback func(StreamStats))` | Sets the callback function to receive metrics |

## StreamStats Structure

```go
type StreamStats struct {
    ItemCount    int64         // Total items processed
    Rate         float64       // Items per second
    AvgLatency   time.Duration // Average processing latency
    MaxLatency   time.Duration // Maximum observed latency
    MinLatency   time.Duration // Minimum observed latency
    WindowStart  time.Time     // Start of current measurement window
    WindowEnd    time.Time     // End of current measurement window
}
```

## Examples

### Basic Throughput Monitoring

```go
// Monitor order processing rate
orderMonitor := streamz.NewMonitor[Order](time.Second).OnStats(func(stats streamz.StreamStats) {
    log.Info("Order processing stats",
        "rate", fmt.Sprintf("%.1f orders/sec", stats.Rate),
        "total", stats.ItemCount,
        "avg_latency", stats.AvgLatency)
})

orders := make(chan Order)
monitored := orderMonitor.Process(ctx, orders)

// Continue processing normally
processed := processor.Process(ctx, monitored)
```

### Multi-Stage Pipeline Monitoring

```go
func buildMonitoredPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Monitor input rate
    inputMonitor := streamz.NewMonitor[Order](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.InputRate.Set(stats.Rate)
        metrics.InputLatency.Observe(stats.AvgLatency.Seconds())
    })
    
    // Validation stage
    validator := streamz.NewFilter(isValidOrder).WithName("valid")
    
    // Monitor after validation
    validationMonitor := streamz.NewMonitor[Order](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.ValidationRate.Set(stats.Rate)
        
        // Calculate validation efficiency
        inputRate := metrics.InputRate.Get()
        if inputRate > 0 {
            efficiency := stats.Rate / inputRate
            metrics.ValidationEfficiency.Set(efficiency)
            
            if efficiency < 0.95 { // Less than 95% pass rate
                log.Warn("High validation failure rate", "efficiency", efficiency)
            }
        }
    })
    
    // Processing stage
    processor := streamz.NewAsyncMapper(expensiveProcessing).WithWorkers(10)
    
    // Monitor final output
    outputMonitor := streamz.NewMonitor[ProcessedOrder](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.OutputRate.Set(stats.Rate)
        metrics.ProcessingLatency.Observe(stats.AvgLatency.Seconds())
        
        // End-to-end latency tracking
        if stats.AvgLatency > 5*time.Second {
            alerting.SendAlert("High processing latency", map[string]interface{}{
                "avg_latency": stats.AvgLatency,
                "rate":        stats.Rate,
            })
        }
    })
    
    // Chain with monitoring at each stage
    inputMonitored := inputMonitor.Process(ctx, orders)
    validated := validator.Process(ctx, inputMonitored)
    validationMonitored := validationMonitor.Process(ctx, validated)
    processed := processor.Process(ctx, validationMonitored)
    return outputMonitor.Process(ctx, processed)
}
```

### Business Metrics Monitoring

```go
type OrderEvent struct {
    OrderID    string
    CustomerID string
    Amount     float64
    Type       string // "created", "paid", "shipped", "delivered"
    Timestamp  time.Time
}

// Monitor business KPIs
businessMonitor := streamz.NewMonitor[OrderEvent](time.Minute).OnStats(func(stats streamz.StreamStats) {
    // Revenue rate (assuming events include amount)
    revenueRate := calculateRevenueRate(stats)
    
    log.Info("Business metrics",
        "events_per_minute", stats.Rate*60,
        "revenue_per_minute", fmt.Sprintf("$%.2f", revenueRate),
        "total_events", stats.ItemCount)
    
    // Export to metrics system
    businessMetrics.EventsPerMinute.Set(stats.Rate * 60)
    businessMetrics.RevenuePerMinute.Set(revenueRate)
    
    // Alert on significant changes
    if stats.Rate < expectedMinRate {
        alerting.SendAlert("Low event rate", map[string]interface{}{
            "current_rate": stats.Rate,
            "expected_min": expectedMinRate,
        })
    }
})

events := make(chan OrderEvent)
monitored := businessMonitor.Process(ctx, events)
```

### Error Rate Monitoring

```go
type ProcessingResult struct {
    Item    Order
    Success bool
    Error   error
    Latency time.Duration
}

// Monitor error rates and patterns
errorMonitor := streamz.NewMonitor[ProcessingResult](30*time.Second).OnStats(func(stats streamz.StreamStats) {
    // This would require custom logic to track errors
    // since standard StreamStats doesn't include error information
    errorRate := calculateErrorRateFromResults(stats)
    
    log.Info("Error monitoring",
        "total_processed", stats.ItemCount,
        "error_rate", fmt.Sprintf("%.2f%%", errorRate*100),
        "avg_latency", stats.AvgLatency)
    
    if errorRate > 0.05 { // 5% error rate threshold
        alerting.SendAlert("High error rate", map[string]interface{}{
            "error_rate":     errorRate,
            "total_items":    stats.ItemCount,
            "avg_latency":    stats.AvgLatency,
            "measurement_window": stats.WindowEnd.Sub(stats.WindowStart),
        })
    }
})
```

## Advanced Monitoring Patterns

### Sliding Window Metrics

```go
type SlidingWindowMonitor[T any] struct {
    monitor      *streamz.Monitor[T]
    windowSize   time.Duration
    measurements []StreamMeasurement
    mutex        sync.RWMutex
}

type StreamMeasurement struct {
    Timestamp time.Time
    Stats     streamz.StreamStats
}

func NewSlidingWindowMonitor[T any](interval, windowSize time.Duration, callback func([]StreamMeasurement)) *SlidingWindowMonitor[T] {
    swm := &SlidingWindowMonitor[T]{
        windowSize:   windowSize,
        measurements: make([]StreamMeasurement, 0),
    }
    
    swm.monitor = streamz.NewMonitor[T](interval).OnStats(func(stats streamz.StreamStats) {
        swm.addMeasurement(stats)
        
        swm.mutex.RLock()
        recentMeasurements := swm.getRecentMeasurements()
        swm.mutex.RUnlock()
        
        callback(recentMeasurements)
    })
    
    return swm
}

func (swm *SlidingWindowMonitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    return swm.monitor.Process(ctx, in)
}

func (swm *SlidingWindowMonitor[T]) addMeasurement(stats streamz.StreamStats) {
    swm.mutex.Lock()
    defer swm.mutex.Unlock()
    
    measurement := StreamMeasurement{
        Timestamp: time.Now(),
        Stats:     stats,
    }
    
    swm.measurements = append(swm.measurements, measurement)
    
    // Remove old measurements
    cutoff := time.Now().Add(-swm.windowSize)
    for i, m := range swm.measurements {
        if m.Timestamp.After(cutoff) {
            swm.measurements = swm.measurements[i:]
            break
        }
    }
}

func (swm *SlidingWindowMonitor[T]) getRecentMeasurements() []StreamMeasurement {
    cutoff := time.Now().Add(-swm.windowSize)
    var recent []StreamMeasurement
    
    for _, m := range swm.measurements {
        if m.Timestamp.After(cutoff) {
            recent = append(recent, m)
        }
    }
    
    return recent
}

// Usage: Track trends over the last 5 minutes
trendMonitor := NewSlidingWindowMonitor(time.Second, 5*time.Minute, func(measurements []StreamMeasurement) {
    if len(measurements) < 2 {
        return
    }
    
    // Calculate trend
    trend := calculateTrend(measurements)
    
    if trend < -0.1 { // 10% decline
        log.Warn("Declining throughput trend", "trend", trend)
    }
})
```

### Percentile Monitoring

```go
type PercentileMonitor[T any] struct {
    monitor     *streamz.Monitor[T]
    latencies   []time.Duration
    maxSamples  int
    mutex       sync.RWMutex
}

func NewPercentileMonitor[T any](interval time.Duration, maxSamples int) *PercentileMonitor[T] {
    pm := &PercentileMonitor[T]{
        latencies:  make([]time.Duration, 0, maxSamples),
        maxSamples: maxSamples,
    }
    
    pm.monitor = streamz.NewMonitor[T](interval).OnStats(func(stats streamz.StreamStats) {
        pm.mutex.RLock()
        latencyCopy := make([]time.Duration, len(pm.latencies))
        copy(latencyCopy, pm.latencies)
        pm.mutex.RUnlock()
        
        if len(latencyCopy) == 0 {
            return
        }
        
        // Sort for percentile calculation
        sort.Slice(latencyCopy, func(i, j int) bool {
            return latencyCopy[i] < latencyCopy[j]
        })
        
        p50 := calculatePercentile(latencyCopy, 0.50)
        p95 := calculatePercentile(latencyCopy, 0.95)
        p99 := calculatePercentile(latencyCopy, 0.99)
        
        log.Info("Latency percentiles",
            "p50", p50,
            "p95", p95,
            "p99", p99,
            "rate", stats.Rate)
        
        // Export to metrics
        metrics.LatencyP50.Set(p50.Seconds())
        metrics.LatencyP95.Set(p95.Seconds())
        metrics.LatencyP99.Set(p99.Seconds())
    })
    
    return pm
}

func (pm *PercentileMonitor[T]) recordLatency(latency time.Duration) {
    pm.mutex.Lock()
    defer pm.mutex.Unlock()
    
    pm.latencies = append(pm.latencies, latency)
    
    // Keep only recent samples
    if len(pm.latencies) > pm.maxSamples {
        pm.latencies = pm.latencies[len(pm.latencies)-pm.maxSamples:]
    }
}

func calculatePercentile(sortedLatencies []time.Duration, percentile float64) time.Duration {
    if len(sortedLatencies) == 0 {
        return 0
    }
    
    index := int(float64(len(sortedLatencies)-1) * percentile)
    return sortedLatencies[index]
}
```

### Custom Metrics Collection

```go
type CustomMetricsMonitor[T any] struct {
    monitor      *streamz.Monitor[T]
    extractor    func(T) map[string]interface{}
    aggregator   *MetricsAggregator
}

type MetricsAggregator struct {
    counters   map[string]int64
    gauges     map[string]float64
    histograms map[string][]float64
    mutex      sync.RWMutex
}

func NewCustomMetricsMonitor[T any](
    interval time.Duration,
    extractor func(T) map[string]interface{},
) *CustomMetricsMonitor[T] {
    
    aggregator := &MetricsAggregator{
        counters:   make(map[string]int64),
        gauges:     make(map[string]float64),
        histograms: make(map[string][]float64),
    }
    
    cmm := &CustomMetricsMonitor[T]{
        extractor:  extractor,
        aggregator: aggregator,
    }
    
    cmm.monitor = streamz.NewMonitor[T](interval).OnStats(func(stats streamz.StreamStats) {
        cmm.aggregator.mutex.RLock()
        counters := make(map[string]int64)
        for k, v := range cmm.aggregator.counters {
            counters[k] = v
        }
        gauges := make(map[string]float64)
        for k, v := range cmm.aggregator.gauges {
            gauges[k] = v
        }
        cmm.aggregator.mutex.RUnlock()
        
        log.Info("Custom metrics",
            "counters", counters,
            "gauges", gauges,
            "stream_rate", stats.Rate)
        
        // Reset counters for next interval
        cmm.aggregator.resetCounters()
    })
    
    return cmm
}

func (cmm *CustomMetricsMonitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        monitored := cmm.monitor.Process(ctx, in)
        
        for {
            select {
            case item, ok := <-monitored:
                if !ok {
                    return
                }
                
                // Extract custom metrics
                metrics := cmm.extractor(item)
                cmm.aggregator.record(metrics)
                
                select {
                case out <- item:
                case <-ctx.Done():
                    return
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

// Example usage for order monitoring
orderMetricsExtractor := func(order Order) map[string]interface{} {
    return map[string]interface{}{
        "revenue":      order.Amount,
        "customer_tier": order.CustomerTier,
        "region":       order.Region,
        "is_vip":       order.IsVIP,
    }
}

customMonitor := NewCustomMetricsMonitor(time.Minute, orderMetricsExtractor)
```

## Integration with Metrics Systems

### Prometheus Integration

```go
type PrometheusMonitor[T any] struct {
    monitor     *streamz.Monitor[T]
    throughput  prometheus.Gauge
    latency     prometheus.Histogram
    totalItems  prometheus.Counter
}

func NewPrometheusMonitor[T any](name string, interval time.Duration) *PrometheusMonitor[T] {
    pm := &PrometheusMonitor[T]{
        throughput: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: fmt.Sprintf("%s_throughput_per_second", name),
            Help: "Items processed per second",
        }),
        latency: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name: fmt.Sprintf("%s_latency_seconds", name),
            Help: "Processing latency in seconds",
            Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        }),
        totalItems: prometheus.NewCounter(prometheus.CounterOpts{
            Name: fmt.Sprintf("%s_items_total", name),
            Help: "Total number of items processed",
        }),
    }
    
    // Register metrics
    prometheus.MustRegister(pm.throughput, pm.latency, pm.totalItems)
    
    pm.monitor = streamz.NewMonitor[T](interval).OnStats(func(stats streamz.StreamStats) {
        pm.throughput.Set(stats.Rate)
        pm.latency.Observe(stats.AvgLatency.Seconds())
        pm.totalItems.Add(float64(stats.ItemCount))
    })
    
    return pm
}

func (pm *PrometheusMonitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    return pm.monitor.Process(ctx, in)
}
```

### OpenTelemetry Integration

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/metric"
)

type OTelMonitor[T any] struct {
    monitor     *streamz.Monitor[T]
    meter       metric.Meter
    throughput  metric.Float64Gauge
    latency     metric.Float64Histogram
    totalItems  metric.Int64Counter
}

func NewOTelMonitor[T any](name string, interval time.Duration) (*OTelMonitor[T], error) {
    meter := otel.Meter("streamz")
    
    throughput, err := meter.Float64Gauge(
        fmt.Sprintf("%s.throughput", name),
        metric.WithDescription("Items processed per second"),
    )
    if err != nil {
        return nil, err
    }
    
    latency, err := meter.Float64Histogram(
        fmt.Sprintf("%s.latency", name),
        metric.WithDescription("Processing latency in seconds"),
    )
    if err != nil {
        return nil, err
    }
    
    totalItems, err := meter.Int64Counter(
        fmt.Sprintf("%s.items.total", name),
        metric.WithDescription("Total number of items processed"),
    )
    if err != nil {
        return nil, err
    }
    
    om := &OTelMonitor[T]{
        meter:      meter,
        throughput: throughput,
        latency:    latency,
        totalItems: totalItems,
    }
    
    om.monitor = streamz.NewMonitor[T](interval).OnStats(func(stats streamz.StreamStats) {
        ctx := context.Background()
        
        om.throughput.Record(ctx, stats.Rate)
        om.latency.Record(ctx, stats.AvgLatency.Seconds())
        om.totalItems.Add(ctx, stats.ItemCount)
    })
    
    return om, nil
}

func (om *OTelMonitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    return om.monitor.Process(ctx, in)
}
```

## Testing

```go
func TestMonitorBasicOperation(t *testing.T) {
    ctx := context.Background()
    
    var capturedStats streamz.StreamStats
    monitor := streamz.NewMonitor[int](100*time.Millisecond).OnStats(func(stats streamz.StreamStats) {
        capturedStats = stats
    })
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5})
    output := monitor.Process(ctx, input)
    
    // Consume all output
    results := collectAll(output)
    
    // Wait for monitor callback
    time.Sleep(200 * time.Millisecond)
    
    assert.Equal(t, []int{1, 2, 3, 4, 5}, results)
    assert.Equal(t, int64(5), capturedStats.ItemCount)
    assert.Greater(t, capturedStats.Rate, 0.0)
}

func TestMonitorThroughputMeasurement(t *testing.T) {
    ctx := context.Background()
    
    var stats []streamz.StreamStats
    monitor := streamz.NewMonitor[int](50*time.Millisecond).OnStats(func(s streamz.StreamStats) {
        stats = append(stats, s)
    })
    
    input := make(chan int)
    output := monitor.Process(ctx, input)
    
    // Send data at known rate
    go func() {
        defer close(input)
        for i := 0; i < 10; i++ {
            input <- i
            time.Sleep(10 * time.Millisecond) // 100 items/sec
        }
    }()
    
    // Consume output
    go func() {
        for range output {
            // Consume items
        }
    }()
    
    // Wait for measurements
    time.Sleep(200 * time.Millisecond)
    
    assert.Greater(t, len(stats), 1)
    
    // Verify reasonable throughput measurement
    for _, s := range stats {
        if s.ItemCount > 0 {
            assert.Greater(t, s.Rate, 50.0)  // At least 50/sec
            assert.Less(t, s.Rate, 200.0)    // At most 200/sec
        }
    }
}
```

## Best Practices

1. **Choose appropriate intervals** - Balance between granularity and overhead
2. **Keep callbacks fast** - Avoid blocking operations in metric callbacks
3. **Monitor at key points** - Place monitors at pipeline bottlenecks and boundaries
4. **Set up alerting** - Use metrics to trigger alerts for anomalies
5. **Track trends** - Look for patterns over time, not just point-in-time values
6. **Correlate metrics** - Compare input/output rates to detect bottlenecks
7. **Test monitoring** - Verify metrics collection in your tests

## Related Processors

- **[Tap](./tap.md)**: Execute side effects without metrics collection
- **[Buffer](./buffer.md)**: Monitor buffer utilization and backpressure
- **[AsyncMapper](./async-mapper.md)**: Monitor concurrent processing performance
- **[Batcher](./batcher.md)**: Monitor batching effectiveness

## See Also

- **[Concepts: Composition](../concepts/composition.md)**: Integrating monitoring into pipelines
- **[Guides: Best Practices](../guides/best-practices.md)**: Production monitoring strategies
- **[Guides: Performance](../guides/performance.md)**: Performance monitoring and optimization
- **[Guides: Patterns](../guides/patterns.md)**: Real-world monitoring patterns