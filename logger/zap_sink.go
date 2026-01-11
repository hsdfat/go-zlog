package logger

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/hsdfat/go-zlog/sink"
	"go.uber.org/zap/zapcore"
)

// zapSinkCore implements zapcore.Core to forward logs to a Sink
type zapSinkCore struct {
	zapcore.LevelEnabler
	sink       sink.Sink
	enc        zapcore.Encoder
	hostname   string
	fields     map[string]any
	callerSkip int
}

// newZapSinkCore creates a new zapcore.Core that writes to a Sink
func newZapSinkCore(s sink.Sink, enc zapcore.Encoder, enab zapcore.LevelEnabler) zapcore.Core {
	hostname, _ := os.Hostname()
	return &zapSinkCore{
		LevelEnabler: enab,
		sink:         s,
		enc:          enc,
		hostname:     hostname,
		fields:       make(map[string]any),
		callerSkip:   0,
	}
}

// With adds structured context to the Core
func (c *zapSinkCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &zapSinkCore{
		LevelEnabler: c.LevelEnabler,
		sink:         c.sink,
		enc:          c.enc.Clone(),
		hostname:     c.hostname,
		fields:       make(map[string]any, len(c.fields)+len(fields)),
		callerSkip:   c.callerSkip,
	}

	// Copy existing fields
	for k, v := range c.fields {
		clone.fields[k] = v
	}

	// Add new fields
	for _, field := range fields {
		clone.fields[field.Key] = fieldValue(field)
	}

	return clone
}

// Check determines whether the supplied Entry should be logged
func (c *zapSinkCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

// Write serializes the Entry and any Fields supplied at the log site and writes them to the Sink
func (c *zapSinkCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	// Merge fields
	allFields := make(map[string]any, len(c.fields)+len(fields))
	for k, v := range c.fields {
		allFields[k] = v
	}
	for _, field := range fields {
		allFields[field.Key] = fieldValue(field)
	}

	// Build log entry
	entry := &sink.LogEntry{
		Timestamp: ent.Time,
		Level:     levelToString(ent.Level),
		Message:   ent.Message,
		Fields:    allFields,
		Hostname:  c.hostname,
	}

	// Add caller information if present
	if ent.Caller.Defined {
		entry.Caller = ent.Caller.String()
	}

	// Add stack trace if present
	if ent.Stack != "" {
		entry.StackTrace = ent.Stack
	}

	// Write to sink asynchronously
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.sink.Write(ctx, entry)
}

// Sync flushes buffered logs
func (c *zapSinkCore) Sync() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.sink.Flush(ctx)
}

// fieldValue extracts the value from a zapcore.Field
func fieldValue(f zapcore.Field) any {
	switch f.Type {
	case zapcore.BoolType:
		return f.Integer == 1
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
		return f.Integer
	case zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type:
		return uint64(f.Integer)
	case zapcore.Float64Type, zapcore.Float32Type:
		return f.Integer
	case zapcore.StringType:
		return f.String
	case zapcore.TimeType:
		if f.Interface != nil {
			return f.Interface.(time.Time)
		}
		return time.Unix(0, f.Integer)
	case zapcore.DurationType:
		return time.Duration(f.Integer)
	case zapcore.ErrorType:
		return f.Interface
	default:
		return f.Interface
	}
}

// levelToString converts zapcore.Level to string
func levelToString(level zapcore.Level) string {
	switch level {
	case zapcore.DebugLevel:
		return "debug"
	case zapcore.InfoLevel:
		return "info"
	case zapcore.WarnLevel:
		return "warn"
	case zapcore.ErrorLevel:
		return "error"
	case zapcore.DPanicLevel, zapcore.PanicLevel:
		return "panic"
	case zapcore.FatalLevel:
		return "fatal"
	default:
		return "unknown"
	}
}

// callerEncoder gets the caller information
func getCaller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	// Trim to just filename
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}
	return file + ":" + string(rune(line))
}
