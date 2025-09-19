# Throttle Processor Redesign Plan

## Executive Summary

The FakeClock implementation successfully eliminated timer delivery races, but revealed a deeper architectural issue: **select non-determinism** in the throttle processor. When both timer and input channels are ready simultaneously, Go's select statement chooses randomly, causing race conditions in timer-based state machines.

## Root Cause Analysis

### The Select Non-Determinism Problem

Current throttle implementation uses a single select with multiple ready channels:

```go
// throttle.go:65 - PROBLEMATIC PATTERN
select {
case result, ok := <-in:    // Input channel
    // Process input (may drop if cooling)
case <-timerC:               // Timer expiry  
    cooling = false          // End cooling period
case <-ctx.Done():
    return
}
```

**Race Sequence:**
1. Timer expires → `timerC` becomes ready
2. Item sent → `in` becomes ready  
3. **Non-deterministic choice**: Go runtime picks randomly
4. If `in` chosen first → Item dropped while still cooling
5. If `timerC` chosen first → Cooling ends, item processes correctly

This creates a race condition where correctness depends on unpredictable runtime behavior.

### Impact on Test Reliability

The test `TestThrottle_CoolingPeriodBehavior` demonstrates the issue:

```go
clock.Advance(50 * time.Millisecond)  // Timer expires
clock.BlockUntilReady()               // Timer ready to fire
in <- NewSuccess(6)                   // Input becomes ready
// RACE: Both channels ready, select picks randomly
```

When select chooses input first, the item is dropped and test hangs waiting for output.

## Solution Architecture

### Design Principle: Deterministic Timer Processing

The solution ensures timer state transitions complete before processing new inputs. This eliminates race conditions by making timer expiry deterministic.

### Implementation Strategy: Two-Phase Select

Replace single select with explicit timer drain followed by input processing:

```go
// Phase 1: Check timer expiry (deterministic)
select {
case <-timerC:
    cooling = false
    timer = nil
    timerC = nil
default:
    // No timer ready, continue
}

// Phase 2: Process input with current state
select {  
case result, ok := <-in:
    // Process with known cooling state
case <-ctx.Done():
    return
}
```

This ensures timer state transitions always happen before input processing.

## Detailed Implementation Plan

### Phase 1: Core Algorithm Redesign

**File:** `/home/zoobzio/code/streamz/throttle.go`

**Changes Required:**

1. **Replace Single Select Loop**
   - Current: One select with all channels
   - New: Two-phase select pattern

2. **Timer Drain Logic**
   ```go
   // Check if timer has expired (non-blocking)
   if timerC != nil {
       select {
       case <-timerC:
           cooling = false
           timer = nil
           timerC = nil
       default:
           // Timer not ready, keep current state
       }
   }
   ```

3. **Input Processing with Known State**
   ```go
   select {
   case result, ok := <-in:
       if !ok {
           if timer != nil {
               timer.Stop()
           }
           return
       }
       // Process with deterministic cooling state
   case <-ctx.Done():
       if timer != nil {
           timer.Stop()
       }
       return
   }
   ```

### Phase 2: State Management Improvements

**Timer Lifecycle Management:**
- Initialize: `timer = nil`, `timerC = nil`, `cooling = false`
- Start cooling: Create timer only when needed
- End cooling: Clear all timer references atomically
- Cleanup: Stop active timer on shutdown

**State Transitions:**
```go
// Start cooling (after emitting item)
cooling = true
timer = th.clock.NewTimer(th.duration)  
timerC = timer.C()

// End cooling (after timer fires)
cooling = false
timer = nil
timerC = nil
```

### Phase 3: Performance Considerations

**Blocking Behavior:**
- Phase 1 timer check is non-blocking (won't hang)
- Phase 2 input processing maintains blocking semantics
- Overall behavior unchanged for users

**CPU Usage:**
- No busy loops (phase 2 still blocks on channels)
- Minimal overhead (one extra select per iteration)
- No goroutine proliferation

## Implementation Details

### New Process Method Structure

```go
func (th *Throttle[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])
    
    go func() {
        defer close(out)
        
        var cooling bool
        var timer Timer
        var timerC <-chan time.Time
        
        for {
            // Phase 1: Deterministic timer processing
            if timerC != nil {
                select {
                case <-timerC:
                    // Cooling period expired
                    cooling = false
                    timer = nil
                    timerC = nil
                default:
                    // Timer not ready, continue with current state
                }
            }
            
            // Phase 2: Input processing with known cooling state
            select {
            case result, ok := <-in:
                if !ok {
                    // Clean up on input close
                    if timer != nil {
                        timer.Stop()
                    }
                    return
                }
                
                // Handle errors (pass through immediately)
                if result.IsError() {
                    select {
                    case out <- result:
                    case <-ctx.Done():
                        return
                    }
                    continue
                }
                
                // Handle success values based on current cooling state
                if !cooling {
                    // Emit item and start cooling
                    select {
                    case out <- result:
                        cooling = true
                        timer = th.clock.NewTimer(th.duration)
                        timerC = timer.C()
                    case <-ctx.Done():
                        return
                    }
                }
                // If cooling, drop item (leading edge behavior)
                
            case <-ctx.Done():
                if timer != nil {
                    timer.Stop()  
                }
                return
            }
        }
    }()
    
    return out
}
```

### Key Design Properties

1. **Deterministic Timer Processing**: Timer always checked first
2. **Non-blocking Timer Check**: Won't hang if timer not ready
3. **Preserved Semantics**: Same external behavior as current implementation  
4. **Clean State Management**: Timer references cleared atomically
5. **Context Cancellation**: Proper cleanup on shutdown

## Testing Strategy

### Unit Test Updates

**File:** `/home/zoobzio/code/streamz/throttle_test.go`

**No test changes required** - the redesign maintains identical external behavior. All existing tests should pass without modification.

**Critical Test:** `TestThrottle_CoolingPeriodBehavior` should pass reliably after fix.

### Race Detection

All tests must pass with `-race` flag:
```bash
go test -race ./... 
```

The two-phase select eliminates the race condition that causes non-deterministic test failures.

### Performance Testing

Verify no performance regression:
- Benchmark existing throttle behavior
- Compare before/after implementation
- CPU usage should be equivalent
- Memory allocation patterns unchanged

## Risk Assessment

### Low Risk Changes

1. **API Compatibility**: External interface unchanged
2. **Behavior Preservation**: Same leading-edge throttling semantics
3. **Error Handling**: Identical error passthrough behavior

### Mitigation Strategies

1. **Comprehensive Testing**: All existing tests must pass
2. **Performance Validation**: Benchmark comparison required
3. **Race Detection**: Continuous testing with `-race` flag
4. **Rollback Plan**: Git commit allows easy reversion

## Validation Criteria

### Success Metrics

1. **Test Reliability**: `TestThrottle_CoolingPeriodBehavior` passes 100 consecutive runs
2. **Race Detection**: All tests pass with `-race` flag
3. **Performance**: No significant regression in benchmarks
4. **API Compatibility**: No breaking changes to public interface

### Failure Indicators

1. Any existing test fails after implementation
2. Race detector reports new issues
3. Performance degrades measurably
4. External behavior changes unexpectedly

## Pattern Implications

### Other Affected Processors

This select non-determinism pattern likely exists in other timer-based processors:

1. **Debounce**: Timer vs input race conditions
2. **Window processors**: Timer expiry vs data arrival races  
3. **Retry mechanisms**: Timer vs success/failure races

Each should be evaluated for similar issues after throttle fix proves the pattern.

### Architectural Guidance

**Timer-Based State Machine Pattern:**
- Always drain timers before processing inputs
- Use non-blocking timer checks for determinism
- Maintain blocking semantics for input processing
- Clear timer references atomically

This pattern can be generalized across the streamz library for consistent timer handling.

## Implementation Timeline

1. **Immediate**: Implement throttle redesign
2. **Validation**: Comprehensive testing with race detection
3. **Performance**: Benchmark comparison
4. **Documentation**: Update any affected code comments
5. **Future**: Evaluate pattern in other processors

## Conclusion

The select non-determinism issue is a fundamental problem with timer-based state machines in Go. The two-phase select pattern provides a clean, deterministic solution while maintaining all existing behavior and performance characteristics.

This fix eliminates the race condition causing test hangs and provides a reusable pattern for other timer-based processors in the streamz library.