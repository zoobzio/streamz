package streamz

import (
	"context"
	"time"
)

// SessionWindow groups items into dynamic windows based on activity gaps.
// A new session starts after a period of inactivity (gap), making it ideal
// for grouping related events that occur in bursts with quiet periods between them.
type SessionWindow[T any] struct {
	keyFunc func(T) string
	name    string
	gap     time.Duration
}

// NewSessionWindow creates a processor that groups items into session-based windows.
// Sessions are defined by periods of activity separated by gaps of inactivity.
// The keyFunc extracts a session key from each item, allowing multiple concurrent sessions.
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
//	// Group user actions into sessions with 30-minute timeout
//	sessions := streamz.NewSessionWindow[UserAction](
//		30*time.Minute,
//		func(action UserAction) string { return action.UserID },
//	)
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
//	logSessions := streamz.NewSessionWindow[LogEntry](
//		5*time.Second,
//		func(log LogEntry) string { return log.RequestID },
//	)
//
// Parameters:
//   - gap: Maximum time between items in the same session
//   - keyFunc: Extracts session identifier from items (for concurrent sessions)
//
// Returns a new SessionWindow processor.
func NewSessionWindow[T any](gap time.Duration, keyFunc func(T) string) *SessionWindow[T] {
	return &SessionWindow[T]{
		gap:     gap,
		name:    "session-window",
		keyFunc: keyFunc,
	}
}

func (w *SessionWindow[T]) Process(ctx context.Context, in <-chan T) <-chan Window[T] {
	out := make(chan Window[T])

	go func() {
		defer close(out)

		sessions := make(map[string]*Window[T])
		timers := make(map[string]*time.Timer)

		for {
			select {
			case <-ctx.Done():
				for _, window := range sessions {
					if len(window.Items) > 0 {
						out <- *window
					}
				}
				return

			case item, ok := <-in:
				if !ok {
					for _, window := range sessions {
						if len(window.Items) > 0 {
							out <- *window
						}
					}
					return
				}

				key := w.keyFunc(item)
				now := time.Now()

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

				timer := time.AfterFunc(w.gap, func() {
					if session, exists := sessions[key]; exists {
						out <- *session
						delete(sessions, key)
						delete(timers, key)
					}
				})
				timers[key] = timer
			}
		}
	}()

	return out
}

func (w *SessionWindow[T]) Name() string {
	return w.name
}
