package main

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/streamz"
)

func TestMVPPipeline(t *testing.T) {
	ctx := context.Background()
	ResetPipeline()
	ResetServices()

	// Generate test logs
	logs := []LogEntry{
		createLogEntry(LogLevelInfo, "api", "Test message 1"),
		createLogEntry(LogLevelError, "web", "Test error"),
		createLogEntry(LogLevelWarn, "auth", "Test warning"),
	}

	// Process logs
	logChan := make(chan LogEntry, len(logs))
	for _, log := range logs {
		logChan <- log
	}
	close(logChan)

	err := processMVP(ctx, logChan)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}

	// Verify metrics
	metrics := MetricsCollector.GetMetrics()
	if metrics.LogsProcessed != 3 {
		t.Errorf("Expected 3 logs processed, got %d", metrics.LogsProcessed)
	}
	if metrics.LogsStored != 3 {
		t.Errorf("Expected 3 logs stored, got %d", metrics.LogsStored)
	}
}

func TestBatchedPipeline(t *testing.T) {
	ctx := context.Background()
	ResetPipeline()
	ResetServices()
	EnableBatching = true

	// Set small batch for testing
	originalBatchSize := BatchSize
	BatchSize = 2
	defer func() { BatchSize = originalBatchSize }()

	// Generate test logs
	logs := make([]LogEntry, 5)
	for i := 0; i < 5; i++ {
		logs[i] = createLogEntry(LogLevelInfo, "test", "Message")
	}

	// Process logs
	logChan := make(chan LogEntry, len(logs))
	for _, log := range logs {
		logChan <- log
	}
	close(logChan)

	err := processBatched(ctx, logChan)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}

	// Verify batching
	_, _, batches := Database.GetStats()
	if batches != 3 { // 5 logs in batches of 2 = 3 batches
		t.Errorf("Expected 3 batches, got %d", batches)
	}
}

func TestSecurityPatternDetection(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		log      LogEntry
		expected bool
	}{
		{
			name: "SQL injection in path",
			log: LogEntry{
				Path:    "/api/users?id=1' OR '1'='1",
				Message: "Invalid request",
			},
			expected: true,
		},
		{
			name: "Path traversal attempt",
			log: LogEntry{
				Path:    "/files/../../../etc/passwd",
				Message: "File access",
			},
			expected: true,
		},
		{
			name: "Normal log",
			log: LogEntry{
				Path:    "/api/users/123",
				Message: "User retrieved",
			},
			expected: false,
		},
	}

	patternMatcher := NewPatternMatcher(SecurityPatterns, Security)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logChan := make(chan LogEntry, 1)
			logChan <- tt.log
			close(logChan)

			threats := patternMatcher.Process(ctx, logChan)

			threatDetected := false
			for range threats {
				threatDetected = true
			}

			if threatDetected != tt.expected {
				t.Errorf("Expected threat detection: %v, got: %v", tt.expected, threatDetected)
			}
		})
	}
}

func TestWindowAggregator(t *testing.T) {
	ctx := context.Background()

	// Create test window
	window := streamz.Window[LogEntry]{
		Items: []LogEntry{
			createErrorLog("api", "Error 1", "Database error"),
			createErrorLog("api", "Error 2", "Database error"),
			createErrorLog("api", "Error 3", "Database error"),
			createLogEntry(LogLevelInfo, "api", "Normal log"),
			createErrorLog("web", "Web error", "404"),
		},
		Start: time.Now().Add(-30 * time.Second),
		End:   time.Now(),
	}

	// Test with threshold of 3
	aggregator := NewWindowAggregator(30*time.Second, 3, Alerting)

	windowChan := make(chan streamz.Window[LogEntry], 1)
	windowChan <- window
	close(windowChan)

	alerts := aggregator.Process(ctx, windowChan)

	alertCount := 0
	for alert := range alerts {
		alertCount++
		if alert.Service != "api" {
			t.Errorf("Expected alert for 'api' service, got %s", alert.Service)
		}
		if alert.Count != 3 {
			t.Errorf("Expected count 3, got %d", alert.Count)
		}
	}

	if alertCount != 1 {
		t.Errorf("Expected 1 alert, got %d", alertCount)
	}
}

func TestLogEnricher(t *testing.T) {
	ctx := context.Background()

	enricher := NewLogEnricher(Users)

	log := createLogEntry(LogLevelInfo, "api", "Test")
	log.UserID = "user-001"

	logChan := make(chan LogEntry, 1)
	logChan <- log
	close(logChan)

	enriched := enricher.Process(ctx, logChan)

	for enrichedLog := range enriched {
		if enrichedLog.Metadata["user_name"] != "Alice Johnson" {
			t.Errorf("Expected user name 'Alice Johnson', got %s", enrichedLog.Metadata["user_name"])
		}
		if enrichedLog.Metadata["processed_at"] == "" {
			t.Error("Expected processed_at timestamp")
		}
	}
}

func TestFullPipelineIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ResetPipeline()
	ResetServices()
	TestMode = true // Enable test mode
	EnableBatching = true
	EnableRealTimeAlerts = true
	EnableSmartAlerting = true
	EnableSecurityScanning = true
	EnableBackpressure = true

	// Generate mixed logs
	logs := make([]LogEntry, 0, 100)

	// Normal logs
	for i := 0; i < 50; i++ {
		logs = append(logs, createLogEntry(LogLevelInfo, "api", "Normal operation"))
	}

	// Some errors (enough to trigger rate-based alerts)
	for i := 0; i < 15; i++ {
		logs = append(logs, createErrorLog("payment", "Payment failed", "Timeout"))
	}
	// Add some critical errors for immediate alerts
	for i := 0; i < 5; i++ {
		critical := createLogEntry(LogLevelCritical, "payment", "Service down")
		logs = append(logs, critical)
	}

	// Security threat
	threat := createHTTPLog("api", "GET", "/api/users?id=1' OR '1'='1", 400, 50*time.Millisecond)
	threat.IP = "10.0.0.99"
	threat.Level = LogLevelWarn // Security issues should be warnings
	logs = append(logs, threat)

	// Process logs
	logChan := make(chan LogEntry)
	go func() {
		defer close(logChan)
		for _, log := range logs {
			select {
			case logChan <- log:
				time.Sleep(10 * time.Millisecond)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Run pipeline and wait for all logs to be consumed
	done := make(chan struct{})
	go func() {
		ProcessLogs(ctx, logChan)
		close(done)
	}()

	// Wait for processing to complete or timeout
	select {
	case <-done:
		// Pipeline finished
	case <-time.After(8 * time.Second):
		// Timeout
	}

	// Give a bit more time for async operations to complete
	time.Sleep(2 * time.Second)

	// Verify results
	metrics := MetricsCollector.GetMetrics()
	if metrics.LogsProcessed < 50 {
		t.Errorf("Expected at least 50 logs processed, got %d", metrics.LogsProcessed)
	}

	if metrics.SecurityThreats < 1 {
		t.Error("Expected at least 1 security threat detected")
	}

	alerts := Alerting.GetAlerts()
	if len(alerts) == 0 {
		t.Error("Expected at least one alert")
	}
}

func BenchmarkPipeline(b *testing.B) {
	ctx := context.Background()
	ResetPipeline()
	ResetServices()
	EnableBatching = true
	EnableBackpressure = true

	// Generate logs
	logs := generateLogs(b.N, true, false)

	b.ResetTimer()

	// Process logs
	logChan := make(chan LogEntry, 1000)
	go func() {
		defer close(logChan)
		for _, log := range logs {
			logChan <- log
		}
	}()

	ProcessLogs(ctx, logChan)

	b.StopTimer()

	metrics := MetricsCollector.GetMetrics()
	b.ReportMetric(float64(metrics.LogsProcessed), "logs_processed")
	b.ReportMetric(float64(metrics.LogsStored), "logs_stored")
	b.ReportMetric(metrics.PeakRate, "peak_logs_per_sec")
}
