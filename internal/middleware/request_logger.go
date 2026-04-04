package middleware

import (
	"context"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"go-tasks-api/internal/shared/logger"
)

// RequestLogger returns middleware that creates a request-scoped logger
// with the request ID bound to all log entries. The logger is stored in
// context and can be retrieved with logger.FromContext.
func RequestLogger(log logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := chimw.GetReqID(r.Context())
			reqLogger := log.WithRequestID(requestID)
			ctx := context.WithValue(r.Context(), logger.LoggerContextKey, reqLogger)

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r.WithContext(ctx))

			duration := time.Since(start)
			reqLogger.LogInfo("request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", duration.Milliseconds(),
			)

			if duration > 5*time.Second {
				reqLogger.LogInfo("slow request detected",
					"method", r.Method,
					"path", r.URL.Path,
					"duration_ms", duration.Milliseconds(),
					"warning", true,
				)
			}
		})
	}
}
