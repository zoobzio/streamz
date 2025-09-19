package streamz

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zoobzio/clockz"
)

// TestWindow_MetadataPreservation tests that window metadata is correctly attached
// to Results emitted by all window processors.
func TestWindow_MetadataPreservation(t *testing.T) {
	ctx := context.Background()
	clock := clockz.NewFakeClockAt(time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC))

	t.Run("TumblingWindow", func(t *testing.T) {
		in := make(chan Result[int], 3)
		window := NewTumblingWindow[int](time.Minute, clock)

		// Add some test data with original metadata
		in <- NewSuccess(1).WithMetadata("source", "api")
		in <- NewSuccess(2).WithMetadata("user_id", "123")
		in <- NewError(3, fmt.Errorf("test"), "processor").WithMetadata("context", "validation")
		close(in)

		results := window.Process(ctx, in)

		// Advance clock to trigger window emission
		clock.Advance(time.Minute)
		clock.BlockUntilReady()

		var collected []Result[int]
		for result := range results {
			collected = append(collected, result)
		}

		if len(collected) != 3 {
			t.Fatalf("expected 3 results, got %d", len(collected))
		}

		// Verify all results have window metadata
		for i, result := range collected {
			// Verify original metadata preserved
			if result.IsSuccess() && result.Value() == 1 {
				source, found, err := result.GetStringMetadata("source")
				if err != nil || !found || source != "api" {
					t.Errorf("result %d: original metadata not preserved", i)
				}
			}

			// Verify window metadata added
			meta, err := GetWindowMetadata(result)
			if err != nil {
				t.Errorf("result %d: failed to get window metadata: %v", i, err)
				continue
			}

			if meta.Type != "tumbling" {
				t.Errorf("result %d: expected window type 'tumbling', got %s", i, meta.Type)
			}
			if meta.Size != time.Minute {
				t.Errorf("result %d: expected window size 1m, got %v", i, meta.Size)
			}
			if meta.Start.IsZero() || meta.End.IsZero() {
				t.Errorf("result %d: window boundaries not set", i)
			}
		}
	})

	t.Run("SlidingWindow", func(t *testing.T) {
		in := make(chan Result[int], 2)
		window := NewSlidingWindow[int](5*time.Minute, clock).WithSlide(time.Minute)

		in <- NewSuccess(1)
		in <- NewSuccess(2)
		close(in)

		results := window.Process(ctx, in)

		// Advance to trigger window emission
		clock.Advance(6 * time.Minute)
		clock.BlockUntilReady()

		var collected []Result[int]
		for result := range results {
			collected = append(collected, result)
		}

		// Should have results (may be duplicated across multiple windows)
		if len(collected) == 0 {
			t.Fatal("expected some results from sliding window")
		}

		// Check first result has sliding window metadata
		meta, err := GetWindowMetadata(collected[0])
		if err != nil {
			t.Fatalf("failed to get window metadata: %v", err)
		}

		if meta.Type != "sliding" {
			t.Errorf("expected window type 'sliding', got %s", meta.Type)
		}
		if meta.Size != 5*time.Minute {
			t.Errorf("expected window size 5m, got %v", meta.Size)
		}
		if meta.Slide == nil || *meta.Slide != time.Minute {
			t.Errorf("expected slide 1m, got %v", meta.Slide)
		}
	})

	t.Run("SessionWindow", func(t *testing.T) {
		keyFunc := func(result Result[string]) string {
			if result.IsSuccess() {
				return result.Value()[:1] // First character as session key
			}
			return result.Error().Item[:1]
		}

		in := make(chan Result[string], 3)
		window := NewSessionWindow(keyFunc, clock).WithGap(time.Minute)

		in <- NewSuccess("alice-1")
		in <- NewSuccess("alice-2")
		in <- NewSuccess("bob-1")
		close(in)

		results := window.Process(ctx, in)

		// Advance to trigger session expiry
		clock.Advance(2 * time.Minute)
		clock.BlockUntilReady()

		var collected []Result[string]
		for result := range results {
			collected = append(collected, result)
		}

		if len(collected) != 3 {
			t.Fatalf("expected 3 results, got %d", len(collected))
		}

		// Group by session key and verify metadata
		sessions := make(map[string][]Result[string])
		for _, result := range collected {
			meta, err := GetWindowMetadata(result)
			if err != nil {
				t.Errorf("failed to get window metadata: %v", err)
				continue
			}

			if meta.Type != "session" {
				t.Errorf("expected window type 'session', got %s", meta.Type)
			}
			if meta.Gap == nil || *meta.Gap != time.Minute {
				t.Errorf("expected gap 1m, got %v", meta.Gap)
			}
			if meta.SessionKey == nil {
				t.Error("expected session key to be set")
			} else {
				sessions[*meta.SessionKey] = append(sessions[*meta.SessionKey], result)
			}
		}

		// Should have alice and bob sessions
		if len(sessions) != 2 {
			t.Errorf("expected 2 sessions, got %d", len(sessions))
		}
		if len(sessions["a"]) != 2 {
			t.Errorf("expected 2 results in alice session, got %d", len(sessions["a"]))
		}
		if len(sessions["b"]) != 1 {
			t.Errorf("expected 1 result in bob session, got %d", len(sessions["b"]))
		}
	})
}

// TestWindowCollector_StructKeys tests the WindowCollector with struct keys for performance.
func TestWindowCollector_StructKeys(t *testing.T) {
	collector := NewWindowCollector[int]()

	// Create Results with identical window metadata to test struct key efficiency
	windowStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(5 * time.Minute)

	meta := WindowMetadata{
		Start: windowStart,
		End:   windowEnd,
		Type:  "tumbling",
		Size:  5 * time.Minute,
	}

	input := make(chan Result[int], 1000)

	// Add 1000 Results with identical metadata
	for i := 0; i < 1000; i++ {
		input <- AddWindowMetadata(NewSuccess(i), meta)
	}
	close(input)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	collections := collector.Process(ctx, input)

	windows := make([]WindowCollection[int], 0, 5)
	for window := range collections {
		windows = append(windows, window)
	}
	duration := time.Since(start)

	if len(windows) != 1 {
		t.Errorf("expected 1 aggregated window, got %d", len(windows))
	}
	if len(windows[0].Results) != 1000 {
		t.Errorf("expected 1000 results in window, got %d", len(windows[0].Results))
	}

	// Performance assertion - should handle 1K items quickly with struct keys
	if duration > 100*time.Millisecond {
		t.Errorf("struct key aggregation took too long: %v", duration)
	}
}

// TestWindowInfo_TypeSafety tests enhanced type safety helpers.
func TestWindowInfo_TypeSafety(t *testing.T) {
	// Create result with valid window metadata
	meta := WindowMetadata{
		Start: time.Now(),
		End:   time.Now().Add(time.Hour),
		Type:  "tumbling",
		Size:  time.Hour,
	}
	result := AddWindowMetadata(NewSuccess(42), meta)

	// Test GetWindowInfo
	info, err := GetWindowInfo(result)
	if err != nil {
		t.Fatalf("GetWindowInfo failed: %v", err)
	}

	if info.Type != WindowTypeTumbling {
		t.Errorf("expected WindowTypeTumbling, got %v", info.Type)
	}
	if info.Size != time.Hour {
		t.Errorf("expected 1 hour size, got %v", info.Size)
	}

	// Test IsInWindow
	inWindow, err := IsInWindow(result, meta.Start.Add(30*time.Minute))
	if err != nil {
		t.Errorf("IsInWindow failed: %v", err)
	}
	if !inWindow {
		t.Error("timestamp should be in window")
	}

	// Test WindowDuration
	duration, err := WindowDuration(result)
	if err != nil {
		t.Errorf("WindowDuration failed: %v", err)
	}
	if duration.Round(time.Second) != time.Hour {
		t.Errorf("expected ~1 hour duration, got %v", duration)
	}

	// Test invalid window type
	invalidResult := NewSuccess(42).
		WithMetadata(MetadataWindowStart, time.Now()).
		WithMetadata(MetadataWindowEnd, time.Now().Add(time.Hour)).
		WithMetadata(MetadataWindowType, "invalid")

	_, err = GetWindowInfo(invalidResult)
	if err == nil {
		t.Error("expected error for invalid window type")
	}
}
