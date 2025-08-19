# streamz Documentation

📚 Complete guide to building production-grade stream processing pipelines with Go channels.

## Getting Started

- **[Introduction](./introduction.md)** - Why streamz and core philosophy
- **[Installation](./installation.md)** - Get up and running quickly
- **[Quick Start Guide](./quick-start.md)** - Build your first pipeline in 5 minutes

## Core Concepts

Understanding the building blocks of streamz:

- **[Processors](./concepts/processors.md)** - The Processor interface and how processors work
- **[Channel Management](./concepts/channels.md)** - How streamz handles channel lifecycle and patterns
- **[Composition](./concepts/composition.md)** - Building complex pipelines from simple parts
- **[Backpressure](./concepts/backpressure.md)** - Handling slow consumers and flow control
- **[Error Handling](./concepts/error-handling.md)** - Error strategies in streaming systems

## Practical Guides

Real-world patterns and best practices:

- **[Common Patterns](./guides/patterns.md)** - Streaming patterns and when to use them
- **[Performance](./guides/performance.md)** - Optimization and benchmarks
- **[Testing](./guides/testing.md)** - Testing stream processing pipelines
- **[Best Practices](./guides/best-practices.md)** - Production recommendations

## API Reference

Complete reference for all processors:

### Core Processors
- **[Batcher](./api/batcher.md)** - Accumulate items into batches
- **[Unbatcher](./api/unbatcher.md)** - Flatten batches to individual items
- **[Filter](./api/filter.md)** - Keep only items matching a predicate
- **[Mapper](./api/mapper.md)** - Transform items with a function
- **[FanOut](./api/fanout.md)** - Duplicate stream to multiple outputs
- **[FanIn](./api/fanin.md)** - Merge multiple streams

### Simple Transformations
- **[Take](./api/take.md)** - Take first N items then close
- **[Skip](./api/skip.md)** - Skip first N items then pass through
- **[Sample](./api/sample.md)** - Random sampling by percentage
- **[Chunk](./api/chunk.md)** - Fixed-size groups
- **[Flatten](./api/flatten.md)** - Expand slices to individual items

### Windowing
- **[TumblingWindow](./api/tumbling-window.md)** - Fixed-size time windows
- **[SlidingWindow](./api/sliding-window.md)** - Overlapping time windows
- **[SessionWindow](./api/session-window.md)** - Activity-based windows

### Buffering & Flow Control
- **[Buffer](./api/buffer.md)** - Add buffering to decouple speeds
- **[DroppingBuffer](./api/dropping-buffer.md)** - Drop items when buffer is full
- **[SlidingBuffer](./api/sliding-buffer.md)** - Keep latest N items

### Advanced Processing
- **[AsyncMapper](./api/async-mapper.md)** - Concurrent processing with order preservation
- **[Dedupe](./api/dedupe.md)** - Remove duplicate items within a time window
- **[Monitor](./api/monitor.md)** - Observe stream metrics

### Flow Control
- **[Tap](./api/tap.md)** - Execute side effects without modifying stream
- **[Throttle](./api/throttle.md)** - Rate limiting (items per second)
- **[Debounce](./api/debounce.md)** - Emit only after quiet period

## Contributing

See the main [README](../README.md) for development setup and contribution guidelines.

## License

MIT License - see [LICENSE](../LICENSE) file for details.