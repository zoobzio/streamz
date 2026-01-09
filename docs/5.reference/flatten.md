---
title: Flatten
description: Expand slices into individual items
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - transformation
---

# Flatten

The Flatten processor expands slices or arrays into individual items, converting a stream of collections into a stream of elements.

## Overview

Flatten is the inverse of batching operations. It takes a stream where each item is a slice/array and emits each element individually. This is useful for unpacking batch processing results or expanding grouped data.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Flatten batches into individual items
flattener := streamz.NewFlatten[string]()

// Convert []string stream to string stream
flattened := flattener.Process(ctx, batches)
```

## Configuration Options

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |

## Usage Examples

### Unpacking Batch Results

```go
// Process in batches then flatten results
batcher := streamz.NewBatcher[Order](streamz.BatchConfig{
    MaxSize:    100,
    MaxLatency: 1 * time.Second,
})

// Batch process for efficiency
batches := batcher.Process(ctx, orders)
results := batchProcessor.Process(ctx, batches) // Returns []Result

// Flatten back to individual results
flattener := streamz.NewFlatten[Result]().
    WithName("result-flattener")

individual := flattener.Process(ctx, results)

for result := range individual {
    notifyUser(result.UserID, result)
}
```

### API Response Expansion

```go
// Expand paginated API responses
type PagedResponse struct {
    Items []Item
    Page  int
    Total int
}

// Fetch pages
pages := fetchAllPages(ctx, apiClient)

// Extract items from pages
itemFlattener := streamz.NewFlatten[Item]()

// Transform pages to item slices
itemArrays := streamz.NewMapper(func(page PagedResponse) []Item {
    return page.Items
}).Process(ctx, pages)

// Flatten to individual items
items := itemFlattener.Process(ctx, itemArrays)

for item := range items {
    processItem(item)
}
```

### Multi-Value Processing

```go
// Process records that produce multiple outputs
expander := streamz.NewMapper(func(user User) []Notification {
    var notifications []Notification
    
    // Generate multiple notifications per user
    if user.Birthday.IsToday() {
        notifications = append(notifications, BirthdayNotification(user))
    }
    
    if user.SubscriptionExpiring() {
        notifications = append(notifications, RenewalNotification(user))
    }
    
    for _, achievement := range user.NewAchievements {
        notifications = append(notifications, AchievementNotification(user, achievement))
    }
    
    return notifications
})

// Expand users to notifications
notificationArrays := expander.Process(ctx, users)

// Flatten to send individually
flattener := streamz.NewFlatten[Notification]()
notifications := flattener.Process(ctx, notificationArrays)

for notification := range notifications {
    sendNotification(notification)
}
```

### CSV Row Expansion

```go
// Expand CSV rows with multiple values
type CSVRow struct {
    ID     string
    Values []string
}

// Parse CSV with multi-value cells
rows := parseCSVWithArrays(ctx, csvFile)

// Expand to individual records
recordMapper := streamz.NewMapper(func(row CSVRow) []Record {
    records := make([]Record, len(row.Values))
    for i, value := range row.Values {
        records[i] = Record{
            ID:    fmt.Sprintf("%s-%d", row.ID, i),
            Value: value,
        }
    }
    return records
})

recordArrays := recordMapper.Process(ctx, rows)

// Flatten for individual processing
flattener := streamz.NewFlatten[Record]()
records := flattener.Process(ctx, recordArrays)
```

### Nested Data Flattening

```go
// Flatten nested structures
type Department struct {
    Name      string
    Employees []Employee
}

departments := streamz.NewFlatten[Employee]()

// Extract employees from departments
employeeArrays := streamz.NewMapper(func(dept Department) []Employee {
    // Add department info to each employee
    for i := range dept.Employees {
        dept.Employees[i].Department = dept.Name
    }
    return dept.Employees
}).Process(ctx, departmentStream)

// Get all employees across departments
allEmployees := departments.Process(ctx, employeeArrays)

// Process individual employees
for employee := range allEmployees {
    updateEmployeeRecord(employee)
}
```

## Performance Notes

- **Time Complexity**: O(m) where m is total elements across all slices
- **Space Complexity**: O(1) - no buffering
- **Characteristics**:
  - Zero-copy element passing
  - Handles empty slices gracefully
  - Maintains order within slices
  - No allocation beyond input slices