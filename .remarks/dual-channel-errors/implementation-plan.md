# Dual-Channel Error Handling Implementation Plan

## Overview
Add dual-channel error handling to streamz with separate success/error channels. Zero backward compatibility. Prove the concept with minimal implementation.

## Core Changes Needed

### 1. New Processor Interface
**File: `/home/zoobzio/code/streamz/api.go`**
- Replace existing `Processor[In, Out]` interface completely
- New interface returns both success and error channels
- Add `StreamError` type for structured error data

```go
type Processor[In, Out any] interface {
    Process(ctx context.Context, in <-chan In) (success <-chan Out, errors <-chan StreamError[In])
    Name() string
}

type StreamError[T any] struct {
    Item      T
    Error     error
    Processor string
    Timestamp time.Time
}
```

### 2. Core Files to Overwrite
Direct overwrites (no compatibility preservation):
- `/home/zoobzio/code/streamz/api.go` - New interface definition
- `/home/zoobzio/code/streamz/fanin.go` - Proof of concept implementation
- `/home/zoobzio/code/streamz/fanin_test.go` - Validation tests

## Proof of Concept Choice: FanIn

**Why FanIn:**
- Simplest multi-channel processor (merge pattern)
- Demonstrates error aggregation from multiple sources  
- Shows error channel merging behavior
- Clear success/failure testing scenarios
- Represents the fundamental pattern for other processors

**What it proves:**
- Interface works for multi-input processors
- Error channels can be properly merged
- Context cancellation works with dual channels
- Goroutine cleanup is correct
- Testing patterns are viable

## Implementation Steps

### Step 1: Core Interface (30 minutes)
1. **Edit `/home/zoobzio/code/streamz/api.go`:**
   - Replace `Processor[In, Out]` interface
   - Add `StreamError[T]` type
   - Update package documentation example
   - Remove old `BatchConfig`/`WindowConfig` (not needed for proof)

### Step 2: FanIn Implementation (90 minutes)
2. **Edit `/home/zoobzio/code/streamz/fanin.go`:**
   - Implement new dual-channel interface
   - Handle error aggregation from multiple inputs
   - Proper goroutine management for both channels
   - Context cancellation for both channels

### Step 3: Core Tests (120 minutes)  
3. **Edit `/home/zoobzio/code/streamz/fanin_test.go`:**
   - Test success channel works (happy path)
   - Test error channel receives errors
   - Test mixed success/error scenarios
   - Test context cancellation
   - Test proper channel closure
   - Test error metadata (processor name, timestamp)

### Step 4: Validation (30 minutes)
4. **Verify implementation:**
   - Run `go test ./fanin_test.go fanin.go api.go`
   - Check both channels close properly
   - Verify no goroutine leaks
   - Confirm error metadata is populated

## Expected Breakage
Everything else will break. This is expected and acceptable:
- All other processors (`mapper.go`, `filter.go`, etc.)
- All examples in `/examples/`
- All integration tests
- All benchmarks
- All documentation

**We don't care.** This is proof-of-concept only.

## Success Criteria
1. **FanIn processor works with dual channels**
2. **Error channel receives structured errors** 
3. **Success channel operates independently**
4. **Tests pass and demonstrate the pattern**
5. **No goroutine leaks in basic usage**

## Time Estimate: 4-6 hours
- Interface design: 30 min
- FanIn implementation: 90 min  
- Test implementation: 120 min
- Validation/debugging: 30-60 min

## Validation Tests Required
```go
func TestFanIn_DualChannel_SuccessOnly(t *testing.T)     // Happy path
func TestFanIn_DualChannel_ErrorsOnly(t *testing.T)     // Error path  
func TestFanIn_DualChannel_Mixed(t *testing.T)          // Mixed scenario
func TestFanIn_DualChannel_Cancellation(t *testing.T)   // Context handling
func TestFanIn_DualChannel_ErrorMetadata(t *testing.T)  // Error structure
```

## Next Phase (Not This Implementation)
After proving the concept:
- Implement pattern for all other processors
- Add error recovery strategies  
- Update examples and documentation
- Add integration tests
- Performance benchmarking

But for now: **Just make FanIn work with dual channels.**