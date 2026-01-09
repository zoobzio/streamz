package testing

import (
	"errors"
	"testing"
	"time"

	streamz "github.com/zoobzio/streamz"
)

func TestCollectResultsWithTimeout(t *testing.T) {
	t.Run("collects all results before channel close", func(t *testing.T) {
		ch := make(chan streamz.Result[int], 3)
		ch <- streamz.NewSuccess(1)
		ch <- streamz.NewSuccess(2)
		ch <- streamz.NewSuccess(3)
		close(ch)

		results := CollectResultsWithTimeout(t, ch, 100*time.Millisecond)

		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("returns on timeout", func(t *testing.T) {
		ch := make(chan streamz.Result[int])
		// Channel never sends or closes

		results := CollectResultsWithTimeout(t, ch, 50*time.Millisecond)

		if len(results) != 0 {
			t.Errorf("expected 0 results on timeout, got %d", len(results))
		}
	})

	t.Run("collects mixed success and error results", func(t *testing.T) {
		ch := make(chan streamz.Result[string], 3)
		ch <- streamz.NewSuccess("ok")
		ch <- streamz.NewError("bad", errors.New("test error"), "test")
		ch <- streamz.NewSuccess("also ok")
		close(ch)

		results := CollectResultsWithTimeout(t, ch, 100*time.Millisecond)

		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}

		successCount := 0
		errorCount := 0
		for _, r := range results {
			if r.IsSuccess() {
				successCount++
			} else {
				errorCount++
			}
		}

		if successCount != 2 {
			t.Errorf("expected 2 successes, got %d", successCount)
		}
		if errorCount != 1 {
			t.Errorf("expected 1 error, got %d", errorCount)
		}
	})
}

func TestCollectValues(t *testing.T) {
	t.Run("collects only successful values", func(t *testing.T) {
		ch := make(chan streamz.Result[int], 4)
		ch <- streamz.NewSuccess(1)
		ch <- streamz.NewError(0, errors.New("error"), "test")
		ch <- streamz.NewSuccess(2)
		ch <- streamz.NewSuccess(3)
		close(ch)

		values := CollectValues(t, ch, 100*time.Millisecond)

		if len(values) != 3 {
			t.Errorf("expected 3 values, got %d", len(values))
		}

		expected := []int{1, 2, 3}
		for i, v := range values {
			if v != expected[i] {
				t.Errorf("value %d: expected %d, got %d", i, expected[i], v)
			}
		}
	})

	t.Run("returns empty slice when all errors", func(t *testing.T) {
		ch := make(chan streamz.Result[int], 2)
		ch <- streamz.NewError(0, errors.New("error1"), "test")
		ch <- streamz.NewError(0, errors.New("error2"), "test")
		close(ch)

		values := CollectValues(t, ch, 100*time.Millisecond)

		if len(values) != 0 {
			t.Errorf("expected 0 values, got %d", len(values))
		}
	})
}

func TestCollectErrors(t *testing.T) {
	t.Run("collects only errors", func(t *testing.T) {
		ch := make(chan streamz.Result[int], 4)
		ch <- streamz.NewSuccess(1)
		ch <- streamz.NewError(0, errors.New("error1"), "test")
		ch <- streamz.NewSuccess(2)
		ch <- streamz.NewError(0, errors.New("error2"), "test")
		close(ch)

		errs := CollectErrors(t, ch, 100*time.Millisecond)

		if len(errs) != 2 {
			t.Errorf("expected 2 errors, got %d", len(errs))
		}
	})

	t.Run("returns empty slice when all successes", func(t *testing.T) {
		ch := make(chan streamz.Result[int], 2)
		ch <- streamz.NewSuccess(1)
		ch <- streamz.NewSuccess(2)
		close(ch)

		errs := CollectErrors(t, ch, 100*time.Millisecond)

		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d", len(errs))
		}
	})
}

func TestSendValues(t *testing.T) {
	t.Run("sends values as success results", func(t *testing.T) {
		values := []string{"a", "b", "c"}
		ch := SendValues(t, values)

		collected := CollectResultsWithTimeout(t, ch, 100*time.Millisecond)

		if len(collected) != 3 {
			t.Errorf("expected 3 results, got %d", len(collected))
		}

		for i, r := range collected {
			if !r.IsSuccess() {
				t.Errorf("result %d: expected success", i)
				continue
			}
			if r.Value() != values[i] {
				t.Errorf("result %d: expected %q, got %q", i, values[i], r.Value())
			}
		}
	})

	t.Run("returns closed channel", func(t *testing.T) {
		ch := SendValues(t, []int{1})

		// Drain the channel
		<-ch

		// Should be closed
		_, ok := <-ch
		if ok {
			t.Error("expected channel to be closed")
		}
	})
}

func TestAssertResultCount(t *testing.T) {
	t.Run("passes when count matches", func(t *testing.T) {
		mockT := &testing.T{}
		results := []streamz.Result[int]{
			streamz.NewSuccess(1),
			streamz.NewSuccess(2),
		}

		AssertResultCount(mockT, results, 2)

		if mockT.Failed() {
			t.Error("expected test to pass")
		}
	})
}

func TestAssertAllSuccess(t *testing.T) {
	t.Run("passes when all success", func(t *testing.T) {
		mockT := &testing.T{}
		results := []streamz.Result[int]{
			streamz.NewSuccess(1),
			streamz.NewSuccess(2),
		}

		AssertAllSuccess(mockT, results)

		if mockT.Failed() {
			t.Error("expected test to pass")
		}
	})
}

func TestAssertAllErrors(t *testing.T) {
	t.Run("passes when all errors", func(t *testing.T) {
		mockT := &testing.T{}
		results := []streamz.Result[int]{
			streamz.NewError(0, errors.New("e1"), "test"),
			streamz.NewError(0, errors.New("e2"), "test"),
		}

		AssertAllErrors(mockT, results)

		if mockT.Failed() {
			t.Error("expected test to pass")
		}
	})
}
