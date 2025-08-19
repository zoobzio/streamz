# streamz Examples

This directory contains real-world examples demonstrating how to build production streaming systems with streamz.

## Available Examples

### 1. [Log Processing](./log-processing/)
**Real-time log analytics with security threat detection**

A production log processing system that evolves from a simple "just store the logs" MVP to a sophisticated real-time analytics platform featuring:
- Efficient batch storage
- Real-time error detection
- Intelligent rate-based alerting
- Security threat pattern matching
- Backpressure handling for traffic spikes

Shows how streamz processors work together to solve real problems like database overload, alert fatigue, and security blindness.

## Running Examples

Each example can be run with:

```bash
cd <example-directory>
go run .
```

Or run specific stages:

```bash
go run . -sprint=3  # Run specific sprint/stage
```

## Key Patterns Demonstrated

- **Stream Processing**: Filtering, mapping, batching, windowing
- **Backpressure Handling**: Buffering, dropping, sampling strategies
- **Error Handling**: Graceful degradation, smart alerting
- **Performance**: From 50% data loss to handling 10x traffic spikes
- **Extensibility**: Custom processors for domain-specific needs
- **Production Concerns**: Monitoring, alerting, efficient storage

## Coming Soon

- **Real-Time Analytics Dashboard** - User behavior analytics with live metrics
- **IoT Data Pipeline** - High-volume sensor data processing
- **Financial Transaction Monitoring** - Fraud detection and compliance
- **Video Stream Processing** - Real-time video analytics

Each example includes:
- Story-driven narrative showing evolution
- Complete, runnable code
- Comprehensive tests
- Performance benchmarks
- Production best practices