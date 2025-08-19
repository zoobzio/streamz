package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"

	"streamz/clock"
)

// TestSessionWindowBasic tests basic session windowing.
func TestSessionWindowBasic(t *testing.T) {
	ctx := context.Background()

	// Create session window with 100ms timeout.
	windower := NewSessionWindow(func(_ int) string { return "all" }, clock.Real).WithGap(100 * time.Millisecond).WithName("test-session")

	input := make(chan int)
	output := windower.Process(ctx, input)

	// Collect windows.
	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	// Send first session.
	go func() {
		input <- 1
		input <- 2
		input <- 3
		// Gap longer than timeout.
		time.Sleep(150 * time.Millisecond)
		// Second session.
		input <- 4
		input <- 5
		// Let second session timeout.
		time.Sleep(150 * time.Millisecond)
		close(input)
	}()

	<-done

	if len(windows) != 2 {
		t.Fatalf("expected 2 session windows, got %d", len(windows))
	}

	// First session.
	if len(windows[0].Items) != 3 {
		t.Errorf("first session: expected 3 items, got %d", len(windows[0].Items))
	}

	// Second session.
	if len(windows[1].Items) != 2 {
		t.Errorf("second session: expected 2 items, got %d", len(windows[1].Items))
	}

	if windower.Name() != "test-session" {
		t.Errorf("expected name 'test-session', got %s", windower.Name())
	}
}

// TestSessionWindowSingleItem tests session with single item.
func TestSessionWindowSingleItem(t *testing.T) {
	ctx := context.Background()

	windower := NewSessionWindow(func(_ string) string { return "all" }, clock.Real).WithGap(50 * time.Millisecond)

	input := make(chan string)
	output := windower.Process(ctx, input)

	var windows []Window[string]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	// Send single item and wait for timeout.
	go func() {
		input <- "item"
		time.Sleep(100 * time.Millisecond)
		close(input)
	}()

	<-done

	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}

	if len(windows[0].Items) != 1 || windows[0].Items[0] != "item" {
		t.Errorf("expected window with single 'item', got %v", windows[0].Items)
	}
}

// TestSessionWindowEmptyInput tests empty input handling.
func TestSessionWindowEmptyInput(t *testing.T) {
	ctx := context.Background()

	windower := NewSessionWindow(func(_ int) string { return "all" }, clock.Real).WithGap(100 * time.Millisecond)

	input := make(chan int)
	close(input)

	output := windower.Process(ctx, input)

	count := 0
	for range output {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 windows for empty input, got %d", count)
	}
}

// TestSessionWindowRapidInput tests handling of rapid input.
func TestSessionWindowRapidInput(t *testing.T) {
	ctx := context.Background()

	windower := NewSessionWindow(func(_ int) string { return "all" }, clock.Real).WithGap(200 * time.Millisecond)

	input := make(chan int)
	output := windower.Process(ctx, input)

	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	// Send rapid burst of items.
	go func() {
		for i := 0; i < 10; i++ {
			input <- i
			time.Sleep(10 * time.Millisecond) // Much less than timeout.
		}
		// Wait for session to timeout.
		time.Sleep(250 * time.Millisecond)
		close(input)
	}()

	<-done

	if len(windows) != 1 {
		t.Fatalf("expected 1 window for continuous input, got %d", len(windows))
	}

	if len(windows[0].Items) != 10 {
		t.Errorf("expected 10 items in window, got %d", len(windows[0].Items))
	}
}

// TestSessionWindowMultipleSessions tests multiple distinct sessions.
func TestSessionWindowMultipleSessions(t *testing.T) {
	ctx := context.Background()

	windower := NewSessionWindow(func(_ string) string { return "all" }, clock.Real).WithGap(50 * time.Millisecond)

	input := make(chan string)
	output := windower.Process(ctx, input)

	var windows []Window[string]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	// Create multiple sessions.
	go func() {
		// Session 1: User activity.
		input <- "login"
		input <- "view_page"
		input <- "click_button"

		// Idle period.
		time.Sleep(100 * time.Millisecond)

		// Session 2: Another burst.
		input <- "search"
		input <- "filter"

		// Idle period.
		time.Sleep(100 * time.Millisecond)

		// Session 3: Final activity.
		input <- "logout"

		// Wait for final timeout.
		time.Sleep(100 * time.Millisecond)
		close(input)
	}()

	<-done

	if len(windows) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(windows))
	}

	expectedSizes := []int{3, 2, 1}
	for i, window := range windows {
		if len(window.Items) != expectedSizes[i] {
			t.Errorf("session %d: expected %d items, got %d", i+1, expectedSizes[i], len(window.Items))
		}
	}
}

// TestSessionWindowContextCancellation tests graceful shutdown.
func TestSessionWindowContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	windower := NewSessionWindow(func(_ int) string { return "all" }, clock.Real).
		WithGap(100 * time.Millisecond)

	input := make(chan int)
	output := windower.Process(ctx, input)

	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	// Send data and create a session
	go func() {
		// First session
		input <- 1
		input <- 2
		input <- 3

		// Wait for session to complete
		time.Sleep(150 * time.Millisecond)

		// Second session that will be interrupted
		input <- 4
		input <- 5

		// Cancel context while session is active
		time.Sleep(50 * time.Millisecond)
		cancel()
		close(input)
	}()

	<-done

	// Should have 1 completed session and possibly 1 flushed session
	if len(windows) < 1 || len(windows) > 2 {
		t.Errorf("expected 1-2 session windows, got %d", len(windows))
	}

	// First session should have 3 items
	if len(windows) > 0 && len(windows[0].Items) != 3 {
		t.Errorf("first session: expected 3 items, got %d", len(windows[0].Items))
	}

	// If there's a second session (flushed on cancel), it should have 2 items
	if len(windows) > 1 && len(windows[1].Items) != 2 {
		t.Errorf("second session: expected 2 items, got %d", len(windows[1].Items))
	}
}

// TestSessionWindowTimeWindows tests window timing.
func TestSessionWindowTimeWindows(t *testing.T) {
	ctx := context.Background()

	windower := NewSessionWindow(func(_ int) string { return "all" }, clock.Real).WithGap(100 * time.Millisecond)

	input := make(chan int)
	output := windower.Process(ctx, input)

	var windows []Window[int]
	done := make(chan bool)
	go func() {
		for window := range output {
			windows = append(windows, window)
		}
		done <- true
	}()

	start := time.Now()

	go func() {
		// First item.
		input <- 1
		firstTime := time.Now()

		// More items in same session.
		time.Sleep(50 * time.Millisecond)
		input <- 2

		time.Sleep(50 * time.Millisecond)
		input <- 3
		lastTime := time.Now()

		// Wait for timeout.
		time.Sleep(150 * time.Millisecond)

		// Verify session duration.
		expectedDuration := lastTime.Sub(firstTime)
		if expectedDuration < 100*time.Millisecond {
			t.Logf("Session duration: %v", expectedDuration)
		}

		close(input)
	}()

	<-done

	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}

	// Check window times are set.
	window := windows[0]
	if window.Start.IsZero() {
		t.Error("window start time not set")
	}
	if window.End.IsZero() {
		t.Error("window end time not set")
	}
	if window.End.Before(window.Start) {
		t.Error("window end time before start time")
	}

	// Window should start around when we started.
	if window.Start.Before(start) {
		t.Error("window started before test began")
	}
}

// BenchmarkSessionWindow benchmarks session window performance.
func BenchmarkSessionWindow(b *testing.B) {
	ctx := context.Background()

	windower := NewSessionWindow(func(_ int) string { return "all" }, clock.Real).WithGap(10 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		input := make(chan int, 100)
		output := windower.Process(ctx, input)

		// Send burst of items.
		for j := 0; j < 100; j++ {
			input <- j
		}
		close(input)

		// Drain output.
		//nolint:revive // empty-block: necessary to drain channel
		for range output {
			// Consume.
		}
	}
}

// Example demonstrates user session tracking with session windows.
func ExampleSessionWindow() {
	ctx := context.Background()

	// Track user activity sessions with 30-second idle timeout.
	type UserAction struct {
		UserID string
		Action string
		Time   time.Time
	}

	// In practice, you'd use a longer timeout like 30 minutes.
	// Using 100ms for example brevity.
	sessionTracker := NewSessionWindow(func(a UserAction) string { return a.UserID }, clock.Real).
		WithGap(100 * time.Millisecond).
		WithName("user-sessions")

	// Simulate user activity stream.
	actions := make(chan UserAction)
	go func() {
		// User starts a session.
		actions <- UserAction{UserID: "user123", Action: "login", Time: time.Now()}
		time.Sleep(10 * time.Millisecond)
		actions <- UserAction{UserID: "user123", Action: "view_home", Time: time.Now()}
		time.Sleep(10 * time.Millisecond)
		actions <- UserAction{UserID: "user123", Action: "search", Time: time.Now()}

		// User goes idle (session ends).
		time.Sleep(150 * time.Millisecond)

		// User returns (new session).
		actions <- UserAction{UserID: "user123", Action: "view_product", Time: time.Now()}
		time.Sleep(10 * time.Millisecond)
		actions <- UserAction{UserID: "user123", Action: "add_to_cart", Time: time.Now()}

		// Wait for session to complete.
		time.Sleep(150 * time.Millisecond)
		close(actions)
	}()

	// Process user sessions.
	sessions := sessionTracker.Process(ctx, actions)

	sessionNum := 1
	for session := range sessions {
		fmt.Printf("Session %d (%d actions):\n", sessionNum, len(session.Items))
		for _, action := range session.Items {
			fmt.Printf("  - %s\n", action.Action)
		}
		sessionNum++
	}

	// Output:
	// Session 1 (3 actions):
	//   - login
	//   - view_home
	//   - search
	// Session 2 (2 actions):
	//   - view_product
	//   - add_to_cart
}
