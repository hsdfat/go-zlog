package sink

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// BufferedSink decouples log producers from the underlying sink. Write only
// enqueues onto a buffered channel and returns immediately (dropping when the
// channel is full); a single background worker goroutine owns ALL contact with
// the underlying sink — batching, flushing, and retry/backoff. A producer log
// call therefore never blocks on network I/O, a slow/dead collector, or a
// mutex, no matter how the underlying sink behaves.
type BufferedSink struct {
	sink   Sink
	config *Config

	ch       chan *LogEntry
	stopChan chan struct{}
	wg       sync.WaitGroup

	droppedCount uint64 // atomic
	sentCount    uint64 // atomic
}

// NewBufferedSink creates a buffered sink and starts its background worker.
func NewBufferedSink(sink Sink, config *Config) *BufferedSink {
	if config == nil {
		config = DefaultConfig()
	}
	bufSize := config.BufferSize
	if bufSize <= 0 {
		bufSize = 1000
	}
	bs := &BufferedSink{
		sink:     sink,
		config:   config,
		ch:       make(chan *LogEntry, bufSize),
		stopChan: make(chan struct{}),
	}
	bs.wg.Add(1)
	go bs.worker()
	return bs
}

// Write enqueues one entry WITHOUT blocking the caller. If the buffer channel
// is full the entry is dropped (counted) rather than blocking the producer —
// log shipping must never stall the application. The DropOnFull config flag is
// intentionally ignored: a non-blocking producer is the whole point.
func (bs *BufferedSink) Write(_ context.Context, entry *LogEntry) error {
	select {
	case bs.ch <- entry:
	default:
		atomic.AddUint64(&bs.droppedCount, 1)
	}
	return nil
}

// WriteBatch enqueues each entry (still non-blocking per entry).
func (bs *BufferedSink) WriteBatch(ctx context.Context, entries []*LogEntry) error {
	for _, e := range entries {
		_ = bs.Write(ctx, e)
	}
	return nil
}

// Flush is best-effort: the worker flushes on its own schedule (FlushInterval)
// and again on Close. It returns nil immediately so a logger Sync() can never
// block the caller on the network.
func (bs *BufferedSink) Flush(_ context.Context) error { return nil }

// worker is the ONLY goroutine that touches the underlying sink. It batches by
// MaxBatchSize and FlushInterval; all send/retry/backoff happens here, off the
// producer path.
func (bs *BufferedSink) worker() {
	defer bs.wg.Done()

	flushInterval := bs.config.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	maxBatch := bs.config.MaxBatchSize
	if maxBatch <= 0 {
		maxBatch = 100
	}
	batch := make([]*LogEntry, 0, maxBatch)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		bs.send(batch)
		batch = batch[:0]
	}

	for {
		select {
		case e := <-bs.ch:
			batch = append(batch, e)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-bs.stopChan:
			// Drain whatever is still queued, then a final flush, then exit.
			for {
				select {
				case e := <-bs.ch:
					batch = append(batch, e)
					if len(batch) >= maxBatch {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// send writes one batch to the underlying sink with bounded retry/backoff.
// It runs ONLY in the worker, so a failing or slow collector stalls log
// shipping (and drops once the channel fills) but never blocks a log producer.
func (bs *BufferedSink) send(batch []*LogEntry) {
	writeTimeout := bs.config.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 5 * time.Second
	}
	retryInterval := bs.config.RetryInterval

	for attempt := 0; attempt <= bs.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(retryInterval):
			case <-bs.stopChan:
				atomic.AddUint64(&bs.droppedCount, uint64(len(batch)))
				return
			}
			retryInterval *= 2
		}

		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err := bs.sink.WriteBatch(ctx, batch)
		cancel()
		if err == nil {
			atomic.AddUint64(&bs.sentCount, uint64(len(batch)))
			return
		}
	}
	atomic.AddUint64(&bs.droppedCount, uint64(len(batch)))
}

// Close stops the worker (which drains + does a final flush) and then closes
// the underlying sink.
func (bs *BufferedSink) Close() error {
	close(bs.stopChan)
	bs.wg.Wait()
	return bs.sink.Close()
}

// IsHealthy reports the underlying sink's health.
func (bs *BufferedSink) IsHealthy() bool { return bs.sink.IsHealthy() }

// Stats returns buffering statistics: total sent, total dropped, and the number
// of entries currently queued in the channel.
func (bs *BufferedSink) Stats() (sent, dropped, buffered uint64) {
	return atomic.LoadUint64(&bs.sentCount), atomic.LoadUint64(&bs.droppedCount), uint64(len(bs.ch))
}
