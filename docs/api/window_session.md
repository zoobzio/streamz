# Session Window

The Session Window processor groups items into dynamic windows based on activity, creating new windows when gaps in activity exceed a timeout.

## Overview

Session windows are ideal for grouping events that occur in bursts with quiet periods between them. Unlike fixed-size windows, session windows have variable durations determined by the actual pattern of events. A new session starts after a period of inactivity exceeds the configured timeout.

## Basic Usage

```go
import (
    "context"
    "time"
    "github.com/zoobzio/streamz"
)

// Create sessions with 5-minute inactivity timeout
windower := streamz.NewSessionWindow[UserEvent](
    5*time.Minute,                    // Inactivity timeout
    func(e UserEvent) string {        // Key function
        return e.UserID
    },
)

sessions := windower.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `timeout` | `time.Duration` | Yes | Inactivity period before closing session |
| `keyFunc` | `func(T) string` | Yes | Function to group items by key |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `WithTimestamp(func(T) time.Time)` | Use custom timestamp instead of arrival time |
| `WithMaxDuration(duration)` | Maximum session duration before forced close |

## Usage Examples

### User Session Tracking

```go
// Track user sessions with 30-minute timeout
sessionWindower := streamz.NewSessionWindow[UserActivity](
    30*time.Minute,
    func(a UserActivity) string {
        return a.UserID
    },
).WithName("user-sessions")

sessions := sessionWindower.Process(ctx, activities)

for session := range sessions {
    duration := session.End.Sub(session.Start)
    
    log.Printf("User %s session: %v duration, %d activities", 
        session.Key, duration, len(session.Items))
    
    // Analyze session behavior
    sessionAnalysis := analyzeSession(session.Items)
    storeSessionMetrics(session.Key, sessionAnalysis)
}
```

### IoT Device Sessions

```go
// Group device readings into sessions
deviceWindower := streamz.NewSessionWindow[DeviceReading](
    2*time.Minute,  // 2-minute inactivity timeout
    func(r DeviceReading) string {
        return r.DeviceID
    },
).WithMaxDuration(1*time.Hour) // Force close long sessions

sessions := deviceWindower.Process(ctx, readings)

for session := range sessions {
    // Process complete device session
    summary := DeviceSessionSummary{
        DeviceID:  session.Key,
        Start:     session.Start,
        End:       session.End,
        Readings:  len(session.Items),
        AvgValue:  calculateAverage(session.Items),
        MaxValue:  findMax(session.Items),
        MinValue:  findMin(session.Items),
    }
    
    publishSessionSummary(summary)
}
```

### Chat Conversation Grouping

```go
// Group chat messages into conversations
chatWindower := streamz.NewSessionWindow[ChatMessage](
    5*time.Minute,  // 5-minute gap ends conversation
    func(m ChatMessage) string {
        // Group by room and participants
        participants := []string{m.From, m.To}
        sort.Strings(participants)
        return fmt.Sprintf("%s:%s", m.RoomID, strings.Join(participants, "-"))
    },
)

conversations := chatWindower.Process(ctx, messages)

for conv := range conversations {
    // Process complete conversation
    analysis := analyzeConversation(conv.Items)
    
    // Store conversation summary
    storeSummary(ConversationSummary{
        ID:           generateConvID(),
        Participants: extractParticipants(conv.Items),
        Start:        conv.Start,
        End:          conv.End,
        Messages:     len(conv.Items),
        Sentiment:    analysis.Sentiment,
        Topics:       analysis.Topics,
    })
}
```

### Transaction Grouping

```go
// Group related transactions
txWindower := streamz.NewSessionWindow[Transaction](
    10*time.Second,  // Quick succession indicates related
    func(tx Transaction) string {
        return tx.AccountID
    },
).WithTimestamp(func(tx Transaction) time.Time {
    return tx.Timestamp
})

sessions := txWindower.Process(ctx, transactions)

for session := range sessions {
    // Check for suspicious patterns
    if len(session.Items) > 10 {
        // Many transactions in quick succession
        investigateFraud(session)
    }
    
    // Calculate session totals
    var total float64
    for _, tx := range session.Items {
        total += tx.Amount
    }
    
    if total > 10000 {
        flagHighValueSession(session.Key, total)
    }
}
```

### API Call Batching

```go
// Batch API calls by client session
apiWindower := streamz.NewSessionWindow[APICall](
    1*time.Second,  // 1-second gap = new batch
    func(call APICall) string {
        return call.ClientID
    },
)

batches := apiWindower.Process(ctx, apiCalls)

for batch := range batches {
    // Process calls as a batch
    responses := make([]APIResponse, len(batch.Items))
    
    err := batchAPIProcess(batch.Items, responses)
    if err != nil {
        log.Printf("Batch processing failed: %v", err)
        // Fall back to individual processing
        for i, call := range batch.Items {
            responses[i] = processIndividual(call)
        }
    }
    
    // Send responses
    for i, resp := range responses {
        sendResponse(batch.Items[i].ClientID, resp)
    }
}
```

## Performance Notes

- **Time Complexity**: O(k) where k is number of active sessions
- **Space Complexity**: O(k*n) where n is items per session
- **Characteristics**:
  - Dynamic window sizes
  - Per-key session tracking
  - Timer management per session
  - Memory proportional to active sessions