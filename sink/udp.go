package sink

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
)

// UDPSinkConfig holds UDP-specific configuration.
type UDPSinkConfig struct {
	*Config
	Address string // "host:port" of the collector
}

// UDPSink sends each log entry as a single JSON datagram over a reused,
// UNCONNECTED UDP socket. An unconnected socket is deliberate: a dead or
// unreachable collector never surfaces an ICMP "connection refused" on a
// later write (that only happens on a *connected* socket from net.Dial), so
// every send stays pure fire-and-forget and never triggers the BufferedSink
// retry-backoff that would otherwise stall the logging hot path.
type UDPSink struct {
	config    *UDPSinkConfig
	conn      *net.UDPConn
	raddr     *net.UDPAddr
	isHealthy atomic.Bool
	lastError atomic.Value
}

// NewUDPSink resolves the collector address once and opens a single
// unconnected UDP socket reused for every send (no per-send dial).
func NewUDPSink(config *UDPSinkConfig) (*UDPSink, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.Config == nil {
		config.Config = DefaultConfig()
	}
	if config.Address == "" {
		return nil, fmt.Errorf("address is required")
	}
	raddr, err := net.ResolveUDPAddr("udp", config.Address)
	if err != nil {
		return nil, fmt.Errorf("resolve udp %s: %w", config.Address, err)
	}
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("open udp socket: %w", err)
	}
	s := &UDPSink{config: config, conn: conn, raddr: raddr}
	s.isHealthy.Store(true)
	return s, nil
}

// Write sends one entry as one JSON datagram.
func (s *UDPSink) Write(ctx context.Context, entry *LogEntry) error {
	s.enrich(entry)
	payload, err := json.Marshal(entry)
	if err != nil {
		s.recordError(fmt.Errorf("marshal log: %w", err))
		return err
	}
	// Fire-and-forget to a fixed addr on the unconnected socket: never blocks
	// and never returns ICMP "connection refused" for a dead collector.
	if _, err := s.conn.WriteToUDP(payload, s.raddr); err != nil {
		s.recordError(fmt.Errorf("udp write: %w", err))
		return err
	}
	s.isHealthy.Store(true)
	return nil
}

// WriteBatch sends one datagram per entry (no multi-line packing — MTU-safe).
func (s *UDPSink) WriteBatch(ctx context.Context, entries []*LogEntry) error {
	for _, e := range entries {
		if err := s.Write(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// Flush is a no-op (BufferedSink owns batching).
func (s *UDPSink) Flush(ctx context.Context) error { return nil }

// Close closes the UDP socket.
func (s *UDPSink) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// IsHealthy reports the last-known socket health.
func (s *UDPSink) IsHealthy() bool { return s.isHealthy.Load() }

// LastError returns the last recorded error.
func (s *UDPSink) LastError() error {
	if v := s.lastError.Load(); v != nil {
		return v.(error)
	}
	return nil
}

// enrich fills service identity from config when the entry omits it.
func (s *UDPSink) enrich(entry *LogEntry) {
	if entry.ServiceName == "" {
		entry.ServiceName = s.config.ServiceName
	}
	if entry.InstanceID == "" {
		entry.InstanceID = s.config.InstanceID
	}
	if entry.Environment == "" {
		entry.Environment = s.config.Environment
	}
}

func (s *UDPSink) recordError(err error) {
	s.isHealthy.Store(false)
	s.lastError.Store(err)
}
