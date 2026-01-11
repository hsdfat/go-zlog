package logger

import (
	"os"

	"github.com/hsdfat/go-zlog/sink"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LoggerI interface {
	Infow(msg string, args ...interface{})
	Warnw(msg string, args ...interface{})
	Errorw(msg string, args ...interface{})
	Debugw(msg string, args ...interface{})
	Fatalw(msg string, args ...interface{})

	Infof(template string, args ...interface{})
	Debugf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})

	Info(args ...interface{})
	Debug(args ...interface{})
	Error(args ...interface{})
	Warn(args ...interface{})
	Fatal(args ...interface{})

	Infoln(args ...interface{})
	Debugln(args ...interface{})
	Errorln(args ...interface{})
	Warnln(args ...interface{})
	Fatalln(args ...interface{})

	With(args ...any) any
}

var (
	level       = zap.NewAtomicLevel()
	remoteSinks []sink.Sink
)

type Logger struct {
	*zap.SugaredLogger
	cores []zapcore.Core
}

// LoggerConfig holds configuration for logger creation
type LoggerConfig struct {
	EnableConsole bool        // Enable console output (default: true)
	RemoteSinks   []sink.Sink // Optional remote sinks (e.g., Loki, HTTP)
}

// NewLogger creates a new logger with default configuration (console only)
func NewLogger() *Logger {
	return NewLoggerWithConfig(&LoggerConfig{
		EnableConsole: true,
		RemoteSinks:   nil,
	})
}

// NewLoggerWithConfig creates a new logger with custom configuration
func NewLoggerWithConfig(config *LoggerConfig) *Logger {
	if config == nil {
		config = &LoggerConfig{EnableConsole: true}
	}

	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// Create cores
	cores := []zapcore.Core{}

	// Add console core if enabled
	if config.EnableConsole {
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(cfg),
			zapcore.AddSync(zapcore.Lock(zapcore.NewMultiWriteSyncer(os.Stderr))),
			level,
		)
		cores = append(cores, consoleCore)
	}

	// Add remote sink cores
	if config.RemoteSinks != nil {
		remoteSinks = config.RemoteSinks
		for _, s := range config.RemoteSinks {
			sinkCore := newZapSinkCore(s, zapcore.NewJSONEncoder(cfg), level)
			cores = append(cores, sinkCore)
		}
	}

	// Create logger with multiple cores
	core := zapcore.NewTee(cores...)
	logger := zap.New(core, zap.AddCaller())

	sugar := logger.Sugar()

	return &Logger{
		SugaredLogger: sugar,
		cores:         cores,
	}
}

func (l *Logger) Infow(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Info(msg)
}

func (l *Logger) Warnw(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Warn(msg)
}

func (l *Logger) Errorw(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Error(msg)
}

func (l *Logger) Debugw(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Debug(msg)
}

func (l *Logger) Fatalw(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Fatal(msg)
}
func (l *Logger) Infof(template string, args ...interface{}) {
	l.SugaredLogger.Infof(template, args...)
}
func (l *Logger) Debugf(template string, args ...interface{}) {
	l.SugaredLogger.Debugf(template, args...)
}
func (l *Logger) Errorf(template string, args ...interface{}) {
	l.SugaredLogger.Errorf(template, args...)
}
func (l *Logger) Warnf(template string, args ...interface{}) {
	l.SugaredLogger.Warnf(template, args...)
}
func (l *Logger) Fatalf(template string, args ...interface{}) {
	l.SugaredLogger.Fatalf(template, args...)
}

func (l *Logger) Info(args ...interface{}) {
	l.SugaredLogger.Info(args...)
}
func (l *Logger) Debug(args ...interface{}) {
	l.SugaredLogger.Debug(args...)
}
func (l *Logger) Error(args ...interface{}) {
	l.SugaredLogger.Error(args...)
}
func (l *Logger) Warn(args ...interface{}) {
	l.SugaredLogger.Warn(args...)
}
func (l *Logger) Fatal(args ...interface{}) {
	l.SugaredLogger.Fatal(args...)
}

func (l *Logger) Infoln(args ...interface{}) {
	l.SugaredLogger.Info(args...)
}
func (l *Logger) Debugln(args ...interface{}) {
	l.SugaredLogger.Debug(args...)
}
func (l *Logger) Errorln(args ...interface{}) {
	l.SugaredLogger.Error(args...)
}
func (l *Logger) Warnln(args ...interface{}) {
	l.SugaredLogger.Warn(args...)
}
func (l *Logger) Fatalln(args ...interface{}) {
	l.SugaredLogger.Fatal(args...)
}

func (l *Logger) With(args ...any) any {
	return &Logger{
		SugaredLogger: l.SugaredLogger.With(args...),
	}
}

var (
	Log LoggerI = NewLogger()
)

func SetLevel(l string) {
	zapLevel, err := zapcore.ParseLevel(l)
	if err != nil {
		zapLevel = zapcore.InfoLevel
	}
	level.SetLevel(zapLevel)
}
