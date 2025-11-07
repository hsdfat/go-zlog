package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LoggerI interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Fatal(msg string, args ...interface{})
	Infof(template string, args ...interface{})
	Debugf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})
}

var (
	level = zap.NewAtomicLevel()
)

type Logger struct {
	*zap.SugaredLogger
}

func NewLogger() *Logger {
	// set caller skip to 2

	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(zapcore.Lock(zapcore.NewMultiWriteSyncer(os.Stderr))),
		level,
	), zap.AddCaller(), zap.AddCallerSkip(1),
	)

	sugar := logger.Sugar()

	return &Logger{
		SugaredLogger: sugar,
	}
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Info(msg)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Warn(msg)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Error(msg)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.SugaredLogger.With(args...).Debug(msg)
}

func (l *Logger) Fatal(msg string, args ...interface{}) {
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
