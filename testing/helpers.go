package testing

import (
	"time"

	streamz "github.com/zoobzio/streamz"
)

// CollectResultsWithTimeout collects all results from a channel with a timeout.
// This is a shared utility function for integration tests to avoid duplication.
func CollectResultsWithTimeout[T any](ch <-chan streamz.Result[T], timeout time.Duration) []streamz.Result[T] {
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
