// Package api provides the REST API for the VettID Service Vault.
//
// The API is designed for backend service integration, allowing services
// to request authentication/authorization from users and manage contracts.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/vettid/vettid-service-vault/vault/internal/config"
	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/internal/identity"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// Server is the REST API server.
type Server struct {
	router         chi.Router
	httpServer     *http.Server
	config         *config.Config
	identity       *identity.ServiceIdentity
	contracts      contract.Store
	negotiator     *contract.Negotiator
	authHandler    *handler.AuthHandler
	authzHandler   *handler.AuthzHandler
	callHandler    *handler.CallHandler
	paymentHandler *handler.PaymentHandler
	secretsHandler *handler.SecretsHandler
	logger         *slog.Logger
}

// ServerConfig holds configuration for the API server.
type ServerConfig struct {
	Config         *config.Config
	Identity       *identity.ServiceIdentity
	Contracts      contract.Store
	Negotiator     *contract.Negotiator
	AuthHandler    *handler.AuthHandler
	AuthzHandler   *handler.AuthzHandler
	CallHandler    *handler.CallHandler
	PaymentHandler *handler.PaymentHandler
	SecretsHandler *handler.SecretsHandler
	Logger         *slog.Logger
}

// NewServer creates a new API server.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Config == nil {
		return nil, fmt.Errorf("config is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		router:         chi.NewRouter(),
		config:         cfg.Config,
		identity:       cfg.Identity,
		contracts:      cfg.Contracts,
		negotiator:     cfg.Negotiator,
		authHandler:    cfg.AuthHandler,
		authzHandler:   cfg.AuthzHandler,
		callHandler:    cfg.CallHandler,
		paymentHandler: cfg.PaymentHandler,
		secretsHandler: cfg.SecretsHandler,
		logger:         logger,
	}

	s.setupRoutes()
	return s, nil
}

// setupRoutes configures all API routes.
func (s *Server) setupRoutes() {
	r := s.router

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(RequestLogger(s.logger))
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	// Health check (no auth)
	r.Get("/health", s.healthCheck)
	r.Get("/ready", s.readyCheck)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Apply rate limiting before authentication
		r.Use(RateLimitMiddleware(s.config.RateLimitPerSec, s.config.RateLimitBurst))
		// Apply API key authentication
		r.Use(s.apiKeyAuth)

		// Auth endpoints (SV-051)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/request", s.handleAuthRequest)
			r.Get("/request/{requestID}", s.getAuthRequest)
		})

		// Authz endpoints (SV-052)
		r.Route("/authz", func(r chi.Router) {
			r.Post("/request", s.handleAuthzRequest)
			r.Get("/request/{requestID}", s.getAuthzRequest)
		})

		// Contract endpoints (SV-053)
		r.Route("/contracts", func(r chi.Router) {
			r.Get("/", s.listContracts)
			r.Get("/{contractID}", s.getContract)
			r.Post("/invite", s.generateInvite)
			r.Delete("/{contractID}", s.cancelContract)
		})

		// Call endpoints (SV-055)
		r.Route("/call", func(r chi.Router) {
			r.Post("/initiate", s.handleCallInitiate)
			r.Post("/{callID}/end", s.handleCallEnd)
			r.Get("/{callID}", s.getCallStatus)
		})

		// Payment endpoints (SV-056)
		r.Route("/payment", func(r chi.Router) {
			r.Post("/request", s.handlePaymentRequest)
			r.Get("/request/{requestID}", s.getPaymentRequest)
			r.Post("/request/{requestID}/complete", s.handlePaymentComplete)
			r.Post("/request/{requestID}/fail", s.handlePaymentFail)
			r.Post("/request/{requestID}/refund", s.handlePaymentRefund)
		})

		// Secrets endpoints (SV-057)
		r.Route("/secrets", func(r chi.Router) {
			r.Post("/store", s.handleSecretStore)
			r.Post("/retrieve", s.handleSecretRetrieve)
			r.Delete("/{secretID}", s.handleSecretDelete)
			r.Get("/", s.handleSecretsList)
			r.Put("/{secretID}", s.handleSecretUpdate)
		})

		// Service info
		r.Get("/info", s.getServiceInfo)
	})
}

// Start starts the HTTP server.
func (s *Server) Start(addr string) error {
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("starting API server", "addr", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// Router returns the chi router for testing.
func (s *Server) Router() chi.Router {
	return s.router
}

// Health check handlers

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyCheck(w http.ResponseWriter, r *http.Request) {
	// TODO: Check connectivity to dependencies
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Service info handler

func (s *Server) getServiceInfo(w http.ResponseWriter, r *http.Request) {
	if s.identity == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "service identity not configured")
		return
	}

	info := map[string]interface{}{
		"service_id":   s.identity.ServiceID,
		"service_name": s.identity.ServiceName,
		"service_type": s.identity.ServiceType,
	}

	if s.identity.Domain != "" {
		info["domain"] = s.identity.Domain
		info["domain_verified"] = s.identity.DomainVerified
	}

	writeJSON(w, http.StatusOK, info)
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.APIError{
		Code:    code,
		Message: message,
	})
}

func readJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
