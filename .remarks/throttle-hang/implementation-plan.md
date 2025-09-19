# FakeClock Timer Synchronization - Implementation Plan

## Context

Based on RAINMAN's diagnosis and design analysis, implementing concrete solution for timer synchronization race condition in FakeClock. Current `BlockUntilReady()` only tracks `AfterFunc` callbacks, ignoring timer channel deliveries, causing race conditions in tests.

## Architecture Decision

**Selected Approach:** Simplified Channel Delivery Tracking
- Add dedicated `sync.WaitGroup` for timer channel deliveries
- Replace non-blocking sends with tracked goroutines
- Extend `BlockUntilReady()` to wait for both callbacks and channel deliveries
- Maintain existing Clock interface compatibility

Rejection of alternatives:
- Sleep-based workarounds: 10-20% failure rate even with 10ms delays
- Per-waiter tracking: Too complex for the benefit gained
- Priority select patterns: Changes processor semantics unnecessarily

## Implementation Steps

### Phase 1: Core Synchronization Changes

#### Step 1.1: Add Delivery Tracking Field
File: `/home/zoobzio/code/streamz/clock_fake.go`

Add new field to FakeClock struct after existing fields:
```go
type FakeClock struct {
    mu          sync.RWMutex
    wg          sync.WaitGroup
    time        time.Time
    waiters     []*waiter
    deliveryWg  sync.WaitGroup  // NEW: Tracks timer channel deliveries
}
```

#### Step 1.2: Create Timer Delivery Helper
Add new method after `setTimeLocked()`:
```go
// deliverTimerValue sends a timer value on a channel with delivery tracking.
// Caller must hold f.mu lock.
func (f *FakeClock) deliverTimerValue(ch chan time.Time, value time.Time) {
    f.deliveryWg.Add(1)
    go func() {
        defer f.deliveryWg.Done()
        ch <- value  // Blocking send ensures delivery
    }()
}
```

**Key Design Points:**
- Blocking send guarantees delivery (no `select` with `default`)
- Goroutine ensures no deadlock if receiver not ready
- `deliveryWg.Add(1)` called before goroutine launch (prevents race)
- Each timer send tracked individually

#### Step 1.3: Update setTimeLocked() Channel Sends
Replace non-blocking sends with tracked calls:

**Current code (lines 182-185):**
```go
if w.destChan != nil {
    select {
    case w.destChan <- t:
    default:
    }
}
```

**New code:**
```go
if w.destChan != nil {
    f.deliverTimerValue(w.destChan, t)
}
```

**Current ticker code (lines 200-203):**
```go
select {
case w.destChan <- w.targetTime:
default:
}
```

**New ticker code:**
```go
f.deliverTimerValue(w.destChan, w.targetTime)
```

#### Step 1.4: Extend BlockUntilReady()
**Current implementation (lines 160-162):**
```go
func (f *FakeClock) BlockUntilReady() {
    f.wg.Wait()
}
```

**New implementation:**
```go
func (f *FakeClock) BlockUntilReady() {
    f.wg.Wait()        // Wait for AfterFunc callbacks
    f.deliveryWg.Wait() // Wait for timer channel deliveries
}
```

### Phase 2: Unit Tests for Verification

#### Step 2.1: Create Synchronization Test
File: `/home/zoobzio/code/streamz/clock_fake_test.go`

Add test after existing tests:
```go
func TestFakeClock_BlockUntilReady_TimerDelivery(t *testing.T) {
    t.Run("single timer", func(t *testing.T) {
        clock := NewFakeClock()
        timer := clock.NewTimer(100 * time.Millisecond)
        
        // Advance clock
        clock.Advance(100 * time.Millisecond)
        
        // BlockUntilReady must guarantee timer delivery
        clock.BlockUntilReady()
        
        // Timer should be ready immediately
        select {
        case <-timer.C():
            // Success
        default:
            t.Fatal("Timer not delivered after BlockUntilReady()")
        }
    })
    
    t.Run("multiple timers", func(t *testing.T) {
        clock := NewFakeClock()
        timer1 := clock.NewTimer(50 * time.Millisecond)
        timer2 := clock.NewTimer(100 * time.Millisecond)
        
        clock.Advance(100 * time.Millisecond)
        clock.BlockUntilReady()
        
        // Both timers should be ready
        select {
        case <-timer1.C():
        default:
            t.Fatal("Timer1 not delivered")
        }
        
        select {
        case <-timer2.C():
        default:
            t.Fatal("Timer2 not delivered")
        }
    })
    
    t.Run("ticker multiple ticks", func(t *testing.T) {
        clock := NewFakeClock()
        ticker := clock.NewTicker(50 * time.Millisecond)
        defer ticker.Stop()
        
        clock.Advance(150 * time.Millisecond) // 3 ticks
        clock.BlockUntilReady()
        
        // Should have 3 ticks ready
        for i := 0; i < 3; i++ {
            select {
            case <-ticker.C():
                // Expected
            default:
                t.Fatalf("Tick %d not delivered", i+1)
            }
        }
    })
}
```

#### Step 2.2: Race Condition Verification Test
```go
func TestFakeClock_NoRaceCondition(t *testing.T) {
    // Reproduces the original throttle test race
    for i := 0; i < 100; i++ { // Run multiple times
        t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
            clock := NewFakeClock()
            timer := clock.NewTimer(50 * time.Millisecond)
            
            // The problematic sequence
            clock.Advance(50 * time.Millisecond)
            clock.BlockUntilReady()
            
            // This must not race with timer delivery
            ready := make(chan bool, 1)
            go func() {
                select {
                case <-timer.C():
                    ready <- true
                case <-time.After(time.Second):
                    ready <- false
                }
            }()
            
            if !<-ready {
                t.Fatal("Race condition: timer not delivered within 1 second")
            }
        })
    }
}
```

### Phase 3: Integration Test Fix

#### Step 3.1: Update Throttle Test
File: `/home/zoobzio/code/streamz/throttle_test.go`

**Current problematic code (around line 518):**
```go
clock.Advance(50 * time.Millisecond)
in <- NewSuccess(6)
```

**Fixed code:**
```go
clock.Advance(50 * time.Millisecond)
clock.BlockUntilReady() // Ensure timer processed before sending item
in <- NewSuccess(6)
```

#### Step 3.2: Verify Fix with Stress Test
Create temporary test to verify fix eliminates race:
```go
func TestThrottle_NoRaceAfterFix(t *testing.T) {
    // Run the exact sequence that was failing
    for run := 0; run < 1000; run++ {
        clock := NewFakeClock()
        throttle := NewThrottle[Result](50*time.Millisecond, clock)
        
        in := make(chan Result)
        out := throttle.Process(context.Background(), in)
        
        // Send initial item to start cooling
        in <- NewSuccess(1)
        <-out
        
        // The race sequence
        clock.Advance(50 * time.Millisecond)
        clock.BlockUntilReady() // This should eliminate race
        in <- NewSuccess(2)
        
        // This should not hang
        select {
        case <-out:
            // Success
        case <-time.After(100 * time.Millisecond):
            t.Fatalf("Hang detected on run %d", run)
        }
        
        close(in)
    }
}
```

### Phase 4: Documentation Updates

#### Step 4.1: Update FakeClock Godocs
Add synchronization behavior documentation:
```go
// BlockUntilReady blocks until all pending operations have completed.
// This includes both AfterFunc callbacks and timer channel deliveries.
// Use this method to ensure deterministic timing in tests after calling
// Advance() or SetTime().
//
// Example:
//   clock.Advance(duration)
//   clock.BlockUntilReady()  // Guarantees timers have fired
//   // Now safe to check timer channels or send additional data
func (f *FakeClock) BlockUntilReady() {
    f.wg.Wait()
    f.deliveryWg.Wait()
}
```

#### Step 4.2: Add Testing Guide Comments
Add to `/home/zoobzio/code/streamz/docs/guides/testing.md`:
```markdown
## Timer Synchronization in Tests

When using FakeClock with timer-based processors (throttle, debounce, batcher):

```go
// WRONG - Race condition possible
clock.Advance(duration)
input <- data  // May race with timer processing

// CORRECT - Deterministic
clock.Advance(duration) 
clock.BlockUntilReady()  // Wait for timer delivery
input <- data  // Timer already processed
```
```

## Performance Analysis

### Memory Overhead
- Additional `sync.WaitGroup` field: 24 bytes
- Per-timer goroutine: ~8KB stack (short-lived)
- Minimal impact on test suites

### CPU Overhead
- One goroutine per timer channel send
- Synchronized waits in `BlockUntilReady()`
- Estimated <5% overhead for timer-heavy tests

### Benchmark Verification
```go
func BenchmarkFakeClock_TimerDelivery(b *testing.B) {
    clock := NewFakeClock()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        timer := clock.NewTimer(time.Millisecond)
        clock.Advance(time.Millisecond)
        clock.BlockUntilReady()
        <-timer.C()
    }
}
```

## Risk Mitigation

### Deadlock Prevention
- Timer delivery uses blocking send in goroutine
- No locks held during channel operations
- WaitGroup ensures proper goroutine lifecycle

### Backward Compatibility
- No interface changes to Clock
- Existing tests unchanged (except for the fix)
- Only affects FakeClock behavior, not RealClock

### Error Conditions
- Clock advancement backward still panics
- Timer Stop()/Reset() behavior unchanged
- Ticker period behavior identical

## Verification Checklist

### Phase 1: Core Implementation
- [ ] Add `deliveryWg sync.WaitGroup` field to FakeClock
- [ ] Implement `deliverTimerValue()` helper method
- [ ] Replace non-blocking sends in `setTimeLocked()`
- [ ] Update `BlockUntilReady()` to wait for deliveries
- [ ] Verify compilation without errors

### Phase 2: Unit Tests
- [ ] Create `TestFakeClock_BlockUntilReady_TimerDelivery()`
- [ ] Add race condition verification test
- [ ] Run tests with `-race` flag
- [ ] All tests pass consistently

### Phase 3: Integration Fix
- [ ] Update `throttle_test.go` line ~518
- [ ] Create stress test for 1000 iterations
- [ ] Zero hangs in stress test
- [ ] Remove any artificial sleep delays

### Phase 4: System Verification
- [ ] Run full test suite: `make test`
- [ ] Run with race detection: `go test -race ./...`
- [ ] Performance benchmarks show <10% overhead
- [ ] No flaky test failures in CI

## Success Metrics

1. **Correctness**: Throttle test passes 1000 consecutive runs
2. **Determinism**: Zero race conditions in timer synchronization tests
3. **Performance**: Timer delivery overhead <5% in benchmarks
4. **Compatibility**: All existing tests pass without modification
5. **Reliability**: No flaky failures related to FakeClock timing

## Files Modified

**Core Implementation:**
- `/home/zoobzio/code/streamz/clock_fake.go` - Add synchronization
- `/home/zoobzio/code/streamz/throttle_test.go` - Fix race condition

**Testing:**
- `clock_fake_test.go` - Add synchronization tests
- Temporary stress tests for verification

**Documentation:**
- `docs/guides/testing.md` - Update timing guidance
- Inline godocs for `BlockUntilReady()`

## Implementation Priority

**Critical Path:**
1. Core FakeClock changes (deliveryWg, deliverTimerValue, BlockUntilReady)
2. Unit tests for synchronization behavior  
3. Fix throttle_test.go race condition
4. Stress test verification

**Follow-up:**
5. Documentation updates
6. Performance benchmarks
7. Audit other timer-based tests

This sequence minimizes risk while providing immediate race condition fix for the failing test. Each phase builds verification confidence before proceeding to integration.

## Expected Outcome

After implementation:
- `TestThrottle_CoolingPeriodBehavior` passes reliably 
- No artificial sleep delays needed in tests
- Deterministic timer behavior in all FakeClock tests
- Foundation for reliable timer-based processor testing
- Zero race conditions in timer synchronization

The fix addresses the root cause rather than symptoms, providing robust foundation for all timer-dependent tests in the streamz package.