# Installation

## Requirements

- **Go 1.21 or later** (required for generics support)
- No external dependencies - streamz uses only the Go standard library

## Install streamz

```bash
go get github.com/zoobzio/streamz
```

## Verify Installation

Create a simple test file to verify everything works:

```go
// main.go
package main

import (
    "context"
    "fmt"
    "github.com/zoobzio/streamz"
)

func main() {
    ctx := context.Background()
    
    // Create a simple filter
    filter := streamz.NewFilter("positive", func(n int) bool {
        return n > 0
    })
    
    // Create input channel
    numbers := make(chan int)
    
    // Process the stream
    positive := filter.Process(ctx, numbers)
    
    // Send some data
    go func() {
        for _, n := range []int{-1, 2, -3, 4, 5} {
            numbers <- n
        }
        close(numbers)
    }()
    
    // Read results
    for n := range positive {
        fmt.Printf("Positive number: %d\n", n)
    }
    
    fmt.Println("streamz is working!")
}
```

Run it:

```bash
go run main.go
```

Expected output:
```
Positive number: 2
Positive number: 4
Positive number: 5
streamz is working!
```

## Import Path

```go
import "github.com/zoobzio/streamz"
```

All processors and utilities are available directly from the `streamz` package:

```go
batcher := streamz.NewBatcher[Order](config)
filter := streamz.NewFilter("valid", isValid)
mapper := streamz.NewMapper("enrich", enrichData)
```

## Development Setup

If you plan to contribute to streamz, you'll need additional tools:

### Install Development Tools

```bash
# Clone the repository
git clone https://github.com/zoobzio/streamz.git
cd streamz

# Install development tools
make install-tools
```

This installs:
- golangci-lint for comprehensive code analysis
- Additional linters and security scanners

### Verify Development Environment

```bash
# Run tests
make test

# Run linters
make lint

# Run benchmarks
make bench

# Run everything
make check
```

## Module Information

```go
module github.com/zoobzio/streamz

go 1.21

// No external dependencies!
```

streamz is designed as a zero-dependency library. It uses only the Go standard library, making it:
- Fast to build
- Easy to vendor
- No version conflicts
- Minimal attack surface

## Next Steps

Now that streamz is installed, check out the [Quick Start Guide](./quick-start.md) to build your first pipeline!