# FromChainable Implementation - Detailed Dry Run Task List

## Overview
This document outlines the complete step-by-step implementation plan for adding the FromChainable adapter and migrating Mapper, Filter, and Tap processors to use pipz as their foundation.

**Key Constraints:**
- pipz.Filter has different semantics than streamz.Filter (need custom implementation)
- Need sentinel errors (ErrFilteredOut, ErrSkipItem) for flow control
- Must follow existing streamz patterns and standards
- Each file needs full testing before moving to the next
- Maintain backward compatibility and existing API surface

## Phase 1: Foundation Setup

### Task 1.1: Add pipz dependency
```bash
# Commands to run:
cd /home/zoobzio/code/streamz
go get github.com/zoobzio/pipz@latest
go mod tidy
```

**Files modified:** `/home/zoobzio/code/streamz/go.mod`, `/home/zoobzio/code/streamz/go.sum`

**Verification:**
```bash
go list -m github.com/zoobzio/pipz
# Should show version information
```

### Task 1.2: Create sentinel errors
**File to create:** `/home/zoobzio/code/streamz/errors.go`

**Content overview:**
- Package-level sentinel errors
- ErrFilteredOut for items that should be skipped
- ErrSkipItem for tap operations that should be skipped
- Godoc comments explaining when each error is used

**Package imports needed:**
```go
import (
    "errors"
)
```

**Test command:**
```bash
go test -run TestSentinelErrors ./...
# Should pass basic error identity checks
```

**Verification steps:**
- [ ] Errors are properly exported
- [ ] Error messages are clear and descriptive
- [ ] errors.Is() works correctly with sentinel errors

### Task 1.3: Create FromChainable adapter
**File to create:** `/home/zoobzio/code/streamz/adapter.go`

**Content overview:**
- FromChainable[T] function that wraps pipz.Chainable[T]
- Proper context handling and cancellation
- Channel management (creation, closing, goroutine cleanup)
- Error handling for sentinel errors vs real errors
- Name extraction from pipz processors

**Package imports needed:**
```go
import (
    "context"
    "errors"
    "github.com/zoobzio/pipz"
)
```

**Test command:**
```bash
go test -run TestFromChainable ./...
# Should pass all adapter functionality tests
```

**Verification steps:**
- [ ] Context cancellation works properly
- [ ] Channels are closed correctly
- [ ] No goroutine leaks
- [ ] Sentinel errors are handled without breaking the stream
- [ ] Real errors are propagated appropriately
- [ ] Name() method works for both named and unnamed pipz processors

## Phase 2: Core Processor Migration

### Task 2.1: Migrate Mapper processor
**File to modify:** `/home/zoobzio/code/streamz/mapper.go`

**Implementation approach:**
1. Keep existing struct and API unchanged
2. Replace Process() method implementation to use FromChainable + pipz.Transform
3. Maintain all existing behavior and documentation

**Key changes:**
- Import pipz package
- Replace goroutine-based implementation with FromChainable wrapper
- Use pipz.Transform for the actual mapping logic
- Keep WithName() fluent API
- Preserve all existing godoc comments

**Package imports to add:**
```go
import (
    "github.com/zoobzio/pipz"
)
```

**Test commands:**
```bash
# Run existing mapper tests
go test -run TestMapper ./...

# Run mapper benchmarks
go test -bench=BenchmarkMapper -benchmem ./...

# Test with race detector
go test -race -run TestMapper ./...
```

**Verification steps:**
- [ ] All existing Mapper tests pass
- [ ] Performance is maintained (benchmark comparison)
- [ ] No race conditions
- [ ] API surface unchanged
- [ ] Documentation accurate

### Task 2.2: Create custom pipz Filter implementation
**File to create:** `/home/zoobzio/code/streamz/internal_filter.go` (or similar)

**Content overview:**
Since pipz.Filter has different semantics, create a custom pipz.Chainable that:
- Takes a predicate function
- Returns ErrFilteredOut for items that don't match predicate
- Implements proper error handling
- Follows pipz patterns

**Implementation details:**
```go
type streamzFilter[T any] struct {
    predicate func(T) bool
    name      string
}

func (f *streamzFilter[T]) Process(ctx context.Context, input T) (T, error) {
    if f.predicate(input) {
        return input, nil
    }
    return input, ErrFilteredOut
}

func (f *streamzFilter[T]) Name() string {
    return f.name
}
```

**Test command:**
```bash
go test -run TestInternalFilter ./...
```

**Verification steps:**
- [ ] Predicate logic works correctly
- [ ] ErrFilteredOut is returned for filtered items
- [ ] Name() method works
- [ ] Implements pipz.Chainable[T] interface

### Task 2.3: Migrate Filter processor
**File to modify:** `/home/zoobzio/code/streamz/filter.go`

**Implementation approach:**
1. Keep existing struct and API unchanged
2. Replace Process() method to use FromChainable + custom filter
3. Maintain all existing behavior and documentation

**Key changes:**
- Import pipz package and internal filter
- Replace goroutine-based implementation with FromChainable wrapper
- Use custom streamzFilter for the actual filtering logic
- Keep WithName() fluent API
- Preserve all existing godoc comments

**Test commands:**
```bash
# Run existing filter tests
go test -run TestFilter ./...

# Run filter benchmarks  
go test -bench=BenchmarkFilter -benchmem ./...

# Test with race detector
go test -race -run TestFilter ./...
```

**Verification steps:**
- [ ] All existing Filter tests pass
- [ ] Performance is maintained
- [ ] No race conditions
- [ ] API surface unchanged
- [ ] Documentation accurate

### Task 2.4: Migrate Tap processor
**File to modify:** `/home/zoobzio/code/streamz/tap.go`

**Implementation approach:**
1. Keep existing struct and API unchanged
2. Replace Process() method to use FromChainable + pipz.Apply
3. Maintain all existing behavior and documentation

**Key changes:**
- Import pipz package
- Replace goroutine-based implementation with FromChainable wrapper
- Use pipz.Apply for the actual tap logic (side effect + pass through)
- Keep WithName() fluent API
- Add Name() method if missing
- Preserve all existing godoc comments

**pipz.Apply implementation:**
```go
pipz.Apply(m.name, func(ctx context.Context, item T) (T, error) {
    t.fn(item)  // Execute side effect
    return item, nil  // Pass through unchanged
})
```

**Test commands:**
```bash
# Run existing tap tests
go test -run TestTap ./...

# Test with race detector
go test -race -run TestTap ./...
```

**Verification steps:**
- [ ] All existing Tap tests pass
- [ ] Side effects execute properly
- [ ] Items pass through unchanged
- [ ] No race conditions
- [ ] API surface unchanged

## Phase 3: Testing and Validation

### Task 3.1: Update test files
**Files to modify:** 
- `/home/zoobzio/code/streamz/mapper_test.go`
- `/home/zoobzio/code/streamz/filter_test.go` 
- `/home/zoobzio/code/streamz/tap_test.go`

**Changes needed:**
- Add tests for new error handling paths
- Test FromChainable adapter directly
- Add tests for sentinel error behavior
- Verify context cancellation works properly

**New test patterns:**
```go
func TestMapperWithPipzIntegration(t *testing.T) {
    // Test that mapper works with other pipz-based processors
}

func TestFilterSentinelErrors(t *testing.T) {
    // Test that ErrFilteredOut is handled correctly
}

func TestFromChainableContextCancellation(t *testing.T) {
    // Test context cancellation behavior
}
```

### Task 3.2: Run comprehensive test suite
**Commands to run:**
```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run benchmarks and compare
go test -bench=. -benchmem ./... > before.bench
# (after changes)
go test -bench=. -benchmem ./... > after.bench
benchcmp before.bench after.bench

# Test coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run reliability tests
go test -run TestReliability ./testing/reliability/...
go test -run TestConcurrent ./testing/reliability/...
```

### Task 3.3: Integration testing
**Commands to run:**
```bash
# Test log-processing example
cd examples/log-processing
go test ./...
go run .

# Run integration tests
cd /home/zoobzio/code/streamz
go test ./testing/integration/...
```

**Verification steps:**
- [ ] All examples still work
- [ ] No performance regressions
- [ ] Integration tests pass
- [ ] No memory leaks or goroutine leaks

### Task 3.4: Documentation verification
**Files to check:**
- Godoc comments still accurate
- README examples still work
- API documentation reflects any changes

**Commands to run:**
```bash
# Generate and review docs
godoc -http=:6060 &
# Visit http://localhost:6060/pkg/github.com/zoobzio/streamz/

# Test README examples
go test -run TestReadmeExamples ./...
```

## Phase 4: Cleanup and Finalization

### Task 4.1: Code cleanup
**Actions:**
- Remove any unused imports
- Ensure consistent code style
- Run linters and fix any issues

**Commands to run:**
```bash
# Format code
go fmt ./...

# Run linters (if golangci-lint is available)
golangci-lint run

# Check for unused dependencies
go mod tidy
```

### Task 4.2: Final testing
**Comprehensive test run:**
```bash
# Full test suite with verbose output
go test -v ./...

# Race condition testing
go test -race -count=10 ./...

# Stress testing
go test -run=TestStress ./testing/reliability/ -timeout=30m

# Benchmark verification
go test -bench=. -count=5 ./...
```

### Task 4.3: Git operations (documentation only - DO NOT COMMIT)
**Commands that would be run:**
```bash
# Stage changes
git add .

# Review changes
git diff --cached

# Create commit (example message):
git commit -m "feat: migrate Mapper, Filter, Tap to use pipz foundation

- Add FromChainable adapter for pipz integration
- Migrate Mapper to use pipz.Transform via FromChainable
- Migrate Filter to use custom pipz.Chainable implementation
- Migrate Tap to use pipz.Apply via FromChainable
- Add sentinel errors ErrFilteredOut and ErrSkipItem
- Maintain full backward compatibility
- All existing tests pass
- No performance regressions

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

## Risk Mitigation

### Potential Issues and Solutions:

1. **Performance regressions**
   - Solution: Extensive benchmarking at each step
   - Rollback plan: Keep original implementations in git history

2. **Breaking changes**
   - Solution: Maintain exact same API surface
   - Verification: All existing tests must pass unchanged

3. **Context handling differences**
   - Solution: Careful testing of cancellation behavior
   - Test: Create specific tests for context edge cases

4. **Error handling changes**
   - Solution: Sentinel errors for flow control only
   - Test: Verify real errors still propagate correctly

5. **Goroutine leaks**
   - Solution: Proper channel closing and context handling
   - Test: Run with race detector and leak detection

## Success Criteria

- [ ] All existing tests pass without modification
- [ ] No performance regressions (< 5% slowdown acceptable)
- [ ] No API surface changes
- [ ] Documentation remains accurate
- [ ] Examples still work
- [ ] Race detector passes
- [ ] Integration tests pass
- [ ] Memory usage remains stable

## Timeline Estimate

- Phase 1: 2-3 hours
- Phase 2: 4-6 hours  
- Phase 3: 3-4 hours
- Phase 4: 1-2 hours

**Total: 10-15 hours**

This is a conservative estimate assuming careful testing at each step.