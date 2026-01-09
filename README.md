# streamz

[![CI Status](https://github.com/zoobzio/streamz/workflows/CI/badge.svg)](https://github.com/zoobzio/streamz/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/streamz/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/streamz)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/streamz)](https://goreportcard.com/report/github.com/zoobzio/streamz)
[![CodeQL](https://github.com/zoobzio/streamz/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/streamz/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/streamz.svg)](https://pkg.go.dev/github.com/zoobzio/streamz)
[![License](https://img.shields.io/github/license/zoobzio/streamz)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/streamz)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/streamz)](https://github.com/zoobzio/streamz/releases)

Type-safe stream processing primitives for Go channels.

Build composable pipelines from simple parts — batching, windowing, flow control — with unified error handling and deterministic testing.

## One Channel, Every Pattern

Every processor shares the same signature:

```go
Process(ctx context.Context, in <-chan Result[T]) <-chan Result[Out]
```

`Result[T]` unifies success and error in a single channel — no dual-channel complexity:

```go
// Filter keeps items matching a predicate
filter := streamz.NewFilter(func(o Order) bool { return o.Total > 0 })

// Mapper transforms items
mapper := streamz.NewMapper(func(o Order) Order {
    o.ProcessedAt = time.Now()
    return o
})

// Batcher collects items by size or time
batcher := streamz.NewBatcher[Order](streamz.BatchConfig{
    MaxSize:    100,
    MaxLatency: time.Second,
})
```

Compose them into pipelines — each output feeds the next input:

```go
filtered := filter.Process(ctx, orders)
mapped := mapper.Process(ctx, filtered)
batched := batcher.Process(ctx, mapped)

for batch := range batched {
    if batch.IsSuccess() {
        bulkInsert(batch.Value())
    }
}
```

One interface. One channel. Every pattern.

## Install

```bash
go get github.com/zoobzio/streamz
```

Requires Go 1.24+.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/zoobzio/streamz"
)

type Order struct {
    ID          string
    Total       float64
    ProcessedAt time.Time
}

func main() {
    ctx := context.Background()

    // Source channel
    orders := make(chan streamz.Result[Order], 10)
    go func() {
        defer close(orders)
        orders <- streamz.Success(Order{ID: "A", Total: 99.99})
        orders <- streamz.Success(Order{ID: "B", Total: 149.99})
        orders <- streamz.Success(Order{ID: "C", Total: 0}) // will be filtered
    }()

    // Build processors
    filter := streamz.NewFilter(func(o Order) bool { return o.Total > 0 })
    mapper := streamz.NewMapper(func(o Order) Order {
        o.ProcessedAt = time.Now()
        return o
    })

    // Compose pipeline
    filtered := filter.Process(ctx, orders)
    processed := mapper.Process(ctx, filtered)

    // Consume results
    for result := range processed {
        if result.IsSuccess() {
            o := result.Value()
            fmt.Printf("Processed: %s ($%.2f) at %v\n", o.ID, o.Total, o.ProcessedAt)
        }
    }
}
```

## Capabilities

| Feature | Description | Docs |
|---------|-------------|------|
| Result[T] Pattern | Unified success/error handling in a single channel | [Concepts](docs/2.learn/2.concepts.md) |
| Processor Interface | Common pattern for all stream operations | [Processors](docs/2.learn/4.processors.md) |
| Batching & Windowing | Time and size-based aggregation | [Architecture](docs/2.learn/3.architecture.md) |
| Flow Control | Throttle, debounce, buffer, backpressure | [Backpressure](docs/2.learn/6.backpressure.md) |
| Deterministic Testing | Clock abstraction for reproducible tests | [Testing](docs/3.guides/1.testing.md) |
| Error Handling | Skip, retry, dead letter queues | [Error Handling](docs/2.learn/7.error-handling.md) |

## Why streamz?

- **Type-safe** — Full compile-time checking with Go generics
- **Composable** — Complex pipelines from simple, reusable parts
- **Unified errors** — `Result[T]` eliminates dual-channel complexity
- **Deterministic testing** — Clock abstraction enables reproducible time-based tests
- **Production ready** — Proper channel lifecycle, no goroutine leaks
- **Minimal dependencies** — Standard library plus [clockz](https://github.com/zoobzio/clockz)

## Composable Stream Architecture

streamz enables a pattern: **define processors once, compose them into any pipeline**.

Your stream operations become reusable building blocks. Validation, enrichment, batching, rate limiting — each is a processor. Combine them in different configurations for different use cases.

```go
// Reusable processors
validate := streamz.NewFilter(isValid)
enrich := streamz.NewAsyncMapper(fetchMetadata).WithWorkers(10)
batch := streamz.NewBatcher[Order](streamz.BatchConfig{MaxSize: 100})
throttle := streamz.NewThrottle[[]Order](100 * time.Millisecond)

// Real-time pipeline
realtime := mapper.Process(ctx, filter.Process(ctx, orders))

// Batch pipeline with rate limiting
batched := throttle.Process(ctx, batch.Process(ctx, enrich.Process(ctx, validate.Process(ctx, orders))))
```

Time-dependent processors use [clockz](https://github.com/zoobzio/clockz) for deterministic testing — advance time explicitly, verify behavior reproducibly.

## Documentation

Full documentation is available in the [docs/](docs/) directory:

### Learn

- [Quickstart](docs/2.learn/1.quickstart.md) — Build your first pipeline
- [Concepts](docs/2.learn/2.concepts.md) — Result[T], processors, composition
- [Architecture](docs/2.learn/3.architecture.md) — Pipeline patterns and design
- [Processors](docs/2.learn/4.processors.md) — Processor interface and lifecycle
- [Channels](docs/2.learn/5.channels.md) — Channel management and cleanup
- [Backpressure](docs/2.learn/6.backpressure.md) — Flow control strategies
- [Error Handling](docs/2.learn/7.error-handling.md) — Error patterns and recovery

### Guides

- [Testing](docs/3.guides/1.testing.md) — Deterministic testing with clock abstraction
- [Best Practices](docs/3.guides/2.best-practices.md) — Production recommendations
- [Performance](docs/3.guides/3.performance.md) — Optimization and benchmarking
- [Patterns](docs/3.guides/4.patterns.md) — Common design patterns

### Cookbook

- [Recipes](docs/4.cookbook/README.md) — Complete examples for common scenarios

### Reference

- [API Reference](docs/5.reference/1.api.md) — Complete processor documentation

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. Run `make help` for available commands.

## License

MIT License — see [LICENSE](LICENSE) for details.
