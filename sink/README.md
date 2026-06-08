# go-zlog Sink Package

Pluggable log sinks for centralized logging with buffering, batching, and retry logic.

## Features

- **Multiple Backends**: Grafana Loki, HTTP, or custom implementations
- **Buffering**: In-memory buffering with configurable size
- **Batching**: Group logs for efficient transmission
- **Retry Logic**: Exponential backoff retry on failures
- **Async Processing**: Non-blocking log writes
- **Health Monitoring**: Track sink health and statistics

## Installation

```bash
go get github.com/hsdfat/go-zlog@latest
```

## Quick Start

### Using with Grafana Loki

```go
package main

import (
    "github.com/hsdfat/go-zlog/logger"
    "github.com/hsdfat/go-zlog/sink"
)

func main() {
    // Create Loki sink
    lokiConfig := &sink.LokiSinkConfig{
        Config: sink.DefaultConfig(),
        URL:    "http://localhost:3100/loki/api/v1/push",
        Labels: map[string]string{
            "service":     "my-app",
            "environment": "production",
        },
    }

    lokiSink, err := sink.NewLokiSink(lokiConfig)
    if err != nil {
        panic(err)
    }

    // Wrap with buffering
    bufferedSink := sink.NewBufferedSink(lokiSink, lokiConfig.Config)
    defer bufferedSink.Close()

    // Create logger with sink
    log := logger.NewLoggerWithConfig(&logger.LoggerConfig{
        EnableConsole: true,
        RemoteSinks:   []sink.Sink{bufferedSink},
    })

    // Use logger
    log.Infow("Application started", "version", "1.0.0")
}
```

### Using Generic HTTP Sink

```go
httpConfig := &sink.HTTPSinkConfig{
    Config:      sink.DefaultConfig(),
    URL:         "https://logs.example.com/api/v1/logs",
    Method:      "POST",
    ContentType: "application/json",
    BearerToken: "your-secret-token",
}

httpSink, err := sink.NewHTTPSink(httpConfig)
if err != nil {
    panic(err)
}

bufferedSink := sink.NewBufferedSink(httpSink, httpConfig.Config)
```

## Configuration

### Sink Config

```go
config := &sink.Config{
    // Service metadata
    ServiceName: "my-app",
    InstanceID:  "instance-01",
    Environment: "production",

    // Buffering
    BufferSize:    1000,           // Number of logs to buffer
    FlushInterval: 5 * time.Second, // Auto-flush interval
    MaxBatchSize:  100,            // Max logs per batch

    // Retry
    MaxRetries:    3,                // Max retry attempts
    RetryInterval: 1 * time.Second,  // Initial retry interval
    RetryTimeout:  30 * time.Second, // Total retry timeout

    // Connection
    ConnTimeout:  10 * time.Second,
    WriteTimeout: 5 * time.Second,

    // Performance
    WorkerPoolSize: 2,  // Concurrent workers

    // Behavior
    DropOnFull: false,  // Drop logs when buffer full
    AsyncWrite: true,   // Async writes
}
```

### Loki-Specific Config

```go
lokiConfig := &sink.LokiSinkConfig{
    Config:      config,
    URL:         "http://loki:3100/loki/api/v1/push",
    TenantID:    "my-tenant",  // Optional multi-tenancy
    BearerToken: "token",      // Optional auth
    Labels: map[string]string{ // Static labels
        "app":         "my-app",
        "environment": "prod",
    },
}
```

## Log Entry Format

Logs are sent with structured metadata:

```json
{
  "timestamp": "2025-01-11T10:30:00Z",
  "level": "info",
  "message": "Request processed",
  "fields": {
    "request_id": "abc123",
    "duration_ms": 45
  },
  "service_name": "my-app",
  "instance_id": "instance-01",
  "environment": "production",
  "hostname": "server-01",
  "caller": "handler.go:42"
}
```

## Buffering & Batching

### How It Works

1. **Buffering**: Logs are added to an in-memory buffer
2. **Batching**: Buffer is split into batches when:
   - Buffer reaches `MaxBatchSize`
   - `FlushInterval` timer triggers
   - `Flush()` is called explicitly
3. **Sending**: Batches are sent to the sink with retry logic

### Buffer Statistics

```go
sent, dropped, buffered := bufferedSink.Stats()
fmt.Printf("Sent: %d, Dropped: %d, Buffered: %d\n",
    sent, dropped, buffered)
```

## Retry Logic

Automatic retry with exponential backoff:

1. **Initial attempt**: Send logs immediately
2. **Retry 1**: Wait 1 second (RetryInterval)
3. **Retry 2**: Wait 2 seconds (exponential backoff)
4. **Retry 3**: Wait 4 seconds
5. **Give up**: After MaxRetries attempts

## Health Monitoring

```go
if sink.IsHealthy() {
    fmt.Println("Sink is operational")
} else {
    if err := sink.LastError(); err != nil {
        fmt.Printf("Sink error: %v\n", err)
    }
}
```

## Custom Sink Implementation

Implement the `Sink` interface:

```go
type CustomSink struct {
    // Your fields
}

func (s *CustomSink) Write(ctx context.Context, entry *sink.LogEntry) error {
    // Send single log entry
    return nil
}

func (s *CustomSink) WriteBatch(ctx context.Context, entries []*sink.LogEntry) error {
    // Send batch of log entries
    return nil
}

func (s *CustomSink) Flush(ctx context.Context) error {
    // Flush any pending logs
    return nil
}

func (s *CustomSink) Close() error {
    // Cleanup resources
    return nil
}

func (s *CustomSink) IsHealthy() bool {
    // Return health status
    return true
}
```

## Best Practices

### 1. Always Use BufferedSink

Wrap your sink with `BufferedSink` for better performance:

```go
rawSink, _ := sink.NewLokiSink(config)
bufferedSink := sink.NewBufferedSink(rawSink, config.Config)
```

### 2. Tune Buffer Size

- **High volume**: Larger buffer (5000+), frequent flush (2-3s)
- **Low volume**: Smaller buffer (500), less frequent flush (10s)

### 3. Handle Errors Gracefully

```go
if err := logger.InitializeWithConfig(cfg); err != nil {
    log.Warnw("Failed to init centralized logging", "error", err)
    // Continue with console logging only
}
```

### 4. Close Sinks on Shutdown

```go
defer bufferedSink.Close()
```

### 5. Monitor Statistics

Periodically check buffer stats:

```go
ticker := time.NewTicker(1 * time.Minute)
go func() {
    for range ticker.C {
        sent, dropped, buffered := bufferedSink.Stats()
        if dropped > 0 {
            log.Warnw("Logs dropped", "count", dropped)
        }
    }
}()
```

## Performance

### Benchmarks

Typical performance on modern hardware:

- **Throughput**: 10,000+ logs/sec with batching
- **Latency**: <1ms for buffered writes
- **Memory**: ~1-5 MB per 1000 buffered logs

### Tuning for High Load

```go
config := &sink.Config{
    BufferSize:     10000,  // Larger buffer
    MaxBatchSize:   1000,   // Larger batches
    FlushInterval:  2 * time.Second,
    WorkerPoolSize: 4,      // More workers
    DropOnFull:     true,   // Prevent blocking
}
```

## Troubleshooting

### Logs Not Appearing

1. Check sink health: `sink.IsHealthy()`
2. Check last error: `sink.LastError()`
3. Verify URL is reachable
4. Check authentication credentials

### High Memory Usage

1. Reduce `BufferSize`
2. Increase `FlushInterval` to flush more often
3. Enable `DropOnFull` mode

### Logs Being Dropped

1. Check stats: `bufferedSink.Stats()`
2. Increase `BufferSize`
3. Increase `MaxBatchSize`
4. Add more `WorkerPoolSize`

## Examples

See [examples](../examples/) directory for complete examples:

- `loki_simple.go` - Basic Loki integration
- `http_custom.go` - Custom HTTP endpoint
- `multi_sink.go` - Multiple sinks simultaneously
- `production.go` - Production-ready configuration

## License

MIT License - See LICENSE file for details
