# Result[T] Metadata Implementation Plan

## Overview

Implementation plan for adding metadata field to Result[T] struct. This focuses exclusively on metadata capability - Window refactoring is a separate concern.

## Core Structural Changes

### 1. Result[T] Struct Modification

```go
type Result[T any] struct {
    value    T
    err      *StreamError[T]
    metadata map[string]interface{}  // NEW: nil by default
}
```

**Implementation Notes:**
- Field is unexported to maintain immutability
- Nil by default for zero overhead on non-metadata Results
- Map provides flexible key-value storage

### 2. Constructor Updates

Existing constructors remain unchanged for backward compatibility:

```go
func NewSuccess[T any](value T) Result[T] {
    return Result[T]{value: value}  // metadata remains nil
}

func NewError[T any](item T, err error, processorName string) Result[T] {
    return Result[T]{err: NewStreamError(item, err, processorName)}  // metadata remains nil
}
```

## Method Additions

### 3. WithMetadata Method (Immutable Pattern)

```go
// WithMetadata returns a new Result with the specified metadata key-value pair.
// This is an immutable operation - the original Result is unchanged.
// Multiple calls can be chained to add multiple metadata entries.
func (r Result[T]) WithMetadata(key string, value interface{}) Result[T] {
    if key == "" {
        return r  // Ignore empty keys
    }
    
    // Create new metadata map
    newMetadata := make(map[string]interface{})
    
    // Copy existing metadata if present
    if r.metadata != nil {
        for k, v := range r.metadata {
            newMetadata[k] = v
        }
    }
    
    // Add new entry
    newMetadata[key] = value
    
    return Result[T]{
        value:    r.value,
        err:      r.err,
        metadata: newMetadata,
    }
}
```

### 4. GetMetadata Method (Type-Safe Access)

```go
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
```

### 5. HasMetadata Helper

```go
// HasMetadata returns true if this Result contains any metadata.
func (r Result[T]) HasMetadata() bool {
    return r.metadata != nil && len(r.metadata) > 0
}
```

### 6. MetadataKeys Helper

```go
// MetadataKeys returns all metadata keys for this Result.
// Returns empty slice if no metadata present.
// Useful for debugging and introspection.
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
```

## Type-Safe Accessor Patterns

### 7. Standard Accessor Functions

For common metadata types, provide typed accessors to reduce casting:

```go
// GetStringMetadata retrieves string metadata with type safety.
func (r Result[T]) GetStringMetadata(key string) (string, bool) {
    value, exists := r.GetMetadata(key)
    if !exists {
        return "", false
    }
    str, ok := value.(string)
    return str, ok
}

// GetTimeMetadata retrieves time.Time metadata with type safety.
func (r Result[T]) GetTimeMetadata(key string) (time.Time, bool) {
    value, exists := r.GetMetadata(key)
    if !exists {
        return time.Time{}, false
    }
    t, ok := value.(time.Time)
    return t, ok
}

// GetIntMetadata retrieves int metadata with type safety.
func (r Result[T]) GetIntMetadata(key string) (int, bool) {
    value, exists := r.GetMetadata(key)
    if !exists {
        return 0, false
    }
    i, ok := value.(int)
    return i, ok
}
```

## Backward Compatibility

### 8. Compatibility Strategy

**Zero Breaking Changes:**
- All existing methods maintain exact same signatures
- All existing constructors work unchanged
- Metadata field is nil by default (zero cost)
- Existing Result[T] behavior preserved

**Migration Path:**
- Code using metadata adds .WithMetadata() calls
- Code not using metadata unchanged
- Gradual adoption possible

**Example Compatibility:**
```go
// Existing code - works unchanged
result := NewSuccess(42)
if result.IsSuccess() {
    value := result.Value()  // Works exactly as before
}

// New code - adds metadata
enhanced := NewSuccess(42).WithMetadata("source", "api")
```

## Testing Strategy

### 9. Unit Test Coverage

**File: result_test.go additions**

```go
func TestWithMetadata(t *testing.T) {
    original := NewSuccess(42)
    enhanced := original.WithMetadata("key1", "value1")
    
    // Immutability verification
    assert.False(t, original.HasMetadata())
    assert.True(t, enhanced.HasMetadata())
    
    // Value preservation
    assert.Equal(t, 42, enhanced.Value())
}

func TestWithMetadata_Chaining(t *testing.T) {
    result := NewSuccess(42).
        WithMetadata("key1", "value1").
        WithMetadata("key2", 100)
    
    v1, ok1 := result.GetMetadata("key1")
    assert.True(t, ok1)
    assert.Equal(t, "value1", v1)
    
    v2, ok2 := result.GetMetadata("key2")
    assert.True(t, ok2)
    assert.Equal(t, 100, v2)
}

func TestGetMetadata_NonExistent(t *testing.T) {
    result := NewSuccess(42)
    value, exists := result.GetMetadata("missing")
    assert.False(t, exists)
    assert.Nil(t, value)
}

func TestMetadata_WithError(t *testing.T) {
    result := NewError(42, errors.New("test"), "processor").
        WithMetadata("error_context", "validation")
    
    assert.True(t, result.IsError())
    assert.True(t, result.HasMetadata())
    
    ctx, ok := result.GetStringMetadata("error_context")
    assert.True(t, ok)
    assert.Equal(t, "validation", ctx)
}

func TestTypedAccessors(t *testing.T) {
    result := NewSuccess(42).
        WithMetadata("string_key", "test").
        WithMetadata("int_key", 100).
        WithMetadata("time_key", time.Now())
    
    // String accessor
    str, ok := result.GetStringMetadata("string_key")
    assert.True(t, ok)
    assert.Equal(t, "test", str)
    
    // Wrong type accessor
    _, ok = result.GetStringMetadata("int_key")
    assert.False(t, ok)
}

func TestMetadataKeys(t *testing.T) {
    result := NewSuccess(42).
        WithMetadata("key1", "value1").
        WithMetadata("key2", "value2")
    
    keys := result.MetadataKeys()
    assert.Len(t, keys, 2)
    assert.Contains(t, keys, "key1")
    assert.Contains(t, keys, "key2")
}

func TestWithMetadata_EmptyKey(t *testing.T) {
    original := NewSuccess(42)
    result := original.WithMetadata("", "value")
    
    // Empty key should be ignored
    assert.False(t, result.HasMetadata())
}
```

### 10. Memory Overhead Tests

```go
func TestMetadata_MemoryOverhead(t *testing.T) {
    // Verify nil metadata has zero overhead
    withoutMeta := NewSuccess(42)
    withMeta := NewSuccess(42).WithMetadata("key", "value")
    
    // Size comparison via unsafe.Sizeof if needed
    // Primarily verify nil metadata doesn't allocate
}

func BenchmarkMetadata_Operations(b *testing.B) {
    result := NewSuccess(42).WithMetadata("key", "value")
    
    b.Run("GetMetadata", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _, _ = result.GetMetadata("key")
        }
    })
    
    b.Run("WithMetadata", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _ = result.WithMetadata("new_key", i)
        }
    })
}
```

## Error Handling

### 11. Panic Prevention Strategy

**Type Assertion Safety:**
- All typed accessors return (value, bool) pattern
- Never panic on type mismatches
- Graceful degradation for unexpected types

**Memory Safety:**
- Nil checks in all metadata operations
- Empty key handling in WithMetadata
- Defensive copying in immutable operations

**Example Safe Usage:**
```go
// Safe type assertion pattern
if start, ok := result.GetTimeMetadata("window_start"); ok {
    // Use start safely
    processWindowStart(start)
} else {
    // Handle missing or wrong-type metadata
    log.Warn("Missing window_start metadata")
}
```

## Performance Considerations

### 12. Optimization Approach

**Nil Check Optimization:**
- Metadata operations short-circuit on nil
- Zero overhead when metadata unused
- Map operations only when needed

**Copy Strategy:**
- WithMetadata creates new map only
- Shallow copy of references (not deep copy)
- Minimal allocation overhead

**Memory Profile:**
```
No metadata:   16 bytes (value + error pointer)
With metadata: 16 + 48 + (entries * ~32) bytes
Typical window: ~150 bytes total
```

## Documentation Requirements

### 13. Godoc Standards

Each method requires comprehensive godoc explaining:
- Purpose and behavior
- Immutability guarantees 
- Type safety considerations
- Usage patterns and examples
- Performance characteristics

**Example:**
```go
// WithMetadata returns a new Result with the specified metadata key-value pair.
// The original Result is unchanged (immutable pattern).
// 
// Multiple calls can be chained to add multiple metadata entries:
//   result.WithMetadata("key1", "value1").WithMetadata("key2", 100)
//
// Empty keys are ignored. Values can be any type but callers must handle
// type assertions when retrieving via GetMetadata.
//
// Performance: O(n) where n is existing metadata count due to map copying.
// Memory: Creates new map with all existing entries plus new entry.
func (r Result[T]) WithMetadata(key string, value interface{}) Result[T]
```

## Implementation Dependencies

### 14. Required Imports

No new imports required:
- Uses built-in map type
- Uses existing interface{} type
- No external dependencies

### 15. Build Requirements

- Go 1.18+ (for generics support - already required)
- No additional build constraints
- Backward compatible with existing build process

## Quality Gates

### 16. Completion Criteria

**Implementation Complete When:**
- [ ] Metadata field added to Result[T] struct
- [ ] WithMetadata method implemented and tested
- [ ] GetMetadata method implemented and tested
- [ ] HasMetadata helper implemented and tested
- [ ] MetadataKeys helper implemented and tested
- [ ] Typed accessor methods implemented and tested
- [ ] Unit tests achieve >80% coverage
- [ ] Benchmark tests verify performance characteristics
- [ ] All linter checks pass
- [ ] Godoc documentation complete for all public methods
- [ ] Backward compatibility verified with existing tests

**Quality Validation:**
- Zero breaking changes to existing API
- Memory overhead tests pass
- Performance benchmarks within acceptable limits
- Error handling prevents panics
- Immutability pattern maintained throughout

This implementation plan provides metadata capability without Window dependencies, maintaining full backward compatibility while enabling future processor enhancements.