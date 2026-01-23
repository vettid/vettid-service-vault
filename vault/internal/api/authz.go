package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// AuthzRequestInput is the input for creating an authz request.
type AuthzRequestInput struct {
	UserID       string                 `json:"user_id"`
	Action       string                 `json:"action"`
	Resource     string                 `json:"resource"`
	Context      map[string]interface{} `json:"context,omitempty"`
	Timeout      string                 `json:"timeout,omitempty"`       // Duration string
	OfflineGrace string                 `json:"offline_grace,omitempty"` // Duration string
	CallbackURL  string                 `json:"callback_url,omitempty"`
}

// AuthzRequestOutput is the output from creating an authz request.
type AuthzRequestOutput struct {
	RequestID string              `json:"request_id"`
	UserID    string              `json:"user_id"`
	Action    string              `json:"action"`
	Resource  string              `json:"resource"`
	Status    types.RequestStatus `json:"status"`
	ExpiresAt string              `json:"expires_at"`
	CreatedAt string              `json:"created_at"`
}

// AuthzRequestStatus is the status of an authz request.
type AuthzRequestStatus struct {
	RequestID    string              `json:"request_id"`
	UserID       string              `json:"user_id"`
	Action       string              `json:"action"`
	Resource     string              `json:"resource"`
	Status       types.RequestStatus `json:"status"`
	ExpiresAt    string              `json:"expires_at"`
	CreatedAt    string              `json:"created_at"`
	RespondedAt  string              `json:"responded_at,omitempty"`
	ApprovalExp  string              `json:"approval_expires_at,omitempty"`
}

// handleAuthzRequest creates a new authorization request.
// POST /api/v1/authz/request
func (s *Server) handleAuthzRequest(w http.ResponseWriter, r *http.Request) {
	if s.authzHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "authz handler not configured")
		return
	}

	var input AuthzRequestInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, err.Error())
		return
	}

	// Validate required fields
	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id is required")
		return
	}
	if input.Action == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "action is required")
		return
	}
	if input.Resource == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "resource is required")
		return
	}

	// Build options
	var opts []handler.AuthzOption

	if input.Context != nil {
		opts = append(opts, handler.WithAuthzContext(input.Context))
	}

	if input.Timeout != "" {
		if d, err := time.ParseDuration(input.Timeout); err == nil {
			opts = append(opts, handler.WithAuthzTimeout(d))
		}
	}

	if input.OfflineGrace != "" {
		if d, err := time.ParseDuration(input.OfflineGrace); err == nil {
			opts = append(opts, handler.WithAuthzOfflineGrace(d))
		}
	}

	if input.CallbackURL != "" {
		opts = append(opts, handler.WithAuthzCallback(input.CallbackURL))
	}

	// Create the request
	req, err := s.authzHandler.RequestAuthz(r.Context(), input.UserID, input.Action, input.Resource, opts...)
	if err != nil {
		// Determine appropriate error code
		code := types.ErrCodeInternal
		status := http.StatusInternalServerError

		if err.Error() == "no active contract" {
			code = types.ErrCodeContractRequired
			status = http.StatusForbidden
		} else if err.Error() == "contract does not grant authorize capability" {
			code = types.ErrCodeCapabilityDenied
			status = http.StatusForbidden
		}

		writeError(w, status, code, err.Error())
		return
	}

	output := AuthzRequestOutput{
		RequestID: req.RequestID,
		UserID:    req.UserID,
		Action:    req.Action,
		Resource:  req.Resource,
		Status:    req.Status,
		ExpiresAt: req.ExpiresAt.Format(time.RFC3339),
		CreatedAt: req.CreatedAt.Format(time.RFC3339),
	}

	writeJSON(w, http.StatusCreated, output)
}

// getAuthzRequest gets the status of an authz request.
// GET /api/v1/authz/request/{requestID}
func (s *Server) getAuthzRequest(w http.ResponseWriter, r *http.Request) {
	if s.authzHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "authz handler not configured")
		return
	}

	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "request ID is required")
		return
	}

	req, found := s.authzHandler.GetRequest(requestID)
	if !found {
		writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "authz request not found")
		return
	}

	status := AuthzRequestStatus{
		RequestID: req.RequestID,
		UserID:    req.UserID,
		Action:    req.Action,
		Resource:  req.Resource,
		Status:    req.Status,
		ExpiresAt: req.ExpiresAt.Format(time.RFC3339),
		CreatedAt: req.CreatedAt.Format(time.RFC3339),
	}

	if req.RespondedAt != nil {
		status.RespondedAt = req.RespondedAt.Format(time.RFC3339)
	}

	if req.Response != nil && req.Response.ExpiresAt != nil {
		status.ApprovalExp = req.Response.ExpiresAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, status)
}
