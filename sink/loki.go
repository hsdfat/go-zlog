package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// LokiSinkConfig holds Loki-specific configuration
type LokiSinkConfig struct {
	*Config
	URL         string            // Loki push API URL (e.g., http://loki:3100/loki/api/v1/push)
	TenantID    string            // Optional tenant ID for multi-tenancy
	Labels      map[string]string // Static labels to add to all logs
	BearerToken string            // Optional bearer token for authentication
	BasicAuth   *BasicAuth        // Optional basic authentication
}

// LokiSink sends logs to Grafana Loki
type LokiSink struct {
	config    *LokiSinkConfig
	client    *http.Client
	isHealthy atomic.Bool
	lastError atomic.Value
}

// lokiPushRequest represents the Loki push API request format
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

// lokiStream represents a single log stream in Loki
type lokiStream struct {
	Stream map[string]string `json:"stream"` // Labels
	Values [][]string        `json:"values"` // [timestamp_ns, log_line]
}

// NewLokiSink creates a new Loki sink
func NewLokiSink(config *LokiSinkConfig) (*LokiSink, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.Config == nil {
		config.Config = DefaultConfig()
	}
	if config.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// Ensure required labels are set
	if config.ServiceName != "" && config.Labels["service"] == "" {
		config.Labels["service"] = config.ServiceName
	}
	if config.Environment != "" && config.Labels["environment"] == "" {
		config.Labels["environment"] = config.Environment
	}
	if config.InstanceID != "" && config.Labels["instance"] == "" {
		config.Labels["instance"] = config.InstanceID
	}

	sink := &LokiSink{
		config: config,
		client: &http.Client{
			Timeout: config.ConnTimeout + config.WriteTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	sink.isHealthy.Store(true)
	return sink, nil
}

// Write sends a single log entry
func (s *LokiSink) Write(ctx context.Context, entry *LogEntry) error {
	return s.WriteBatch(ctx, []*LogEntry{entry})
}

// WriteBatch sends multiple log entries to Loki
func (s *LokiSink) WriteBatch(ctx context.Context, entries []*LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Group entries by their labels (for Loki streams)
	streamMap := make(map[string]*lokiStream)

	for _, entry := range entries {
		// Build labels for this entry
		labels := s.buildLabels(entry)
		streamKey := s.labelsToKey(labels)

		// Get or create stream
		stream, exists := streamMap[streamKey]
		if !exists {
			stream = &lokiStream{
				Stream: labels,
				Values: make([][]string, 0),
			}
			streamMap[streamKey] = stream
		}

		// Convert entry to Loki format
		timestamp := strconv.FormatInt(entry.Timestamp.UnixNano(), 10)
		logLine := s.formatLogLine(entry)
		stream.Values = append(stream.Values, []string{timestamp, logLine})
	}

	// Build Loki push request
	streams := make([]lokiStream, 0, len(streamMap))
	for _, stream := range streamMap {
		streams = append(streams, *stream)
	}

	pushReq := lokiPushRequest{
		Streams: streams,
	}

	// Serialize to JSON
	payload, err := json.Marshal(pushReq)
	if err != nil {
		s.recordError(fmt.Errorf("failed to marshal logs: %w", err))
		return err
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.URL, bytes.NewReader(payload))
	if err != nil {
		s.recordError(fmt.Errorf("failed to create request: %w", err))
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if s.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", s.config.TenantID)
	}

	// Add authentication
	if s.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.BearerToken)
	} else if s.config.BasicAuth != nil {
		req.SetBasicAuth(s.config.BasicAuth.Username, s.config.BasicAuth.Password)
	}

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		s.recordError(fmt.Errorf("failed to send logs: %w", err))
		return err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("Loki error: %d %s - %s", resp.StatusCode, resp.Status, string(body))
		s.recordError(err)
		return err
	}

	s.isHealthy.Store(true)
	return nil
}

// buildLabels creates the label set for a log entry
func (s *LokiSink) buildLabels(entry *LogEntry) map[string]string {
	labels := make(map[string]string)

	// Copy static labels
	for k, v := range s.config.Labels {
		labels[k] = v
	}

	// Add dynamic labels
	labels["level"] = entry.Level
	if entry.Hostname != "" {
		labels["hostname"] = entry.Hostname
	}

	return labels
}

// labelsToKey creates a unique key for a label set
func (s *LokiSink) labelsToKey(labels map[string]string) string {
	// Simple JSON serialization for grouping
	data, _ := json.Marshal(labels)
	return string(data)
}

// formatLogLine formats a log entry as a single line for Loki
func (s *LokiSink) formatLogLine(entry *LogEntry) string {
	// Create a structured log line
	logData := map[string]any{
		"msg": entry.Message,
	}

	// Add fields
	if len(entry.Fields) > 0 {
		for k, v := range entry.Fields {
			logData[k] = v
		}
	}

	// Add caller if present
	if entry.Caller != "" {
		logData["caller"] = entry.Caller
	}

	// Add stack trace if present
	if entry.StackTrace != "" {
		logData["stack_trace"] = entry.StackTrace
	}

	// Serialize to JSON
	data, _ := json.Marshal(logData)
	return string(data)
}

// Flush is a no-op for Loki sink (handled by BufferedSink)
func (s *LokiSink) Flush(ctx context.Context) error {
	return nil
}

// Close closes the HTTP client
func (s *LokiSink) Close() error {
	s.client.CloseIdleConnections()
	return nil
}

// IsHealthy returns the health status
func (s *LokiSink) IsHealthy() bool {
	return s.isHealthy.Load()
}

// LastError returns the last error encountered
func (s *LokiSink) LastError() error {
	if val := s.lastError.Load(); val != nil {
		return val.(error)
	}
	return nil
}

// recordError records an error and marks the sink as unhealthy
func (s *LokiSink) recordError(err error) {
	s.isHealthy.Store(false)
	s.lastError.Store(err)
}
