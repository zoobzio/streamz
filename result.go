package streamz

import (
	"context"
	"fmt"
	"time"
)

// Result represents either a successful value or an error in stream processing.
// This is a proof of concept for unified error handling that eliminates dual-channel patterns.
// It follows the Result type pattern common in functional programming languages.
// Metadata support added to carry context through stream processing pipelines.
type Result[T any] struct {
	value    T
	err      *StreamError[T]
	metadata map[string]interface{} // nil by default for zero overhead
}

// NewSuccess creates a Result containing a successful value.
func NewSuccess[T any](value T) Result[T] {
	return Result[T]{value: value}
}

// NewError creates a Result containing an error.
func NewError[T any](item T, err error, processorName string) Result[T] {
	return Result[T]{err: NewStreamError(item, err, processorName)}
}

// IsError returns true if this Result contains an error.
func (r Result[T]) IsError() bool {
	return r.err != nil
}

// IsSuccess returns true if this Result contains a successful value.
func (r Result[T]) IsSuccess() bool {
	return r.err == nil
}

// Value returns the successful value.
// Panics if called on a Result containing an error - always check IsSuccess() first.
func (r Result[T]) Value() T {
	if r.err != nil {
		panic("called Value() on Result containing an error")
	}
	return r.value
}

// Error returns the StreamError.
// Returns nil if this Result contains a successful value.
func (r Result[T]) Error() *StreamError[T] {
	return r.err
}

// ValueOr returns the successful value if present, otherwise returns the fallback.
func (r Result[T]) ValueOr(fallback T) T {
	if r.err != nil {
		return fallback
	}
	return r.value
}

// Map applies a function to the value if this Result is successful.
// If this Result contains an error, returns the error unchanged.
// Metadata is preserved through successful transformations.
func (r Result[T]) Map(fn func(T) T) Result[T] {
	if r.err != nil {
		return r // Propagate error with metadata
	}

	result := NewSuccess(fn(r.value))
	if r.metadata != nil {
		// Preserve metadata through transformation
		result.metadata = r.metadata
	}
	return result
}

// MapError applies a function to transform the error if this Result contains an error.
// If this Result is successful, returns the success value unchanged.
// Metadata is preserved through error transformations.
func (r Result[T]) MapError(fn func(*StreamError[T]) *StreamError[T]) Result[T] {
	if r.err == nil {
		return r // Propagate success with metadata
	}

	result := Result[T]{
		value: r.value,
		err:   fn(r.err),
	}

	if r.metadata != nil {
		// Preserve metadata through error transformation
		result.metadata = r.metadata
	}
	return result
}

// Standard metadata keys for common use cases.
const (
	MetadataWindowStart = "window_start" // time.Time - window start time
	MetadataWindowEnd   = "window_end"   // time.Time - window end time
	MetadataWindowType  = "window_type"  // string - "tumbling", "sliding", "session"
	MetadataWindowSize  = "window_size"  // time.Duration - window duration
	MetadataWindowSlide = "window_slide" // time.Duration - slide interval (sliding only)
	MetadataWindowGap   = "window_gap"   // time.Duration - activity gap (session only)
	MetadataSessionKey  = "session_key"  // string - session identifier (session only)
	MetadataSource      = "source"       // string - data source identifier
	MetadataTimestamp   = "timestamp"    // time.Time - processing timestamp
	MetadataProcessor   = "processor"    // string - processor that added metadata
	MetadataRetryCount  = "retry_count"  // int - number of retries attempted
	MetadataSessionID   = "session_id"   // string - session identifier
)

// WithMetadata returns a new Result with the specified metadata key-value pair.
// This is a thread-safe immutable operation - the original Result is unchanged.
// Multiple calls can be chained to add multiple metadata entries.
// Returns error for empty keys to prevent silent failures.
func (r Result[T]) WithMetadata(key string, value interface{}) Result[T] {
	if key == "" {
		return r // Ignore empty keys (documented behavior)
	}

	var newMetadata map[string]interface{}
	if r.metadata == nil {
		// Optimize for first metadata entry
		newMetadata = map[string]interface{}{key: value}
	} else {
		// Pre-size map for optimal allocation
		newMetadata = make(map[string]interface{}, len(r.metadata)+1)

		// Safe copy - metadata is immutable after Result creation
		// No concurrent modification possible on immutable Result
		for k, v := range r.metadata {
			newMetadata[k] = v
		}
		newMetadata[key] = value
	}

	return Result[T]{
		value:    r.value,
		err:      r.err,
		metadata: newMetadata,
	}
}

// GetMetadata retrieves a metadata value by key.
// Returns the value and true if the key exists, nil and false otherwise.
// The caller must type-assert the returned value to the expected type.
func (r Result[T]) GetMetadata(key string) (interface{}, bool) {
	if r.metadata == nil {
		return nil, false
	}
	value, exists := r.metadata[key]
	return value, exists
}

// HasMetadata returns true if this Result contains any metadata.
func (r Result[T]) HasMetadata() bool {
	return len(r.metadata) > 0
}

// MetadataKeys returns all metadata keys for this Result.
// Returns empty slice if no metadata present.
func (r Result[T]) MetadataKeys() []string {
	if r.metadata == nil {
		return []string{}
	}

	keys := make([]string, 0, len(r.metadata))
	for key := range r.metadata {
		keys = append(keys, key)
	}
	return keys
}

// GetStringMetadata retrieves string metadata with enhanced type safety.
// Returns: (value, found, error)
// - found=false, error=nil: key not present
// - found=false, error!=nil: key present but wrong type
// - found=true, error=nil: successful retrieval.
func (r Result[T]) GetStringMetadata(key string) (value string, found bool, err error) {
	metaValue, exists := r.GetMetadata(key)
	if !exists {
		return "", false, nil
	}
	str, ok := metaValue.(string)
	if !ok {
		return "", false, fmt.Errorf("metadata key %q has type %T, expected string", key, metaValue)
	}
	return str, true, nil
}

// GetTimeMetadata retrieves time.Time metadata with enhanced type safety.
func (r Result[T]) GetTimeMetadata(key string) (time.Time, bool, error) {
	value, exists := r.GetMetadata(key)
	if !exists {
		return time.Time{}, false, nil
	}
	t, ok := value.(time.Time)
	if !ok {
		return time.Time{}, false, fmt.Errorf("metadata key %q has type %T, expected time.Time", key, value)
	}
	return t, true, nil
}

// GetIntMetadata retrieves int metadata with enhanced type safety.
func (r Result[T]) GetIntMetadata(key string) (value int, found bool, err error) {
	metaValue, exists := r.GetMetadata(key)
	if !exists {
		return 0, false, nil
	}
	i, ok := metaValue.(int)
	if !ok {
		return 0, false, fmt.Errorf("metadata key %q has type %T, expected int", key, metaValue)
	}
	return i, true, nil
}

// GetDurationMetadata retrieves time.Duration metadata with enhanced type safety.
func (r Result[T]) GetDurationMetadata(key string) (time.Duration, bool, error) {
	value, exists := r.GetMetadata(key)
	if !exists {
		return 0, false, nil
	}
	duration, ok := value.(time.Duration)
	if !ok {
		return 0, false, fmt.Errorf("metadata key %q has type %T, expected time.Duration", key, value)
	}
	return duration, true, nil
}

// WindowMetadata encapsulates window-related metadata operations.
//
//nolint:govet // Field ordering optimized for readability and logical grouping over memory layout
type WindowMetadata struct {
	Size       time.Duration
	Slide      *time.Duration
	Gap        *time.Duration
	SessionKey *string
	Start      time.Time
	End        time.Time
	Type       string
}

// AddWindowMetadata adds complete window metadata to a Result[T].
func AddWindowMetadata[T any](result Result[T], meta WindowMetadata) Result[T] {
	enhanced := result.
		WithMetadata(MetadataWindowStart, meta.Start).
		WithMetadata(MetadataWindowEnd, meta.End).
		WithMetadata(MetadataWindowType, meta.Type).
		WithMetadata(MetadataWindowSize, meta.Size)

	if meta.Slide != nil {
		enhanced = enhanced.WithMetadata(MetadataWindowSlide, *meta.Slide)
	}
	if meta.Gap != nil {
		enhanced = enhanced.WithMetadata(MetadataWindowGap, *meta.Gap)
	}
	if meta.SessionKey != nil {
		enhanced = enhanced.WithMetadata(MetadataSessionKey, *meta.SessionKey)
	}

	return enhanced
}

// GetWindowMetadata extracts window metadata from a Result[T].
func GetWindowMetadata[T any](result Result[T]) (WindowMetadata, error) {
	start, found, err := result.GetTimeMetadata(MetadataWindowStart)
	if err != nil || !found {
		return WindowMetadata{}, fmt.Errorf("window start time not found or invalid: %w", err)
	}

	end, found, err := result.GetTimeMetadata(MetadataWindowEnd)
	if err != nil || !found {
		return WindowMetadata{}, fmt.Errorf("window end time not found or invalid: %w", err)
	}

	windowType, found, err := result.GetStringMetadata(MetadataWindowType)
	if err != nil || !found {
		return WindowMetadata{}, fmt.Errorf("window type not found or invalid: %w", err)
	}

	// Size, Slide, Gap, and SessionKey are optional
	meta := WindowMetadata{
		Start: start,
		End:   end,
		Type:  windowType,
	}

	// Extract optional fields using time.Duration type assertion for size/slide/gap
	if sizeVal, found := result.GetMetadata(MetadataWindowSize); found {
		if size, ok := sizeVal.(time.Duration); ok {
			meta.Size = size
		}
	}
	if slideVal, found := result.GetMetadata(MetadataWindowSlide); found {
		if slide, ok := slideVal.(time.Duration); ok {
			meta.Slide = &slide
		}
	}
	if gapVal, found := result.GetMetadata(MetadataWindowGap); found {
		if gap, ok := gapVal.(time.Duration); ok {
			meta.Gap = &gap
		}
	}
	if sessionKey, found, err := result.GetStringMetadata(MetadataSessionKey); found && err == nil {
		meta.SessionKey = &sessionKey
	}

	return meta, nil
}

// windowKey provides efficient window identification without string allocation.
type windowKey struct {
	startNano int64 // time.Time.UnixNano() for precise boundary identification
	endNano   int64 // time.Time.UnixNano() for precise boundary identification
}

// WindowCollector aggregates Results with matching window metadata.
type WindowCollector[T any] struct {
	name string
}

// WindowCollection represents aggregated results from a single window.
type WindowCollection[T any] struct {
	Start   time.Time
	End     time.Time
	Meta    WindowMetadata
	Results []Result[T]
}

// NewWindowCollector creates a new WindowCollector for aggregating Results by window.
func NewWindowCollector[T any]() *WindowCollector[T] {
	return &WindowCollector[T]{name: "window-collector"}
}

// Process aggregates Results with matching window metadata into WindowCollections.
// Uses struct-based keys to eliminate string allocation overhead for high performance.
func (c *WindowCollector[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan WindowCollection[T] {
	out := make(chan WindowCollection[T])

	go func() {
		defer close(out)

		// Group Results by window boundaries using struct keys (RAINMAN optimization)
		windows := make(map[windowKey][]Result[T])
		windowMeta := make(map[windowKey]WindowMetadata)

		for {
			select {
			case <-ctx.Done():
				// Emit all collected windows
				c.emitAllWindows(ctx, out, windows, windowMeta)
				return

			case result, ok := <-in:
				if !ok {
					// Input closed, emit all windows
					c.emitAllWindows(ctx, out, windows, windowMeta)
					return
				}

				meta, err := GetWindowMetadata(result)
				if err != nil {
					// Skip Results without window metadata
					continue
				}

				// Create struct-based window key (eliminates string allocation)
				key := windowKey{
					startNano: meta.Start.UnixNano(),
					endNano:   meta.End.UnixNano(),
				}

				windows[key] = append(windows[key], result)
				windowMeta[key] = meta
			}
		}
	}()

	return out
}

// emitAllWindows emits all collected windows as WindowCollections.
func (*WindowCollector[T]) emitAllWindows(ctx context.Context, out chan<- WindowCollection[T], windows map[windowKey][]Result[T], meta map[windowKey]WindowMetadata) {
	for key, results := range windows {
		if len(results) > 0 {
			windowMeta := meta[key]
			collection := WindowCollection[T]{
				Start:   windowMeta.Start,
				End:     windowMeta.End,
				Results: results,
				Meta:    windowMeta,
			}

			select {
			case out <- collection:
			case <-ctx.Done():
				return
			}
		}
	}
}

// Values returns all successful values from the window collection.
func (wc WindowCollection[T]) Values() []T {
	var values []T
	for _, result := range wc.Results {
		if result.IsSuccess() {
			values = append(values, result.Value())
		}
	}
	return values
}

// Errors returns all errors from the window collection.
func (wc WindowCollection[T]) Errors() []*StreamError[T] {
	var errors []*StreamError[T]
	for _, result := range wc.Results {
		if result.IsError() {
			errors = append(errors, result.Error())
		}
	}
	return errors
}

// Count returns the total number of results in the window collection.
func (wc WindowCollection[T]) Count() int {
	return len(wc.Results)
}

// SuccessCount returns the number of successful results in the window collection.
func (wc WindowCollection[T]) SuccessCount() int {
	count := 0
	for _, result := range wc.Results {
		if result.IsSuccess() {
			count++
		}
	}
	return count
}

// ErrorCount returns the number of error results in the window collection.
func (wc WindowCollection[T]) ErrorCount() int {
	count := 0
	for _, result := range wc.Results {
		if result.IsError() {
			count++
		}
	}
	return count
}

// WindowInfo provides type-safe access to window metadata.
//
//nolint:govet // Field ordering optimized for readability and logical grouping over memory layout
type WindowInfo struct {
	Size       time.Duration
	Slide      *time.Duration
	Gap        *time.Duration
	SessionKey *string
	Start      time.Time
	End        time.Time
	Type       WindowType
}

// WindowType represents the type of window.
type WindowType string

// Window type constants.
const (
	WindowTypeTumbling WindowType = "tumbling"
	WindowTypeSliding  WindowType = "sliding"
	WindowTypeSession  WindowType = "session"
)

// GetWindowInfo extracts and validates window metadata with enhanced type safety.
func GetWindowInfo[T any](result Result[T]) (WindowInfo, error) {
	meta, err := GetWindowMetadata(result)
	if err != nil {
		return WindowInfo{}, err
	}

	windowType := WindowType(meta.Type)
	switch windowType {
	case WindowTypeTumbling, WindowTypeSliding, WindowTypeSession:
		// Valid types
	default:
		return WindowInfo{}, fmt.Errorf("invalid window type: %s", meta.Type)
	}

	return WindowInfo{
		Start:      meta.Start,
		End:        meta.End,
		Type:       windowType,
		Size:       meta.Size,
		Slide:      meta.Slide,
		Gap:        meta.Gap,
		SessionKey: meta.SessionKey,
	}, nil
}

// IsInWindow checks if a timestamp falls within the Result's window.
func IsInWindow[T any](result Result[T], timestamp time.Time) (bool, error) {
	info, err := GetWindowInfo(result)
	if err != nil {
		return false, err
	}

	return !timestamp.Before(info.Start) && timestamp.Before(info.End), nil
}

// WindowDuration returns the actual window duration.
func WindowDuration[T any](result Result[T]) (time.Duration, error) {
	info, err := GetWindowInfo(result)
	if err != nil {
		return 0, err
	}

	return info.End.Sub(info.Start), nil
}
