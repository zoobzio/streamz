package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zoobzio/streamz"
)

// Global pipeline configuration that evolves through sprints.
var (
	// Sprint flags - these get enabled as we progress
	EnableBatching         = false
	EnableRealTimeAlerts   = false
	EnableSmartAlerting    = false
	EnableSecurityScanning = false
	EnableBackpressure     = false

	// Configuration
	BatchSize          = 100
	BatchTimeout       = 5 * time.Second
	ErrorRateThreshold = 10
	AlertWindow        = 30 * time.Second
	BufferSize         = 10000
	SampleRate         = 0.1 // Sample 10% during overload

	// Metrics
	MetricsCollector = NewMetricsCollector()
)

// ProcessLogs is the main log processing pipeline that evolves through sprints.
func ProcessLogs(ctx context.Context, logs <-chan LogEntry) error {
	// Sprint 1: MVP - Direct database writes (inefficient)
	if !EnableBatching {
		return processMVP(ctx, logs)
	}

	// Sprint 2: Add batching for efficiency
	if !EnableRealTimeAlerts {
		return processBatched(ctx, logs)
	}

	// Sprint 3: Add real-time error detection
	if !EnableSmartAlerting {
		return processWithAlerts(ctx, logs)
	}

	// Sprint 4: Add smart rate-based alerting
	if !EnableSecurityScanning {
		return processWithSmartAlerts(ctx, logs)
	}

	// Sprint 5: Add security threat detection
	if !EnableBackpressure {
		return processWithSecurity(ctx, logs)
	}

	// Sprint 6: Full production system with backpressure
	return processFullProduction(ctx, logs)
}

// Sprint 1: MVP - Just store logs (inefficient)
func processMVP(ctx context.Context, logs <-chan LogEntry) error {
	fmt.Println("🏃 Running Sprint 1: MVP Pipeline (direct writes)")

	// Add metrics collection
	metricsProcessor := MetricsCollector.Process(ctx, logs)

	// Process logs one by one (inefficient!)
	for log := range metricsProcessor {
		// Write directly to database
		err := Database.WriteLogs(ctx, []LogEntry{log})
		if err != nil {
			MetricsCollector.IncrementDropped(1)
			fmt.Printf("❌ Failed to write log: %v\n", err)
		} else {
			MetricsCollector.IncrementStored(1)
		}

		// This is slow and will cause problems under load!
		if MetricsCollector.GetMetrics().LogsProcessed%100 == 0 {
			metrics := MetricsCollector.GetMetrics()
			fmt.Printf("📊 Processed: %d, Stored: %d, Dropped: %d\n",
				metrics.LogsProcessed, metrics.LogsStored, metrics.LogsDropped)
		}
	}

	return nil
}

// Sprint 2: Add batching for efficient storage
func processBatched(ctx context.Context, logs <-chan LogEntry) error {
	fmt.Println("🏃 Running Sprint 2: Batched Pipeline")

	// Add metrics collection
	metricsProcessor := MetricsCollector.Process(ctx, logs)

	// Create batcher for efficient database writes
	batcher := streamz.NewBatcher[LogEntry](streamz.BatchConfig{
		MaxSize:    BatchSize,
		MaxLatency: BatchTimeout,
	}, streamz.RealClock)

	// Process batches
	batches := batcher.Process(ctx, metricsProcessor)

	for batch := range batches {
		// Create log batch
		logBatch := LogBatch{
			ID:        fmt.Sprintf("batch-%d", time.Now().UnixNano()),
			Logs:      batch,
			Count:     len(batch),
			StartTime: batch[0].Timestamp,
			EndTime:   batch[len(batch)-1].Timestamp,
			Duration:  batch[len(batch)-1].Timestamp.Sub(batch[0].Timestamp),
		}

		// Write batch to database (much more efficient!)
		err := Database.WriteBatch(ctx, logBatch)
		if err != nil {
			MetricsCollector.IncrementDropped(int64(len(batch)))
			fmt.Printf("❌ Failed to write batch: %v\n", err)
		} else {
			MetricsCollector.IncrementStored(int64(len(batch)))
			if TestMode {
				fmt.Printf("✅ Stored batch of %d logs\n", len(batch))
			}
		}
	}

	return nil
}

// Sprint 3: Add real-time error detection
func processWithAlerts(ctx context.Context, logs <-chan LogEntry) error {
	fmt.Println("🏃 Running Sprint 3: Real-time Alerts Pipeline")

	// Add metrics collection
	metricsProcessor := MetricsCollector.Process(ctx, logs)

	// Fan out for parallel processing
	fanout := streamz.NewFanOut[LogEntry](2)
	streams := fanout.Process(ctx, metricsProcessor)

	// Stream 1: Batch storage (existing functionality)
	go func() {
		batcher := streamz.NewBatcher[LogEntry](streamz.BatchConfig{
			MaxSize:    BatchSize,
			MaxLatency: BatchTimeout,
		}, streamz.RealClock)

		batches := batcher.Process(ctx, streams[0])
		for batch := range batches {
			logBatch := LogBatch{
				ID:        fmt.Sprintf("batch-%d", time.Now().UnixNano()),
				Logs:      batch,
				Count:     len(batch),
				StartTime: batch[0].Timestamp,
				EndTime:   batch[len(batch)-1].Timestamp,
			}

			if err := Database.WriteBatch(ctx, logBatch); err == nil {
				MetricsCollector.IncrementStored(int64(len(batch)))
			} else {
				MetricsCollector.IncrementDropped(int64(len(batch)))
			}
		}
	}()

	// Stream 2: Real-time error detection
	errorFilter := streamz.NewFilter(func(log LogEntry) bool {
		return log.Level == LogLevelError || log.Level == LogLevelCritical
	}).WithName("error-filter")

	errors := errorFilter.Process(ctx, streams[1])

	// Send immediate alerts for each error (causes alert fatigue!)
	for errorLog := range errors {
		alert := Alert{
			ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			Type:      AlertTypeError,
			Severity:  AlertSeverityError,
			Service:   errorLog.Service,
			Title:     fmt.Sprintf("Error in %s", errorLog.Service),
			Message:   errorLog.Message,
			Timestamp: time.Now(),
			Context: map[string]interface{}{
				"log_id": errorLog.ID,
				"error":  errorLog.Error,
			},
		}

		if err := Alerting.SendAlert(ctx, alert); err == nil {
			MetricsCollector.IncrementAlerts(1)
		}
	}

	return nil
}

// Sprint 4: Add smart rate-based alerting
func processWithSmartAlerts(ctx context.Context, logs <-chan LogEntry) error {
	fmt.Println("🏃 Running Sprint 4: Smart Alerting Pipeline")

	// Add metrics collection
	metricsProcessor := MetricsCollector.Process(ctx, logs)

	// Fan out for parallel processing
	fanout := streamz.NewFanOut[LogEntry](3)
	streams := fanout.Process(ctx, metricsProcessor)

	// Stream 1: Batch storage
	go func() {
		batcher := streamz.NewBatcher[LogEntry](streamz.BatchConfig{
			MaxSize:    BatchSize,
			MaxLatency: BatchTimeout,
		}, streamz.RealClock)

		batches := batcher.Process(ctx, streams[0])
		for batch := range batches {
			logBatch := LogBatch{
				ID:    fmt.Sprintf("batch-%d", time.Now().UnixNano()),
				Logs:  batch,
				Count: len(batch),
			}

			if err := Database.WriteBatch(ctx, logBatch); err == nil {
				MetricsCollector.IncrementStored(int64(len(batch)))
			}
		}
	}()

	// Stream 2: Smart windowed alerting
	go func() {
		// Window logs for rate-based detection
		windower := streamz.NewTumblingWindow[LogEntry](AlertWindow, streamz.RealClock)

		windows := windower.Process(ctx, streams[1])

		// Aggregate windows to detect spikes
		aggregator := NewWindowAggregator(AlertWindow, ErrorRateThreshold, Alerting)
		alerts := aggregator.Process(ctx, windows)

		for alert := range alerts {
			if err := Alerting.SendAlert(ctx, alert); err == nil {
				MetricsCollector.IncrementAlerts(1)
				if TestMode {
					fmt.Printf("📊 Smart Alert: %s (severity: %s)\n", alert.Title, alert.Severity)
				}
			}
		}
	}()

	// Stream 3: Critical errors still get immediate alerts
	criticalFilter := streamz.NewFilter(func(log LogEntry) bool {
		return log.Level == LogLevelCritical
	}).WithName("critical-filter")

	criticals := criticalFilter.Process(ctx, streams[2])

	for critical := range criticals {
		alert := Alert{
			ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			Type:      AlertTypeError,
			Severity:  AlertSeverityCritical,
			Service:   critical.Service,
			Title:     fmt.Sprintf("CRITICAL: %s", critical.Service),
			Message:   critical.Message,
			Timestamp: time.Now(),
		}

		if err := Alerting.SendAlert(ctx, alert); err == nil {
			MetricsCollector.IncrementAlerts(1)
		}
	}

	return nil
}

// Sprint 5: Add security threat detection
func processWithSecurity(ctx context.Context, logs <-chan LogEntry) error {
	fmt.Println("🏃 Running Sprint 5: Security Monitoring Pipeline")

	// Add metrics collection
	metricsProcessor := MetricsCollector.Process(ctx, logs)

	// Enrich logs with user information
	enricher := NewLogEnricher(Users)
	enriched := enricher.Process(ctx, metricsProcessor)

	// Fan out for parallel processing
	fanout := streamz.NewFanOut[LogEntry](4)
	streams := fanout.Process(ctx, enriched)

	// Stream 1: Batch storage
	go func() {
		batcher := streamz.NewBatcher[LogEntry](streamz.BatchConfig{
			MaxSize:    BatchSize,
			MaxLatency: BatchTimeout,
		}, streamz.RealClock)

		batches := batcher.Process(ctx, streams[0])
		for batch := range batches {
			logBatch := LogBatch{
				ID:    fmt.Sprintf("batch-%d", time.Now().UnixNano()),
				Logs:  batch,
				Count: len(batch),
			}

			if err := Database.WriteBatch(ctx, logBatch); err == nil {
				MetricsCollector.IncrementStored(int64(len(batch)))
			}
		}
	}()

	// Stream 2: Smart windowed alerting
	go func() {
		windower := streamz.NewTumblingWindow[LogEntry](AlertWindow, streamz.RealClock)

		windows := windower.Process(ctx, streams[1])
		aggregator := NewWindowAggregator(AlertWindow, ErrorRateThreshold, Alerting)
		alerts := aggregator.Process(ctx, windows)

		for alert := range alerts {
			if err := Alerting.SendAlert(ctx, alert); err == nil {
				MetricsCollector.IncrementAlerts(1)
			}
		}
	}()

	// Stream 3: Security threat detection
	go func() {
		// Pattern matching for security threats
		patternMatcher := NewPatternMatcher(SecurityPatterns, Security)
		threats := patternMatcher.Process(ctx, streams[2])

		// Deduplicate security alerts (same threat from same IP within 5 minutes)
		deduper := streamz.NewDedupe(func(alert SecurityAlert) string {
			return fmt.Sprintf("%s-%s-%s", alert.Pattern, alert.MatchedLog.IP, alert.MatchedLog.UserID)
		}, streamz.RealClock).WithTTL(5 * time.Minute)

		dedupedThreats := deduper.Process(ctx, threats)

		for threat := range dedupedThreats {
			if err := Alerting.SendAlert(ctx, threat.Alert); err == nil {
				MetricsCollector.IncrementThreats(1)
				if TestMode {
					fmt.Printf("🔒 Security Threat: %s from IP %s\n", threat.Pattern, threat.MatchedLog.IP)
				}
			}
		}
	}()

	// Stream 4: Critical errors
	criticalFilter := streamz.NewFilter(func(log LogEntry) bool {
		return log.Level == LogLevelCritical
	}).WithName("critical-filter")

	criticals := criticalFilter.Process(ctx, streams[3])

	for critical := range criticals {
		alert := Alert{
			ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			Type:      AlertTypeError,
			Severity:  AlertSeverityCritical,
			Service:   critical.Service,
			Title:     fmt.Sprintf("CRITICAL: %s", critical.Service),
			Message:   critical.Message,
			Timestamp: time.Now(),
		}

		Alerting.SendAlert(ctx, alert)
	}

	return nil
}

// Sprint 6: Full production system with backpressure handling
func processFullProduction(ctx context.Context, logs <-chan LogEntry) error {
	fmt.Println("🏃 Running Sprint 6: Full Production Pipeline")

	// Add metrics collection
	metricsProcessor := MetricsCollector.Process(ctx, logs)

	// Add monitoring for observability
	monitor := streamz.NewMonitor[LogEntry](5*time.Second, streamz.RealClock).OnStats(func(stats streamz.StreamStats) {
		rate := stats.Rate
		if TestMode {
			fmt.Printf("📈 Processing rate: %.0f logs/sec (peak: %.0f)\n", rate, MetricsCollector.GetMetrics().PeakRate)
		}

		// Adaptive sampling during overload
		if rate > 10000 && SampleRate < 1.0 {
			fmt.Println("⚡ High load detected - enabling sampling")
		}
	})

	monitored := monitor.Process(ctx, metricsProcessor)

	// Add buffering for burst handling
	buffer := streamz.NewBuffer[LogEntry](BufferSize)
	buffered := buffer.Process(ctx, monitored)

	// Add dropping buffer for overload protection
	dropper := streamz.NewDroppingBuffer[LogEntry](BufferSize / 2).OnDrop(func(log LogEntry) {
		MetricsCollector.IncrementDropped(1)
	})
	protected := dropper.Process(ctx, buffered)

	// Sample during extreme load
	sampler := streamz.NewSample[LogEntry](1.0) // Start with no sampling
	sampled := sampler.Process(ctx, protected)

	// Enrich logs
	enricher := NewLogEnricher(Users)
	enriched := enricher.Process(ctx, sampled)

	// Fan out for parallel processing
	fanout := streamz.NewFanOut[LogEntry](4)
	streams := fanout.Process(ctx, enriched)

	// Use a wait group to coordinate all streams
	var wg sync.WaitGroup

	// Stream 1: Batch storage with adaptive batching
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use larger batches during high load
		adaptiveBatcher := streamz.NewBatcher[LogEntry](streamz.BatchConfig{
			MaxSize:    BatchSize * 5, // Larger batches for efficiency
			MaxLatency: BatchTimeout,
		}, streamz.RealClock)

		batches := adaptiveBatcher.Process(ctx, streams[0])
		for batch := range batches {
			logBatch := LogBatch{
				ID:    fmt.Sprintf("batch-%d", time.Now().UnixNano()),
				Logs:  batch,
				Count: len(batch),
			}

			if err := Database.WriteBatch(ctx, logBatch); err == nil {
				MetricsCollector.IncrementStored(int64(len(batch)))
			} else {
				MetricsCollector.IncrementDropped(int64(len(batch)))
			}
		}
	}()

	// Stream 2: Smart windowed alerting
	wg.Add(1)
	go func() {
		defer wg.Done()
		windower := streamz.NewTumblingWindow[LogEntry](AlertWindow, streamz.RealClock)
		windows := windower.Process(ctx, streams[1])
		aggregator := NewWindowAggregator(AlertWindow, ErrorRateThreshold, Alerting)
		alerts := aggregator.Process(ctx, windows)

		for alert := range alerts {
			if err := Alerting.SendAlert(ctx, alert); err == nil {
				MetricsCollector.IncrementAlerts(1)
			}
		}
	}()

	// Stream 3: Security threat detection
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Pattern matching for security threats
		patternMatcher := NewPatternMatcher(SecurityPatterns, Security)
		threats := patternMatcher.Process(ctx, streams[2])

		// Deduplicate security alerts (same threat from same IP within 5 minutes)
		deduper := streamz.NewDedupe(func(alert SecurityAlert) string {
			return fmt.Sprintf("%s-%s-%s", alert.Pattern, alert.MatchedLog.IP, alert.MatchedLog.UserID)
		}, streamz.RealClock).WithTTL(5 * time.Minute)

		dedupedThreats := deduper.Process(ctx, threats)

		for threat := range dedupedThreats {
			if err := Alerting.SendAlert(ctx, threat.Alert); err == nil {
				MetricsCollector.IncrementThreats(1)
				if TestMode {
					fmt.Printf("🔒 Security Threat: %s from IP %s\n", threat.Pattern, threat.MatchedLog.IP)
				}
			}
		}
	}()

	// Stream 4: Critical errors
	criticalFilter := streamz.NewFilter(func(log LogEntry) bool {
		return log.Level == LogLevelCritical
	}).WithName("critical-filter")

	criticals := criticalFilter.Process(ctx, streams[3])

	for critical := range criticals {
		alert := Alert{
			ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			Type:      AlertTypeError,
			Severity:  AlertSeverityCritical,
			Service:   critical.Service,
			Title:     fmt.Sprintf("CRITICAL: %s", critical.Service),
			Message:   critical.Message,
			Timestamp: time.Now(),
		}

		Alerting.SendAlert(ctx, alert)
		MetricsCollector.IncrementAlerts(1)
	}

	// Wait for all streams to complete
	wg.Wait()
	return nil
}

// ResetPipeline resets all pipeline configuration.
func ResetPipeline() {
	EnableBatching = false
	EnableRealTimeAlerts = false
	EnableSmartAlerting = false
	EnableSecurityScanning = false
	EnableBackpressure = false
	MetricsCollector = NewMetricsCollector()
}
