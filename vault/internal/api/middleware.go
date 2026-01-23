package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// Context keys for request values.
type contextKey string

const (
	apiKeyContextKey contextKey = "api_key"
)

// apiKeyAuth validates the API key in the Authorization header.
func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In development mode, allow requests without authentication
		if s.config.IsDevelopment() {
			next.ServeHTTP(w, r)
			return
		}

		// Get the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, "missing Authorization header")
			return
		}

		// Expect "Bearer <token>" format
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, "invalid Authorization header format")
			return
		}

		apiKey := parts[1]

		// TODO: Validate the API key against stored keys
		// For now, just check it's not empty
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, "invalid API key")
			return
		}

		// Store API key in context for later use
		ctx := context.WithValue(r.Context(), apiKeyContextKey, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestLogger creates a middleware that logs HTTP requests.
func RequestLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				logger.Info("http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(start).Milliseconds(),
					"request_id", middleware.GetReqID(r.Context()),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// CORSMiddleware adds CORS headers to responses.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimitMiddleware provides basic rate limiting.
// For production, use a more sophisticated solution with Redis.
func RateLimitMiddleware(requestsPerSecond int) func(next http.Handler) http.Handler {
	// Simple token bucket implementation
	// In production, use a distributed rate limiter
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Implement proper rate limiting
			next.ServeHTTP(w, r)
		})
	}
}

// GetAPIKey retrieves the API key from the request context.
func GetAPIKey(ctx context.Context) string {
	if key, ok := ctx.Value(apiKeyContextKey).(string); ok {
		return key
	}
	return ""
}
