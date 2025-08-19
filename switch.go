package streamz

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// SwitchCase represents a single case in the switch processor.
type SwitchCase[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	Name      string
	Condition func(T) bool
	Processor Processor[T, T]
}

// Switch routes items to different processors based on conditions.
// It works like a switch statement, evaluating conditions in order
// and processing items with the first matching case.
//
// The Switch processor provides:
//   - Multiple conditional branches.
//   - Default case for unmatched items.
//   - Early termination on first match.
//   - Named cases for monitoring.
//   - Type-safe condition evaluation.
//
// Example:
//
//	// Route orders based on priority and value.
//	orderSwitch := streamz.NewSwitch[Order]().
//	    Case("urgent", isUrgentOrder, urgentProcessor).
//	    Case("high-value", isHighValueOrder, highValueProcessor).
//	    Case("international", isInternationalOrder, intlProcessor).
//	    Default(standardProcessor).
//	    WithName("order-router")
//
//	// Process orders through appropriate pipelines.
//	output := orderSwitch.Process(ctx, orders)
//
// Performance characteristics:
//   - O(n) condition evaluation where n = number of cases.
//   - Minimal overhead for condition checking.
//   - Parallel processing within each case.
type Switch[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	cases       []SwitchCase[T]
	defaultProc Processor[T, T]
	name        string
	bufferSize  int

	// Statistics.
	processed    atomic.Int64
	caseMatches  []atomic.Int64 // One counter per case.
	defaultCount atomic.Int64
}

// NewSwitch creates a new switch processor.
// Items are evaluated against conditions in the order cases were added.
//
// Default configuration:
//   - No cases (must add at least one case or default).
//   - Buffer size: 1.
//   - Name: "switch".
func NewSwitch[T any]() *Switch[T] {
	return &Switch[T]{
		cases:       make([]SwitchCase[T], 0),
		name:        "switch",
		bufferSize:  1,
		caseMatches: make([]atomic.Int64, 0),
	}
}

// Case adds a new case to the switch.
// Cases are evaluated in the order they are added.
func (s *Switch[T]) Case(name string, condition func(T) bool, processor Processor[T, T]) *Switch[T] {
	if name == "" {
		name = fmt.Sprintf("case-%d", len(s.cases))
	}

	s.cases = append(s.cases, SwitchCase[T]{
		Name:      name,
		Condition: condition,
		Processor: processor,
	})

	// Add counter for this case.
	s.caseMatches = append(s.caseMatches, atomic.Int64{})

	return s
}

// Default sets the processor for items that don't match any case.
// If no default is set, unmatched items are dropped.
func (s *Switch[T]) Default(processor Processor[T, T]) *Switch[T] {
	s.defaultProc = processor
	return s
}

// WithBufferSize sets the buffer size for internal channels.
// This helps handle speed differences between cases.
func (s *Switch[T]) WithBufferSize(size int) *Switch[T] {
	if size < 0 {
		size = 0
	}
	s.bufferSize = size
	return s
}

// WithName sets a custom name for this processor.
func (s *Switch[T]) WithName(name string) *Switch[T] {
	s.name = name
	return s
}

// Process implements the Processor interface.
func (s *Switch[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	// Create channels for each case.
	caseInputs := make([]chan T, len(s.cases))
	for i := range caseInputs {
		caseInputs[i] = make(chan T, s.bufferSize)
	}

	var defaultInput chan T
	if s.defaultProc != nil {
		defaultInput = make(chan T, s.bufferSize)
	}

	// Start all processors.
	var wg sync.WaitGroup

	// Start case processors.
	for i, switchCase := range s.cases {
		wg.Add(1)
		caseProc := switchCase.Processor
		caseInput := caseInputs[i]

		go func() {
			defer wg.Done()
			caseOutput := caseProc.Process(ctx, caseInput)
			for item := range caseOutput {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Start default processor.
	if s.defaultProc != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defaultOutput := s.defaultProc.Process(ctx, defaultInput)
			for item := range defaultOutput {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Route input items.
	go func() {
		defer func() {
			// Close all inputs.
			for _, ch := range caseInputs {
				close(ch)
			}
			if defaultInput != nil {
				close(defaultInput)
			}
		}()

		for item := range in {
			s.processed.Add(1)

			// Find matching case.
			matched := false
			for i, switchCase := range s.cases {
				if switchCase.Condition(item) {
					select {
					case caseInputs[i] <- item:
						s.caseMatches[i].Add(1)
						matched = true
					case <-ctx.Done():
						return
					}
					break // First match wins.
				}
			}

			// Send to default if no match.
			if !matched && defaultInput != nil {
				select {
				case defaultInput <- item:
					s.defaultCount.Add(1)
				case <-ctx.Done():
					return
				}
			}
			// If no match and no default, item is dropped.
		}
	}()

	// Close output when all processors are done.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// GetStats returns statistics about case matches.
func (s *Switch[T]) GetStats() SwitchStats {
	stats := SwitchStats{
		CaseMatches: make(map[string]int64),
		TotalItems:  s.processed.Load(),
	}

	// Collect case matches
	for i, switchCase := range s.cases {
		stats.CaseMatches[switchCase.Name] = s.caseMatches[i].Load()
	}

	// Add default matches
	stats.CaseMatches["_default"] = s.defaultCount.Load()

	return stats
}

// Name returns the processor name.
func (s *Switch[T]) Name() string {
	return s.name
}

// SwitchStats contains statistics about switch operations.
type SwitchStats struct { //nolint:govet // logical field grouping preferred over memory optimization
	CaseMatches map[string]int64 // Count of matches per case (including "_default")
	TotalItems  int64            // Total items processed
}

// MatchRate returns the percentage of items matching a specific case.
func (s SwitchStats) MatchRate(caseName string) float64 {
	if s.TotalItems == 0 {
		return 0
	}

	count, ok := s.CaseMatches[caseName]
	if !ok {
		return 0
	}

	return float64(count) / float64(s.TotalItems) * 100
}
