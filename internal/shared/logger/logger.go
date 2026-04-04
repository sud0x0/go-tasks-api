package logger

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// LoggerContextKey is the context key for storing the request-scoped logger.
const LoggerContextKey contextKey = "logger"

// Logger is the canonical logging interface used across all packages.
type Logger interface {
	LogError(simplifiedError, actualError error)
	LogInfo(message string, args ...any)
	LogDebug(message string)
	WithRequestID(requestID string) Logger
}

// FromContext retrieves the request-scoped logger from context.
// Falls back to the provided default logger if none is present.
func FromContext(ctx context.Context, defaultLogger Logger) Logger {
	if l, ok := ctx.Value(LoggerContextKey).(Logger); ok {
		return l
	}
	return defaultLogger
}

// SlogLogger implements the Logger interface using slog.
type SlogLogger struct {
	logger    *slog.Logger
	requestID string
}

// NewLogger creates and returns a new Logger configured with the given log level.
// Log levels:
//   - "development" - shows all logs including debug. use for local development.
//   - "production"  - shows info, warnings, and errors. use for deployed environments.
//   - "quiet"       - shows only warnings and errors.
//   - "silent"      - shows only errors.
func NewLogger(logLevel string) Logger {
	var level slog.Level
	switch logLevel {
	case "development", "dev":
		level = slog.LevelDebug
	case "production", "prod":
		level = slog.LevelInfo
	case "quiet":
		level = slog.LevelWarn
	case "silent", "errors":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.String(a.Key, t.Format(time.RFC3339))
				}
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	slogLogger := slog.New(handler)
	slog.SetDefault(slogLogger)

	return &SlogLogger{logger: slogLogger}
}

// WithRequestID returns a new logger with the request ID bound to all log entries.
func (l *SlogLogger) WithRequestID(requestID string) Logger {
	return &SlogLogger{
		logger:    l.logger,
		requestID: requestID,
	}
}

// LogError logs an error with context. The simplifiedError is the sanitised message
// safe to surface; actualError is the raw underlying error for internal diagnostics.
func (l *SlogLogger) LogError(simplifiedError error, actualError error) {
	attrs := []any{slog.String("error", simplifiedError.Error())}
	if l.requestID != "" {
		attrs = append(attrs, slog.String("request_id", l.requestID))
	}
	if actualError != nil {
		attrs = append(attrs, slog.String("actual_error", actualError.Error()))
	}
	l.logger.Error("error occurred", attrs...)
}

// LogInfo logs a structured info message with optional key-value attributes.
func (l *SlogLogger) LogInfo(message string, args ...any) {
	if l.requestID != "" {
		args = append([]any{slog.String("request_id", l.requestID)}, args...)
	}
	l.logger.Info(message, args...)
}

// LogDebug logs a debug message.
func (l *SlogLogger) LogDebug(message string) {
	if l.requestID != "" {
		l.logger.Debug(message, slog.String("request_id", l.requestID))
	} else {
		l.logger.Debug(message)
	}
}
