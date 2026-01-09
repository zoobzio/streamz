// Package testing provides test utilities for streamz.
package testing

import (
	"testing"
	"time"

	streamz "github.com/zoobzio/streamz"
)

// CollectResultsWithTimeout collects all results from a channel with a timeout.
// This is a shared utility function for integration tests to avoid duplication.
func CollectResultsWithTimeout[T any](t *testing.T, ch <-chan streamz.Result[T], timeout time.Duration) []streamz.Result[T] {
	t.Helper()

	var results []streamz.Result[T]
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case result, ok := <-ch:
			if !ok {
				return results
			}
			results = append(results, result)
		case <-timer.C:
			return results
		}
	}
}

// CollectValues collects all successful values from a Result channel with a timeout.
// Returns only the values, ignoring errors.
func CollectValues[T any](t *testing.T, ch <-chan streamz.Result[T], timeout time.Duration) []T {
	t.Helper()

	results := CollectResultsWithTimeout(t, ch, timeout)
	values := make([]T, 0, len(results))
	for _, r := range results {
		if r.IsSuccess() {
			values = append(values, r.Value())
		}
	}
	return values
}

// CollectErrors collects all errors from a Result channel with a timeout.
// Returns only the errors, ignoring successes.
func CollectErrors[T any](t *testing.T, ch <-chan streamz.Result[T], timeout time.Duration) []error {
	t.Helper()

	results := CollectResultsWithTimeout(t, ch, timeout)
	errs := make([]error, 0)
	for _, r := range results {
		if r.IsError() {
			errs = append(errs, r.Error())
		}
	}
	return errs
}

// SendValues sends a slice of values to a channel as successful Results.
// Closes the channel after all values are sent.
func SendValues[T any](t *testing.T, values []T) <-chan streamz.Result[T] {
	t.Helper()

	ch := make(chan streamz.Result[T], len(values))
	for _, v := range values {
		ch <- streamz.NewSuccess(v)
	}
	close(ch)
	return ch
}

// AssertResultCount verifies the expected number of results were received.
func AssertResultCount[T any](t *testing.T, results []streamz.Result[T], expected int) {
	t.Helper()

	if len(results) != expected {
		t.Errorf("expected %d results, got %d", expected, len(results))
	}
}

// AssertAllSuccess verifies all results are successful.
func AssertAllSuccess[T any](t *testing.T, results []streamz.Result[T]) {
	t.Helper()

	for i, r := range results {
		if r.IsError() {
			t.Errorf("result %d: expected success, got error: %v", i, r.Error())
		}
	}
}

// AssertAllErrors verifies all results are errors.
func AssertAllErrors[T any](t *testing.T, results []streamz.Result[T]) {
	t.Helper()

	for i, r := range results {
		if r.IsSuccess() {
			t.Errorf("result %d: expected error, got success with value: %v", i, r.Value())
		}
	}
}
