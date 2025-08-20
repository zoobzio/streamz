package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zoobzio/streamz"
)

// Custom processors that extend streamz for domain-specific needs.

// PatternMatcher detects security threats by matching patterns in logs.
type PatternMatcher struct {
	name     string
	patterns []SecurityPattern
	service  *SecurityService
}

// NewPatternMatcher creates a new security pattern matcher.
func NewPatternMatcher(patterns []SecurityPattern, service *SecurityService) *PatternMatcher {
	return &PatternMatcher{
		name:     "security-pattern-matcher",
		patterns: patterns,
		service:  service,
	}
}

// Name returns the processor name.
func (pm *PatternMatcher) Name() string {
	return pm.name
}

// Process analyzes logs for security threats.
func (pm *PatternMatcher) Process(ctx context.Context, in <-chan LogEntry) <-chan SecurityAlert {
	out := make(chan SecurityAlert)

	go func() {
		defer close(out)

		for {
			select {
			case log, ok := <-in:
				if !ok {
					return
				}

				// Analyze log for security threats
				if alert, err := pm.service.AnalyzeLog(ctx, log); err == nil && alert != nil {
					select {
					case out <- *alert:
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

// WindowAggregator counts events within time windows and generates alerts.
type WindowAggregator struct {
	name           string
	windowSize     time.Duration
	alertThreshold int
	alertService   *AlertService
}

// NewWindowAggregator creates a new window aggregator for rate-based alerting.
func NewWindowAggregator(windowSize time.Duration, threshold int, alertService *AlertService) *WindowAggregator {
	return &WindowAggregator{
		name:           "window-aggregator",
		windowSize:     windowSize,
		alertThreshold: threshold,
		alertService:   alertService,
	}
}

// Name returns the processor name.
func (wa *WindowAggregator) Name() string {
	return wa.name
}

// Process aggregates logs in windows and generates alerts for anomalies.
func (wa *WindowAggregator) Process(ctx context.Context, in <-chan streamz.Window[LogEntry]) <-chan Alert {
	out := make(chan Alert)

	go func() {
		defer close(out)

		for {
			select {
			case window, ok := <-in:
				if !ok {
					return
				}

				// Count errors by service
				errorCounts := make(map[string]int)
				var totalErrors int

				for _, log := range window.Items {
					if log.Level == LogLevelError || log.Level == LogLevelCritical {
						errorCounts[log.Service]++
						totalErrors++
					}
				}

				// Generate alerts for services exceeding threshold
				for service, count := range errorCounts {
					if count >= wa.alertThreshold {
						alert := Alert{
							ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
							Type:      AlertTypeErrorSpike,
							Severity:  AlertSeverityWarning,
							Service:   service,
							Title:     fmt.Sprintf("Error spike detected in %s", service),
							Message:   fmt.Sprintf("Detected %d errors in %v window (threshold: %d)", count, wa.windowSize, wa.alertThreshold),
							Timestamp: time.Now(),
							Count:     count,
							Window:    wa.windowSize,
							Context: map[string]interface{}{
								"window_start": window.Start,
								"window_end":   window.End,
								"total_logs":   len(window.Items),
							},
						}

						// Upgrade severity if really bad
						if count >= wa.alertThreshold*2 {
							alert.Severity = AlertSeverityError
						}
						if count >= wa.alertThreshold*3 {
							alert.Severity = AlertSeverityCritical
						}

						select {
						case out <- alert:
						case <-ctx.Done():
							return
						}
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// LogEnricher adds user information to logs.
type LogEnricher struct {
	name        string
	userService *UserService
	cache       sync.Map // Simple cache for user lookups
}

// NewLogEnricher creates a new log enricher.
func NewLogEnricher(userService *UserService) *LogEnricher {
	return &LogEnricher{
		name:        "log-enricher",
		userService: userService,
	}
}

// Name returns the processor name.
func (le *LogEnricher) Name() string {
	return le.name
}

// Process enriches logs with additional information.
func (le *LogEnricher) Process(ctx context.Context, in <-chan LogEntry) <-chan LogEntry {
	out := make(chan LogEntry)

	go func() {
		defer close(out)

		for {
			select {
			case log, ok := <-in:
				if !ok {
					return
				}

				// Enrich with user information if available
				if log.UserID != "" {
					// Check cache first
					if cached, ok := le.cache.Load(log.UserID); ok {
						log.Metadata["user_name"] = cached.(string)
					} else {
						// Lookup user
						if userName, err := le.userService.LookupUser(ctx, log.UserID); err == nil {
							log.Metadata["user_name"] = userName
							le.cache.Store(log.UserID, userName)
						}
					}
				}

				// Add processing timestamp
				log.Metadata["processed_at"] = time.Now().Format(time.RFC3339)

				select {
				case out <- log:
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

// RateLimiter implements a simple rate limiter for log processing.
type RateLimiter struct {
	name       string
	ratePerSec int
	tokens     int64
	lastRefill time.Time
	mutex      sync.Mutex
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(ratePerSec int) *RateLimiter {
	return &RateLimiter{
		name:       fmt.Sprintf("rate-limiter-%d", ratePerSec),
		ratePerSec: ratePerSec,
		tokens:     int64(ratePerSec),
		lastRefill: time.Now(),
	}
}

// Name returns the processor name.
func (rl *RateLimiter) Name() string {
	return rl.name
}

// Process rate limits the log stream.
func (rl *RateLimiter) Process(ctx context.Context, in <-chan LogEntry) <-chan LogEntry {
	out := make(chan LogEntry)

	go func() {
		defer close(out)

		ticker := time.NewTicker(time.Second / time.Duration(rl.ratePerSec))
		defer ticker.Stop()

		for {
			select {
			case log, ok := <-in:
				if !ok {
					return
				}

				// Wait for token
				<-ticker.C

				select {
				case out <- log:
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

// MetricsCollectorProcessor collects metrics about log processing.
type MetricsCollectorProcessor struct {
	metrics   *Metrics
	startTime time.Time
	mu        sync.RWMutex // Protects rate fields
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollectorProcessor {
	return &MetricsCollectorProcessor{
		metrics:   &Metrics{StartTime: time.Now()},
		startTime: time.Now(),
	}
}

// Name returns the processor name.
func (mc *MetricsCollectorProcessor) Name() string {
	return "metrics-collector"
}

// Process collects metrics while passing logs through.
func (mc *MetricsCollectorProcessor) Process(ctx context.Context, in <-chan LogEntry) <-chan LogEntry {
	out := make(chan LogEntry)

	go func() {
		defer close(out)

		lastRateCalc := time.Now()
		var count int64

		for {
			select {
			case log, ok := <-in:
				if !ok {
					return
				}

				atomic.AddInt64(&mc.metrics.LogsProcessed, 1)
				count++

				// Update rate every second
				if time.Since(lastRateCalc) >= time.Second {
					rate := float64(count) / time.Since(lastRateCalc).Seconds()
					mc.mu.Lock()
					mc.metrics.CurrentRate = rate
					if rate > mc.metrics.PeakRate {
						mc.metrics.PeakRate = rate
					}
					mc.mu.Unlock()
					count = 0
					lastRateCalc = time.Now()
				}

				select {
				case out <- log:
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

// GetMetrics returns current metrics.
func (mc *MetricsCollectorProcessor) GetMetrics() Metrics {
	mc.mu.RLock()
	m := Metrics{
		StartTime:       mc.metrics.StartTime,
		CurrentRate:     mc.metrics.CurrentRate,
		PeakRate:        mc.metrics.PeakRate,
		AverageLatency:  mc.metrics.AverageLatency,
		LastError:       mc.metrics.LastError,
		LastErrorTime:   mc.metrics.LastErrorTime,
	}
	mc.mu.RUnlock()
	
	// Use atomic loads for all counter fields
	m.LogsProcessed = atomic.LoadInt64(&mc.metrics.LogsProcessed)
	m.LogsStored = atomic.LoadInt64(&mc.metrics.LogsStored)
	m.LogsDropped = atomic.LoadInt64(&mc.metrics.LogsDropped)
	m.AlertsSent = atomic.LoadInt64(&mc.metrics.AlertsSent)
	m.SecurityThreats = atomic.LoadInt64(&mc.metrics.SecurityThreats)
	m.BatchesCreated = atomic.LoadInt64(&mc.metrics.BatchesCreated)
	m.DatabaseWrites = atomic.LoadInt64(&mc.metrics.DatabaseWrites)
	m.ProcessingErrors = atomic.LoadInt64(&mc.metrics.ProcessingErrors)
	
	return m
}

// IncrementStored increments the stored logs counter.
func (mc *MetricsCollectorProcessor) IncrementStored(count int64) {
	atomic.AddInt64(&mc.metrics.LogsStored, count)
}

// IncrementDropped increments the dropped logs counter.
func (mc *MetricsCollectorProcessor) IncrementDropped(count int64) {
	atomic.AddInt64(&mc.metrics.LogsDropped, count)
}

// IncrementAlerts increments the alerts counter.
func (mc *MetricsCollectorProcessor) IncrementAlerts(count int64) {
	atomic.AddInt64(&mc.metrics.AlertsSent, count)
}

// IncrementThreats increments the security threats counter.
func (mc *MetricsCollectorProcessor) IncrementThreats(count int64) {
	atomic.AddInt64(&mc.metrics.SecurityThreats, count)
}
