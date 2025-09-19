package streamz

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewStreamError(t *testing.T) {
	item := "test-item"
	err := errors.New("test error")
	processorName := "test-processor"

	before := time.Now()
	streamErr := NewStreamError(item, err, processorName)
	after := time.Now()

	// Verify all fields are set correctly
	if streamErr.Item != item {
		t.Errorf("Expected Item to be %q, got %q", item, streamErr.Item)
	}

	if !errors.Is(streamErr.Err, err) {
		t.Errorf("Expected Err to be %v, got %v", err, streamErr.Err)
	}

	if streamErr.ProcessorName != processorName {
		t.Errorf("Expected ProcessorName to be %q, got %q", processorName, streamErr.ProcessorName)
	}

	// Verify timestamp is reasonable (between before and after)
	if streamErr.Timestamp.Before(before) || streamErr.Timestamp.After(after) {
		t.Errorf("Expected Timestamp to be between %v and %v, got %v", before, after, streamErr.Timestamp)
	}
}

func TestStreamError_Error(t *testing.T) {
	item := 42
	err := errors.New("division by zero")
	processorName := "divider"
	timestamp := time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)

	streamErr := &StreamError[int]{
		Item:          item,
		Err:           err,
		ProcessorName: processorName,
		Timestamp:     timestamp,
	}

	result := streamErr.Error()
	expected := "StreamError[divider]: division by zero (item: 42, time: 2023-12-25T10:30:00Z)"

	if result != expected {
		t.Errorf("Expected Error() to return %q, got %q", expected, result)
	}
}

func TestStreamError_String(t *testing.T) {
	item := "hello"
	err := errors.New("encoding failed")
	processorName := "encoder"
	timestamp := time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)

	streamErr := &StreamError[string]{
		Item:          item,
		Err:           err,
		ProcessorName: processorName,
		Timestamp:     timestamp,
	}

	result := streamErr.String()
	expected := "StreamError[encoder]: encoding failed (item: hello, time: 2023-12-25T10:30:00Z)"

	if result != expected {
		t.Errorf("Expected String() to return %q, got %q", expected, result)
	}
}

func TestStreamError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	item := "test"

	streamErr := NewStreamError(item, originalErr, "test-processor")

	unwrapped := streamErr.Unwrap()
	if !errors.Is(unwrapped, originalErr) {
		t.Errorf("Expected Unwrap() to return %v, got %v", originalErr, unwrapped)
	}
}

func TestStreamError_UnwrapWithNilError(t *testing.T) {
	item := "test"

	streamErr := &StreamError[string]{
		Item:          item,
		Err:           nil,
		ProcessorName: "test-processor",
		Timestamp:     time.Now(),
	}

	unwrapped := streamErr.Unwrap()
	if unwrapped != nil {
		t.Errorf("Expected Unwrap() to return nil, got %v", unwrapped)
	}
}

func TestStreamError_WithDifferentTypes(t *testing.T) {
	tests := []struct {
		name string
		item interface{}
	}{
		{"int", 123},
		{"string", "test-string"},
		{"struct", struct{ Name string }{Name: "test"}},
		{"slice", []int{1, 2, 3}},
		{"map", map[string]int{"key": 42}},
		{"nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New("test error")

			// This tests that StreamError works with any type T
			switch v := tt.item.(type) {
			case int:
				streamErr := NewStreamError(v, err, "test-processor")
				if streamErr.Item != v {
					t.Errorf("Expected Item to be %v, got %v", v, streamErr.Item)
				}
			case string:
				streamErr := NewStreamError(v, err, "test-processor")
				if streamErr.Item != v {
					t.Errorf("Expected Item to be %v, got %v", v, streamErr.Item)
				}
			default:
				// For complex types, just verify the constructor works
				streamErr := NewStreamError(v, err, "test-processor")
				if !errors.Is(streamErr.Err, err) {
					t.Errorf("Expected Err to be %v, got %v", err, streamErr.Err)
				}
			}
		})
	}
}

func TestStreamError_StringFormatsComplexTypes(t *testing.T) {
	complexItem := map[string][]int{
		"numbers": {1, 2, 3},
		"more":    {4, 5, 6},
	}
	err := errors.New("processing failed")
	processorName := "complex-processor"

	streamErr := NewStreamError(complexItem, err, processorName)
	result := streamErr.String()

	// Verify the string contains key components
	if !strings.Contains(result, processorName) {
		t.Errorf("Expected string to contain processor name %q", processorName)
	}
	if !strings.Contains(result, "processing failed") {
		t.Errorf("Expected string to contain error message")
	}
	if !strings.Contains(result, "numbers") {
		t.Errorf("Expected string to contain item representation")
	}
}

func TestStreamError_ErrorInterfaceImplementation(t *testing.T) {
	streamErr := NewStreamError("item", errors.New("test"), "processor")

	// Verify it implements the error interface
	var err error = streamErr
	if err.Error() != streamErr.Error() {
		t.Errorf("Error interface implementation doesn't match Error() method")
	}
}
