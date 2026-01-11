package sink

import (
	"context"
	"time"
)

// LogEntry represents a structured log entry to be sent to remote sink
type LogEntry struct {
	Timestamp   time.Time         `json:"timestamp"`
	Level       string            `json:"level"`
	Message     string            `json:"message"`
	Fields      map[string]any    `json:"fields,omitempty"`
	ServiceName string            `json:"service_name"`
	InstanceID  string            `json:"instance_id,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Hostname    string            `json:"hostname,omitempty"`
	Caller      string            `json:"caller,omitempty"`
	StackTrace  string            `json:"stack_trace,omitempty"`
}

// Sink interface for pluggable log destinations
type Sink interface {
	// Write sends a single log entry to the sink
	Write(ctx context.Context, entry *LogEntry) error

	// WriteBatch sends multiple log entries in a batch
	WriteBatch(ctx context.Context, entries []*LogEntry) error

	// Flush ensures all buffered logs are sent
	Flush(ctx context.Context) error

	// Close gracefully shuts down the sink
	Close() error

	// IsHealthy checks if the sink is operational
	IsHealthy() bool
}

// Config holds common configuration for all sinks
type Config struct {
	// Service metadata
	ServiceName string
	InstanceID  string
	Environment string

	// Buffering configuration
	BufferSize      int           // Number of logs to buffer before flushing
	FlushInterval   time.Duration // Time interval to flush buffer
	MaxBatchSize    int           // Maximum number of logs in a single batch

	// Retry configuration
	MaxRetries      int           // Maximum number of retry attempts
	RetryInterval   time.Duration // Initial retry interval (exponential backoff)
	RetryTimeout    time.Duration // Maximum time to retry

	// Connection configuration
	ConnTimeout     time.Duration // Connection timeout
	WriteTimeout    time.Duration // Write operation timeout

	// Performance tuning
	WorkerPoolSize  int           // Number of concurrent workers for sending logs

	// Behavior configuration
	DropOnFull      bool          // Drop logs if buffer is full (instead of blocking)
	AsyncWrite      bool          // Write logs asynchronously
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ServiceName:     "unknown",
		Environment:     "development",
		BufferSize:      1000,
		FlushInterval:   5 * time.Second,
		MaxBatchSize:    100,
		MaxRetries:      3,
		RetryInterval:   1 * time.Second,
		RetryTimeout:    30 * time.Second,
		ConnTimeout:     10 * time.Second,
		WriteTimeout:    5 * time.Second,
		WorkerPoolSize:  2,
		DropOnFull:      false,
		AsyncWrite:      true,
	}
}
