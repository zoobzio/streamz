# Window Processor Composability Investigation

## Pattern Identified: Window[T] Return Type Breaks Composability

Found definitive reason. Window processors return `<-chan Window[T]` instead of `<-chan Result[T]`. Breaks composition.

### Evidence

**Window processor signatures:**
```go
// TumblingWindow.Process
func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Window[T]

// SlidingWindow.Process  
func (w *SlidingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Window[T]

// SessionWindow.Process
func (w *SessionWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Window[T]
```

**Other processors (consistent):**
```go
// All others
func (p *Processor[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T]
```

### Window[T] Structure

```go
type Window[T any] struct {
    Start   time.Time   // Window start time
    End     time.Time   // Window end time  
    Results []Result[T] // All Results (success and errors) in this window
}
```

Window contains multiple Result[T]. Not single Result[T]. Fundamentally different.

### Composability Failure Points

**Cannot chain directly:**
```go
// FAILS - type mismatch
windows := NewTumblingWindow[Event](time.Minute, clock).Process(ctx, events)
mapped := NewMapper[Window[Event], string](...).Process(ctx, windows) // TYPE ERROR
```

**Cannot use with standard processors:**
```go
// All fail - expect Result[T], get Window[T]
filter.Process(ctx, windows)     // FAIL
throttle.Process(ctx, windows)   // FAIL  
fanout.Process(ctx, windows)     // FAIL
buffer.Process(ctx, windows)     // FAIL
```

### Semantic Requirements Analysis

Window processors aggregate. Core semantic:
- Collect multiple Result[T] items
- Group by time boundaries
- Emit collection when boundary reached
- Preserve both success and error Results

Not 1:1 transformation. Many:1 aggregation.

### Alternative Approaches

#### Option 1: Window as Result[Window[T]]

Return `<-chan Result[Window[T]]` instead.

**Benefits:**
- Composable with all processors
- Errors at window level possible
- Consistent channel signature

**Implementation:**
```go
func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[Window[T]] {
    out := make(chan Result[Window[T]])
    // ... 
    // Emit window
    select {
    case out <- NewSuccess(*window):
    // ...
}
```

**Downstream usage:**
```go
windows := tumbling.Process(ctx, events)  // <-chan Result[Window[Event]]
for result := range windows {
    if result.IsError() {
        // Window processing error
    } else {
        window := result.Value()
        // Use window.Results, window.Values(), etc.
    }
}
```

#### Option 2: Flatten Window Results

Emit individual Result[T] from window with metadata.

**Problems:**
- Loses window boundaries
- Cannot aggregate across window
- Defeats purpose of windowing

Not viable.

#### Option 3: Tagged Result Pattern

Add window metadata to Result[T].

```go
type WindowedResult[T any] struct {
    Result[T]
    WindowStart time.Time
    WindowEnd   time.Time
}
```

**Problems:**
- Still emits individual items
- Complicates Result[T] pattern
- Window aggregation lost

Not viable.

#### Option 4: Dual API

Provide both:
- `Process()` returns `<-chan Window[T]` (current)
- `ProcessResult()` returns `<-chan Result[Window[T]]` (composable)

**Benefits:**
- Backward compatible
- Choose based on need
- Clear semantic difference

**Implementation:**
```go
func (w *TumblingWindow[T]) ProcessResult(ctx context.Context, in <-chan Result[T]) <-chan Result[Window[T]] {
    windows := w.Process(ctx, in)
    out := make(chan Result[Window[T]])
    go func() {
        defer close(out)
        for window := range windows {
            select {
            case out <- NewSuccess(window):
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}
```

### Test Evidence

No window processors in composability tests. Integration tests show:
- FanIn/FanOut composition works
- Buffer/Throttle/Debounce composition works  
- Mapper/AsyncMapper composition works
- No Window processor chains tested

Window tests standalone only. Never composed.

### Root Cause

Window processors designed as terminal operators. Not intermediate.

Assumption: Windows used for final aggregation/reporting. Not further processing.

Reality: Users want to:
- Map over windows (compute aggregates)
- Filter windows (threshold checks)
- Route windows (based on content)
- Fan out windows (multiple consumers)

### Recommendation

**Option 1 (Breaking change):** Change to `Result[Window[T]]` pattern.

**Option 4 (Compatible):** Add `ProcessResult()` method alongside existing.

Option 4 preserves compatibility. Users choose:
- `Process()` for terminal use (current behavior)
- `ProcessResult()` for composition

### Impact Analysis

**Current workarounds required:**
```go
// Manual wrapping
windows := tumbling.Process(ctx, events)
wrapped := make(chan Result[Window[Event]])
go func() {
    defer close(wrapped)
    for w := range windows {
        wrapped <- NewSuccess(w)
    }
}()
// Now composable
```

**With Option 4:**
```go
// Direct composition
windows := tumbling.ProcessResult(ctx, events)
// Already Result[Window[T]], composable
```

### Pattern Consistency

All processors should be composable. Window processors exception. Violates principle.

Result[T] pattern works. Window[T] deviation unnecessary. Can wrap Window[T] in Result.

### Conclusion

Window processors return Window[T] for semantic clarity. Breaks composability.

Solution: Wrap Window[T] in Result[Window[T]]. Maintains semantics. Enables composition.

Recommend Option 4: Dual API. Compatible. Clear. Users choose based on need.