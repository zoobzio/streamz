# Testing Documentation for Streamz

Welcome to the comprehensive testing guide for the Streamz stream processing library. Testing is a first-class citizen in this project - we believe that robust testing is essential for building reliable stream processing pipelines.

This document serves as the authoritative guide for:
- **Contributors**: Learn how to write and run tests for your contributions
- **Maintainers**: Understand our testing standards and quality gates
- **Users**: Gain confidence in the reliability of the library

## Testing Architecture Overview

The Streamz testing architecture follows Go best practices with a clear separation between different types of tests:

### Directory Structure

```
streamz/
├── *_test.go              # Unit tests (test individual processors)
└── testing/               # All non-unit tests
    ├── helpers/           # Shared test utilities
    ├── integration/       # End-to-end pipeline tests  
    ├── benchmarks/        # Performance tests
    └── reliability/       # Stress and concurrency tests
```

## Test Categories

### 1. Unit Tests (Root Directory)

**Purpose**: Test individual processors in isolation  
**Location**: `*_test.go` files alongside source files  
**Coverage Goal**: >95% for core processors, >90% for utilities

**Characteristics**:
- Fast execution (< 100ms per test)
- No external dependencies
- Deterministic results
- Focus on single processor behavior
- Comprehensive edge case coverage

**Example**:
```go
func TestMapperBasicFunctionality(t *testing.T) {
    // Test single mapper processor
}
```

### 2. Integration Tests

**Purpose**: Test complete pipelines and processor interactions  
**Location**: `testing/integration/`  
**Focus**: Real-world workflows and processor composition

**Test Types**:
- Pipeline composition tests
- Error propagation validation
- Complex workflow scenarios
- Windowing and batching integration

**Example Scenarios**:
- Data processing pipelines (filter → transform → batch → aggregate)
- Event processing workflows  
- Error recovery through multi-stage pipelines

### 3. Reliability Tests

**Purpose**: Verify robustness under stress and concurrent access  
**Location**: `testing/reliability/`  
**Requirements**: Must pass with `-race` flag

**Test Types**:
- Concurrent access safety
- High-load stress testing
- Resource leak detection
- Error recovery under stress
- Long-running scenarios

**Key Features**:
- Race condition detection
- Goroutine leak monitoring
- Memory usage validation
- Performance under load

### 4. Benchmark Tests

**Purpose**: Performance measurement and regression detection  
**Location**: `testing/benchmarks/`  
**Execution**: `go test -bench=.`

**Benchmark Categories**:
- Single processor throughput
- Pipeline composition overhead
- Memory allocation patterns
- Concurrent processing scalability

## Test Execution

### Running All Tests

```bash
# All tests with coverage
go test -v -race -coverprofile=coverage.out ./...

# Unit tests only
go test -v ./

# Integration tests
go test -v ./testing/integration/...

# Reliability tests (with race detector)
go test -v -race ./testing/reliability/...

# Benchmarks
go test -v -bench=. ./testing/benchmarks/...

# Stress tests (long-running, skipped in short mode)
go test -v -timeout=30m ./testing/reliability/...
```

### Coverage Analysis

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Quality Standards

### Test Requirements

1. **Deterministic**: No flaky tests or timing dependencies
2. **Fast**: Unit tests < 100ms, integration tests < 5s
3. **Isolated**: No shared state between tests
4. **Comprehensive**: Cover happy paths, edge cases, and error conditions
5. **Race-safe**: All tests must pass with `-race` flag

### Coverage Targets

- Core processors: >95%
- Utility functions: >90%
- Integration workflows: >85%
- Error paths: >80%

### Naming Conventions

- Unit tests: `TestProcessorName_Scenario`
- Integration tests: `TestPipeline*`, `TestWorkflow*`
- Reliability tests: `TestRace*`, `TestStress*`, `TestResourceLeak*`
- Benchmarks: `BenchmarkProcessor*`, `BenchmarkPipeline*`

## Test Helpers

The `testing/helpers/` package provides reusable utilities:

### Data Generation
- `GenerateSequence(start, end)` - Create test data channels
- `GenerateSequenceSlice(start, end)` - Create test data slices

### Collection and Assertion
- `CollectAll[T](ch)` - Consume all channel values
- `AssertStreamEqual[T](expected, actual)` - Compare stream results
- `AssertStreamContains[T](expected, actual)` - Verify item presence

### Test Processors
- `TestProcessor[T]` - Configurable processor for testing
- `Identity[T]`, `Double`, `IsEven`, `IsOdd` - Common transform functions

### Concurrency Utilities
- `ConcurrentCollector[T]` - Thread-safe result collection
- `CollectAllWithTimeout` - Collection with timeout protection

## Error Scenarios

### Tested Error Conditions

1. **Context Cancellation**: Proper cleanup when context is cancelled
2. **Channel Closure**: Graceful handling of closed input channels  
3. **Processor Failures**: Error propagation through pipelines
4. **Resource Exhaustion**: Behavior under memory/CPU pressure
5. **Race Conditions**: Concurrent access to shared resources

### Error Recovery Patterns

- Graceful degradation under high load
- Proper resource cleanup on failures
- Error isolation between processors
- Recovery from transient failures

## Performance Baselines

### Throughput Benchmarks
- Single processor: >100k ops/sec
- Simple pipeline: >50k ops/sec  
- Complex pipeline: >10k ops/sec

### Latency Targets
- Single processor: <10μs p99
- Pipeline (3 stages): <50μs p99
- End-to-end workflow: <1ms p99

### Resource Usage
- Memory: <10MB for standard workloads
- Goroutines: No leaks after processing
- Channels: Proper cleanup and garbage collection

## Continuous Integration

### Pre-commit Checks
```bash
# Formatting
gofmt -w .

# Linting  
golangci-lint run

# Tests
go test -race -short ./...

# Coverage check
go test -coverprofile=coverage.out ./...
```

### CI Pipeline
1. Code formatting verification
2. Linting with comprehensive rules
3. Unit tests with race detection
4. Integration test suite
5. Benchmark regression testing
6. Coverage reporting

## Test Data Management

### Test Data Principles
- Deterministic data generation
- Minimal external dependencies
- Self-contained test scenarios
- Realistic data patterns

### Mock Strategy
- Mock external dependencies at boundaries
- Use interfaces for testability
- Avoid mocking internal components
- Focus on behavior verification

## Maintenance

### Test Maintenance Guidelines
- Update tests with API changes
- Remove obsolete test scenarios
- Refactor shared test utilities
- Monitor test execution time
- Review flaky test reports

### Performance Monitoring
- Track benchmark trends
- Monitor resource usage patterns
- Identify performance regressions
- Optimize slow test scenarios

## Contributing Tests

When contributing to Streamz, please ensure your code includes:

1. **Unit Tests**: Test your processor in isolation with various inputs
2. **Integration Tests**: If your change affects pipelines, add integration tests
3. **Benchmarks**: For performance-critical code, add benchmark tests
4. **Documentation**: Update this guide if you introduce new testing patterns

All contributions must maintain or improve our test coverage metrics.

## Getting Help

If you have questions about testing:
- Check the existing test examples in the codebase
- Review the test helpers in `testing/helpers/`
- Open an issue for testing-related questions
- Consult the main [README](README.md) for project overview

---

This testing strategy ensures comprehensive coverage while maintaining fast feedback cycles and reliable results. Testing is not an afterthought - it's integral to building robust stream processing pipelines.