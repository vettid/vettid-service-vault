package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// AuthRequestInput is the input for creating an auth request.
type AuthRequestInput struct {
	UserID       string                 `json:"user_id"`
	Purpose      string                 `json:"purpose"`
	Context      map[string]interface{} `json:"context,omitempty"`
	Timeout      string                 `json:"timeout,omitempty"`       // Duration string (e.g., "5m")
	OfflineGrace string                 `json:"offline_grace,omitempty"` // Duration string (e.g., "24h")
	CallbackURL  string                 `json:"callback_url,omitempty"`
}

// AuthRequestOutput is the output from creating an auth request.
type AuthRequestOutput struct {
	RequestID string              `json:"request_id"`
	UserID    string              `json:"user_id"`
	Purpose   string              `json:"purpose"`
	Status    types.RequestStatus `json:"status"`
	ExpiresAt string              `json:"expires_at"`
	CreatedAt string              `json:"created_at"`
}

// AuthRequestStatus is the status of an auth request.
type AuthRequestStatus struct {
	RequestID   string              `json:"request_id"`
	UserID      string              `json:"user_id"`
	Purpose     string              `json:"purpose"`
	Status      types.RequestStatus `json:"status"`
	ExpiresAt   string              `json:"expires_at"`
	CreatedAt   string              `json:"created_at"`
	RespondedAt string              `json:"responded_at,omitempty"`
}

// handleAuthRequest creates a new authentication request.
// POST /api/v1/auth/request
func (s *Server) handleAuthRequest(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "auth handler not configured")
		return
	}

	var input AuthRequestInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, err.Error())
		return
	}

	// Validate required fields
	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id is required")
		return
	}
	if input.Purpose == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "purpose is required")
		return
	}

	// Build options
	var opts []handler.AuthOption

	if input.Context != nil {
		opts = append(opts, handler.WithAuthContext(input.Context))
	}

	if input.Timeout != "" {
		if d, err := time.ParseDuration(input.Timeout); err == nil {
			opts = append(opts, handler.WithAuthTimeout(d))
		}
	}

	if input.OfflineGrace != "" {
		if d, err := time.ParseDuration(input.OfflineGrace); err == nil {
			opts = append(opts, handler.WithOfflineGrace(d))
		}
	}

	if input.CallbackURL != "" {
		opts = append(opts, handler.WithAuthCallback(input.CallbackURL))
	}

	// Create the request
	req, err := s.authHandler.RequestAuth(r.Context(), input.UserID, input.Purpose, opts...)
	if err != nil {
		// Determine appropriate error code
		code := types.ErrCodeInternal
		status := http.StatusInternalServerError

		if err.Error() == "no active contract" {
			code = types.ErrCodeContractRequired
			status = http.StatusForbidden
		} else if err.Error() == "contract does not grant authenticate capability" {
			code = types.ErrCodeCapabilityDenied
			status = http.StatusForbidden
		}

		writeError(w, status, code, err.Error())
		return
	}

	output := AuthRequestOutput{
		RequestID: req.RequestID,
		UserID:    req.UserID,
		Purpose:   req.Purpose,
		Status:    req.Status,
		ExpiresAt: req.ExpiresAt.Format(time.RFC3339),
		CreatedAt: req.CreatedAt.Format(time.RFC3339),
	}

	writeJSON(w, http.StatusCreated, output)
}

// getAuthRequest gets the status of an auth request.
// GET /api/v1/auth/request/{requestID}
func (s *Server) getAuthRequest(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "auth handler not configured")
		return
	}

	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "request ID is required")
		return
	}

	req, found := s.authHandler.GetRequest(requestID)
	if !found {
		writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "auth request not found")
		return
	}

	status := AuthRequestStatus{
		RequestID: req.RequestID,
		UserID:    req.UserID,
		Purpose:   req.Purpose,
		Status:    req.Status,
		ExpiresAt: req.ExpiresAt.Format(time.RFC3339),
		CreatedAt: req.CreatedAt.Format(time.RFC3339),
	}

	if req.RespondedAt != nil {
		status.RespondedAt = req.RespondedAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, status)
}
