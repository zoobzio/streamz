# Testing Strategy - StreamZ

## Overview

This directory contains comprehensive tests for the StreamZ streaming framework, organized by test type and purpose. Our testing strategy focuses on proving processor composability, Result[T] pattern effectiveness, and real-world reliability.

## Directory Structure

```
testing/
├── README.md              # This file - testing strategy overview
├── integration/           # Integration tests proving processor composability  
│   ├── README.md         # Integration test documentation
│   └── result_composability_test.go  # Tests proving Result[T] pattern works across processor types
└── helpers/              # Shared test utilities (if needed)
    └── README.md         # Test helper documentation
```

## Test Categories

### Unit Tests
**Location**: Root directory (`*_test.go` files alongside source)
**Purpose**: Test individual processors in isolation
**Coverage**: Each `.go` file has corresponding `_test.go` file
**Focus**: 
- Individual processor behavior
- Error handling within single processors  
- Resource cleanup and goroutine safety
- Performance benchmarks

### Integration Tests
**Location**: `testing/integration/`
**Purpose**: Test processor combinations and Result[T] composability
**Focus**:
- Processor chaining (FanIn -> FanOut, etc.)
- Result[T] pattern across different processor types
- Complex pipeline scenarios
- Error propagation through multi-stage pipelines

## Test Execution

### Run All Tests
```bash
# Unit tests + integration tests
go test ./...

# With race detection
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run Specific Test Categories
```bash
# Unit tests only (root directory)
go test .

# Integration tests only
go test ./testing/integration/

# Specific integration test
go test ./testing/integration/ -run TestResultComposability
```

### Performance Testing
```bash
# Run benchmarks
go test -bench=. ./...

# Memory allocation profiling
go test -bench=. -benchmem ./...
```

## Testing Principles

1. **Prove Composability**: Tests must demonstrate that processors work together correctly
2. **Error Propagation**: Verify errors flow correctly through complex pipelines  
3. **Resource Safety**: No goroutine leaks, proper channel cleanup
4. **Real-World Scenarios**: Test patterns that developers actually use
5. **Result[T] Pattern**: Prove unified error handling works across all processor types

## Result[T] Testing Strategy

The Result[T] pattern is being validated across different processor characteristics:

- **Many-to-1**: FanIn merges multiple Result[T] streams
- **1-to-Many**: FanOut distributes Result[T] to multiple outputs
- **1-to-1**: Standard processors (Mapper, Filter) transform Result[T]
- **Complex Flows**: Combinations proving full composability

## Coverage Goals

- **Unit Tests**: >95% for individual processors
- **Integration Tests**: Cover all major processor combination patterns
- **Error Paths**: Every error condition should have test coverage
- **Resource Safety**: All goroutine and channel lifecycle scenarios

## Adding New Tests

When adding new processors or features:

1. **Unit Tests**: Always create corresponding `*_test.go` file in root
2. **Integration Tests**: Add to existing integration test files if they enhance the narrative
3. **New Test Files**: Only create new integration test files for major new capabilities
4. **Documentation**: Update this README if adding new test categories

Remember: Tests tell the story of what the package can do. Make sure your tests contribute to a coherent narrative about StreamZ capabilities.