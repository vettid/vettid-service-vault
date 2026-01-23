// Package api provides the REST API for the VettID Service Vault.
package api

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// SecretStoreRequest is the request body for storing a secret.
type SecretStoreRequest struct {
	UserID      string                 `json:"user_id"`
	SecretType  string                 `json:"secret_type"` // minor, critical, user_owned
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Data        string                 `json:"data"`        // Base64-encoded secret data
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	ExpiresIn   int                    `json:"expires_in,omitempty"` // Seconds for request timeout
}

// SecretStoreResponse is returned when a secret store request is created.
type SecretStoreResponse struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SecretRetrieveRequest is the request body for retrieving a secret.
type SecretRetrieveRequest struct {
	UserID      string `json:"user_id"`
	SecretID    string `json:"secret_id"`
	Purpose     string `json:"purpose,omitempty"`
	CallbackURL string `json:"callback_url,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"` // Seconds for request timeout
}

// SecretRetrieveResponse is returned when a secret retrieve request is created.
type SecretRetrieveResponse struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	Data      string    `json:"data,omitempty"` // Base64-encoded, only if immediately available
	ExpiresAt time.Time `json:"expires_at"`
}

// SecretListRequest is the request body for listing secrets.
type SecretListRequest struct {
	UserID      string `json:"user_id"`
	SecretType  string `json:"secret_type,omitempty"` // Filter by type
	CallbackURL string `json:"callback_url,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
}

// SecretListResponse is returned when listing secrets.
type SecretListResponse struct {
	RequestID string    `json:"request_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SecretMetadataResponse contains metadata about a secret (no actual data).
type SecretMetadataResponse struct {
	SecretID    string                 `json:"secret_id"`
	Name        string                 `json:"name"`
	SecretType  string                 `json:"secret_type"`
	Description string                 `json:"description,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// SecretUpdateRequest is the request body for updating a secret.
type SecretUpdateRequest struct {
	UserID      string                 `json:"user_id"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Data        string                 `json:"data,omitempty"` // Base64-encoded, optional
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	ExpiresIn   int                    `json:"expires_in,omitempty"`
}

// handleSecretStore initiates a request to store a secret.
// POST /api/v1/secrets/store
func (s *Server) handleSecretStore(w http.ResponseWriter, r *http.Request) {
	if s.secretsHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "secrets handler not configured")
		return
	}

	var req SecretStoreRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id is required")
		return
	}
	if req.SecretType == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "secret_type is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "name is required")
		return
	}
	if req.Data == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "data is required")
		return
	}

	// Validate secret type
	secretType, err := handler.ParseSecretType(req.SecretType)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, err.Error())
		return
	}

	// Decode base64 data
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "data must be valid base64")
		return
	}

	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 5 * time.Minute
	}

	// Build options
	var opts []handler.SecretOption
	if req.Description != "" {
		opts = append(opts, handler.WithSecretDescription(req.Description))
	}
	if req.Metadata != nil {
		opts = append(opts, handler.WithSecretMetadata(req.Metadata))
	}
	if req.CallbackURL != "" {
		opts = append(opts, handler.WithSecretCallback(req.CallbackURL))
	}
	if expiresIn != 5*time.Minute {
		opts = append(opts, handler.WithSecretTimeout(expiresIn))
	}

	secretReq, err := s.secretsHandler.StoreSecret(r.Context(), req.UserID, req.Name, data, secretType, opts...)
	if err != nil {
		s.logger.Error("failed to create store secret request", "error", err, "user_id", req.UserID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, SecretStoreResponse{
		RequestID: secretReq.RequestID,
		Status:    "pending",
		ExpiresAt: secretReq.ExpiresAt,
	})
}

// handleSecretRetrieve initiates a request to retrieve a secret.
// POST /api/v1/secrets/retrieve
func (s *Server) handleSecretRetrieve(w http.ResponseWriter, r *http.Request) {
	if s.secretsHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "secrets handler not configured")
		return
	}

	var req SecretRetrieveRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id is required")
		return
	}
	if req.SecretID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "secret_id is required")
		return
	}

	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 5 * time.Minute
	}

	// Build options
	var opts []handler.SecretOption
	if req.CallbackURL != "" {
		opts = append(opts, handler.WithSecretCallback(req.CallbackURL))
	}
	if expiresIn != 5*time.Minute {
		opts = append(opts, handler.WithSecretTimeout(expiresIn))
	}

	secretReq, err := s.secretsHandler.RetrieveSecret(r.Context(), req.UserID, req.SecretID, opts...)
	if err != nil {
		s.logger.Error("failed to create retrieve secret request", "error", err, "user_id", req.UserID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, SecretRetrieveResponse{
		RequestID: secretReq.RequestID,
		Status:    "pending",
		ExpiresAt: secretReq.ExpiresAt,
	})
}

// handleSecretDelete initiates a request to delete a secret.
// DELETE /api/v1/secrets/{secretID}
func (s *Server) handleSecretDelete(w http.ResponseWriter, r *http.Request) {
	if s.secretsHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "secrets handler not configured")
		return
	}

	secretID := chi.URLParam(r, "secretID")
	if secretID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "secret_id is required")
		return
	}

	// User ID must be provided in query param or header
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id query parameter is required")
		return
	}

	callbackURL := r.URL.Query().Get("callback_url")

	// Build options
	var opts []handler.SecretOption
	if callbackURL != "" {
		opts = append(opts, handler.WithSecretCallback(callbackURL))
	}

	secretReq, err := s.secretsHandler.DeleteSecret(r.Context(), userID, secretID, opts...)
	if err != nil {
		s.logger.Error("failed to create delete secret request", "error", err, "user_id", userID, "secret_id", secretID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"request_id": secretReq.RequestID,
		"status":     "pending",
		"expires_at": secretReq.ExpiresAt,
	})
}

// handleSecretsList initiates a request to list a user's secrets.
// GET /api/v1/secrets
func (s *Server) handleSecretsList(w http.ResponseWriter, r *http.Request) {
	if s.secretsHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "secrets handler not configured")
		return
	}

	// Parse query parameters
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id query parameter is required")
		return
	}

	secretTypeStr := r.URL.Query().Get("secret_type")
	callbackURL := r.URL.Query().Get("callback_url")

	if secretTypeStr != "" {
		_, err := handler.ParseSecretType(secretTypeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, err.Error())
			return
		}
	}

	// Build options
	var opts []handler.SecretOption
	if callbackURL != "" {
		opts = append(opts, handler.WithSecretCallback(callbackURL))
	}

	secretReq, err := s.secretsHandler.ListSecrets(r.Context(), userID, opts...)
	if err != nil {
		s.logger.Error("failed to create list secrets request", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, SecretListResponse{
		RequestID: secretReq.RequestID,
		Status:    "pending",
		ExpiresAt: secretReq.ExpiresAt,
	})
}

// handleSecretUpdate initiates a request to update a secret.
// PUT /api/v1/secrets/{secretID}
func (s *Server) handleSecretUpdate(w http.ResponseWriter, r *http.Request) {
	if s.secretsHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "secrets handler not configured")
		return
	}

	secretID := chi.URLParam(r, "secretID")
	if secretID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "secret_id is required")
		return
	}

	var req SecretUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "invalid request body")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id is required")
		return
	}

	// Decode base64 data if provided
	var data []byte
	if req.Data != "" {
		var err error
		data, err = base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "data must be valid base64")
			return
		}
	}

	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 5 * time.Minute
	}

	// Build options
	var opts []handler.SecretOption
	if req.Description != "" {
		opts = append(opts, handler.WithSecretDescription(req.Description))
	}
	if req.Metadata != nil {
		opts = append(opts, handler.WithSecretMetadata(req.Metadata))
	}
	if req.CallbackURL != "" {
		opts = append(opts, handler.WithSecretCallback(req.CallbackURL))
	}
	if expiresIn != 5*time.Minute {
		opts = append(opts, handler.WithSecretTimeout(expiresIn))
	}

	secretReq, err := s.secretsHandler.UpdateSecret(r.Context(), req.UserID, secretID, data, opts...)
	if err != nil {
		s.logger.Error("failed to create update secret request", "error", err, "user_id", req.UserID, "secret_id", secretID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"request_id": secretReq.RequestID,
		"status":     "pending",
		"expires_at": secretReq.ExpiresAt,
	})
}
