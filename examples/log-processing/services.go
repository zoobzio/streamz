package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Mock external services for demonstration.
// In production, these would be real database connections, alert systems, etc.

// DatabaseService simulates a database for storing logs.
type DatabaseService struct {
	writeLatency time.Duration
	failureRate  float64
	totalWrites  int64
	failedWrites int64
	batchWrites  int64
	isHealthy    bool
	mutex        sync.Mutex
}

// NewDatabaseService creates a mock database service.
func NewDatabaseService() *DatabaseService {
	return &DatabaseService{
		writeLatency: 50 * time.Millisecond, // Simulated write latency
		failureRate:  0.0,
		isHealthy:    true,
	}
}

// WriteLogs simulates writing logs to database (inefficient single writes).
func (db *DatabaseService) WriteLogs(ctx context.Context, logs []LogEntry) error {
	if !db.isHealthy {
		atomic.AddInt64(&db.failedWrites, int64(len(logs)))
		return fmt.Errorf("database is unhealthy")
	}

	// Simulate individual writes (inefficient)
	for _, log := range logs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(db.writeLatency):
			atomic.AddInt64(&db.totalWrites, 1)

			// Simulate random failures
			if rand.Float64() < db.failureRate {
				atomic.AddInt64(&db.failedWrites, 1)
				return fmt.Errorf("database write failed for log %s", log.ID)
			}
		}
	}

	return nil
}

// WriteBatch simulates efficient batch writing to database.
func (db *DatabaseService) WriteBatch(ctx context.Context, batch LogBatch) error {
	if !db.isHealthy {
		atomic.AddInt64(&db.failedWrites, int64(batch.Count))
		return fmt.Errorf("database is unhealthy")
	}

	// Batch writes are much more efficient
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(db.writeLatency + time.Duration(batch.Count)*time.Millisecond):
		atomic.AddInt64(&db.totalWrites, int64(batch.Count))
		atomic.AddInt64(&db.batchWrites, 1)

		if rand.Float64() < db.failureRate {
			atomic.AddInt64(&db.failedWrites, int64(batch.Count))
			return fmt.Errorf("batch write failed for %d logs", batch.Count)
		}
	}

	return nil
}

// SetHealthy sets the database health status.
func (db *DatabaseService) SetHealthy(healthy bool) {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.isHealthy = healthy
}

// SetFailureRate sets the simulated failure rate.
func (db *DatabaseService) SetFailureRate(rate float64) {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.failureRate = rate
}

// GetStats returns database statistics.
func (db *DatabaseService) GetStats() (total, failed, batches int64) {
	return atomic.LoadInt64(&db.totalWrites),
		atomic.LoadInt64(&db.failedWrites),
		atomic.LoadInt64(&db.batchWrites)
}

// AlertService simulates an alerting system (PagerDuty, Slack, etc.).
type AlertService struct {
	alerts      []Alert
	sendLatency time.Duration
	failureRate float64
	totalAlerts int64
	mutex       sync.Mutex
}

// NewAlertService creates a mock alert service.
func NewAlertService() *AlertService {
	return &AlertService{
		sendLatency: 100 * time.Millisecond, // Simulated API call latency
		failureRate: 0.0,
		alerts:      make([]Alert, 0),
	}
}

// SendAlert simulates sending an alert.
func (as *AlertService) SendAlert(ctx context.Context, alert Alert) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(as.sendLatency):
		if rand.Float64() < as.failureRate {
			return fmt.Errorf("failed to send alert: %s", alert.Title)
		}

		as.mutex.Lock()
		as.alerts = append(as.alerts, alert)
		as.mutex.Unlock()

		atomic.AddInt64(&as.totalAlerts, 1)

		// In demo mode, print critical alerts
		if TestMode && alert.Severity == AlertSeverityCritical {
			fmt.Printf("🚨 ALERT: %s\n", alert.Title)
		}

		return nil
	}
}

// GetAlerts returns all sent alerts.
func (as *AlertService) GetAlerts() []Alert {
	as.mutex.Lock()
	defer as.mutex.Unlock()

	result := make([]Alert, len(as.alerts))
	copy(result, as.alerts)
	return result
}

// GetAlertCount returns the total number of alerts sent.
func (as *AlertService) GetAlertCount() int64 {
	return atomic.LoadInt64(&as.totalAlerts)
}

// UserService simulates a user information service.
type UserService struct {
	users         map[string]string // userID -> userName
	lookupLatency time.Duration
	cacheHitRate  float64
	mutex         sync.RWMutex
}

// NewUserService creates a mock user service.
func NewUserService() *UserService {
	return &UserService{
		users: map[string]string{
			"user-001": "Alice Johnson",
			"user-002": "Bob Smith",
			"user-003": "Charlie Brown",
			"user-004": "Diana Prince",
			"user-005": "Eve Wilson",
			"admin":    "System Admin",
			"attacker": "Malicious User",
		},
		lookupLatency: 20 * time.Millisecond,
		cacheHitRate:  0.8, // 80% cache hit rate
	}
}

// LookupUser simulates looking up user information.
func (us *UserService) LookupUser(ctx context.Context, userID string) (string, error) {
	// Simulate cache hit (fast)
	if rand.Float64() < us.cacheHitRate {
		us.mutex.RLock()
		name, exists := us.users[userID]
		us.mutex.RUnlock()

		if exists {
			return name, nil
		}
	}

	// Simulate cache miss (slower)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(us.lookupLatency):
		us.mutex.RLock()
		name, exists := us.users[userID]
		us.mutex.RUnlock()

		if !exists {
			return "Unknown User", nil
		}
		return name, nil
	}
}

// SecurityService simulates a security analysis service.
type SecurityService struct {
	detectedThreats []SecurityAlert
	analysisLatency time.Duration
	mutex           sync.Mutex
}

// NewSecurityService creates a mock security service.
func NewSecurityService() *SecurityService {
	return &SecurityService{
		analysisLatency: 5 * time.Millisecond,
		detectedThreats: make([]SecurityAlert, 0),
	}
}

// AnalyzeLog simulates security analysis of a log entry.
func (ss *SecurityService) AnalyzeLog(ctx context.Context, log LogEntry) (*SecurityAlert, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(ss.analysisLatency):
		// Check against security patterns
		for _, pattern := range SecurityPatterns {
			for _, p := range pattern.Patterns {
				if containsPattern(log, p) {
					alert := SecurityAlert{
						Alert: Alert{
							ID:        fmt.Sprintf("sec-%d", time.Now().UnixNano()),
							Type:      AlertTypeSecurityThreat,
							Severity:  pattern.Severity,
							Service:   log.Service,
							Title:     fmt.Sprintf("%s detected", pattern.Name),
							Message:   pattern.Description,
							Timestamp: time.Now(),
							Context: map[string]interface{}{
								"pattern": p,
								"log_id":  log.ID,
								"ip":      log.IP,
								"user_id": log.UserID,
							},
						},
						Pattern:     pattern.Name,
						MatchedLog:  log,
						Confidence:  0.95,
						Remediation: getRemediation(pattern.Type),
					}

					ss.mutex.Lock()
					ss.detectedThreats = append(ss.detectedThreats, alert)
					ss.mutex.Unlock()

					return &alert, nil
				}
			}
		}

		return nil, nil // No threat detected
	}
}

// GetDetectedThreats returns all detected security threats.
func (ss *SecurityService) GetDetectedThreats() []SecurityAlert {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	result := make([]SecurityAlert, len(ss.detectedThreats))
	copy(result, ss.detectedThreats)
	return result
}

// Helper functions

func containsPattern(log LogEntry, pattern string) bool {
	// Check in message
	if containsIgnoreCase(log.Message, pattern) {
		return true
	}

	// Check in path for web requests
	if log.Path != "" && containsIgnoreCase(log.Path, pattern) {
		return true
	}

	// Check in error message
	if log.Error != "" && containsIgnoreCase(log.Error, pattern) {
		return true
	}

	// Check in metadata
	for _, v := range log.Metadata {
		if containsIgnoreCase(v, pattern) {
			return true
		}
	}

	return false
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(contains(s, substr) || contains(toLower(s), toLower(substr)))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func toLower(s string) string {
	// Simple lowercase conversion for ASCII
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func getRemediation(threatType string) string {
	switch threatType {
	case "sql_injection":
		return "Block IP, review database queries, enable prepared statements"
	case "path_traversal":
		return "Block IP, review file access logs, validate input paths"
	case "brute_force":
		return "Enable rate limiting, consider account lockout, review authentication logs"
	case "xss":
		return "Block request, sanitize user input, review content security policy"
	default:
		return "Review security logs and take appropriate action"
	}
}

// Global service instances
var (
	Database = NewDatabaseService()
	Alerting = NewAlertService()
	Users    = NewUserService()
	Security = NewSecurityService()
)

// TestMode speeds up demos
var TestMode = false

// ResetServices resets all services to initial state.
func ResetServices() {
	Database = NewDatabaseService()
	Alerting = NewAlertService()
	Users = NewUserService()
	Security = NewSecurityService()
}
