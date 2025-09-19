package streamz

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func TestNewSuccess(t *testing.T) {
	value := 42
	result := NewSuccess(value)

	if result.IsError() {
		t.Error("Expected NewSuccess to create successful Result")
	}

	if !result.IsSuccess() {
		t.Error("Expected NewSuccess to create successful Result")
	}

	if result.Value() != value {
		t.Errorf("Expected Value() to return %d, got %d", value, result.Value())
	}

	if result.Error() != nil {
		t.Error("Expected Error() to return nil for successful Result")
	}
}

func TestNewError(t *testing.T) {
	item := "failed-item"
	err := errors.New("test error")
	processorName := "test-processor"

	result := NewError(item, err, processorName)

	if !result.IsError() {
		t.Error("Expected NewError to create error Result")
	}

	if result.IsSuccess() {
		t.Error("Expected NewError to create error Result")
	}

	streamErr := result.Error()
	if streamErr == nil {
		t.Fatal("Expected Error() to return StreamError")
	}

	if streamErr.Item != item {
		t.Errorf("Expected Item to be %q, got %q", item, streamErr.Item)
	}

	if !errors.Is(streamErr.Err, err) {
		t.Errorf("Expected Err to be %v, got %v", err, streamErr.Err)
	}

	if streamErr.ProcessorName != processorName {
		t.Errorf("Expected ProcessorName to be %q, got %q", processorName, streamErr.ProcessorName)
	}
}

func TestResult_ValuePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected Value() to panic on error Result")
		}
	}()

	result := NewError("item", errors.New("test"), "processor")
	result.Value() // Should panic
}

func TestResult_ValueOr(t *testing.T) {
	// Test successful Result
	successResult := NewSuccess(42)
	if successResult.ValueOr(999) != 42 {
		t.Error("Expected ValueOr to return actual value for success Result")
	}

	// Test error Result
	errorResult := NewError(42, errors.New("test"), "processor")
	if errorResult.ValueOr(999) != 999 {
		t.Error("Expected ValueOr to return fallback value for error Result")
	}
}

func TestResult_Map(t *testing.T) {
	// Test mapping successful Result
	successResult := NewSuccess(10)
	mappedResult := successResult.Map(func(x int) int { return x * 2 })

	if mappedResult.IsError() {
		t.Error("Expected mapped successful Result to remain successful")
	}

	if mappedResult.Value() != 20 {
		t.Errorf("Expected mapped value to be 20, got %d", mappedResult.Value())
	}

	// Test mapping error Result (should propagate error unchanged)
	originalErr := errors.New("original error")
	errorResult := NewError(10, originalErr, "test-processor")
	mappedErrorResult := errorResult.Map(func(x int) int { return x * 2 })

	if !mappedErrorResult.IsError() {
		t.Error("Expected mapped error Result to remain error")
	}

	if !errors.Is(mappedErrorResult.Error().Err, originalErr) {
		t.Error("Expected error to be propagated unchanged")
	}

	if mappedErrorResult.Error().Item != 10 {
		t.Error("Expected error item to be propagated unchanged")
	}
}

func TestResult_MapError(t *testing.T) {
	// Test mapping error Result
	originalErr := errors.New("original error")
	errorResult := NewError("item", originalErr, "processor")

	mappedResult := errorResult.MapError(func(se *StreamError[string]) *StreamError[string] {
		return NewStreamError(se.Item, errors.New("mapped error"), "mapped-processor")
	})

	if !mappedResult.IsError() {
		t.Error("Expected mapped error Result to remain error")
	}

	mappedStreamErr := mappedResult.Error()
	if mappedStreamErr.Err.Error() != "mapped error" {
		t.Errorf("Expected mapped error message to be 'mapped error', got %q", mappedStreamErr.Err.Error())
	}

	if mappedStreamErr.ProcessorName != "mapped-processor" {
		t.Errorf("Expected mapped processor name to be 'mapped-processor', got %q", mappedStreamErr.ProcessorName)
	}

	// Test mapping successful Result (should propagate success unchanged)
	successResult := NewSuccess("success-value")
	mappedSuccessResult := successResult.MapError(func(_ *StreamError[string]) *StreamError[string] {
		return NewStreamError("should not be called", errors.New("should not happen"), "should-not-execute")
	})

	if mappedSuccessResult.IsError() {
		t.Error("Expected mapped successful Result to remain successful")
	}

	if mappedSuccessResult.Value() != "success-value" {
		t.Error("Expected success value to be propagated unchanged")
	}
}

func TestResult_TypeSafety(t *testing.T) {
	// Test that Result[T] maintains type safety for different types

	// String Result
	stringResult := NewSuccess("hello")
	if stringResult.Value() != "hello" {
		t.Errorf("Expected string value 'hello', got %q", stringResult.Value())
	}

	// Int Result
	intResult := NewSuccess(42)
	if intResult.Value() != 42 {
		t.Errorf("Expected int value 42, got %d", intResult.Value())
	}

	// Struct Result
	type TestStruct struct {
		Name string
		ID   int
	}

	structValue := TestStruct{Name: "test", ID: 123}
	structResult := NewSuccess(structValue)
	if structResult.Value().Name != "test" || structResult.Value().ID != 123 {
		t.Errorf("Expected struct value %+v, got %+v", structValue, structResult.Value())
	}

	// Error with struct
	structErrorResult := NewError(structValue, errors.New("struct error"), "test")
	if structErrorResult.Error().Item.Name != "test" {
		t.Error("Expected struct error to preserve item type safety")
	}
}

func TestResult_ChainedTransformations(t *testing.T) {
	// Test that Map operations can be chained
	result := NewSuccess(5)

	chained := result.
		Map(func(x int) int { return x * 2 }). // 10
		Map(func(x int) int { return x + 1 }). // 11
		Map(func(x int) int { return x * 3 })  // 33

	if chained.IsError() {
		t.Error("Expected chained operations on success to remain successful")
	}

	if chained.Value() != 33 {
		t.Errorf("Expected chained result to be 33, got %d", chained.Value())
	}

	// Test that error breaks the chain
	errorResult := NewError(5, errors.New("chain breaker"), "test")

	chainedWithError := errorResult.
		Map(func(x int) int { return x * 100 }). // Should not execute
		Map(func(x int) int { return x + 999 })  // Should not execute

	if !chainedWithError.IsError() {
		t.Error("Expected chained operations on error to remain error")
	}

	if chainedWithError.Error().Err.Error() != "chain breaker" {
		t.Error("Expected original error to be preserved through chain")
	}
}

func TestResult_ZeroValue(t *testing.T) {
	// Test zero value behavior
	var result Result[int]

	// Zero value should be a successful Result with zero value of T
	if result.IsError() {
		t.Error("Expected zero value Result to be successful")
	}

	if !result.IsSuccess() {
		t.Error("Expected zero value Result to be successful")
	}

	if result.Value() != 0 {
		t.Errorf("Expected zero value Result to contain zero value of T (0), got %d", result.Value())
	}

	if result.Error() != nil {
		t.Error("Expected zero value Result to have nil error")
	}
}

func TestResult_ErrorTimestamp(t *testing.T) {
	// Test that error Results preserve timestamp information
	before := time.Now()
	result := NewError("test", errors.New("test error"), "test-processor")
	after := time.Now()

	if !result.IsError() {
		t.Fatal("Expected error Result")
	}

	timestamp := result.Error().Timestamp
	if timestamp.Before(before) || timestamp.After(after) {
		t.Error("Expected timestamp to be set correctly when creating error Result")
	}
}

func TestResult_ErrorUnwrapping(t *testing.T) {
	// Test that StreamError supports Go 1.13+ error unwrapping
	originalErr := errors.New("root cause")
	result := NewError("item", originalErr, "processor")

	streamErr := result.Error()

	// Test that Unwrap works
	if !errors.Is(errors.Unwrap(streamErr), originalErr) {
		t.Error("Expected StreamError to support error unwrapping")
	}

	// Test that errors.Is works
	if !errors.Is(streamErr, originalErr) {
		t.Error("Expected errors.Is to work with StreamError")
	}
}

func TestResult_NilErrorSafety(t *testing.T) {
	// Test behavior when nil is passed as error (should not panic)
	result := NewError("item", nil, "processor")

	if !result.IsError() {
		t.Error("Expected Result with nil error to still be considered error Result")
	}

	streamErr := result.Error()
	if streamErr == nil {
		t.Fatal("Expected StreamError to be created even with nil underlying error")
	}

	if streamErr.Err != nil {
		t.Error("Expected underlying error to remain nil")
	}
}

// ===========================================
// METADATA TESTS - PHASE 1 IMPLEMENTATION
// ===========================================

func TestWithMetadata_Basic(t *testing.T) {
	result := NewSuccess(42)

	// Test adding first metadata
	withMeta := result.WithMetadata("key1", "value1")
	if !withMeta.HasMetadata() {
		t.Error("Expected Result to have metadata after WithMetadata")
	}

	value, exists := withMeta.GetMetadata("key1")
	if !exists {
		t.Error("Expected metadata key1 to exist")
	}
	if value != "value1" {
		t.Errorf("Expected metadata value 'value1', got %v", value)
	}

	// Test chaining metadata
	withMore := withMeta.WithMetadata("key2", 123)
	if len(withMore.MetadataKeys()) != 2 {
		t.Errorf("Expected 2 metadata keys, got %d", len(withMore.MetadataKeys()))
	}

	// Verify original result unchanged
	if result.HasMetadata() {
		t.Error("Expected original Result to remain unchanged")
	}
}

func TestWithMetadata_EmptyKey(t *testing.T) {
	result := NewSuccess(42).WithMetadata("valid", "value")

	// Empty key should be ignored
	unchanged := result.WithMetadata("", "should be ignored")

	if len(unchanged.MetadataKeys()) != 1 {
		t.Error("Expected empty key to be ignored")
	}

	// Should return identical Result
	if len(result.MetadataKeys()) != len(unchanged.MetadataKeys()) {
		t.Error("Expected Result to be unchanged with empty key")
	}
}

func TestGetMetadata_NonExistent(t *testing.T) {
	result := NewSuccess(42)

	// Test missing key on Result without metadata
	value, exists := result.GetMetadata("nonexistent")
	if exists {
		t.Error("Expected nonexistent key to return false")
	}
	if value != nil {
		t.Error("Expected nonexistent key to return nil value")
	}

	// Test missing key on Result with metadata
	withMeta := result.WithMetadata("exists", "value")
	value, exists = withMeta.GetMetadata("nonexistent")
	if exists {
		t.Error("Expected nonexistent key to return false")
	}
	if value != nil {
		t.Error("Expected nonexistent key to return nil value")
	}
}

func TestHasMetadata(t *testing.T) {
	result := NewSuccess(42)

	// No metadata initially
	if result.HasMetadata() {
		t.Error("Expected new Result to have no metadata")
	}

	// After adding metadata
	withMeta := result.WithMetadata("key", "value")
	if !withMeta.HasMetadata() {
		t.Error("Expected Result with metadata to return true")
	}

	// Empty metadata map edge case
	// (This can't happen with current implementation, but testing robustness)
}

func TestMetadataKeys(t *testing.T) {
	result := NewSuccess(42)

	// No metadata
	keys := result.MetadataKeys()
	if len(keys) != 0 {
		t.Errorf("Expected empty keys slice, got %v", keys)
	}

	// Single metadata entry
	withOne := result.WithMetadata("key1", "value1")
	keys = withOne.MetadataKeys()
	if len(keys) != 1 {
		t.Errorf("Expected 1 key, got %d", len(keys))
	}
	if keys[0] != "key1" {
		t.Errorf("Expected key 'key1', got %q", keys[0])
	}

	// Multiple metadata entries
	withTwo := withOne.WithMetadata("key2", "value2")
	keys = withTwo.MetadataKeys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}

	// Check both keys present (order not guaranteed)
	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}
	if !keyMap["key1"] || !keyMap["key2"] {
		t.Errorf("Expected keys 'key1' and 'key2', got %v", keys)
	}
}

func TestTypedAccessors_String(t *testing.T) {
	result := NewSuccess(42).WithMetadata("string_key", "test_value").WithMetadata("int_key", 123)

	// Successful string retrieval
	value, found, err := result.GetStringMetadata("string_key")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !found {
		t.Error("Expected key to be found")
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got %q", value)
	}

	// Missing key
	_, found, err = result.GetStringMetadata("missing_key")
	if err != nil {
		t.Errorf("Expected no error for missing key, got %v", err)
	}
	if found {
		t.Error("Expected missing key to return found=false")
	}

	// Wrong type
	_, found, err = result.GetStringMetadata("int_key")
	if err == nil {
		t.Error("Expected error for wrong type")
	}
	if found {
		t.Error("Expected wrong type to return found=false")
	}
	if err.Error() != `metadata key "int_key" has type int, expected string` {
		t.Errorf("Expected specific error message, got %v", err)
	}
}

func TestTypedAccessors_Time(t *testing.T) {
	now := time.Now()
	result := NewSuccess(42).WithMetadata("time_key", now).WithMetadata("string_key", "not_time")

	// Successful time retrieval
	value, found, err := result.GetTimeMetadata("time_key")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !found {
		t.Error("Expected key to be found")
	}
	if !value.Equal(now) {
		t.Errorf("Expected %v, got %v", now, value)
	}

	// Wrong type
	_, found, err = result.GetTimeMetadata("string_key")
	if err == nil {
		t.Error("Expected error for wrong type")
	}
	if found {
		t.Error("Expected wrong type to return found=false")
	}
}

func TestTypedAccessors_Int(t *testing.T) {
	result := NewSuccess(42).WithMetadata("int_key", 123).WithMetadata("string_key", "not_int")

	// Successful int retrieval
	value, found, err := result.GetIntMetadata("int_key")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !found {
		t.Error("Expected key to be found")
	}
	if value != 123 {
		t.Errorf("Expected 123, got %d", value)
	}

	// Wrong type
	_, found, err = result.GetIntMetadata("string_key")
	if err == nil {
		t.Error("Expected error for wrong type")
	}
	if found {
		t.Error("Expected wrong type to return found=false")
	}
}

func TestStandardMetadataKeys(t *testing.T) {
	now := time.Now()
	result := NewSuccess(42).
		WithMetadata(MetadataWindowStart, now).
		WithMetadata(MetadataSource, "api").
		WithMetadata(MetadataRetryCount, 3)

	// Test standard key access
	start, found, err := result.GetTimeMetadata(MetadataWindowStart)
	if err != nil || !found {
		t.Errorf("Expected to find window start, got found=%v, err=%v", found, err)
	}
	if !start.Equal(now) {
		t.Error("Expected window start time to match")
	}

	source, found, err := result.GetStringMetadata(MetadataSource)
	if err != nil || !found {
		t.Errorf("Expected to find source, got found=%v, err=%v", found, err)
	}
	if source != "api" {
		t.Errorf("Expected source 'api', got %q", source)
	}

	retries, found, err := result.GetIntMetadata(MetadataRetryCount)
	if err != nil || !found {
		t.Errorf("Expected to find retry count, got found=%v, err=%v", found, err)
	}
	if retries != 3 {
		t.Errorf("Expected retry count 3, got %d", retries)
	}
}

func TestMap_PreservesMetadata(t *testing.T) {
	original := NewSuccess(10).
		WithMetadata("source", "api").
		WithMetadata("timestamp", time.Now())

	mapped := original.Map(func(x int) int { return x * 2 })

	if !mapped.IsSuccess() {
		t.Error("Expected mapped Result to be successful")
	}
	if mapped.Value() != 20 {
		t.Errorf("Expected mapped value 20, got %d", mapped.Value())
	}

	// Verify metadata preserved
	source, found, err := mapped.GetStringMetadata("source")
	if err != nil || !found {
		t.Errorf("Expected to find source metadata, got found=%v, err=%v", found, err)
	}
	if source != "api" {
		t.Errorf("Expected source 'api', got %q", source)
	}

	_, found, err = mapped.GetTimeMetadata("timestamp")
	if err != nil || !found {
		t.Errorf("Expected to find timestamp metadata, got found=%v, err=%v", found, err)
	}
}

func TestMapError_PreservesMetadata(t *testing.T) {
	original := NewError(42, errors.New("test error"), "processor").
		WithMetadata("context", "validation").
		WithMetadata("retry_count", 3)

	mapped := original.MapError(func(se *StreamError[int]) *StreamError[int] {
		return NewStreamError(se.Item,
			fmt.Errorf("wrapped: %w", se.Err),
			se.ProcessorName)
	})

	if !mapped.IsError() {
		t.Error("Expected mapped Result to be error")
	}
	if !strings.Contains(fmt.Sprintf("%v", mapped.Error().Err), "wrapped:") {
		t.Error("Expected error to be wrapped")
	}

	// Verify metadata preserved through error transformation
	context, found, err := mapped.GetStringMetadata("context")
	if err != nil || !found {
		t.Errorf("Expected to find context metadata, got found=%v, err=%v", found, err)
	}
	if context != "validation" {
		t.Errorf("Expected context 'validation', got %q", context)
	}

	retries, found, err := mapped.GetIntMetadata("retry_count")
	if err != nil || !found {
		t.Errorf("Expected to find retry_count metadata, got found=%v, err=%v", found, err)
	}
	if retries != 3 {
		t.Errorf("Expected retry count 3, got %d", retries)
	}
}

func TestWithMetadata_ConcurrentAccess(t *testing.T) {
	base := NewSuccess(42).WithMetadata("base", "value")

	// Test concurrent WithMetadata calls
	var wg sync.WaitGroup
	results := make([]Result[int], 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			results[id] = base.WithMetadata(fmt.Sprintf("key_%d", id), id)
		}(i)
	}
	wg.Wait()

	// Verify all Results are valid and distinct
	for i, result := range results {
		if !result.IsSuccess() {
			t.Errorf("Result %d should be successful", i)
		}
		if result.Value() != 42 {
			t.Errorf("Result %d should have value 42, got %d", i, result.Value())
		}

		// Should have base metadata plus new key
		if !result.HasMetadata() {
			t.Errorf("Result %d should have metadata", i)
		}
		keys := result.MetadataKeys()
		if len(keys) != 2 {
			t.Errorf("Result %d should have 2 metadata keys, got %d", i, len(keys))
		}

		value, found, err := result.GetIntMetadata(fmt.Sprintf("key_%d", i))
		if err != nil {
			t.Errorf("Result %d metadata access error: %v", i, err)
		}
		if !found {
			t.Errorf("Result %d should have key_%d", i, i)
		}
		if value != i {
			t.Errorf("Result %d should have value %d, got %d", i, i, value)
		}
	}
}

func TestGetMetadata_ConcurrentRead(t *testing.T) {
	result := NewSuccess(42).
		WithMetadata("key1", "value1").
		WithMetadata("key2", 100).
		WithMetadata("key3", time.Now())

	// Multiple goroutines reading metadata concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 300)

	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if val, ok := result.GetMetadata("key1"); !ok || val != "value1" {
				errors <- fmt.Errorf("key1 read failed")
			}
		}()
		go func() {
			defer wg.Done()
			if val, ok := result.GetMetadata("key2"); !ok || val != 100 {
				errors <- fmt.Errorf("key2 read failed")
			}
		}()
		go func() {
			defer wg.Done()
			if _, ok := result.GetMetadata("key3"); !ok {
				errors <- fmt.Errorf("key3 read failed")
			}
		}()
	}
	wg.Wait()
	close(errors)

	// Verify no race conditions occurred
	for err := range errors {
		t.Error(err)
	}
}

func TestMetadata_MemoryOverhead(t *testing.T) {
	// Verify nil metadata has minimal overhead
	//nolint:staticcheck,wastedassign // Variables are used in later assertions
	withoutMeta := NewSuccess(42)
	//nolint:staticcheck,wastedassign // Variables are used in later assertions
	withMeta := NewSuccess(42).WithMetadata("key", "value")

	// Basic size verification
	baselineSize := unsafe.Sizeof(withoutMeta)
	metaSize := unsafe.Sizeof(withMeta)

	// Should be same size - metadata is pointer
	if baselineSize != metaSize {
		t.Errorf("Expected same struct size, got baseline=%d, meta=%d", baselineSize, metaSize)
	}

	// Memory allocation test
	var m1, m2 runtime.MemStats

	// Baseline without metadata
	runtime.GC()
	runtime.ReadMemStats(&m1)

	results := make([]Result[int], 1000)
	for i := range results {
		results[i] = NewSuccess(i)
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)
	baselineAlloc := m2.Alloc - m1.Alloc

	// With metadata
	runtime.GC()
	runtime.ReadMemStats(&m1)

	metaResults := make([]Result[int], 1000)
	for i := range metaResults {
		metaResults[i] = NewSuccess(i).WithMetadata("index", i)
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)
	metaAlloc := m2.Alloc - m1.Alloc

	// Metadata should add reasonable overhead (not excessive)
	overhead := float64(metaAlloc) / float64(baselineAlloc)
	if overhead >= 3.0 {
		t.Errorf("Metadata overhead too high: %.2fx baseline (limit: 3.0x)", overhead)
	}

	// Keep references to prevent GC optimization
	if results == nil || metaResults == nil {
		t.Error("Results should not be nil")
	}
}

func BenchmarkMetadata_Operations(b *testing.B) {
	result := NewSuccess(42).WithMetadata("key", "value")

	b.Run("GetMetadata", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = result.GetMetadata("key")
		}
	})

	b.Run("WithMetadata_New", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = result.WithMetadata("new_key", i)
		}
	})

	b.Run("WithMetadata_Chain", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewSuccess(i).
				WithMetadata("key1", "value1").
				WithMetadata("key2", "value2").
				WithMetadata("key3", "value3")
		}
	})

	b.Run("TypedAccess", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			//nolint:errcheck // Benchmark doesn't need to check errors
			_, _, _ = result.GetStringMetadata("key")
		}
	})
}
