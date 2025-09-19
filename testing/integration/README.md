# Integration Tests - StreamZ

## Purpose

Integration tests prove that StreamZ processors compose correctly and that the Result[T] pattern works across different processor types. These tests focus on real-world pipeline scenarios rather than isolated processor behavior.

## Test Files

### `result_composability_test.go`
Tests the Result[T] pattern across different processor characteristics:

- **FanIn + FanOut Combination**: Proves Result[T] works for both many-to-1 and 1-to-many patterns
- **Error Propagation**: Verifies errors maintain context through complex pipelines
- **Complex Flow**: Multi-stage pipelines demonstrating real-world usage patterns

## Key Test Scenarios

### TestResultComposability_FanInFanOut
- Creates multiple input streams with mixed success/error Results
- Merges them with FanIn (many-to-1)
- Distributes with FanOut (1-to-many) 
- Verifies all outputs receive identical sequences
- Proves Result[T] composability across processor types

### TestResultComposability_ErrorPropagation  
- Tests error context preservation through FanIn/FanOut pipeline
- Verifies error metadata (ProcessorName, Item, Timestamp) survives
- Ensures errors don't corrupt successful values in the same stream

### TestResultComposability_ComplexFlow
- Multi-stage pipeline simulating real data processing
- Multiple sources -> merge -> distribute -> process -> recombine
- Demonstrates Result[T] pattern scales to complex scenarios

## Running Integration Tests

```bash
# Run all integration tests
go test ./testing/integration/

# Run specific Result[T] composability tests
go test ./testing/integration/ -run TestResultComposability

# With verbose output to see pipeline progression
go test -v ./testing/integration/

# With race detection (important for concurrent pipeline testing)
go test -race ./testing/integration/
```

## What These Tests Prove

1. **Result[T] Universality**: The pattern works regardless of processor type (1-to-1, many-to-1, 1-to-many)
2. **Error Isolation**: Errors in one stream don't affect other streams
3. **Context Preservation**: Error metadata survives complex pipeline transformations  
4. **Performance**: Concurrent processing doesn't introduce race conditions
5. **Resource Safety**: Complex pipelines clean up properly (no goroutine leaks)

## Adding New Integration Tests

When adding new processors or Result[T] capabilities:

1. **Extend Existing Tests**: Add new processor to existing composability tests
2. **New Scenarios**: Only create new test functions for fundamentally different patterns
3. **Real-World Focus**: Test patterns developers will actually use
4. **Error Cases**: Include failure scenarios, not just happy paths

Integration tests should tell the story of what StreamZ can do in production, not just verify individual functions work.