package streamz

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Switch routes Result[T] to multiple output channels based on predicate evaluation.
// Errors bypass predicate evaluation and go directly to the error channel.
// Successful values are evaluated by the predicate to determine routing.
type Switch[T any, K comparable] struct {
	predicate  func(T) K            // Evaluates successful values only (8 bytes pointer)
	routes     map[K]chan Result[T] // Route key to output channel mapping (8 bytes pointer)
	errorChan  chan Result[T]       // Dedicated error channel (8 bytes pointer)
	defaultKey *K                   // Optional default route for unknown keys (8 bytes pointer)
	name       string               // 16 bytes (pointer + len)
	mu         sync.RWMutex         // 24 bytes
	bufferSize int                  // 8 bytes (aligned)
}

// SwitchConfig configures Switch behavior.
type SwitchConfig[K comparable] struct {
	DefaultKey *K  // Route for unknown predicate results (nil = drop)
	BufferSize int // Per-route channel buffer size (0 = unbuffered)
}

// NewSwitch creates a Switch with full configuration options.
func NewSwitch[T any, K comparable](predicate func(T) K, config SwitchConfig[K]) *Switch[T, K] {
	return &Switch[T, K]{
		name:       "switch",
		predicate:  predicate,
		routes:     make(map[K]chan Result[T]),
		errorChan:  make(chan Result[T], config.BufferSize),
		defaultKey: config.DefaultKey,
		bufferSize: config.BufferSize,
	}
}

// NewSwitchSimple creates a Switch with default configuration (unbuffered, no default route).
func NewSwitchSimple[T any, K comparable](predicate func(T) K) *Switch[T, K] {
	return NewSwitch(predicate, SwitchConfig[K]{
		BufferSize: 0,   // Unbuffered channels
		DefaultKey: nil, // No default route - drop unknown keys
	})
}

// Process routes input Results to output channels based on predicate evaluation.
// Returns read-only channel maps for routes and errors.
// All channels are closed when processing completes or context is canceled.
func (s *Switch[T, K]) Process(ctx context.Context, in <-chan Result[T]) (routes map[K]<-chan Result[T], errors <-chan Result[T]) {
	go func() {
		defer func() {
			s.mu.Lock()
			for _, ch := range s.routes {
				close(ch)
			}
			close(s.errorChan)
			s.mu.Unlock()
		}()

		// Main processing loop with context integration
		for {
			select {
			case <-ctx.Done():
				return
			case result, ok := <-in:
				if !ok {
					return
				}
				s.routeResult(ctx, result)
			}
		}
	}()

	// Return empty read-only map initially - routes are managed through AddRoute method
	routes = make(map[K]<-chan Result[T])
	errors = s.errorChan
	return
}

// routeResult handles routing logic for a single Result[T].
func (s *Switch[T, K]) routeResult(ctx context.Context, result Result[T]) {
	if result.IsError() {
		// Errors bypass predicate, go to error channel
		s.sendToErrorChannel(ctx, result)
		return
	}

	// Evaluate predicate on successful value only
	var routeKey K
	var panicRecovered bool

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicRecovered = true
				// Create new error Result for predicate panic
				err := fmt.Errorf("predicate panic: %v", r)
				errorResult := NewError(result.Value(), err, "switch").
					WithMetadata(MetadataProcessor, "switch").
					WithMetadata(MetadataTimestamp, time.Now())
				s.sendToErrorChannel(ctx, errorResult)
			}
		}()
		routeKey = s.predicate(result.Value())
	}()

	if panicRecovered {
		return
	}

	// Route to appropriate channel
	s.routeToChannel(ctx, routeKey, result)
}

// routeToChannel handles routing to specific channels with proper error handling.
func (s *Switch[T, K]) routeToChannel(ctx context.Context, key K, result Result[T]) {
	s.mu.RLock()
	ch, exists := s.routes[key]
	s.mu.RUnlock()

	// Complete route-not-found behavior
	if !exists {
		if s.defaultKey != nil {
			// Recursive call to handle default route (which must exist)
			s.routeToChannel(ctx, *s.defaultKey, result)
			return
		}
		// No default route - drop message
		return
	}

	// Add routing metadata using existing constants
	enhanced := result.
		WithMetadata("route", key).
		WithMetadata(MetadataProcessor, "switch").
		WithMetadata(MetadataTimestamp, time.Now())

	// Send with context cancellation support
	select {
	case ch <- enhanced:
		// Successfully routed
	case <-ctx.Done():
		// Context canceled, stop processing
		return
	}
}

// sendToErrorChannel handles error channel routing with context support.
func (s *Switch[T, K]) sendToErrorChannel(ctx context.Context, result Result[T]) {
	enhanced := result.
		WithMetadata(MetadataProcessor, "switch").
		WithMetadata(MetadataTimestamp, time.Now())

	select {
	case s.errorChan <- enhanced:
		// Successfully sent to error channel
	case <-ctx.Done():
		// Context canceled, stop processing
		return
	}
}

// getOrCreateRoute handles lazy channel creation with proper locking.
func (s *Switch[T, K]) getOrCreateRoute(key K) chan Result[T] {
	s.mu.RLock()
	if ch, exists := s.routes[key]; exists {
		s.mu.RUnlock()
		return ch
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if ch, exists := s.routes[key]; exists {
		return ch
	}

	// Create new channel with configured buffer size
	ch := make(chan Result[T], s.bufferSize)
	s.routes[key] = ch
	return ch
}

// AddRoute explicitly creates a route for the given key.
func (s *Switch[T, K]) AddRoute(key K) <-chan Result[T] {
	ch := s.getOrCreateRoute(key)
	return ch
}

// RemoveRoute removes a route and closes its channel.
func (s *Switch[T, K]) RemoveRoute(key K) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch, exists := s.routes[key]
	if !exists {
		return false
	}

	delete(s.routes, key)
	close(ch)
	return true
}

// HasRoute checks if a route exists for the given key.
func (s *Switch[T, K]) HasRoute(key K) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.routes[key]
	return exists
}

// RouteKeys returns all current route keys.
func (s *Switch[T, K]) RouteKeys() []K {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]K, 0, len(s.routes))
	for key := range s.routes {
		keys = append(keys, key)
	}
	return keys
}

// ErrorChannel returns read-only access to the error channel.
func (s *Switch[T, K]) ErrorChannel() <-chan Result[T] {
	return s.errorChan
}
