package aprsutils

import (
	"context"
	"log"
	"os"
)

// Logger construct a basic interface for logger
type Logger interface {
	Debug(context.Context, ...interface{})
	Info(context.Context, ...interface{})
	Warn(context.Context, ...interface{})
	Error(context.Context, ...interface{})
}

// NewLogger creates a new logger
func NewLogger() Logger {
	logger := defaultLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
	return logger
}

// defaultLogger is a sets of default internal logger methods
type defaultLogger struct {
	logger *log.Logger
}

// Debug build Debug level log
func (l defaultLogger) Debug(ctx context.Context, args ...interface{}) {
	l.logger.Printf("[Debug] %v", args)
}

// Info build Info level log
func (l defaultLogger) Info(ctx context.Context, args ...interface{}) {
	l.logger.Printf("[Info] %v", args)
}

// Warn build Warn level log
func (l defaultLogger) Warn(ctx context.Context, args ...interface{}) {
	l.logger.Printf("[Warn] %v", args)
}

// Error build Error level log
func (l defaultLogger) Error(ctx context.Context, args ...interface{}) {
	l.logger.Printf("[Error] %v", args)
}
