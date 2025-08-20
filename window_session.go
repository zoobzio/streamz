package streamz

import (
	"context"
	"sync"
	"time"
)

// SessionWindow groups items into dynamic windows based on activity gaps.
// A new session starts after a period of inactivity (gap), making it ideal
// for grouping related events that occur in bursts with quiet periods between them.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type SessionWindow[T any] struct {
	name    string
	clock   Clock
	keyFunc func(T) string
	gap     time.Duration
}

// NewSessionWindow creates a processor that groups items into session-based windows.
// Sessions are defined by periods of activity separated by gaps of inactivity.
// The keyFunc extracts a session key from each item, allowing multiple concurrent sessions.
// Use the fluent API to configure optional behavior like gap duration.
//
// When to use:
//   - User activity tracking (web sessions, app usage)
//   - Grouping related log entries or transactions
//   - Detecting work patterns with natural breaks
//   - Conversation threading in chat applications
//   - Batch processing of related events
//
// Example:
//
//	// Group user actions into sessions (30-minute default gap)
//	sessions := streamz.NewSessionWindow(
//		func(action UserAction) string { return action.UserID },
//		Real,
//	)
//
//	// With custom gap duration
//	sessions := streamz.NewSessionWindow(
//		func(action UserAction) string { return action.UserID },
//		Real,
//	).WithGap(30*time.Minute)
//
//	userSessions := sessions.Process(ctx, actions)
//	for session := range userSessions {
//		fmt.Printf("User %s session: %d actions from %s to %s\n",
//			session.Items[0].UserID,
//			len(session.Items),
//			session.Start.Format("15:04"),
//			session.End.Format("15:04"))
//		analyzeSession(session.Items)
//	}
//
//	// Group related log entries with 5-second gaps
//	logSessions := streamz.NewSessionWindow(
//		func(log LogEntry) string { return log.RequestID },
//		Real,
//	).WithGap(5*time.Second)
//
// Parameters:
//   - keyFunc: Extracts session identifier from items (for concurrent sessions)
//   - clock: Clock interface for time operations
//
// Returns a new SessionWindow processor with fluent configuration.
func NewSessionWindow[T any](keyFunc func(T) string, clock Clock) *SessionWindow[T] {
	return &SessionWindow[T]{
		gap:     30 * time.Minute, // sensible default
		name:    "session-window",
		keyFunc: keyFunc,
		clock:   clock,
	}
}

// WithGap sets the maximum time between items in the same session.
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

func (w *SessionWindow[T]) Process(ctx context.Context, in <-chan T) <-chan Window[T] {
	out := make(chan Window[T])

	go func() {
		defer close(out)

		sessions := make(map[string]*Window[T])
		timers := make(map[string]Timer)
		var mu sync.Mutex

		for {
			select {
			case <-ctx.Done():
				// Stop all timers before exiting.
				mu.Lock()
				for _, timer := range timers {
					timer.Stop()
				}
				for _, window := range sessions {
					if len(window.Items) > 0 {
						select {
						case out <- *window:
						case <-ctx.Done():
						}
					}
				}
				mu.Unlock()
				return

			case item, ok := <-in:
				if !ok {
					// Stop all timers before exiting.
					mu.Lock()
					for _, timer := range timers {
						timer.Stop()
					}
					for _, window := range sessions {
						if len(window.Items) > 0 {
							select {
							case out <- *window:
							case <-ctx.Done():
							}
						}
					}
					mu.Unlock()
					return
				}

				key := w.keyFunc(item)
				now := w.clock.Now()

				mu.Lock()
				if session, exists := sessions[key]; exists {
					session.Items = append(session.Items, item)
					session.End = now.Add(w.gap)

					if timer, ok := timers[key]; ok {
						timer.Stop()
					}
				} else {
					sessions[key] = &Window[T]{
						Items: []T{item},
						Start: now,
						End:   now.Add(w.gap),
					}
				}

				timer := w.clock.AfterFunc(w.gap, func() {
					mu.Lock()
					defer mu.Unlock()

					if session, exists := sessions[key]; exists {
						// Make a copy to send.
						windowCopy := *session
						delete(sessions, key)
						delete(timers, key)

						// Send with lock held to prevent race.
						select {
						case out <- windowCopy:
						case <-ctx.Done():
						}
					}
				})
				timers[key] = timer
				mu.Unlock()
			}
		}
	}()

	return out
}

func (w *SessionWindow[T]) Name() string {
	return w.name
}
