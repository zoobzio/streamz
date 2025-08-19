package streamz

import (
	"context"
	"fmt"
	"sync"
)

// Route represents a single routing destination with a predicate and processor.
type Route[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	Name      string
	Predicate func(T) bool
	Processor Processor[T, T]
}

// RouterOutput contains the outputs from all routes.
type RouterOutput[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	Routes map[string]<-chan T
}

// Router implements content-based routing, sending items to different
// processors based on predicates. It evaluates each item against route
// predicates and sends it to matching routes.
//
// The Router supports multiple routing strategies:
//   - First-match: Item goes to first matching route only.
//   - All-matches: Item goes to all matching routes.
//   - Default route: Fallback for items matching no routes.
//
// Key features:
//   - Content-based routing with custom predicates.
//   - Multiple routing strategies (first-match, all-matches).
//   - Optional default route for unmatched items.
//   - Named routes for easy identification.
//   - Concurrent processing of all routes.
//   - Graceful shutdown and context cancellation.
//
// When to use:
//   - Splitting streams based on content (e.g., by type, priority).
//   - Implementing conditional processing pipelines.
//   - Load distribution based on item properties.
//   - A/B testing with traffic splitting.
//   - Multi-tenant processing with tenant-based routing.
//
// Example:
//
//	// Basic routing by type.
//	router := streamz.NewRouter[Order]().
//		AddRoute("high-value", func(o Order) bool {
//			return o.Total > 1000
//		}, highValueProcessor).
//		AddRoute("international", func(o Order) bool {
//			return o.Country != "US"
//		}, internationalProcessor).
//		WithDefault(standardProcessor)
//
//	outputs := router.Process(ctx, orders)
//	highValueOrders := outputs.Routes["high-value"]
//	internationalOrders := outputs.Routes["international"]
//	standardOrders := outputs.Routes["default"]
//
//	// All-matches routing.
//	router := streamz.NewRouter[Event]().
//		AllMatches(). // Send to all matching routes.
//		AddRoute("audit", func(e Event) bool {
//			return e.Type == "security"
//		}, auditProcessor).
//		AddRoute("alerts", func(e Event) bool {
//			return e.Severity == "critical"
//		}, alertProcessor)
//
// Performance characteristics:
//   - Minimal overhead for predicate evaluation.
//   - Concurrent processing of all routes.
//   - No buffering between router and route processors.
//   - Memory efficient with direct channel passing.
type Router[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	routes       []Route[T]
	defaultRoute *Route[T]
	allMatches   bool
	name         string
	bufferSize   int
}

// NewRouter creates a new content-based router.
// By default, uses first-match strategy where items are sent
// to the first route whose predicate returns true.
//
// Use the fluent API to configure routes and routing behavior.
//
// Returns a new Router with fluent configuration methods.
func NewRouter[T any]() *Router[T] {
	return &Router[T]{
		routes:     make([]Route[T], 0),
		allMatches: false,
		name:       "router",
		bufferSize: 0, // Unbuffered by default.
	}
}

// AddRoute adds a named route with a predicate and processor.
// Routes are evaluated in the order they are added.
//
// Parameters:
//   - name: Unique name for the route (used as key in output map).
//   - predicate: Function that returns true if item should be routed here.
//   - processor: Processor to handle items matching the predicate.
//
// Returns the Router for method chaining.
func (r *Router[T]) AddRoute(name string, predicate func(T) bool, processor Processor[T, T]) *Router[T] {
	r.routes = append(r.routes, Route[T]{
		Name:      name,
		Predicate: predicate,
		Processor: processor,
	})
	return r
}

// WithDefault sets a default processor for items that don't match any route.
// The default route is named "default" in the output map.
//
// Without a default route, unmatched items are dropped.
func (r *Router[T]) WithDefault(processor Processor[T, T]) *Router[T] {
	r.defaultRoute = &Route[T]{
		Name:      "default",
		Predicate: func(T) bool { return true },
		Processor: processor,
	}
	return r
}

// AllMatches enables routing to all matching routes instead of just the first.
// When enabled, an item will be sent to every route whose predicate returns true.
func (r *Router[T]) AllMatches() *Router[T] {
	r.allMatches = true
	return r
}

// FirstMatch explicitly sets first-match routing strategy (default).
// Items are sent only to the first route whose predicate returns true.
func (r *Router[T]) FirstMatch() *Router[T] {
	r.allMatches = false
	return r
}

// WithBufferSize sets the buffer size for route input channels.
// This can help prevent blocking when routes process at different speeds.
//
// Default is 0 (unbuffered).
func (r *Router[T]) WithBufferSize(size int) *Router[T] {
	if size < 0 {
		size = 0
	}
	r.bufferSize = size
	return r
}

// WithName sets a custom name for this processor.
// Useful for monitoring and debugging.
func (r *Router[T]) WithName(name string) *Router[T] {
	r.name = name
	return r
}

// Process implements the Processor interface, routing items to appropriate processors.
// Returns a RouterOutput containing channels for all routes.
func (r *Router[T]) Process(ctx context.Context, in <-chan T) RouterOutput[T] {
	// Create input channels for all routes.
	routeInputs := make(map[string]chan T)
	routeOutputs := make(map[string]<-chan T)

	// Create channels and start processors for each route.
	for _, route := range r.routes {
		input := make(chan T, r.bufferSize)
		routeInputs[route.Name] = input
		routeOutputs[route.Name] = route.Processor.Process(ctx, input)
	}

	// Add default route if configured.
	if r.defaultRoute != nil {
		input := make(chan T, r.bufferSize)
		routeInputs[r.defaultRoute.Name] = input
		routeOutputs[r.defaultRoute.Name] = r.defaultRoute.Processor.Process(ctx, input)
	}

	// Start routing goroutine.
	go func() {
		// Ensure all route inputs are closed when done.
		defer func() {
			for _, ch := range routeInputs {
				close(ch)
			}
		}()

		for {
			select {
			case item, ok := <-in:
				if !ok {
					return
				}

				routed := false

				// Evaluate routes.
				for _, route := range r.routes {
					if route.Predicate(item) {
						select {
						case routeInputs[route.Name] <- item:
							routed = true
							if !r.allMatches {
								goto doneRouting // First match only
							}
						case <-ctx.Done():
							return
						}
					}
				}
			doneRouting:

				// Send to default if no routes matched.
				if !routed && r.defaultRoute != nil {
					select {
					case routeInputs[r.defaultRoute.Name] <- item:
					case <-ctx.Done():
						return
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return RouterOutput[T]{
		Routes: routeOutputs,
	}
}

// Name returns the processor name for debugging and monitoring.
func (r *Router[T]) Name() string {
	return r.name
}

// ProcessToSingle collects all route outputs into a single channel.
// This is useful when you want router functionality but need a single
// output channel for compatibility with other processors.
//
// Items from all routes are merged into the output channel.
// The order is non-deterministic when multiple routes are active.
func (r *Router[T]) ProcessToSingle(ctx context.Context, in <-chan T) <-chan T {
	routerOutput := r.Process(ctx, in)
	out := make(chan T)

	// Start goroutine to merge all outputs.
	go func() {
		defer close(out)

		var wg sync.WaitGroup

		// Start a goroutine for each route output.
		for routeName, routeChan := range routerOutput.Routes {
			wg.Add(1)
			go func(_ string, ch <-chan T) {
				defer wg.Done()
				for {
					select {
					case item, ok := <-ch:
						if !ok {
							return
						}
						select {
						case out <- item:
						case <-ctx.Done():
							return
						}
					case <-ctx.Done():
						return
					}
				}
			}(routeName, routeChan)
		}

		// Wait for all routes to complete.
		wg.Wait()
	}()

	return out
}

// RouteStats provides statistics about routing operations.
type RouteStats struct { //nolint:govet // logical field grouping preferred over memory optimization
	RouteName   string
	ItemsRouted int64
}

// GetRouteNames returns the names of all configured routes.
func (r *Router[T]) GetRouteNames() []string {
	names := make([]string, 0, len(r.routes)+1)
	for _, route := range r.routes {
		names = append(names, route.Name)
	}
	if r.defaultRoute != nil {
		names = append(names, r.defaultRoute.Name)
	}
	return names
}

// RouterBuilder provides a more complex routing configuration API
// for advanced use cases.
type RouterBuilder[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	router *Router[T]
}

// NewRouterBuilder creates a builder for complex routing configurations.
func NewRouterBuilder[T any]() *RouterBuilder[T] {
	return &RouterBuilder[T]{
		router: NewRouter[T](),
	}
}

// Route adds a route using a fluent sub-builder pattern.
func (rb *RouterBuilder[T]) Route(name string) *RouteBuilder[T] {
	return &RouteBuilder[T]{
		builder: rb,
		name:    name,
	}
}

// Build returns the configured Router.
func (rb *RouterBuilder[T]) Build() *Router[T] {
	return rb.router
}

// RouteBuilder provides fluent configuration for individual routes.
type RouteBuilder[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	builder   *RouterBuilder[T]
	name      string
	predicate func(T) bool
}

// When sets the predicate for this route.
func (rb *RouteBuilder[T]) When(predicate func(T) bool) *RouteBuilder[T] {
	rb.predicate = predicate
	return rb
}

// To sets the processor for this route and returns to the main builder.
func (rb *RouteBuilder[T]) To(processor Processor[T, T]) *RouterBuilder[T] {
	if rb.predicate == nil {
		panic(fmt.Sprintf("route %q missing predicate", rb.name))
	}
	rb.builder.router.AddRoute(rb.name, rb.predicate, processor)
	return rb.builder
}
