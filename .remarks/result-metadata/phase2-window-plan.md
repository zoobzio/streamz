# Phase 2: Window[T] Elimination Implementation Plan

**Status**: DRAFT  
**Date**: 2025-09-17  
**Implementation**: Window[T] elimination and Result[T] metadata migration
**Dependencies**: Phase 1 Result[T] metadata implementation (COMPLETE)

## Objective

Eliminate the Window[T] type completely and refactor all windowing functions (tumbling, sliding, session) to emit Result[T] with window metadata instead of Window[T] collections. This simplifies the type system, improves composability, and enables seamless metadata flow through windowing operations.

## Current State Analysis

### Existing Window[T] Implementation
```go
type Window[T any] struct {
    Start   time.Time   // Window start time
    End     time.Time   // Window end time
    Results []Result[T] // All Results (success and errors) in this window
}
```

### Window[T] Methods to Replace
- `Values() []T` - Extract successful values only
- `Errors() []*StreamError[T]` - Extract errors only  
- `Count() int` - Total result count
- `SuccessCount() int` - Success count
- `ErrorCount() int` - Error count

### Current Window Processors
1. **TumblingWindow[T]** - Fixed-size non-overlapping windows
2. **SlidingWindow[T]** - Overlapping time-based windows
3. **SessionWindow[T]** - Activity-gap based dynamic windows

All emit `<-chan Window[T]` which fragments the data flow and prevents Result[T] composability.

## Implementation Strategy

### Core Transformation Pattern

**Before (Window[T]):**
```go
// Window contains multiple Results
Window[Event] {
    Start: 2025-01-01T10:00:00Z,
    End:   2025-01-01T10:05:00Z,
    Results: [Result[Event]{...}, Result[Event]{...}, ...]
}
```

**After (Result[T] with metadata):**
```go
// Each Result carries window metadata individually
Result[Event] {
    value: Event{...},
    metadata: {
        "window_start": 2025-01-01T10:00:00Z,
        "window_end":   2025-01-01T10:05:00Z,
        "window_type":  "tumbling",
        "window_size":  5*time.Minute
    }
}
```

### Key Architectural Changes

1. **Window processors emit `<-chan Result[T]` instead of `<-chan Window[T]`**
2. **Window boundaries stored as metadata in each Result[T]**
3. **Window aggregation handled by downstream processors, not window processors**
4. **Simplified processor composition with unified Result[T] type**

## Phase 2 Implementation Plan

### Stage 1: Metadata Constants and Helpers

#### 1.1 Extended Metadata Constants
```go
// Additional window-specific metadata keys
const (
    MetadataWindowStart = "window_start"   // time.Time - existing
    MetadataWindowEnd   = "window_end"     // time.Time - existing
    MetadataWindowType  = "window_type"    // string - "tumbling", "sliding", "session"
    MetadataWindowSize  = "window_size"    // time.Duration - window duration
    MetadataWindowSlide = "window_slide"   // time.Duration - slide interval (sliding only)
    MetadataWindowGap   = "window_gap"     // time.Duration - activity gap (session only)
    MetadataSessionKey  = "session_key"    // string - session identifier (session only)
)
```

#### 1.2 Window Metadata Helpers
```go
// WindowMetadata encapsulates window-related metadata operations
type WindowMetadata struct {
    Start     time.Time
    End       time.Time
    Type      string
    Size      time.Duration
    Slide     *time.Duration // nil for non-sliding windows
    Gap       *time.Duration // nil for non-session windows
    SessionKey *string       // nil for non-session windows
}

// AddWindowMetadata adds complete window metadata to a Result[T]
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

// GetWindowMetadata extracts window metadata from a Result[T]
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
    
    // Extract optional fields
    if size, found, _ := result.GetTimeMetadata(MetadataWindowSize); found {
        meta.Size = size
    }
    if slide, found, _ := result.GetTimeMetadata(MetadataWindowSlide); found {
        meta.Slide = &slide
    }
    if gap, found, _ := result.GetTimeMetadata(MetadataWindowGap); found {
        meta.Gap = &gap
    }
    if sessionKey, found, _ := result.GetStringMetadata(MetadataSessionKey); found {
        meta.SessionKey = &sessionKey
    }
    
    return meta, nil
}
```

### Stage 2: Window Processor Refactoring

#### 2.1 TumblingWindow Refactor

**Current Signature:**
```go
func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Window[T]
```

**New Signature:**
```go
func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T]
```

**Implementation Strategy:**
```go
func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])

    go func() {
        defer close(out)

        ticker := w.clock.NewTicker(w.size)
        defer ticker.Stop()

        now := w.clock.Now()
        currentWindow := WindowMetadata{
            Start: now,
            End:   now.Add(w.size),
            Type:  "tumbling",
            Size:  w.size,
        }
        
        var windowResults []Result[T]

        for {
            select {
            case <-ctx.Done():
                // Emit remaining results with window metadata
                w.emitWindowResults(ctx, out, windowResults, currentWindow)
                return

            case result, ok := <-in:
                if !ok {
                    // Input closed, emit remaining results
                    w.emitWindowResults(ctx, out, windowResults, currentWindow)
                    return
                }
                windowResults = append(windowResults, result)

            case <-ticker.C():
                // Window expired, emit all results with window metadata
                w.emitWindowResults(ctx, out, windowResults, currentWindow)
                
                // Create new window
                windowResults = nil
                now := w.clock.Now()
                currentWindow = WindowMetadata{
                    Start: now,
                    End:   now.Add(w.size),
                    Type:  "tumbling",
                    Size:  w.size,
                }
            }
        }
    }()

    return out
}

func (w *TumblingWindow[T]) emitWindowResults(ctx context.Context, out chan<- Result[T], results []Result[T], meta WindowMetadata) {
    for _, result := range results {
        enhanced := AddWindowMetadata(result, meta)
        select {
        case out <- enhanced:
        case <-ctx.Done():
            return
        }
    }
}
```

#### 2.2 SlidingWindow Refactor

**Implementation Strategy:**
```go
func (w *SlidingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])

    go func() {
        defer close(out)

        // Special case: if slide equals size, behave like tumbling window
        if w.slide == w.size {
            w.processTumblingMode(ctx, in, out)
            return
        }

        ticker := w.clock.NewTicker(w.slide)
        defer ticker.Stop()

        // Track active windows and their results
        type windowState struct {
            meta    WindowMetadata
            results []Result[T]
        }
        
        windows := make(map[time.Time]*windowState)
        var firstItemTime time.Time
        var firstItemReceived bool

        for {
            select {
            case <-ctx.Done():
                // Emit all remaining windows
                w.emitAllWindows(ctx, out, windows)
                return

            case result, ok := <-in:
                if !ok {
                    // Input closed, emit all windows
                    w.emitAllWindows(ctx, out, windows)
                    return
                }

                now := w.clock.Now()
                if !firstItemReceived {
                    firstItemTime = now
                    firstItemReceived = true
                }

                // Add to existing windows that should contain this item
                for start, window := range windows {
                    if !start.After(now) && now.Before(window.meta.End) {
                        window.results = append(window.results, result)
                    }
                }

                // Create new window at current slide boundary if needed
                currentWindowStart := now.Truncate(w.slide)
                if _, exists := windows[currentWindowStart]; !exists && !currentWindowStart.Before(firstItemTime) {
                    slidePtr := &w.slide
                    windows[currentWindowStart] = &windowState{
                        meta: WindowMetadata{
                            Start: currentWindowStart,
                            End:   currentWindowStart.Add(w.size),
                            Type:  "sliding",
                            Size:  w.size,
                            Slide: slidePtr,
                        },
                        results: []Result[T]{result},
                    }
                }

            case <-ticker.C():
                now := w.clock.Now()
                // Emit expired windows
                expiredStarts := make([]time.Time, 0)
                
                for start, window := range windows {
                    if !window.meta.End.After(now) {
                        w.emitWindowResults(ctx, out, window.results, window.meta)
                        expiredStarts = append(expiredStarts, start)
                    }
                }
                
                // Clean up expired windows
                for _, start := range expiredStarts {
                    delete(windows, start)
                }
            }
        }
    }()

    return out
}

func (w *SlidingWindow[T]) emitAllWindows(ctx context.Context, out chan<- Result[T], windows map[time.Time]*windowState) {
    for _, window := range windows {
        if len(window.results) > 0 {
            w.emitWindowResults(ctx, out, window.results, window.meta)
        }
    }
}

func (w *SlidingWindow[T]) emitWindowResults(ctx context.Context, out chan<- Result[T], results []Result[T], meta WindowMetadata) {
    for _, result := range results {
        enhanced := AddWindowMetadata(result, meta)
        select {
        case out <- enhanced:
        case <-ctx.Done():
            return
        }
    }
}
```

#### 2.3 SessionWindow Refactor

**Implementation Strategy:**
```go
func (w *SessionWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])

    go func() {
        defer close(out)

        type sessionState struct {
            meta         WindowMetadata
            results      []Result[T]
            lastActivity time.Time
        }

        sessions := make(map[string]*sessionState)

        checkInterval := w.gap / 4
        if checkInterval < 10*time.Millisecond {
            checkInterval = 10 * time.Millisecond
        }
        ticker := w.clock.NewTicker(checkInterval)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                // Emit all remaining sessions
                w.emitAllSessions(ctx, out, sessions)
                return

            case result, ok := <-in:
                if !ok {
                    // Input closed - emit all remaining sessions
                    w.emitAllSessions(ctx, out, sessions)
                    return
                }

                key := w.keyFunc(result)
                now := w.clock.Now()

                if session, exists := sessions[key]; exists {
                    // Extend existing session
                    session.results = append(session.results, result)
                    session.meta.End = now.Add(w.gap)
                    session.lastActivity = now
                } else {
                    // Create new session
                    gapPtr := &w.gap
                    keyPtr := &key
                    sessions[key] = &sessionState{
                        meta: WindowMetadata{
                            Start:      now,
                            End:        now.Add(w.gap),
                            Type:       "session",
                            Gap:        gapPtr,
                            SessionKey: keyPtr,
                        },
                        results:      []Result[T]{result},
                        lastActivity: now,
                    }
                }

            case <-ticker.C():
                // Periodic session expiry check
                now := w.clock.Now()
                expiredKeys := make([]string, 0)

                for key, session := range sessions {
                    if now.Sub(session.lastActivity) >= w.gap {
                        expiredKeys = append(expiredKeys, key)
                        w.emitWindowResults(ctx, out, session.results, session.meta)
                    }
                }

                // Clean up expired sessions
                for _, key := range expiredKeys {
                    delete(sessions, key)
                }
            }
        }
    }()

    return out
}

func (w *SessionWindow[T]) emitAllSessions(ctx context.Context, out chan<- Result[T], sessions map[string]*sessionState) {
    for _, session := range sessions {
        if len(session.results) > 0 {
            w.emitWindowResults(ctx, out, session.results, session.meta)
        }
    }
}

func (w *SessionWindow[T]) emitWindowResults(ctx context.Context, out chan<- Result[T], results []Result[T], meta WindowMetadata) {
    for _, result := range results {
        enhanced := AddWindowMetadata(result, meta)
        select {
        case out <- enhanced:
        case <-ctx.Done():
            return
        }
    }
}
```

### Stage 3: Window[T] Type Elimination

#### 3.1 Remove Window[T] Definition
Complete removal of:
```go
// DELETE: No longer needed
type Window[T any] struct {
    Start   time.Time
    End     time.Time  
    Results []Result[T]
}

// DELETE: All methods become metadata-based
func (w Window[T]) Values() []T { ... }
func (w Window[T]) Errors() []*StreamError[T] { ... }
func (w Window[T]) Count() int { ... }
func (w Window[T]) SuccessCount() int { ... }
func (w Window[T]) ErrorCount() int { ... }
```

#### 3.2 Window Aggregation Processors

Since window boundaries are now metadata, we need new processors to aggregate windowed Results:

```go
// WindowCollector aggregates Results with matching window metadata
type WindowCollector[T any] struct {
    name string
}

// WindowCollection represents aggregated results from a single window
type WindowCollection[T any] struct {
    Start   time.Time
    End     time.Time
    Results []Result[T]
    Meta    WindowMetadata
}

func NewWindowCollector[T any]() *WindowCollector[T] {
    return &WindowCollector[T]{name: "window-collector"}
}

func (c *WindowCollector[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan WindowCollection[T] {
    out := make(chan WindowCollection[T])

    go func() {
        defer close(out)

        // Group Results by window boundaries
        windows := make(map[string][]Result[T])
        windowMeta := make(map[string]WindowMetadata)

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

                // Create window key from start+end times
                windowKey := fmt.Sprintf("%d-%d", meta.Start.UnixNano(), meta.End.UnixNano())
                
                windows[windowKey] = append(windows[windowKey], result)
                windowMeta[windowKey] = meta
            }
        }
    }()

    return out
}

func (c *WindowCollector[T]) emitAllWindows(ctx context.Context, out chan<- WindowCollection[T], windows map[string][]Result[T], meta map[string]WindowMetadata) {
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

// Provide Window[T] compatibility methods
func (wc WindowCollection[T]) Values() []T {
    var values []T
    for _, result := range wc.Results {
        if result.IsSuccess() {
            values = append(values, result.Value())
        }
    }
    return values
}

func (wc WindowCollection[T]) Errors() []*StreamError[T] {
    var errors []*StreamError[T]
    for _, result := range wc.Results {
        if result.IsError() {
            errors = append(errors, result.Error())
        }
    }
    return errors
}

func (wc WindowCollection[T]) Count() int {
    return len(wc.Results)
}

func (wc WindowCollection[T]) SuccessCount() int {
    count := 0
    for _, result := range wc.Results {
        if result.IsSuccess() {
            count++
        }
    }
    return count
}

func (wc WindowCollection[T]) ErrorCount() int {
    count := 0
    for _, result := range wc.Results {
        if result.IsError() {
            count++
        }
    }
    return count
}
```

### Stage 4: Type-Safe Window Accessor Patterns

#### 4.1 Window Metadata Type Safety

```go
// WindowInfo provides type-safe access to window metadata
type WindowInfo struct {
    Start      time.Time
    End        time.Time
    Type       WindowType
    Size       time.Duration
    Slide      *time.Duration
    Gap        *time.Duration
    SessionKey *string
}

type WindowType string

const (
    WindowTypeTumbling WindowType = "tumbling"
    WindowTypeSliding  WindowType = "sliding"
    WindowTypeSession  WindowType = "session"
)

// GetWindowInfo extracts and validates window metadata
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

// IsInWindow checks if a timestamp falls within the Result's window
func IsInWindow[T any](result Result[T], timestamp time.Time) (bool, error) {
    info, err := GetWindowInfo(result)
    if err != nil {
        return false, err
    }
    
    return !timestamp.Before(info.Start) && timestamp.Before(info.End), nil
}

// WindowDuration returns the actual window duration
func WindowDuration[T any](result Result[T]) (time.Duration, error) {
    info, err := GetWindowInfo(result)
    if err != nil {
        return 0, err
    }
    
    return info.End.Sub(info.Start), nil
}
```

#### 4.2 Window Querying API

```go
// WindowQuery provides fluent API for window-based filtering
type WindowQuery[T any] struct {
    windowType *WindowType
    minSize    *time.Duration
    maxSize    *time.Duration
    sessionKey *string
    timeRange  *TimeRange
}

type TimeRange struct {
    Start time.Time
    End   time.Time
}

func NewWindowQuery[T any]() *WindowQuery[T] {
    return &WindowQuery[T]{}
}

func (q *WindowQuery[T]) WithType(windowType WindowType) *WindowQuery[T] {
    q.windowType = &windowType
    return q
}

func (q *WindowQuery[T]) WithSizeRange(min, max time.Duration) *WindowQuery[T] {
    q.minSize = &min
    q.maxSize = &max
    return q
}

func (q *WindowQuery[T]) WithSessionKey(key string) *WindowQuery[T] {
    q.sessionKey = &key
    return q
}

func (q *WindowQuery[T]) WithTimeRange(start, end time.Time) *WindowQuery[T] {
    q.timeRange = &TimeRange{Start: start, End: end}
    return q
}

func (q *WindowQuery[T]) Matches(result Result[T]) bool {
    info, err := GetWindowInfo(result)
    if err != nil {
        return false
    }
    
    if q.windowType != nil && info.Type != *q.windowType {
        return false
    }
    
    if q.minSize != nil && info.Size < *q.minSize {
        return false
    }
    
    if q.maxSize != nil && info.Size > *q.maxSize {
        return false
    }
    
    if q.sessionKey != nil && (info.SessionKey == nil || *info.SessionKey != *q.sessionKey) {
        return false
    }
    
    if q.timeRange != nil {
        if info.Start.Before(q.timeRange.Start) || info.End.After(q.timeRange.End) {
            return false
        }
    }
    
    return true
}

// Filter creates a processor that filters Results based on window criteria
func (q *WindowQuery[T]) Filter() func(context.Context, <-chan Result[T]) <-chan Result[T] {
    return func(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
        out := make(chan Result[T])
        
        go func() {
            defer close(out)
            
            for {
                select {
                case <-ctx.Done():
                    return
                case result, ok := <-in:
                    if !ok {
                        return
                    }
                    
                    if q.Matches(result) {
                        select {
                        case out <- result:
                        case <-ctx.Done():
                            return
                        }
                    }
                }
            }
        }()
        
        return out
    }
}
```

## Backward Compatibility Strategy

### Migration Path for Existing Code

#### Option 1: Compatibility Layer (Recommended)
```go
// Legacy Window[T] compatibility - temporary bridge
func CollectWindow[T any](ctx context.Context, in <-chan Result[T]) <-chan Window[T] {
    collector := NewWindowCollector[T]()
    collections := collector.Process(ctx, in)
    
    out := make(chan Window[T])
    
    go func() {
        defer close(out)
        
        for collection := range collections {
            window := Window[T]{
                Start:   collection.Start,
                End:     collection.End,
                Results: collection.Results,
            }
            
            select {
            case out <- window:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}
```

#### Option 2: Direct Migration
```go
// Before: Window[T] usage
tumbling := NewTumblingWindow[Event](5*time.Minute, RealClock)
windows := tumbling.Process(ctx, eventResults)

for window := range windows {
    values := window.Values()
    errors := window.Errors()
    processWindowData(values, errors)
}

// After: Result[T] with metadata usage
tumbling := NewTumblingWindow[Event](5*time.Minute, RealClock)
results := tumbling.Process(ctx, eventResults)

// Option A: Collect into windows when needed
collector := NewWindowCollector[Event]()
windows := collector.Process(ctx, results)

for window := range windows {
    values := window.Values()
    errors := window.Errors()
    processWindowData(values, errors)
}

// Option B: Process individual Results with window context
for result := range results {
    if info, err := GetWindowInfo(result); err == nil {
        processResultWithWindow(result, info)
    }
}
```

## Testing Strategy

### Stage 5: Comprehensive Test Coverage

#### 5.1 Metadata Preservation Tests
```go
func TestTumblingWindow_MetadataPreservation(t *testing.T) {
    window := NewTumblingWindow[int](time.Minute, NewTestClock())
    
    input := make(chan Result[int], 3)
    input <- NewSuccess(1).WithMetadata("source", "api")
    input <- NewSuccess(2).WithMetadata("user_id", "123")
    input <- NewError(3, errors.New("test"), "processor").WithMetadata("context", "validation")
    close(input)
    
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    
    results := window.Process(ctx, input)
    
    var collected []Result[int]
    for result := range results {
        collected = append(collected, result)
    }
    
    assert.Len(t, collected, 3)
    
    // Verify original metadata preserved
    for _, result := range collected {
        if result.IsSuccess() && result.Value() == 1 {
            source, found, err := result.GetStringMetadata("source")
            assert.NoError(t, err)
            assert.True(t, found)
            assert.Equal(t, "api", source)
        }
        
        // Verify window metadata added
        info, err := GetWindowInfo(result)
        assert.NoError(t, err)
        assert.Equal(t, WindowTypeTumbling, info.Type)
        assert.Equal(t, time.Minute, info.Size)
        assert.NotZero(t, info.Start)
        assert.NotZero(t, info.End)
    }
}
```

#### 5.2 Window Boundary Accuracy Tests
```go
func TestSlidingWindow_WindowBoundaryAccuracy(t *testing.T) {
    clock := NewTestClock()
    window := NewSlidingWindow[int](5*time.Minute, clock).WithSlide(time.Minute)
    
    // Test overlapping window assignment
    input := make(chan Result[int], 6)
    
    // Send events at specific times
    clock.SetTime(time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC))
    input <- NewSuccess(1)
    
    clock.Advance(2 * time.Minute)
    input <- NewSuccess(2)
    
    clock.Advance(2 * time.Minute)
    input <- NewSuccess(3)
    
    close(input)
    
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    
    results := window.Process(ctx, input)
    
    // Group results by window boundaries
    windowGroups := make(map[string][]Result[int])
    for result := range results {
        info, err := GetWindowInfo(result)
        assert.NoError(t, err)
        
        key := fmt.Sprintf("%d-%d", info.Start.Unix(), info.End.Unix())
        windowGroups[key] = append(windowGroups[key], result)
    }
    
    // Verify event 2 appears in multiple overlapping windows
    eventTwoWindows := 0
    for _, group := range windowGroups {
        for _, result := range group {
            if result.IsSuccess() && result.Value() == 2 {
                eventTwoWindows++
                break
            }
        }
    }
    assert.Greater(t, eventTwoWindows, 1, "Event 2 should appear in multiple overlapping windows")
}
```

#### 5.3 Session Window Key Isolation Tests
```go
func TestSessionWindow_KeyIsolation(t *testing.T) {
    keyFunc := func(result Result[string]) string {
        if result.IsSuccess() {
            return result.Value()[:1] // First character as session key
        }
        return result.Error().Item()[:1]
    }
    
    window := NewSessionWindow(keyFunc, NewTestClock()).WithGap(time.Minute)
    
    input := make(chan Result[string], 4)
    input <- NewSuccess("alice-1")
    input <- NewSuccess("bob-1")
    input <- NewSuccess("alice-2")
    input <- NewSuccess("bob-2")
    close(input)
    
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    
    results := window.Process(ctx, input)
    
    // Group by session key from metadata
    sessions := make(map[string][]Result[string])
    for result := range results {
        info, err := GetWindowInfo(result)
        assert.NoError(t, err)
        assert.NotNil(t, info.SessionKey)
        
        sessions[*info.SessionKey] = append(sessions[*info.SessionKey], result)
    }
    
    assert.Len(t, sessions, 2, "Should have two distinct sessions")
    assert.Contains(t, sessions, "a", "Should have alice session")
    assert.Contains(t, sessions, "b", "Should have bob session")
    
    assert.Len(t, sessions["a"], 2, "Alice session should have 2 events")
    assert.Len(t, sessions["b"], 2, "Bob session should have 2 events")
}
```

#### 5.4 Window Collection Aggregation Tests
```go
func TestWindowCollector_Aggregation(t *testing.T) {
    collector := NewWindowCollector[int]()
    
    // Create Results with identical window metadata
    windowStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
    windowEnd := windowStart.Add(5 * time.Minute)
    
    input := make(chan Result[int], 3)
    
    meta := WindowMetadata{
        Start: windowStart,
        End:   windowEnd,
        Type:  "tumbling",
        Size:  5 * time.Minute,
    }
    
    input <- AddWindowMetadata(NewSuccess(1), meta)
    input <- AddWindowMetadata(NewSuccess(2), meta)
    input <- AddWindowMetadata(NewError(3, errors.New("test"), "processor"), meta)
    close(input)
    
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    
    collections := collector.Process(ctx, input)
    
    var windows []WindowCollection[int]
    for window := range collections {
        windows = append(windows, window)
    }
    
    assert.Len(t, windows, 1, "Should aggregate into single window")
    
    window := windows[0]
    assert.Equal(t, windowStart, window.Start)
    assert.Equal(t, windowEnd, window.End)
    assert.Len(t, window.Results, 3)
    
    assert.Equal(t, 2, window.SuccessCount())
    assert.Equal(t, 1, window.ErrorCount())
    assert.Equal(t, []int{1, 2}, window.Values())
    assert.Len(t, window.Errors(), 1)
}
```

### Stage 6: Performance Validation

#### 6.1 Memory Overhead Comparison
```go
func BenchmarkWindow_MemoryComparison(b *testing.B) {
    b.Run("Legacy_Window_Type", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            results := make([]Result[int], 100)
            for j := range results {
                results[j] = NewSuccess(j)
            }
            
            window := Window[int]{
                Start:   time.Now(),
                End:     time.Now().Add(time.Minute),
                Results: results,
            }
            
            _ = window.Values() // Force evaluation
        }
    })
    
    b.Run("Metadata_Based_Window", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            meta := WindowMetadata{
                Start: time.Now(),
                End:   time.Now().Add(time.Minute),
                Type:  "tumbling",
                Size:  time.Minute,
            }
            
            results := make([]Result[int], 100)
            for j := range results {
                results[j] = AddWindowMetadata(NewSuccess(j), meta)
            }
            
            // Simulate value extraction
            var values []int
            for _, result := range results {
                if result.IsSuccess() {
                    values = append(values, result.Value())
                }
            }
        }
    })
}
```

#### 6.2 Throughput Performance Tests
```go
func BenchmarkWindowProcessor_Throughput(b *testing.B) {
    b.Run("TumblingWindow_Legacy", func(b *testing.B) {
        // Benchmark would test old Window[T] output
    })
    
    b.Run("TumblingWindow_Metadata", func(b *testing.B) {
        window := NewTumblingWindow[int](time.Minute, NewTestClock())
        
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            input := make(chan Result[int], 1000)
            for j := 0; j < 1000; j++ {
                input <- NewSuccess(j)
            }
            close(input)
            
            ctx, cancel := context.WithCancel(context.Background())
            results := window.Process(ctx, input)
            
            // Consume all results
            for range results {
            }
            cancel()
        }
    })
}
```

## Implementation Validation Criteria

### Quality Gates for Phase 2

#### Functional Requirements ✓
- [ ] All three window processors emit `<-chan Result[T]` instead of `<-chan Window[T]`
- [ ] Window metadata correctly attached to each Result[T]
- [ ] Original metadata preserved through windowing operations
- [ ] Window boundary accuracy maintained for all window types
- [ ] Session key isolation working correctly
- [ ] WindowCollector accurately aggregates Results by window boundaries

#### Performance Requirements ✓
- [ ] Memory overhead <20% increase vs Window[T] approach for typical workloads
- [ ] Throughput degradation <10% for windowing operations
- [ ] Metadata access performance <1μs per operation
- [ ] Window boundary calculations maintain sub-millisecond precision

#### Compatibility Requirements ✓
- [ ] Compatibility layer enables gradual migration from Window[T]
- [ ] All existing window processor configuration options preserved
- [ ] WindowCollection provides identical API to legacy Window[T] type
- [ ] Migration path documented with clear examples

#### Type Safety Requirements ✓
- [ ] Window metadata type validation prevents runtime errors
- [ ] GetWindowInfo provides comprehensive error checking
- [ ] WindowQuery API enables type-safe window filtering
- [ ] No panic conditions under any input scenarios

### Implementation Timeline

**Total Estimated Duration**: 8 days

- **Stage 1** (Metadata Constants & Helpers): 1 day
- **Stage 2** (Window Processor Refactoring): 3 days
- **Stage 3** (Window[T] Elimination): 1 day
- **Stage 4** (Type-Safe Accessors): 2 days
- **Stage 5** (Testing): 1 day

### Risk Assessment

#### Low Risk ✓
- **Metadata preservation**: Phase 1 implementation provides proven foundation
- **Type safety**: Enhanced error handling eliminates panic conditions
- **Performance**: Metadata overhead well-characterized and bounded

#### Medium Risk ⚠️
- **Complex window logic**: Sliding/session windows have intricate boundary management
- **Migration complexity**: Existing codebases may require significant changes

#### High Risk ❌
- **Breaking changes**: Complete elimination of Window[T] requires coordinated migration

### Mitigation Strategies

1. **Comprehensive Testing**: Extensive boundary condition and edge case validation
2. **Gradual Migration**: Compatibility layer enables incremental adoption
3. **Performance Monitoring**: Continuous benchmarking throughout implementation
4. **Rollback Plan**: Ability to restore Window[T] type if issues discovered

## Expected Benefits

### Architectural Improvements
1. **Unified Type System**: Single Result[T] type throughout pipeline
2. **Enhanced Composability**: Metadata enables flexible processor chaining
3. **Simplified Debugging**: Window context attached to each Result
4. **Improved Extensibility**: Metadata supports future windowing features

### Operational Benefits
1. **Reduced Complexity**: Elimination of dual-type channel patterns
2. **Better Observability**: Window metadata enables detailed monitoring
3. **Flexible Aggregation**: Windows can be collected/ignored as needed
4. **Memory Efficiency**: Metadata shared across Results in same window

## Post-Implementation Tasks

### Documentation Updates
- [ ] Update all windowing examples to use Result[T] with metadata
- [ ] Create migration guide from Window[T] to metadata approach
- [ ] Document window metadata standards and best practices
- [ ] Add performance characteristics documentation

### Integration Testing
- [ ] Test integration with existing processors (map, filter, etc.)
- [ ] Validate memory usage in high-throughput scenarios
- [ ] Stress test complex windowing scenarios
- [ ] Verify metadata flow through multi-stage pipelines

### Performance Optimization
- [ ] Profile metadata allocation patterns
- [ ] Optimize window boundary calculation algorithms
- [ ] Benchmark against realistic workloads
- [ ] Identify and resolve performance bottlenecks

## Conclusion

Phase 2 eliminates the Window[T] type while maintaining all windowing functionality through Result[T] metadata. This approach provides:

1. **Type System Unification**: Single Result[T] type enables seamless composition
2. **Enhanced Flexibility**: Metadata-driven approach supports diverse use cases
3. **Backward Compatibility**: Migration path preserves existing functionality
4. **Performance Efficiency**: Metadata overhead offset by reduced type complexity

The implementation provides a robust foundation for simplified stream processing architecture while maintaining the reliability and performance standards required for production systems.

**Implementation Status**: ✅ READY FOR IMPLEMENTATION

**Risk Level**: 🟡 MEDIUM - Significant changes with comprehensive mitigation

**Quality Confidence**: 🟢 HIGH - Well-defined requirements with extensive validation plan