package streamz

import (
	"context"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

func TestSessionWindow_BasicOperation(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	keyFunc := func(r Result[string]) string {
		if r.IsSuccess() {
			return r.Value()[:4] // Use first 4 chars as session key
		}
		return "error"
	}

	window := NewSessionWindow(keyFunc, clock).WithGap(100 * time.Millisecond)

	input := make(chan Result[string], 3)
	input <- NewSuccess("user1-action1")
	input <- NewSuccess("user1-action2")
	input <- NewSuccess("user1-action3")
	close(input)

	output := window.Process(ctx, input)

	// Wait for session to expire
	clock.Advance(150 * time.Millisecond)

	var results []Result[string]
	for r := range output {
		results = append(results, r)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results in session, got %d", len(results))
	}
}

func TestSessionWindow_MultipleSessions(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	keyFunc := func(r Result[string]) string {
		if r.IsSuccess() {
			return r.Value()[:5]
		}
		return "error"
	}

	window := NewSessionWindow(keyFunc, clock).WithGap(100 * time.Millisecond)

	input := make(chan Result[string])
	output := window.Process(ctx, input)

	go func() {
		// Two different sessions
		input <- NewSuccess("user1-action1")
		input <- NewSuccess("user2-action1")
		input <- NewSuccess("user1-action2")
		input <- NewSuccess("user2-action2")

		clock.Advance(150 * time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		close(input)
	}()

	var results []Result[string]
	for r := range output {
		results = append(results, r)
	}

	if len(results) != 4 {
		t.Errorf("expected 4 results across 2 sessions, got %d", len(results))
	}
}

func TestSessionWindow_ErrorInSession(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	keyFunc := func(r Result[int]) string {
		return "session1"
	}

	window := NewSessionWindow(keyFunc, clock).WithGap(100 * time.Millisecond)

	input := make(chan Result[int], 3)
	input <- NewSuccess(1)
	input <- NewError(0, nil, "test")
	input <- NewSuccess(2)
	close(input)

	output := window.Process(ctx, input)
	clock.Advance(150 * time.Millisecond)

	var successCount, errorCount int
	for r := range output {
		if r.IsSuccess() {
			successCount++
		} else {
			errorCount++
		}
	}

	if successCount != 2 {
		t.Errorf("expected 2 successes, got %d", successCount)
	}
	if errorCount != 1 {
		t.Errorf("expected 1 error, got %d", errorCount)
	}
}

func TestSessionWindow_SessionExtension(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	keyFunc := func(r Result[int]) string {
		return "session1"
	}

	window := NewSessionWindow(keyFunc, clock).WithGap(100 * time.Millisecond)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	go func() {
		input <- NewSuccess(1)
		clock.Advance(50 * time.Millisecond)
		time.Sleep(5 * time.Millisecond)

		// This should extend the session
		input <- NewSuccess(2)
		clock.Advance(50 * time.Millisecond)
		time.Sleep(5 * time.Millisecond)

		// This should also extend the session
		input <- NewSuccess(3)

		// Now let the session expire
		clock.Advance(150 * time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		close(input)
	}()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	// All items should be in the same session due to extension
	if len(results) != 3 {
		t.Errorf("expected 3 results in extended session, got %d", len(results))
	}
}

func TestSessionWindow_SeparateSessions(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()

	keyFunc := func(r Result[int]) string {
		return "session1"
	}

	window := NewSessionWindow(keyFunc, clock).WithGap(100 * time.Millisecond)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	go func() {
		// First session
		input <- NewSuccess(1)
		input <- NewSuccess(2)

		// Let session expire
		clock.Advance(150 * time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		// Second session (after gap)
		input <- NewSuccess(3)
		input <- NewSuccess(4)

		clock.Advance(150 * time.Millisecond)
		time.Sleep(20 * time.Millisecond)

		close(input)
	}()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	if len(results) != 4 {
		t.Errorf("expected 4 results across 2 sessions, got %d", len(results))
	}
}

func TestSessionWindow_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clock := clockz.NewFakeClock()

	keyFunc := func(r Result[int]) string {
		return "session1"
	}

	window := NewSessionWindow(keyFunc, clock).WithGap(100 * time.Millisecond)

	input := make(chan Result[int])
	output := window.Process(ctx, input)

	// Unbuffered sends block until processor reads - guarantees synchronization
	input <- NewSuccess(1)
	input <- NewSuccess(2)
	cancel()

	var results []Result[int]
	for r := range output {
		results = append(results, r)
	}

	// Should emit partial session on cancellation
	if len(results) != 2 {
		t.Errorf("expected 2 results on cancellation, got %d", len(results))
	}
}

func TestSessionWindow_WithName(t *testing.T) {
	clock := clockz.NewFakeClock()
	window := NewSessionWindow(func(r Result[int]) string { return "s" }, clock).
		WithName("custom-session")

	if window.Name() != "custom-session" {
		t.Errorf("expected name 'custom-session', got %q", window.Name())
	}
}

func TestSessionWindow_DefaultGap(t *testing.T) {
	clock := clockz.NewFakeClock()
	window := NewSessionWindow(func(r Result[int]) string { return "s" }, clock)

	// Default gap should be 30 minutes
	if window.gap != 30*time.Minute {
		t.Errorf("expected default gap 30m, got %v", window.gap)
	}
}

func TestSessionWindow_WindowMetadataFields(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClock()
	gap := 100 * time.Millisecond

	keyFunc := func(r Result[int]) string {
		return "user123"
	}

	window := NewSessionWindow(keyFunc, clock).WithGap(gap)

	input := make(chan Result[int], 1)
	input <- NewSuccess(42)
	close(input)

	output := window.Process(ctx, input)
	clock.Advance(150 * time.Millisecond)

	result := <-output
	meta, err := GetWindowMetadata(result)
	if err != nil {
		t.Fatalf("expected window metadata: %v", err)
	}

	if meta.Type != "session" {
		t.Errorf("expected type 'session', got %q", meta.Type)
	}
	if meta.Gap == nil || *meta.Gap != gap {
		t.Errorf("expected gap %v, got %v", gap, meta.Gap)
	}
	if meta.SessionKey == nil || *meta.SessionKey != "user123" {
		t.Errorf("expected session key 'user123', got %v", meta.SessionKey)
	}
}
