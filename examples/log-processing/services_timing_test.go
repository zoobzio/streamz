package main

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestServicesDeterministicTiming verifies that all services use injected clock
// for deterministic testing behavior.
func TestServicesDeterministicTiming(t *testing.T) {
	fakeClock := clockz.NewFake()

	// Test DatabaseService timing control
	t.Run("DatabaseService", func(t *testing.T) {
		db := NewDatabaseService(fakeClock)

		logs := []LogEntry{{ID: "test-1", Message: "test"}}

		done := make(chan error, 1)
		go func() {
			done <- db.WriteLogs(context.Background(), logs)
		}()

		// Advance fake clock to trigger completion
		fakeClock.Advance(50 * time.Millisecond)

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("WriteLogs failed: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("WriteLogs did not complete after advancing fake clock")
		}
	})

	// Test AlertService timing control
	t.Run("AlertService", func(t *testing.T) {
		alerts := NewAlertService(fakeClock)

		alert := Alert{ID: "test", Title: "test alert"}

		done := make(chan error, 1)
		go func() {
			done <- alerts.SendAlert(context.Background(), alert)
		}()

		// Advance fake clock to trigger completion
		fakeClock.Advance(100 * time.Millisecond)

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("SendAlert failed: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("SendAlert did not complete after advancing fake clock")
		}
	})

	// Test UserService timing control
	t.Run("UserService", func(t *testing.T) {
		users := NewUserService(fakeClock)

		done := make(chan string, 1)
		go func() {
			// Force cache miss to trigger timing delay
			users.cacheHitRate = 0.0
			name, _ := users.LookupUser(context.Background(), "user-001")
			done <- name
		}()

		// Advance fake clock to trigger completion
		fakeClock.Advance(20 * time.Millisecond)

		select {
		case name := <-done:
			if name != "Alice Johnson" {
				t.Fatalf("Expected 'Alice Johnson', got '%s'", name)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("LookupUser did not complete after advancing fake clock")
		}
	})

	// Test SecurityService timing control
	t.Run("SecurityService", func(t *testing.T) {
		security := NewSecurityService(fakeClock)

		log := LogEntry{ID: "test", Message: "normal log"}

		done := make(chan *SecurityAlert, 1)
		go func() {
			alert, _ := security.AnalyzeLog(context.Background(), log)
			done <- alert
		}()

		// Advance fake clock to trigger completion
		fakeClock.Advance(5 * time.Millisecond)

		select {
		case alert := <-done:
			if alert != nil {
				t.Fatalf("Expected no alert for normal log, got %v", alert)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("AnalyzeLog did not complete after advancing fake clock")
		}
	})
}
