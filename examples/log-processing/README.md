# Real-Time Log Processing Pipeline

This example demonstrates building a production-ready log processing system that evolves from a simple "just store the logs" MVP to a sophisticated real-time analytics platform with security threat detection, intelligent alerting, and efficient storage.

## The Story

You're building the log processing system for "TechCorp", a rapidly growing SaaS platform. What starts as a simple logging requirement quickly evolves into a critical system processing millions of events per hour, detecting security threats in real-time, and providing actionable insights that save the company from major incidents.

## Running the Example

```bash
# Run the full demo showing evolution through sprints
go run .

# Run with specific sprint stages
go run . -sprint=1  # MVP - Just store logs
go run . -sprint=2  # Add batching for efficiency
go run . -sprint=3  # Add real-time error detection
go run . -sprint=4  # Add intelligent rate-based alerting
go run . -sprint=5  # Add security threat detection
go run . -sprint=6  # Full production system with backpressure

# Run tests
go test -v

# Run benchmarks
go test -bench=. -benchmem
```

## Key Features Demonstrated

1. **Stream Processing Patterns**: Filtering, mapping, batching, windowing, parallel processing
2. **Backpressure Handling**: Graceful degradation during traffic spikes
3. **Real-Time Analytics**: Error rate detection, security pattern matching
4. **Performance Optimization**: From 50% data loss to handling 10x traffic spikes
5. **Custom Processors**: Extending streamz for domain-specific needs
6. **Production Concerns**: Monitoring, alerting, efficient storage

## Architecture Evolution

### Sprint 1: MVP (Day 1)
"Just get the logs into the database!"
- Direct channel processing
- Simple database writes
- **Problem**: During the first production incident, database overwhelmed, 50% of logs lost!

### Sprint 2: Efficient Storage (Week 1)
"We're losing critical debugging data!"
- Add batching for bulk inserts
- Reduce database load by 90%
- **Problem**: Critical errors buried in batches, taking minutes to surface!

### Sprint 3: Real-Time Alerting (Week 2)
"We need to know about errors immediately!"
- Add parallel processing paths
- Immediate alerts for critical errors
- **Problem**: Alert fatigue - 500 alerts per hour, ops team overwhelmed!

### Sprint 4: Intelligent Alerting (Week 3)
"Alert on patterns, not individual errors!"
- Add windowing for rate-based detection
- Alert on error spikes, not every error
- **Result**: 80% reduction in false alarms, ops team loves it!

### Sprint 5: Security Monitoring (Week 4)
"We're blind to security threats!"
- Add pattern matching for threats
- Real-time detection of SQL injection, brute force attempts
- **Result**: Detected and prevented 3 breach attempts in first week!

### Sprint 6: Production Scale (Week 6)
"Black Friday is coming!"
- Add backpressure handling
- Implement sampling for extreme load
- Dynamic buffering and monitoring
- **Achievement**: Handled Black Friday 10x traffic spike with zero data loss!

## Example Output

The demo shows real metrics as it evolves:
- Log processing rates (logs/second)
- Storage efficiency (database writes reduced)
- Alert quality (false positive rate)
- Security threats detected
- System performance under load

## Code Structure

- `main.go` - Demo runner showing progression through sprints
- `types.go` - Core types (LogEntry, Alert, SecurityPattern)
- `services.go` - Mock external services (database, alerting, security)
- `pipeline.go` - Pipeline evolution with detailed sprint documentation
- `processors.go` - Custom processors (PatternMatcher, WindowAggregator)
- `pipeline_test.go` - Comprehensive test coverage

## Key Lessons

This example demonstrates how streamz enables:
- Building streaming systems that scale with your needs
- Handling real-world challenges (backpressure, alert fatigue, security)
- Extending with custom processors when needed
- Maintaining code clarity as complexity grows
- Measuring and proving improvements with metrics

## Performance Metrics

See how the system evolves:
- **Sprint 1**: 1,000 logs/sec, 50% loss during spikes
- **Sprint 2**: 10,000 logs/sec, efficient batching
- **Sprint 3**: Real-time processing, <100ms alert latency
- **Sprint 4**: 80% fewer false alerts
- **Sprint 5**: 3 security threats detected and prevented
- **Sprint 6**: 100,000 logs/sec sustained, zero data loss

## Next Steps

After running this example, you'll understand:
- How to build production streaming systems with streamz
- When to use different processors (Filter, Batcher, Window, etc.)
- How to handle backpressure and overload scenarios
- How to extend streamz with custom processors
- How to measure and optimize streaming pipelines