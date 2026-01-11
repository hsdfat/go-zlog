package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// HTTPSinkConfig holds HTTP-specific configuration
type HTTPSinkConfig struct {
	*Config
	URL         string            // HTTP endpoint URL
	Method      string            // HTTP method (default: POST)
	Headers     map[string]string // Additional HTTP headers
	ContentType string            // Content-Type header (default: application/json)
	BearerToken string            // Optional bearer token for authentication
	BasicAuth   *BasicAuth        // Optional basic authentication
}

// BasicAuth holds basic authentication credentials
type BasicAuth struct {
	Username string
	Password string
}

// HTTPSink sends logs to an HTTP endpoint
type HTTPSink struct {
	config     *HTTPSinkConfig
	client     *http.Client
	isHealthy  atomic.Bool
	lastError  atomic.Value
}

// NewHTTPSink creates a new HTTP sink
func NewHTTPSink(config *HTTPSinkConfig) (*HTTPSink, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.Config == nil {
		config.Config = DefaultConfig()
	}
	if config.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}
	if config.Method == "" {
		config.Method = http.MethodPost
	}
	if config.ContentType == "" {
		config.ContentType = "application/json"
	}

	sink := &HTTPSink{
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
func (s *HTTPSink) Write(ctx context.Context, entry *LogEntry) error {
	return s.WriteBatch(ctx, []*LogEntry{entry})
}

// WriteBatch sends multiple log entries
func (s *HTTPSink) WriteBatch(ctx context.Context, entries []*LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Serialize entries to JSON
	payload, err := json.Marshal(map[string]any{
		"logs": entries,
	})
	if err != nil {
		s.recordError(fmt.Errorf("failed to marshal logs: %w", err))
		return err
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, s.config.Method, s.config.URL, bytes.NewReader(payload))
	if err != nil {
		s.recordError(fmt.Errorf("failed to create request: %w", err))
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", s.config.ContentType)
	for key, value := range s.config.Headers {
		req.Header.Set(key, value)
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
		err := fmt.Errorf("HTTP error: %d %s - %s", resp.StatusCode, resp.Status, string(body))
		s.recordError(err)
		return err
	}

	s.isHealthy.Store(true)
	return nil
}

// Flush is a no-op for HTTP sink (handled by BufferedSink)
func (s *HTTPSink) Flush(ctx context.Context) error {
	return nil
}

// Close closes the HTTP client
func (s *HTTPSink) Close() error {
	s.client.CloseIdleConnections()
	return nil
}

// IsHealthy returns the health status
func (s *HTTPSink) IsHealthy() bool {
	return s.isHealthy.Load()
}

// LastError returns the last error encountered
func (s *HTTPSink) LastError() error {
	if val := s.lastError.Load(); val != nil {
		return val.(error)
	}
	return nil
}

// recordError records an error and marks the sink as unhealthy
func (s *HTTPSink) recordError(err error) {
	s.isHealthy.Store(false)
	s.lastError.Store(err)
}
