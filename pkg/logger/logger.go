package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger for the application.
type Logger struct {
	*zap.Logger
}

// New creates a new logger configured for the given environment.
func New(env string) (*Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig = zap.NewProductionEncoderConfig()
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.DisableStacktrace = env == "production"

	if env != "production" {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	lg, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("logger: build failed: %w", err)
	}

	return &Logger{Logger: lg}, nil
}

// WithContext attaches tracing context to logs.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	if ctx == nil {
		return l
	}
	return &Logger{Logger: l.Logger.With(zap.String("context", fmt.Sprintf("%p", ctx)))}
}

// Sync flushes any buffered log entries.
func (l *Logger) Sync() {
	_ = l.Logger.Sync()
}
