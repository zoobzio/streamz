package main

import (
	"fmt"
	"time"
)

// LogEntry represents a single log event from any service in the system.
type LogEntry struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"` // INFO, WARN, ERROR, CRITICAL
	Service   string            `json:"service"`
	Message   string            `json:"message"`
	UserID    string            `json:"user_id,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	IP        string            `json:"ip,omitempty"`
	Path      string            `json:"path,omitempty"`
	Method    string            `json:"method,omitempty"`
	Status    int               `json:"status,omitempty"`
	Duration  time.Duration     `json:"duration,omitempty"`
	Error     string            `json:"error,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Alert represents a notification that should be sent to the ops team.
type Alert struct {
	ID        string                 `json:"id"`
	Type      AlertType              `json:"type"`
	Severity  AlertSeverity          `json:"severity"`
	Service   string                 `json:"service"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	Count     int                    `json:"count,omitempty"`  // For rate-based alerts
	Window    time.Duration          `json:"window,omitempty"` // For rate-based alerts
	Context   map[string]interface{} `json:"context,omitempty"`
}

// AlertType defines the type of alert.
type AlertType string

const (
	AlertTypeError          AlertType = "error"
	AlertTypeErrorSpike     AlertType = "error_spike"
	AlertTypeSecurityThreat AlertType = "security_threat"
	AlertTypePerformance    AlertType = "performance"
	AlertTypeSystemHealth   AlertType = "system_health"
)

// AlertSeverity defines how critical an alert is.
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityError    AlertSeverity = "error"
	AlertSeverityCritical AlertSeverity = "critical"
)

// SecurityPattern represents a pattern that might indicate a security threat.
type SecurityPattern struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // sql_injection, brute_force, path_traversal, etc.
	Patterns    []string `json:"patterns"`
	Severity    AlertSeverity
	Description string `json:"description"`
}

// LogBatch represents a batch of logs ready for storage.
type LogBatch struct {
	ID        string        `json:"id"`
	Logs      []LogEntry    `json:"logs"`
	Count     int           `json:"count"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
}

// SecurityAlert represents a detected security threat.
type SecurityAlert struct {
	Alert
	Pattern     string   `json:"pattern"`
	MatchedLog  LogEntry `json:"matched_log"`
	Confidence  float64  `json:"confidence"`
	Remediation string   `json:"remediation"`
}

// Metrics tracks pipeline performance.
type Metrics struct {
	StartTime        time.Time
	LogsProcessed    int64
	LogsStored       int64
	LogsDropped      int64
	BatchesCreated   int64
	AlertsSent       int64
	SecurityThreats  int64
	DatabaseWrites   int64
	ProcessingErrors int64
	CurrentRate      float64
	PeakRate         float64
	AverageLatency   time.Duration
	LastError        error
	LastErrorTime    time.Time
}

// LogLevel constants for filtering.
const (
	LogLevelInfo     = "INFO"
	LogLevelWarn     = "WARN"
	LogLevelError    = "ERROR"
	LogLevelCritical = "CRITICAL"
)

// Common security patterns to detect.
var SecurityPatterns = []SecurityPattern{
	{
		Name: "SQL Injection",
		Type: "sql_injection",
		Patterns: []string{
			"' OR '1'='1",
			"'; DROP TABLE",
			"1=1",
			"admin'--",
			"' OR 1=1--",
			"UNION SELECT",
			"'; DELETE FROM",
		},
		Severity:    AlertSeverityCritical,
		Description: "Potential SQL injection attack detected",
	},
	{
		Name: "Path Traversal",
		Type: "path_traversal",
		Patterns: []string{
			"../",
			"..\\",
			"/etc/passwd",
			"/etc/shadow",
			"C:\\Windows\\",
			"%2e%2e/",
			"..%2F",
		},
		Severity:    AlertSeverityCritical,
		Description: "Potential path traversal attack detected",
	},
	{
		Name: "Brute Force",
		Type: "brute_force",
		Patterns: []string{
			"failed login",
			"authentication failed",
			"invalid password",
			"access denied",
		},
		Severity:    AlertSeverityWarning,
		Description: "Multiple failed authentication attempts detected",
	},
	{
		Name: "XSS Attack",
		Type: "xss",
		Patterns: []string{
			"<script>",
			"javascript:",
			"onerror=",
			"onload=",
			"<iframe",
			"document.cookie",
			"alert(",
		},
		Severity:    AlertSeverityError,
		Description: "Potential XSS attack detected",
	},
}

// String representations for better output.
func (l LogEntry) String() string {
	return fmt.Sprintf("[%s] %s %s: %s", l.Timestamp.Format("15:04:05"), l.Level, l.Service, l.Message)
}

func (a Alert) String() string {
	return fmt.Sprintf("[%s] %s: %s - %s", a.Severity, a.Type, a.Service, a.Title)
}

func (b LogBatch) String() string {
	return fmt.Sprintf("Batch[%d logs, %v duration]", b.Count, b.Duration)
}

// Helper functions for creating test data.
func createLogEntry(level, service, message string) LogEntry {
	return LogEntry{
		ID:        fmt.Sprintf("log-%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Level:     level,
		Service:   service,
		Message:   message,
		Metadata:  make(map[string]string),
	}
}

func createHTTPLog(service, method, path string, status int, duration time.Duration) LogEntry {
	log := createLogEntry(LogLevelInfo, service, fmt.Sprintf("%s %s %d (%v)", method, path, status, duration))
	log.Method = method
	log.Path = path
	log.Status = status
	log.Duration = duration
	return log
}

func createErrorLog(service, message, errorMsg string) LogEntry {
	log := createLogEntry(LogLevelError, service, message)
	log.Error = errorMsg
	return log
}
