---
title: Mapper
description: Transform items synchronously with a function
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - transformation
---

# Mapper

Transforms each item in a stream by applying a transformation function. The mapper is a fundamental processor for data transformation and enrichment.

## Overview

The Mapper processor applies a pure transformation function to each item flowing through the stream. It maintains the one-to-one relationship between input and output items while potentially changing the item type.

**Type Signature:** `chan In → chan Out`

## When to Use

- **Data transformation** - Convert between data formats or types
- **Data enrichment** - Add computed fields or external data
- **Format conversion** - Convert between different representations
- **Data cleaning** - Normalize or standardize data
- **Field extraction** - Extract specific fields from complex structures
- **Unit conversion** - Convert measurements or currencies

## Constructor

```go
func NewMapper[In, Out any](mapper func(In) Out) *Mapper[In, Out]
```

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `mapper` | `func(In) Out` | Yes | Pure transformation function |

## Fluent Methods

| Method | Description |
|--------|-------------|
| `WithName(name string)` | Sets a custom name for debugging and monitoring (default: "mapper") |

## Examples

### Basic Type Conversion

```go
// Convert integers to strings
toString := streamz.NewMapper(func(n int) string {
    return fmt.Sprintf("number-%d", n)
}).WithName("to-string")

numbers := sendData(ctx, []int{1, 2, 3})
strings := toString.Process(ctx, numbers)
results := collectAll(strings) // ["number-1", "number-2", "number-3"]
```

### Mathematical Transformations

```go
// Square numbers
square := streamz.NewMapper(func(n int) int {
    return n * n
}).WithName("square")

// Convert to percentage
toPercentage := streamz.NewMapper(func(ratio float64) string {
    return fmt.Sprintf("%.1f%%", ratio*100)
}).WithName("percentage")

numbers := sendData(ctx, []int{1, 2, 3, 4})
squared := square.Process(ctx, numbers)
results := collectAll(squared) // [1, 4, 9, 16]
```

### Data Structure Transformation

```go
type User struct {
    ID       string
    Name     string
    Email    string
    Age      int
    Country  string
}

type UserProfile struct {
    UserID      string
    DisplayName string
    Contact     string
    Demographics Demographics
}

type Demographics struct {
    Age     int
    Country string
}

// Transform User to UserProfile
userToProfile := streamz.NewMapper(func(user User) UserProfile {
    return UserProfile{
        UserID:      user.ID,
        DisplayName: user.Name,
        Contact:     user.Email,
        Demographics: Demographics{
            Age:     user.Age,
            Country: user.Country,
        },
    }
}).WithName("user-to-profile")

users := []User{
    {ID: "1", Name: "Alice", Email: "alice@example.com", Age: 30, Country: "US"},
    {ID: "2", Name: "Bob", Email: "bob@example.com", Age: 25, Country: "UK"},
}

input := sendData(ctx, users)
profiles := userToProfile.Process(ctx, input)
results := collectAll(profiles)
```

### Data Enrichment

```go
type Order struct {
    ID         string
    CustomerID string
    Amount     float64
    Status     string
}

type EnrichedOrder struct {
    Order
    ProcessedAt time.Time
    Currency    string
    Tax         float64
    Total       float64
}

// Enrich orders with computed fields
enrichOrder := streamz.NewMapper(func(order Order) EnrichedOrder {
    tax := order.Amount * 0.08 // 8% tax
    
    return EnrichedOrder{
        Order:       order,
        ProcessedAt: time.Now(),
        Currency:    "USD",
        Tax:         tax,
        Total:       order.Amount + tax,
    }
}).WithName("enrich-order")

orders := []Order{
    {ID: "1", CustomerID: "cust1", Amount: 100.00, Status: "pending"},
    {ID: "2", CustomerID: "cust2", Amount: 250.00, Status: "pending"},
}

input := sendData(ctx, orders)
enriched := enrichOrder.Process(ctx, input)
results := collectAll(enriched)
```

### JSON Processing

```go
// Parse JSON strings to structs
type Config struct {
    Name    string `json:"name"`
    Enabled bool   `json:"enabled"`
    Value   int    `json:"value"`
}

jsonParser := streamz.NewMapper(func(jsonStr string) Config {
    var config Config
    if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
        // Return zero value on parse error
        log.Warn("Failed to parse JSON", "json", jsonStr, "error", err)
        return Config{}
    }
    return config
}).WithName("json-parser")

jsonStrings := []string{
    `{"name":"feature1","enabled":true,"value":42}`,
    `{"name":"feature2","enabled":false,"value":0}`,
}

input := sendData(ctx, jsonStrings)
configs := jsonParser.Process(ctx, input)
results := collectAll(configs)
```

## Pipeline Integration

### Sequential Transformation

```go
// Chain multiple transformations
orders := make(chan Order)

// Step 1: Enrich with timestamps
timestamper := streamz.NewMapper(func(order Order) EnrichedOrder {
    return EnrichedOrder{Order: order, ProcessedAt: time.Now()}
}).WithName("timestamp")

// Step 2: Add currency conversion
currencyConverter := streamz.NewMapper(func(order EnrichedOrder) EnrichedOrder {
    order.AmountUSD = convertToUSD(order.Amount, order.Currency)
    return order
}).WithName("currency")

// Step 3: Add customer tier
tierEnricher := streamz.NewMapper(func(order EnrichedOrder) EnrichedOrder {
    order.CustomerTier = getCustomerTier(order.CustomerID)
    return order
}).WithName("tier")

step1 := timestamper.Process(ctx, orders)
step2 := currencyConverter.Process(ctx, step1)
final := tierEnricher.Process(ctx, step2)
```

### Filter then Map

```go
// Filter invalid orders, then enrich valid ones
validator := streamz.NewFilter(func(order Order) bool {
    return order.Amount > 0 && order.CustomerID != ""
}).WithName("valid")

enricher := streamz.NewMapper(func(order Order) EnrichedOrder {
    return enrichWithCustomerData(order)
}).WithName("enrich")

validated := validator.Process(ctx, orders)
enriched := enricher.Process(ctx, validated)
```

### Map then Batch

```go
// Transform individual items, then batch them
transformer := streamz.NewMapper(transformOrder).WithName("transform")
batcher := streamz.NewBatcher[TransformedOrder](streamz.BatchConfig{
    MaxSize:    100,
    MaxLatency: 5 * time.Second,
})

transformed := transformer.Process(ctx, orders)
batched := batcher.Process(ctx, transformed)
```

## Performance Characteristics

### Throughput
- **Excellent for pure functions**: Minimal overhead when transformation is simple
- **Function-bound**: Performance primarily limited by transformation function complexity
- **Single-threaded**: Each mapper runs in a single goroutine

### Memory Usage
- **Constant overhead**: No internal buffering or state
- **Function dependent**: Memory usage depends on transformation complexity
- **Zero copy potential**: Can avoid copying when transforming references

### Latency
- **Low latency**: Items are processed and forwarded immediately
- **Function bound**: Latency determined by transformation function execution time
- **No buffering**: Each item flows through immediately

## Advanced Usage

### Context-Aware Mapping

```go
// Use context for request-scoped data
type RequestContext struct {
    UserID    string
    SessionID string
    Timestamp time.Time
}

// Note: For context-aware mapping, implement a custom processor
// The standard mapper uses a simple function signature for better composability

// Use with context
requestCtx := context.WithValue(ctx, "request", RequestContext{
    UserID:    "user123",
    SessionID: "session456",
    Timestamp: time.Now(),
})

processed := contextMapper.Process(requestCtx, rawData)
```

### Conditional Transformation

```go
// Transform based on item properties
conditionalMapper := streamz.NewMapper(func(order Order) ProcessedOrder {
    processed := ProcessedOrder{
        ID:       order.ID,
        Amount:   order.Amount,
        Status:   order.Status,
    }
    
    // Different processing based on amount
    if order.Amount > 1000 {
        processed.Priority = "high"
        processed.ReviewRequired = true
        processed.ApprovalLevel = "manager"
    } else if order.Amount > 100 {
        processed.Priority = "medium"
        processed.ReviewRequired = false
        processed.ApprovalLevel = "auto"
    } else {
        processed.Priority = "low"
        processed.ReviewRequired = false
        processed.ApprovalLevel = "auto"
    }
    
    return processed
}).WithName("conditional")
```

### External Service Integration

```go
// Enrich with external service data (synchronous)
type CustomerService interface {
    GetCustomer(ctx context.Context, id string) (Customer, error)
}

// Note: For external service integration with context and error handling,
// use AsyncMapper which supports context and error handling
```

## Error Handling

### Safe Transformations

Handle potential errors in transformation functions:

```go
// Safe JSON parsing with error handling
safeJSONMapper := streamz.NewMapper(func(jsonStr string) Result {
    var data map[string]interface{}
    
    if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
        return Result{
            Success: false,
            Error:   err.Error(),
            Data:    nil,
        }
    }
    
    return Result{
        Success: true,
        Error:   "",
        Data:    data,
    }
}).WithName("safe-json")

// Handle division by zero
safeDivider := streamz.NewMapper(func(pair NumberPair) float64 {
    if pair.Denominator == 0 {
        log.Warn("Division by zero attempted", "numerator", pair.Numerator)
        return 0 // or math.Inf(1) for positive infinity
    }
    return pair.Numerator / pair.Denominator
}).WithName("safe-divide")
```

### Panic Recovery

```go
type SafeMapper[In, Out any] struct {
    name   string
    mapper func(In) Out
    defaultValue Out
}

func (s *SafeMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    out := make(chan Out)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                result := s.defaultValue
                func() {
                    defer func() {
                        if r := recover(); r != nil {
                            log.Error("Mapper function panicked", 
                                "mapper", s.name, 
                                "panic", r, 
                                "item", item)
                            result = s.defaultValue
                        }
                    }()
                    result = s.mapper(item)
                }()
                
                select {
                case out <- result:
                case <-ctx.Done():
                    return
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

### Identity Mapping

```go
// Pass through without changes (useful for type conversion)
identity := streamz.NewMapper(func(item T) T {
    return item
}).WithName("identity")
```

### Field Extraction

```go
// Extract specific fields
idExtractor := streamz.NewMapper(func(user User) string {
    return user.ID
}).WithName("extract-id")

// Extract multiple fields
summaryExtractor := streamz.NewMapper(func(order Order) OrderSummary {
    return OrderSummary{
        ID:     order.ID,
        Amount: order.Amount,
        Status: order.Status,
    }
}).WithName("extract-summary")
```

### Data Validation and Cleaning

```go
// Clean and validate email addresses
emailCleaner := streamz.NewMapper(func(email string) string {
    cleaned := strings.TrimSpace(strings.ToLower(email))
    
    // Basic validation
    if !strings.Contains(cleaned, "@") {
        return ""
    }
    
    return cleaned
}).WithName("clean-email")

// Normalize phone numbers
phoneCleaner := streamz.NewMapper(func(phone string) string {
    // Remove all non-digit characters
    cleaned := regexp.MustCompile(`[^\d]`).ReplaceAllString(phone, "")
    
    // Format as (XXX) XXX-XXXX for US numbers
    if len(cleaned) == 10 {
        return fmt.Sprintf("(%s) %s-%s", cleaned[:3], cleaned[3:6], cleaned[6:])
    }
    
    return cleaned
}).WithName("clean-phone")
```

### Aggregation Preparation

```go
// Prepare data for aggregation
aggregationPrep := streamz.NewMapper(func(event Event) AggregationKey {
    return AggregationKey{
        Date:     event.Timestamp.Truncate(24 * time.Hour), // Day granularity
        Category: event.Category,
        UserType: getUserType(event.UserID),
        Value:    event.Value,
    }
}).WithName("prep-for-aggregation")
```

## Testing

```go
func TestMapper(t *testing.T) {
    ctx := context.Background()
    
    doubler := streamz.NewMapper(func(n int) int {
        return n * 2
    }).WithName("double")
    
    input := sendData(ctx, []int{1, 2, 3})
    output := doubler.Process(ctx, input)
    results := collectAll(output)
    
    expected := []int{2, 4, 6}
    assert.Equal(t, expected, results)
}

func TestMapperTypeConversion(t *testing.T) {
    ctx := context.Background()
    
    toString := streamz.NewMapper(func(n int) string {
        return fmt.Sprintf("%d", n)
    }).WithName("to-string")
    
    input := sendData(ctx, []int{1, 2, 3})
    output := toString.Process(ctx, input)
    results := collectAll(output)
    
    expected := []string{"1", "2", "3"}
    assert.Equal(t, expected, results)
}

// Note: Context-aware mapping requires custom processor implementation
// The standard mapper uses a simpler function signature for better composability
```

## Best Practices

1. **Keep functions pure** - Avoid side effects in mapper functions
2. **Handle errors gracefully** - Return sensible defaults for invalid input
3. **Use context appropriately** - Leverage context for timeouts and request-scoped data
4. **Minimize allocations** - Reuse objects when possible
5. **Add appropriate logging** - Log warnings for data quality issues
6. **Test edge cases** - Test with null, empty, and invalid inputs
7. **Document transformations** - Make the mapping logic clear and documented

## Related Processors

- **[Filter](./filter.md)**: Conditional processing instead of transformation
- **[AsyncMapper](./async-mapper.md)**: Concurrent transformation for I/O-bound operations
- **[Batcher](./batcher.md)**: Group items before transformation
- **[FanOut](./fanout.md)**: Transform and duplicate to multiple outputs

## See Also

- **[Concepts: Processors](../concepts/processors.md)**: Understanding processor fundamentals
- **[Concepts: Composition](../concepts/composition.md)**: Chaining mappers with other processors
- **[Guides: Patterns](../guides/patterns.md)**: Real-world transformation patterns
- **[Guides: Performance](../guides/performance.md)**: Optimizing mapper performance