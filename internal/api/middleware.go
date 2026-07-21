package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/manavsingla/taskflow/internal/metrics"
)

// requestLogger logs one line per request and records taskflow_http_request_duration_seconds.
// It wraps the ResponseWriter to capture the status code chi's Recoverer/handlers set,
// since a plain http.ResponseWriter doesn't expose what was written after the fact.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			duration := time.Since(start)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			log.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", duration.Milliseconds(),
			)

			metrics.HTTPRequestDuration.
				WithLabelValues(r.URL.Path, r.Method, strconv.Itoa(status)).
				Observe(duration.Seconds())
		})
	}
}
