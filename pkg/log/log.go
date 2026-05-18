package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	stdlog "log"
)

type Config struct {
	Level string `envconfig:"LEVEL" default:"info"`
}

type contextKey struct{}

func InitLogger(appEnv string, cfg Config) *slog.Logger {
	level := parseLevel(cfg.Level)
	handler := newHandler(appEnv, level, os.Stdout)
	logger := slog.New(handler)

	slog.SetDefault(logger)
	stdlog.SetFlags(0)
	stdlog.SetOutput(slog.NewLogLogger(handler, level).Writer())

	return logger
}

func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		logger = slog.Default()
	}
	return context.WithValue(ctx, contextKey{}, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

func newHandler(appEnv string, level slog.Level, writer io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if isProd(appEnv) {
		return slog.NewJSONHandler(writer, opts)
	}
	return slog.NewTextHandler(writer, opts)
}

func isProd(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func parseLevel(level string) slog.Level {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "warning" {
		return slog.LevelWarn
	}

	var parsed slog.Level
	if err := parsed.UnmarshalText([]byte(level)); err != nil {
		return slog.LevelInfo
	}
	return parsed
}
