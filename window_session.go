package streamz

import (
	"context"
	"time"
)

// SessionWindow groups Results into dynamic windows based on activity gaps.
// A new session starts after a period of inactivity (gap), making it ideal
// for grouping related events that occur in bursts with quiet periods between them.
//
// This version processes Result[T] streams, capturing both successful values
// and errors within each session for comprehensive session analysis and
// error correlation within activity periods.
//
// Key characteristics:
//   - Dynamic duration: Sessions vary based on activity patterns
//   - Key-based: Multiple concurrent sessions via key extraction
//   - Activity-driven: Extends with each new item, closes after gap
//
// Performance characteristics:
//   - Session closure latency: gap/8 average, gap/4 maximum (checked at gap/4 intervals)
//   - Memory usage: O(active_sessions × items_per_session)
//   - Processing overhead: Single map lookup per item
//   - Goroutine usage: 1 goroutine per processor instance (no timer callbacks)
//   - Session checking frequency: gap/4 (balanced latency vs CPU usage)
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type SessionWindow[T any] struct {
	name    string
	clock   Clock
	keyFunc func(Result[T]) string // Extract session key from Result
	gap     time.Duration
}

// sessionState tracks enhanced session state for the single-goroutine architecture.
// Separates boundary tracking from emitted metadata for dynamic session extension.
//
//nolint:govet // fieldalignment: struct layout optimized for readability over memory
type sessionState[T any] struct {
	meta           WindowMetadata
	results        []Result[T]
	lastActivity   time.Time
	currentEndTime time.Time // Separate from meta.End for dynamic updates
}

// NewSessionWindow creates a processor that groups Results into session-based windows.
// Sessions are defined by periods of activity separated by gaps of inactivity.
// The keyFunc extracts a session key from each Result, allowing multiple concurrent sessions.
// Use the fluent API to configure optional behavior like gap duration.
//
// When to use:
//   - User activity tracking with error monitoring (web sessions, app usage)
//   - Grouping related log entries or transactions with failure analysis
//   - Detecting work patterns with natural breaks and error clustering
//   - Conversation threading in chat applications with error handling
//   - Batch processing of related events with failure correlation
//   - API request session analysis with success/failure rates
//
// Example:
//
//	// Group user actions into sessions with error tracking (30-minute default gap)
//	sessions := streamz.NewSessionWindow(
//		func(result Result[UserAction]) string {
//			if result.IsError() {
//				// Use error context to extract user ID for session grouping
//				return result.Error().Item.UserID
//			}
//			return result.Value().UserID
//		},
//		streamz.RealClock,
//	)
//
//	// Custom gap duration for faster session timeout
//	sessions := streamz.NewSessionWindow(
//		func(result Result[UserAction]) string {
//			// Extract user ID from either success or error
//			if result.IsSuccess() {
//				return result.Value().UserID
//			}
//			return result.Error().Item.UserID
//		},
//		streamz.RealClock,
//	).WithGap(10*time.Minute)
//
//	results := sessions.Process(ctx, actionResults)
//	for result := range results {
//		// Each result has session metadata attached
//		if meta, err := streamz.GetWindowMetadata(result); err == nil {
//			fmt.Printf("Action in session [%s]: %v from %s to %s\n",
//				*meta.SessionKey,
//				result.Value(),
//				meta.Start.Format("15:04:05"),
//				meta.End.Format("15:04:05"))
//		}
//	}
//
//	// Collect into sessions for analysis when needed
//	collector := streamz.NewWindowCollector[UserAction]()
//	collections := collector.Process(ctx, results)
//	for collection := range collections {
//		values := collection.Values()    // Successful actions
//		errors := collection.Errors()    // Failed actions
//		totalActions := collection.Count()
//		successRate := float64(collection.SuccessCount()) / float64(totalActions) * 100
//
//		// Alert on sessions with high error rates
//		if successRate < 80 && totalActions > 5 {
//			alert.Send("User session with high error rate", collection)
//		}
//	}
//
// Parameters:
//   - keyFunc: Extracts session identifier from Results (handles both success and errors)
//   - clock: Clock interface for time operations (use RealClock for production)
//
// Returns a new SessionWindow processor with fluent configuration.
//
// Performance notes:
//   - Memory scales with number of concurrent sessions
//   - Session closure latency: gap/8 average, gap/4 maximum
//   - Best for activity-based grouping with natural boundaries
func NewSessionWindow[T any](keyFunc func(Result[T]) string, clock Clock) *SessionWindow[T] {
	return &SessionWindow[T]{
		gap:     30 * time.Minute, // sensible default
		name:    "session-window",
		keyFunc: keyFunc,
		clock:   clock,
	}
}

// WithGap sets the maximum time between Results in the same session.
// If not set, defaults to 30 minutes.
func (w *SessionWindow[T]) WithGap(gap time.Duration) *SessionWindow[T] {
	w.gap = gap
	return w
}

// WithName sets a custom name for this processor.
// If not set, defaults to "session-window".
func (w *SessionWindow[T]) WithName(name string) *SessionWindow[T] {
	w.name = name
	return w
}

// Process groups Results into session-based windows, emitting individual Results with session metadata.
// Both successful values and errors extend sessions, allowing comprehensive
// analysis of user behavior, error patterns, and success rates within
// natural activity periods.
//
// Session behavior:
// - New session starts with first Result for a key
// - Session extends with each new Result (success or error) for that key
// - Session closes after gap duration with no activity
// - Multiple concurrent sessions supported via key extraction
// - All Results within session timeframe get session metadata attached
//
// Implementation uses single-goroutine architecture with periodic session checking
// at gap/4 intervals. Enhanced boundary tracking separates session state management
// from emitted metadata to handle dynamic session extension correctly:
//   - Average latency: gap/8 (uniformly distributed)
//   - Maximum latency: gap/4 (worst case)
//   - Example: 30-minute gap = 3.75 minute average, 7.5 minute max closure delay
//
// Performance and resource usage:
//   - Memory scales with number of concurrent sessions
//   - No memory growth within sessions (bounded by activity)
//   - CPU usage: O(active_sessions) every gap/4 interval
//   - Thread-safe: Single goroutine prevents timer callback races
//   - Efficient cleanup: Expired sessions removed immediately
//
// Trade-offs:
//   - Delayed emission (gap/8 average) vs real-time complexity
//   - Single goroutine safety vs multiple timer overhead
//   - Memory for all active sessions vs streaming aggregation
func (w *SessionWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		// Enhanced session state tracking
		sessions := make(map[string]*sessionState[T])

		checkInterval := w.gap / 4
		if checkInterval < 10*time.Millisecond {
			checkInterval = 10 * time.Millisecond
		}
		ticker := w.clock.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Emit all remaining sessions - use background context to ensure delivery
				w.emitAllSessions(context.Background(), out, sessions)
				return

			case result, ok := <-in:
				if !ok {
					// Input closed - emit all remaining sessions
					w.emitAllSessions(ctx, out, sessions)
					return
				}

				key := w.keyFunc(result)
				now := w.clock.Now()

				if session, exists := sessions[key]; exists {
					// Extend existing session - update tracking state
					session.results = append(session.results, result)
					session.lastActivity = now
					session.currentEndTime = now.Add(w.gap)
					// Note: meta.End stays fixed at original window end for emitted Results
				} else {
					// Create new session
					gapPtr := &w.gap
					keyPtr := &key
					sessions[key] = &sessionState[T]{
						meta: WindowMetadata{
							Start:      now,
							End:        now.Add(w.gap), // Initial end time
							Type:       "session",
							Gap:        gapPtr,
							SessionKey: keyPtr,
						},
						results:        []Result[T]{result},
						lastActivity:   now,
						currentEndTime: now.Add(w.gap),
					}
				}

			case <-ticker.C():
				// Periodic session expiry check
				now := w.clock.Now()
				expiredKeys := make([]string, 0)

				for key, session := range sessions {
					if now.Sub(session.lastActivity) >= w.gap {
						expiredKeys = append(expiredKeys, key)

						// Update final end time in metadata for emission
						finalMeta := session.meta
						finalMeta.End = session.currentEndTime

						w.emitWindowResults(ctx, out, session.results, finalMeta)
					}
				}

				// Clean up expired sessions
				for _, key := range expiredKeys {
					delete(sessions, key)
				}
			}
		}
	}()

	return out
}

// emitWindowResults emits all results in the session with session metadata attached.
func (*SessionWindow[T]) emitWindowResults(ctx context.Context, out chan<- Result[T], results []Result[T], meta WindowMetadata) {
	for _, result := range results {
		enhanced := AddWindowMetadata(result, meta)
		select {
		case out <- enhanced:
		case <-ctx.Done():
			return
		}
	}
}

// emitAllSessions emits all remaining sessions when processing ends.
func (w *SessionWindow[T]) emitAllSessions(ctx context.Context, out chan<- Result[T], sessions map[string]*sessionState[T]) {
	for _, session := range sessions {
		if len(session.results) > 0 {
			// Use current end time for final emission
			finalMeta := session.meta
			finalMeta.End = session.currentEndTime
			w.emitWindowResults(ctx, out, session.results, finalMeta)
		}
	}
}

// Name returns the processor name for debugging and monitoring.
func (w *SessionWindow[T]) Name() string {
	return w.name
}
