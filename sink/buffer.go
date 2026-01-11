package sink

import (
	"context"
	"sync"
	"time"
)

// BufferedSink wraps a Sink with buffering and batching capabilities
type BufferedSink struct {
	sink          Sink
	config        *Config
	buffer        []*LogEntry
	bufferMu      sync.Mutex
	flushTicker   *time.Ticker
	stopChan      chan struct{}
	wg            sync.WaitGroup
	droppedCount  uint64
	sentCount     uint64
}

// NewBufferedSink creates a new buffered sink wrapper
func NewBufferedSink(sink Sink, config *Config) *BufferedSink {
	if config == nil {
		config = DefaultConfig()
	}

	bs := &BufferedSink{
		sink:        sink,
		config:      config,
		buffer:      make([]*LogEntry, 0, config.BufferSize),
		flushTicker: time.NewTicker(config.FlushInterval),
		stopChan:    make(chan struct{}),
	}

	// Start background flusher
	bs.wg.Add(1)
	go bs.backgroundFlusher()

	return bs
}

// Write adds a log entry to the buffer
func (bs *BufferedSink) Write(ctx context.Context, entry *LogEntry) error {
	bs.bufferMu.Lock()
	defer bs.bufferMu.Unlock()

	// Check if buffer is full
	if len(bs.buffer) >= bs.config.BufferSize {
		if bs.config.DropOnFull {
			bs.droppedCount++
			return nil // Drop the log
		}
		// Flush synchronously if buffer is full and not dropping
		if err := bs.flushBuffer(ctx); err != nil {
			return err
		}
	}

	// Add to buffer
	bs.buffer = append(bs.buffer, entry)

	// Flush immediately if buffer reaches max batch size
	if len(bs.buffer) >= bs.config.MaxBatchSize {
		return bs.flushBuffer(ctx)
	}

	return nil
}

// WriteBatch adds multiple log entries to the buffer
func (bs *BufferedSink) WriteBatch(ctx context.Context, entries []*LogEntry) error {
	for _, entry := range entries {
		if err := bs.Write(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// Flush forces a flush of the buffer
func (bs *BufferedSink) Flush(ctx context.Context) error {
	bs.bufferMu.Lock()
	defer bs.bufferMu.Unlock()
	return bs.flushBuffer(ctx)
}

// flushBuffer sends buffered logs to the underlying sink (must be called with lock held)
func (bs *BufferedSink) flushBuffer(ctx context.Context) error {
	if len(bs.buffer) == 0 {
		return nil
	}

	// Create a copy of the buffer to send
	toSend := make([]*LogEntry, len(bs.buffer))
	copy(toSend, bs.buffer)

	// Clear the buffer immediately
	bs.buffer = bs.buffer[:0]

	// Send in batches
	for i := 0; i < len(toSend); i += bs.config.MaxBatchSize {
		end := i + bs.config.MaxBatchSize
		if end > len(toSend) {
			end = len(toSend)
		}

		batch := toSend[i:end]
		if err := bs.retryWriteBatch(ctx, batch); err != nil {
			// Re-add failed logs to buffer if not dropping
			if !bs.config.DropOnFull {
				bs.buffer = append(bs.buffer, batch...)
			} else {
				bs.droppedCount += uint64(len(batch))
			}
			return err
		}
		bs.sentCount += uint64(len(batch))
	}

	return nil
}

// retryWriteBatch attempts to write a batch with retry logic
func (bs *BufferedSink) retryWriteBatch(ctx context.Context, batch []*LogEntry) error {
	var lastErr error
	retryInterval := bs.config.RetryInterval

	for attempt := 0; attempt <= bs.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			select {
			case <-time.After(retryInterval):
			case <-ctx.Done():
				return ctx.Err()
			case <-bs.stopChan:
				return nil
			}
			retryInterval *= 2
		}

		// Create timeout context for this attempt
		writeCtx, cancel := context.WithTimeout(ctx, bs.config.WriteTimeout)
		err := bs.sink.WriteBatch(writeCtx, batch)
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
	}

	return lastErr
}

// backgroundFlusher periodically flushes the buffer
func (bs *BufferedSink) backgroundFlusher() {
	defer bs.wg.Done()

	for {
		select {
		case <-bs.flushTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), bs.config.WriteTimeout)
			_ = bs.Flush(ctx) // Ignore errors in background flush
			cancel()

		case <-bs.stopChan:
			// Final flush on shutdown
			ctx, cancel := context.WithTimeout(context.Background(), bs.config.WriteTimeout*2)
			_ = bs.Flush(ctx)
			cancel()
			return
		}
	}
}

// Close gracefully shuts down the buffered sink
func (bs *BufferedSink) Close() error {
	close(bs.stopChan)
	bs.flushTicker.Stop()
	bs.wg.Wait()
	return bs.sink.Close()
}

// IsHealthy checks if the underlying sink is healthy
func (bs *BufferedSink) IsHealthy() bool {
	return bs.sink.IsHealthy()
}

// Stats returns buffering statistics
func (bs *BufferedSink) Stats() (sent, dropped, buffered uint64) {
	bs.bufferMu.Lock()
	defer bs.bufferMu.Unlock()
	return bs.sentCount, bs.droppedCount, uint64(len(bs.buffer))
}
