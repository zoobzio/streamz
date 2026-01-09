---
title: Filter
description: Keep only items matching a predicate
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - transformation
---

# Filter

Selectively passes items through a stream based on a predicate function. Only items for which the predicate returns true are emitted to the output channel.

## Overview

The Filter processor is one of the most fundamental stream processing operations. It evaluates each incoming item against a predicate function and only forwards items that match the criteria.

**Type Signature:** `chan T → chan T`

## When to Use

- **Data validation** - Remove invalid or malformed items
- **Business rule filtering** - Apply conditional logic to data flow
- **Quality control** - Filter out items that don't meet quality standards
- **Security filtering** - Remove potentially dangerous content
- **Performance optimization** - Reduce downstream processing load
- **A/B testing** - Route specific subsets of data

## Constructor

```go
func NewFilter[T any](predicate func(T) bool) *Filter[T]
```

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `predicate` | `func(T) bool` | Yes | Function that returns true for items to keep |

## Fluent Methods

| Method | Description |
|--------|-------------|
| `WithName(name string)` | Sets a custom name for debugging and monitoring (default: "filter") |

## Examples

### Basic Filtering

```go
// Filter positive numbers
positive := streamz.NewFilter(func(n int) bool {
    return n > 0
}).WithName("positive")

numbers := sendData(ctx, []int{-2, -1, 0, 1, 2, 3})
filtered := positive.Process(ctx, numbers)
results := collectAll(filtered) // [1, 2, 3]
```

### String Filtering

```go
// Filter non-empty strings
nonEmpty := streamz.NewFilter(func(s string) bool {
    return strings.TrimSpace(s) != ""
}).WithName("non-empty")

strings := sendData(ctx, []string{"hello", "", " ", "world", "\t"})
filtered := nonEmpty.Process(ctx, strings)
results := collectAll(filtered) // ["hello", "world"]
```

### Complex Business Logic

```go
// Filter valid orders
type Order struct {
    ID         string
    Amount     float64
    CustomerID string
    Status     string
}

validOrders := streamz.NewFilter(func(order Order) bool {
    return order.ID != "" &&
           order.Amount > 0 &&
           order.CustomerID != "" &&
           order.Status == "pending"
}).WithName("valid-orders")

orders := []Order{
    {ID: "1", Amount: 100, CustomerID: "cust1", Status: "pending"},
    {ID: "2", Amount: 0, CustomerID: "cust2", Status: "pending"},     // Invalid amount
    {ID: "", Amount: 50, CustomerID: "cust3", Status: "pending"},     // No ID
    {ID: "4", Amount: 75, CustomerID: "cust4", Status: "completed"},  // Wrong status
}

input := sendData(ctx, orders)
valid := validOrders.Process(ctx, input)
results := collectAll(valid) // Only order 1 passes
```

### Data Quality Filtering

```go
// Filter users with valid email addresses
import "regexp"

emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

validEmails := streamz.NewFilter(func(user User) bool {
    return emailRegex.MatchString(user.Email)
}).WithName("valid-emails")

users := []User{
    {Name: "Alice", Email: "alice@example.com"},
    {Name: "Bob", Email: "invalid-email"},
    {Name: "Charlie", Email: "charlie@domain.co.uk"},
}

input := sendData(ctx, users)
validated := validEmails.Process(ctx, input)
results := collectAll(validated) // Alice and Charlie only
```

## Pipeline Integration

### Sequential Filtering

```go
// Apply multiple filters in sequence
orders := make(chan Order)

// Filter 1: Basic validation
basicValid := streamz.NewFilter(func(o Order) bool {
    return o.ID != "" && o.Amount > 0
}).WithName("basic-valid")

// Filter 2: Business rules
businessValid := streamz.NewFilter(func(o Order) bool {
    return o.Amount <= 10000 && o.CustomerID != ""
}).WithName("business-valid")

// Filter 3: Status check
statusValid := streamz.NewFilter(func(o Order) bool {
    return o.Status == "pending" || o.Status == "confirmed"
}).WithName("status-valid")

step1 := basicValid.Process(ctx, orders)
step2 := businessValid.Process(ctx, step1)
final := statusValid.Process(ctx, step2)
```

### Combined with Transformation

```go
// Filter then transform
validator := streamz.NewFilter(isValidOrder).WithName("valid")
enricher := streamz.NewMapper(enrichOrder).WithName("enrich")

validated := validator.Process(ctx, orders)
enriched := enricher.Process(ctx, validated)
```

### Filter After Processing

```go
// Process then filter results
processor := streamz.NewMapper(processOrder).WithName("process")
successFilter := streamz.NewFilter(func(result ProcessResult) bool {
    return result.Success
}).WithName("successful")

processed := processor.Process(ctx, orders)
successful := successFilter.Process(ctx, processed)
```

## Performance Characteristics

### Throughput
- **Best case**: Near-native channel performance when all items pass
- **Worst case**: Minimal overhead when all items are filtered out
- **Typical**: Overhead is negligible compared to predicate evaluation time

### Memory Usage
- **Constant memory**: No internal buffering or accumulation
- **Zero allocations**: No heap allocations in the filter logic itself
- **Predicate dependent**: Memory usage depends on predicate function complexity

### Latency
- **Minimal latency**: Items are processed and forwarded immediately
- **Predicate bound**: Latency primarily determined by predicate execution time
- **No batching**: Each item is evaluated independently

## Advanced Usage

### Stateful Filtering

For more complex filtering that requires state, implement a custom processor:

```go
type StatefulFilter[T any] struct {
    name      string
    predicate func(T, map[string]interface{}) bool
    state     map[string]interface{}
    mutex     sync.RWMutex
}

func (s *StatefulFilter[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                s.mutex.RLock()
                shouldPass := s.predicate(item, s.state)
                s.mutex.RUnlock()
                
                if shouldPass {
                    select {
                    case out <- item:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

// Example: Filter duplicate IDs
deduplicator := &StatefulFilter[Order]{
    name: "deduplicator",
    state: make(map[string]interface{}),
    predicate: func(order Order, state map[string]interface{}) bool {
        if _, exists := state[order.ID]; exists {
            return false // Duplicate
        }
        state[order.ID] = true
        return true
    },
}
```

### Dynamic Filtering

Change filter criteria at runtime:

```go
type DynamicFilter[T any] struct {
    name      string
    predicate atomic.Value // Stores func(T) bool
}

func NewDynamicFilter[T any](name string, initialPredicate func(T) bool) *DynamicFilter[T] {
    df := &DynamicFilter[T]{name: name}
    df.predicate.Store(initialPredicate)
    return df
}

func (d *DynamicFilter[T]) UpdatePredicate(newPredicate func(T) bool) {
    d.predicate.Store(newPredicate)
}

func (d *DynamicFilter[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                predicate := d.predicate.Load().(func(T) bool)
                if predicate(item) {
                    select {
                    case out <- item:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

// Usage
filter := NewDynamicFilter("dynamic", func(n int) bool { return n > 0 })

// Later, change criteria
filter.UpdatePredicate(func(n int) bool { return n > 10 })
```

## Error Handling

### Predicate Panics

Handle panics in predicate functions gracefully:

```go
type SafeFilter[T any] struct {
    name      string
    predicate func(T) bool
}

func (s *SafeFilter[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                shouldPass := false
                func() {
                    defer func() {
                        if r := recover(); r != nil {
                            log.Error("Filter predicate panicked", 
                                "filter", s.name, 
                                "panic", r, 
                                "item", item)
                            shouldPass = false // Default to filtering out on panic
                        }
                    }()
                    shouldPass = s.predicate(item)
                }()
                
                if shouldPass {
                    select {
                    case out <- item:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}
```

## Common Patterns

### Multi-Criteria Filtering

```go
// Combine multiple conditions
complexFilter := streamz.NewFilter(func(user User) bool {
    return user.Age >= 18 &&                    // Legal age
           user.Country == "US" &&              // Geographic restriction
           user.Verified &&                     // Account verification
           len(user.Email) > 0 &&               // Has email
           !user.Suspended &&                   // Not suspended
           time.Since(user.LastLogin) < 30*24*time.Hour // Active user
}).WithName("complex")
```

### Conditional Filtering

```go
// Filter based on external state
type ConditionalFilter struct {
    enabled bool
    filter  *streamz.Filter[Order]
}

func (cf *ConditionalFilter) Process(ctx context.Context, in <-chan Order) <-chan Order {
    if !cf.enabled {
        // Pass through without filtering
        return in
    }
    return cf.filter.Process(ctx, in)
}
```

### Sampling Filter

```go
// Filter for sampling (keep every Nth item)
func NewSamplingFilter[T any](name string, interval int) *streamz.Filter[T] {
    count := int64(0)
    return streamz.NewFilter(func(item T) bool {
        current := atomic.AddInt64(&count, 1)
        return current%int64(interval) == 0
    }).WithName(name)
}

// Keep every 10th item
sampler := NewSamplingFilter[Event]("sampler", 10)
```

## Testing

```go
func TestFilter(t *testing.T) {
    ctx := context.Background()
    
    filter := streamz.NewFilter(func(n int) bool {
        return n%2 == 0
    }).WithName("even")
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5, 6})
    output := filter.Process(ctx, input)
    results := collectAll(output)
    
    expected := []int{2, 4, 6}
    assert.Equal(t, expected, results)
}

func TestFilterEmptyInput(t *testing.T) {
    ctx := context.Background()
    
    filter := streamz.NewFilter(func(n int) bool { return true }).WithName("any")
    
    input := sendData(ctx, []int{})
    output := filter.Process(ctx, input)
    results := collectAll(output)
    
    assert.Empty(t, results)
}

func TestFilterAllFiltered(t *testing.T) {
    ctx := context.Background()
    
    filter := streamz.NewFilter(func(n int) bool { return false }).WithName("none")
    
    input := sendData(ctx, []int{1, 2, 3})
    output := filter.Process(ctx, input)
    results := collectAll(output)
    
    assert.Empty(t, results)
}
```

## Best Practices

1. **Use descriptive names** - Make filter purpose clear from the name
2. **Keep predicates simple** - Complex logic should be broken down
3. **Handle edge cases** - Consider null/empty values in predicates
4. **Test thoroughly** - Test with various input combinations
5. **Monitor filter rates** - Track how many items are being filtered out
6. **Avoid side effects** - Predicates should be pure functions

## Related Processors

- **[Mapper](./mapper.md)**: Transform items instead of filtering
- **[Sample](./sample.md)**: Random sampling alternative to filtering
- **[Take](./take.md)**: Limit number of items processed
- **[Skip](./skip.md)**: Skip first N items

## See Also

- **[Concepts: Processors](../concepts/processors.md)**: Understanding processor fundamentals
- **[Concepts: Composition](../concepts/composition.md)**: Combining filters with other processors
- **[Guides: Patterns](../guides/patterns.md)**: Real-world filtering patterns