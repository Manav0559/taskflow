// Package logger provides one structured (JSON) logger construction point so every
// service emits logs in the same shape for aggregation (e.g. Loki/ELK) in production.
package logger

import (
	"log/slog"
	"os"
)

func New(service string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler).With("service", service)
}
