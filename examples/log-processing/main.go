// RealClock-Time Log Processing Pipeline Example
// ========================================
// This demonstrates building a production log processing system using streamz.
// The system evolves from a simple MVP to a sophisticated real-time analytics
// platform with security threat detection and intelligent alerting.
//
// Run with: go run .
// Run specific sprint: go run . -sprint=3

package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"time"
)

func main() {
	var sprint int
	flag.IntVar(&sprint, "sprint", 0, "Run specific sprint (1-6), 0 for full demo")
	flag.Parse()

	// Enable test mode for faster demos
	TestMode = true

	fmt.Println("=== RealClock-Time Log Processing Pipeline Demo ===")
	fmt.Println("Building a production log analytics system that evolves from MVP to enterprise-grade")
	fmt.Println()

	ctx := context.Background()

	if sprint > 0 {
		// Run specific sprint
		runSprint(ctx, sprint)
	} else {
		// Run full evolution demo
		runFullDemo(ctx)
	}
}

func runFullDemo(ctx context.Context) {
	// Sprint 1: MVP
	fmt.Println("📦 SPRINT 1: MVP - Just Store the Logs!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Boss: 'We need to start storing our logs!'")
	fmt.Println("Dev: 'No problem, I'll just write them to the database...'")
	fmt.Println()

	ResetPipeline()
	ResetServices()

	// Generate some logs
	logs := generateLogs(1000, false, false)

	// Process with MVP pipeline
	processChan := make(chan LogEntry, len(logs))
	for _, log := range logs {
		processChan <- log
	}
	close(processChan)

	ProcessLogs(ctx, processChan)

	metrics := MetricsCollector.GetMetrics()
	fmt.Printf("\n📊 Results: Processed %d logs, Stored %d, Dropped %d\n",
		metrics.LogsProcessed, metrics.LogsStored, metrics.LogsDropped)

	// Simulate the problem
	fmt.Println("\n💥 During the first traffic spike...")
	fmt.Println("Database: CPU 100%, Writes timing out!")
	fmt.Println("Result: 50% of logs lost during incident 😱")
	fmt.Println()
	time.Sleep(2 * time.Second)

	// Sprint 2: Batching
	fmt.Println("\n📦 SPRINT 2: Efficient Storage - Batching FTW!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Database team: 'Please stop hammering our database!'")
	fmt.Println("Dev: 'Let me batch these writes...'")
	fmt.Println()

	ResetPipeline()
	ResetServices()
	EnableBatching = true

	logs = generateLogs(5000, false, false)
	processChan = make(chan LogEntry, len(logs))
	for _, log := range logs {
		processChan <- log
	}
	close(processChan)

	ProcessLogs(ctx, processChan)

	dbWrites, _, batches := Database.GetStats()
	fmt.Printf("\n📊 Results: %d logs in %d batches (%.1fx fewer DB writes!)\n",
		dbWrites, batches, float64(dbWrites)/float64(batches))
	fmt.Println("Database team: 'Much better! CPU down to 10%!'")

	fmt.Println("\n😕 But wait...")
	fmt.Println("Ops: 'Why did it take 5 minutes to know about that critical error?'")
	fmt.Println("Dev: 'Well, it was stuck in a batch...'")
	time.Sleep(2 * time.Second)

	// Sprint 3: RealClock-time alerts
	fmt.Println("\n\n📦 SPRINT 3: RealClock-Time Error Detection!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Ops: 'We need immediate alerts for errors!'")
	fmt.Println("Dev: 'I'll add parallel processing...'")
	fmt.Println()

	ResetPipeline()
	ResetServices()
	EnableBatching = true
	EnableRealTimeAlerts = true

	// Generate logs with errors
	logs = generateLogs(1000, true, false)
	go simulateLogStream(ctx, logs, 100*time.Millisecond)

	time.Sleep(3 * time.Second)

	alertCount := Alerting.GetAlertCount()
	fmt.Printf("\n📊 Results: Sent %d immediate alerts\n", alertCount)
	fmt.Println("\n😰 2 hours later...")
	fmt.Println("Ops: 'STOP! 500 alerts per hour is killing us!'")
	fmt.Println("Dev: 'Maybe we need smarter alerting...'")
	time.Sleep(2 * time.Second)

	// Sprint 4: Smart alerting
	fmt.Println("\n\n📦 SPRINT 4: Intelligent Rate-Based Alerting!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Ops: 'Alert us about patterns, not every single error!'")
	fmt.Println("Dev: 'Time for windowed aggregation...'")
	fmt.Println()

	ResetPipeline()
	ResetServices()
	EnableBatching = true
	EnableRealTimeAlerts = true
	EnableSmartAlerting = true

	// Simulate error spike
	fmt.Println("Simulating normal traffic with error spike...")
	logs = generateErrorSpike()
	go simulateLogStream(ctx, logs, 50*time.Millisecond)

	time.Sleep(5 * time.Second)

	alerts := Alerting.GetAlerts()
	fmt.Printf("\n📊 Results: %d smart alerts (80%% reduction!)\n", len(alerts))
	for _, alert := range alerts {
		if alert.Type == AlertTypeErrorSpike {
			fmt.Printf("   - %s: %d errors in %v window\n", alert.Service, alert.Count, alert.Window)
		}
	}
	fmt.Println("Ops: 'This is perfect! We can actually sleep now!'")
	time.Sleep(2 * time.Second)

	// Sprint 5: Security
	fmt.Println("\n\n📦 SPRINT 5: Security Threat Detection!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Security team: 'We're blind to attacks in our logs!'")
	fmt.Println("Dev: 'Let me add pattern matching...'")
	fmt.Println()

	ResetPipeline()
	ResetServices()
	EnableBatching = true
	EnableRealTimeAlerts = true
	EnableSmartAlerting = true
	EnableSecurityScanning = true

	// Generate logs with security threats
	logs = generateLogsWithThreats()
	go simulateLogStream(ctx, logs, 100*time.Millisecond)

	time.Sleep(5 * time.Second)

	threats := Security.GetDetectedThreats()
	fmt.Printf("\n🔒 Detected %d security threats!\n", len(threats))
	for _, threat := range threats {
		fmt.Printf("   - %s from IP %s (pattern: %s)\n",
			threat.Pattern, threat.MatchedLog.IP, threat.Alert.Context["pattern"])
		fmt.Printf("     Remediation: %s\n", threat.Remediation)
	}
	fmt.Println("\nSecurity team: 'We just prevented 3 breach attempts!'")
	time.Sleep(2 * time.Second)

	// Sprint 6: Production scale
	fmt.Println("\n\n📦 SPRINT 6: Production Scale - Handle the Storm!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Boss: 'Black Friday is coming. Can we handle 10x traffic?'")
	fmt.Println("Dev: 'Time for backpressure handling and optimization...'")
	fmt.Println()

	ResetPipeline()
	ResetServices()
	EnableBatching = true
	EnableRealTimeAlerts = true
	EnableSmartAlerting = true
	EnableSecurityScanning = true
	EnableBackpressure = true

	// Simulate massive traffic spike
	fmt.Println("Simulating Black Friday traffic spike (10x normal)...")
	logs = generateLogs(50000, true, true)

	start := time.Now()
	go simulateLogStream(ctx, logs, 1*time.Millisecond) // Very fast!

	time.Sleep(10 * time.Second)

	metrics = MetricsCollector.GetMetrics()
	duration := time.Since(start)
	rate := float64(metrics.LogsProcessed) / duration.Seconds()

	fmt.Printf("\n🎉 RESULTS: Handled %.0f logs/second!\n", rate)
	fmt.Printf("   - Processed: %d logs\n", metrics.LogsProcessed)
	fmt.Printf("   - Stored: %d logs\n", metrics.LogsStored)
	fmt.Printf("   - Dropped: %d logs (%.1f%%)\n", metrics.LogsDropped,
		float64(metrics.LogsDropped)/float64(metrics.LogsProcessed)*100)
	fmt.Printf("   - Peak rate: %.0f logs/second\n", metrics.PeakRate)
	fmt.Printf("   - Alerts sent: %d\n", metrics.AlertsSent)
	fmt.Printf("   - Threats detected: %d\n", metrics.SecurityThreats)

	fmt.Println("\n✅ System handled 10x traffic spike successfully!")
	fmt.Println("Boss: 'Excellent work! The system performed flawlessly on Black Friday!'")
}

func runSprint(ctx context.Context, sprint int) {
	ResetPipeline()
	ResetServices()

	fmt.Printf("Running Sprint %d only...\n\n", sprint)

	// Enable features up to requested sprint
	if sprint >= 2 {
		EnableBatching = true
	}
	if sprint >= 3 {
		EnableRealTimeAlerts = true
	}
	if sprint >= 4 {
		EnableSmartAlerting = true
	}
	if sprint >= 5 {
		EnableSecurityScanning = true
	}
	if sprint >= 6 {
		EnableBackpressure = true
	}

	// Generate appropriate test data
	var logs []LogEntry
	switch sprint {
	case 1:
		logs = generateLogs(1000, false, false)
	case 2:
		logs = generateLogs(5000, false, false)
	case 3, 4:
		logs = generateLogs(2000, true, false)
	case 5:
		logs = generateLogsWithThreats()
	case 6:
		logs = generateLogs(10000, true, true)
	}

	// Process logs
	if sprint <= 2 {
		// Batch processing
		processChan := make(chan LogEntry, len(logs))
		for _, log := range logs {
			processChan <- log
		}
		close(processChan)
		ProcessLogs(ctx, processChan)
	} else {
		// Stream processing
		go simulateLogStream(ctx, logs, 50*time.Millisecond)
		time.Sleep(10 * time.Second)
	}

	// Print results
	printSprintResults(sprint)
}

func simulateLogStream(ctx context.Context, logs []LogEntry, delay time.Duration) {
	logChan := make(chan LogEntry)

	// Start pipeline
	go ProcessLogs(ctx, logChan)

	// Stream logs
	for _, log := range logs {
		select {
		case logChan <- log:
			if delay > 0 {
				time.Sleep(delay)
			}
		case <-ctx.Done():
			close(logChan)
			return
		}
	}
	close(logChan)
}

func generateLogs(count int, includeErrors bool, includeHighVolume bool) []LogEntry {
	logs := make([]LogEntry, 0, count)
	services := []string{"api", "web", "auth", "payment", "inventory"}

	for i := 0; i < count; i++ {
		level := LogLevelInfo
		if includeErrors && rand.Float32() < 0.1 { // 10% errors
			if rand.Float32() < 0.1 { // 10% of errors are critical
				level = LogLevelCritical
			} else {
				level = LogLevelError
			}
		} else if rand.Float32() < 0.2 { // 20% warnings
			level = LogLevelWarn
		}

		service := services[rand.Intn(len(services))]
		log := createLogEntry(level, service, generateMessage(level, service))

		// Add HTTP request info
		if rand.Float32() < 0.7 { // 70% are HTTP requests
			methods := []string{"GET", "POST", "PUT", "DELETE"}
			paths := []string{"/api/orders", "/api/users", "/api/products", "/checkout", "/login"}
			log.Method = methods[rand.Intn(len(methods))]
			log.Path = paths[rand.Intn(len(paths))]
			log.Status = 200
			if level == LogLevelError {
				log.Status = 500
			}
			log.Duration = time.Duration(rand.Intn(1000)) * time.Millisecond
		}

		// Add user info
		if rand.Float32() < 0.5 {
			log.UserID = fmt.Sprintf("user-%03d", rand.Intn(5)+1)
		}

		// Add IP
		log.IP = fmt.Sprintf("192.168.1.%d", rand.Intn(255))

		logs = append(logs, log)
	}

	return logs
}

func generateErrorSpike() []LogEntry {
	logs := make([]LogEntry, 0, 1000)

	// Normal traffic
	for i := 0; i < 400; i++ {
		level := LogLevelInfo
		if rand.Float32() < 0.02 { // 2% error rate (normal)
			level = LogLevelError
		}
		log := createLogEntry(level, "api", generateMessage(level, "api"))
		logs = append(logs, log)
	}

	// Error spike!
	for i := 0; i < 200; i++ {
		level := LogLevelError
		if rand.Float32() < 0.2 { // Some critical
			level = LogLevelCritical
		}
		log := createLogEntry(level, "payment", "Payment processing failed")
		log.Error = "Connection timeout to payment gateway"
		logs = append(logs, log)
	}

	// Back to normal
	for i := 0; i < 400; i++ {
		level := LogLevelInfo
		if rand.Float32() < 0.02 {
			level = LogLevelError
		}
		log := createLogEntry(level, "api", generateMessage(level, "api"))
		logs = append(logs, log)
	}

	return logs
}

func generateLogsWithThreats() []LogEntry {
	logs := generateLogs(500, true, false)

	// Add SQL injection attempts
	for i := 0; i < 3; i++ {
		log := createHTTPLog("api", "GET", "/api/users?id=1' OR '1'='1", 400, 50*time.Millisecond)
		log.Level = LogLevelWarn
		log.Message = "Invalid query parameter"
		log.IP = "10.0.0.99"
		log.UserID = "attacker"
		logs = append(logs, log)
	}

	// Add path traversal attempts
	for i := 0; i < 2; i++ {
		log := createHTTPLog("web", "GET", "/files/../../../etc/passwd", 403, 10*time.Millisecond)
		log.Level = LogLevelWarn
		log.Message = "Access denied"
		log.IP = "10.0.0.66"
		logs = append(logs, log)
	}

	// Add brute force attempts
	for i := 0; i < 10; i++ {
		log := createLogEntry(LogLevelWarn, "auth", "Failed login attempt")
		log.Error = "Invalid password"
		log.IP = "10.0.0.77"
		log.UserID = fmt.Sprintf("admin%d", i)
		logs = append(logs, log)
	}

	// Shuffle logs
	rand.Shuffle(len(logs), func(i, j int) {
		logs[i], logs[j] = logs[j], logs[i]
	})

	return logs
}

func generateMessage(level, service string) string {
	switch level {
	case LogLevelError:
		errors := []string{
			"Database connection failed",
			"Failed to process request",
			"Internal server error",
			"Service unavailable",
			"Timeout waiting for response",
		}
		return errors[rand.Intn(len(errors))]
	case LogLevelWarn:
		warnings := []string{
			"High memory usage detected",
			"Slow query performance",
			"Rate limit approaching",
			"Cache miss",
			"Deprecated API usage",
		}
		return warnings[rand.Intn(len(warnings))]
	case LogLevelCritical:
		critical := []string{
			"Service is down",
			"Data corruption detected",
			"Security breach attempt",
			"Out of memory",
			"Disk full",
		}
		return critical[rand.Intn(len(critical))]
	default:
		info := []string{
			"Request processed successfully",
			"User logged in",
			"Order completed",
			"Cache updated",
			"Health check passed",
		}
		return info[rand.Intn(len(info))]
	}
}

func printSprintResults(sprint int) {
	metrics := MetricsCollector.GetMetrics()

	fmt.Printf("\n📊 Sprint %d Results:\n", sprint)
	fmt.Printf("   - Logs processed: %d\n", metrics.LogsProcessed)
	fmt.Printf("   - Logs stored: %d\n", metrics.LogsStored)

	if sprint >= 2 {
		_, _, batches := Database.GetStats()
		fmt.Printf("   - Batches created: %d\n", batches)
	}

	if sprint >= 3 {
		fmt.Printf("   - Alerts sent: %d\n", metrics.AlertsSent)
	}

	if sprint >= 5 {
		fmt.Printf("   - Security threats: %d\n", metrics.SecurityThreats)
	}

	if sprint >= 6 {
		fmt.Printf("   - Peak rate: %.0f logs/sec\n", metrics.PeakRate)
		if metrics.LogsDropped > 0 {
			fmt.Printf("   - Logs dropped: %d (%.1f%%)\n",
				metrics.LogsDropped,
				float64(metrics.LogsDropped)/float64(metrics.LogsProcessed)*100)
		}
	}
}
