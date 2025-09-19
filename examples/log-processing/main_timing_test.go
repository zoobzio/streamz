package main

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
	"github.com/zoobzio/streamz"
)

func TestMainTimingAbstraction(t *testing.T) {
	// Test that main demo functions accept clock parameter
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	// Test runFullDemo accepts clock
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("runFullDemo panicked: %v", r)
			}
		}()
		runFullDemo(ctx, clock)
	}()

	// Test runSprint accepts clock
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("runSprint panicked: %v", r)
			}
		}()
		runSprint(ctx, 1, clock)
	}()

	// Test simulateLogStream accepts clock
	logs := generateLogs(10, false, false)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("simulateLogStream panicked: %v", r)
			}
		}()
		simulateLogStream(ctx, logs, 1*time.Millisecond, clock)
	}()

	// Let fake clock advance to ensure no blocking
	clock.Advance(1 * time.Second)
}

func TestClockConfiguration(t *testing.T) {
	// Test that both real and fake clocks work
	realClock := streamz.RealClock
	fakeClock := clockz.NewFakeClock()

	if realClock == nil {
		t.Error("RealClock is nil")
	}

	if fakeClock == nil {
		t.Error("FakeClock is nil")
	}

	// Verify they implement the same interface
	var _ streamz.Clock = realClock
	var _ streamz.Clock = fakeClock
}
