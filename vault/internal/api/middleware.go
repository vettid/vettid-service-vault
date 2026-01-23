package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"sync"
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
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, "invalid API key")
			return
		}

		// Validate the API key against stored hashed keys
		// Hash the incoming key and compare using constant-time comparison
		if !s.validateAPIKey(apiKey) {
			writeError(w, http.StatusUnauthorized, types.ErrCodeUnauthorized, "invalid API key")
			return
		}

		// Store API key hash in context for later use (not the raw key)
		keyHash := hashAPIKey(apiKey)
		ctx := context.WithValue(r.Context(), apiKeyContextKey, keyHash)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validateAPIKey checks if the provided API key matches any of the configured hashed keys.
// Uses constant-time comparison to prevent timing attacks.
func (s *Server) validateAPIKey(apiKey string) bool {
	if len(s.config.APIKeys) == 0 {
		// No keys configured - in development this might be intentional
		// In production, this should fail closed
		return s.config.IsDevelopment()
	}

	keyHash := hashAPIKey(apiKey)
	keyHashBytes, err := hex.DecodeString(keyHash)
	if err != nil {
		return false
	}

	for _, storedHash := range s.config.APIKeys {
		storedBytes, err := hex.DecodeString(storedHash)
		if err != nil {
			continue
		}
		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare(keyHashBytes, storedBytes) == 1 {
			return true
		}
	}
	return false
}

// hashAPIKey computes the SHA-256 hash of an API key and returns it as a hex string.
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
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

// tokenBucket represents a token bucket for rate limiting.
type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

// rateLimiter manages rate limiting across multiple clients.
type rateLimiter struct {
	buckets         sync.Map // map[string]*tokenBucket
	tokensPerSecond float64
	burstSize       float64
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
}

// newRateLimiter creates a new rate limiter.
func newRateLimiter(tokensPerSecond, burstSize int) *rateLimiter {
	rl := &rateLimiter{
		tokensPerSecond: float64(tokensPerSecond),
		burstSize:       float64(burstSize),
		cleanupInterval: 5 * time.Minute,
		stopCleanup:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// allow checks if a request from the given client is allowed.
func (rl *rateLimiter) allow(clientID string) bool {
	bucket, _ := rl.buckets.LoadOrStore(clientID, &tokenBucket{
		tokens:     rl.burstSize,
		lastRefill: time.Now(),
	})

	tb := bucket.(*tokenBucket)
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * rl.tokensPerSecond
	if tb.tokens > rl.burstSize {
		tb.tokens = rl.burstSize
	}
	tb.lastRefill = now

	// Check if we have at least one token
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// cleanup periodically removes stale entries.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			staleThreshold := time.Now().Add(-10 * time.Minute)
			rl.buckets.Range(func(key, value interface{}) bool {
				tb := value.(*tokenBucket)
				tb.mu.Lock()
				if tb.lastRefill.Before(staleThreshold) {
					rl.buckets.Delete(key)
				}
				tb.mu.Unlock()
				return true
			})
		case <-rl.stopCleanup:
			return
		}
	}
}

// RateLimitMiddleware provides token bucket rate limiting per client IP.
// For distributed deployments, consider using Redis for shared state.
func RateLimitMiddleware(requestsPerSecond, burstSize int) func(next http.Handler) http.Handler {
	limiter := newRateLimiter(requestsPerSecond, burstSize)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use client IP as the rate limit key
			// In production with load balancers, use X-Forwarded-For
			clientIP := getClientIP(r)

			if !limiter.allow(clientIP) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusTooManyRequests, types.ErrCodeRateLimited, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (used by proxies/load balancers)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list (original client)
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	// Remove port if present
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// GetAPIKey retrieves the API key from the request context.
func GetAPIKey(ctx context.Context) string {
	if key, ok := ctx.Value(apiKeyContextKey).(string); ok {
		return key
	}
	return ""
}
