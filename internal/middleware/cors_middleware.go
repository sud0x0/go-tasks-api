package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORS handles cross-origin resource sharing.
// Allowed origins are read from CORS_ALLOWED_ORIGINS (comma-separated).
// Never use * in production — always specify explicit origins.
func CORS(next http.Handler) http.Handler {
	allowedOrigins := parseOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin != "" && isAllowed(origin, allowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID, Authorization, X-Refresh-Token")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}

		// Handle preflight.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func parseOrigins(raw string) []string {
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

func isAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == origin {
			return true
		}
	}
	return false
}
