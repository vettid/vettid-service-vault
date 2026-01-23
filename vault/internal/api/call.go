// Package api provides the REST API for the VettID Service Vault.
package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// CallInitiateRequest is the request body for initiating a call.
type CallInitiateRequest struct {
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"` // voice, video
	Purpose     string                 `json:"purpose,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	ICEServers  []types.ICEServer      `json:"ice_servers,omitempty"`
	Offer       *RTCSessionDescription `json:"offer,omitempty"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	ExpiresIn   int                    `json:"expires_in,omitempty"` // Seconds until request expires
}

// RTCSessionDescription matches the WebRTC SDP format.
type RTCSessionDescription struct {
	Type string `json:"type"` // offer, answer
	SDP  string `json:"sdp"`
}

// CallInitiateResponse is returned when a call is initiated.
type CallInitiateResponse struct {
	CallID    string    `json:"call_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CallStatusResponse is the response for getting call status.
type CallStatusResponse struct {
	CallID      string                 `json:"call_id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Answer      *RTCSessionDescription `json:"answer,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	EndedAt     *time.Time             `json:"ended_at,omitempty"`
	Duration    *int64                 `json:"duration_seconds,omitempty"`
	EndedReason string                 `json:"ended_reason,omitempty"`
}

// handleCallInitiate initiates a call with a user.
// POST /api/v1/call/initiate
func (s *Server) handleCallInitiate(w http.ResponseWriter, r *http.Request) {
	if s.callHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "call handler not configured")
		return
	}

	var req CallInitiateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id is required")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "type is required (voice or video)")
		return
	}
	if req.Type != "voice" && req.Type != "video" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "type must be 'voice' or 'video'")
		return
	}

	// Convert to handler request
	callType := handler.CallTypeVoice
	if req.Type == "video" {
		callType = handler.CallTypeVideo
	}

	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 60 * time.Second // Default 60 second timeout for call acceptance
	}

	// Convert offer if present
	var offer *handler.RTCSessionDescription
	if req.Offer != nil {
		offer = &handler.RTCSessionDescription{
			Type: req.Offer.Type,
			SDP:  req.Offer.SDP,
		}
	}

	// Build options
	var opts []handler.CallOption
	if req.Purpose != "" {
		opts = append(opts, handler.WithCallPurpose(req.Purpose))
	}
	if req.Context != nil {
		opts = append(opts, handler.WithCallContext(req.Context))
	}
	if req.CallbackURL != "" {
		opts = append(opts, handler.WithCallCallback(req.CallbackURL))
	}
	if expiresIn != 60*time.Second {
		opts = append(opts, handler.WithCallTimeout(expiresIn))
	}

	callReq, err := s.callHandler.InitiateCall(r.Context(), req.UserID, callType, offer, opts...)
	if err != nil {
		s.logger.Error("failed to initiate call", "error", err, "user_id", req.UserID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, CallInitiateResponse{
		CallID:    callReq.RequestID, // RequestID is the initial ID before call is established
		Status:    "pending",
		ExpiresAt: callReq.ExpiresAt,
	})
}

// handleCallEnd ends an active call.
// POST /api/v1/call/{callID}/end
func (s *Server) handleCallEnd(w http.ResponseWriter, r *http.Request) {
	if s.callHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "call handler not configured")
		return
	}

	callID := chi.URLParam(r, "callID")
	if callID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "call_id is required")
		return
	}

	// Optional reason in request body
	var body struct {
		Reason string `json:"reason,omitempty"`
	}
	readJSON(r, &body) // Ignore error - body is optional

	if err := s.callHandler.EndCall(r.Context(), callID, body.Reason); err != nil {
		s.logger.Error("failed to end call", "error", err, "call_id", callID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}

// getCallStatus returns the current status of a call.
// GET /api/v1/call/{callID}
func (s *Server) getCallStatus(w http.ResponseWriter, r *http.Request) {
	if s.callHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "call handler not configured")
		return
	}

	callID := chi.URLParam(r, "callID")
	if callID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "call_id is required")
		return
	}

	call, found := s.callHandler.GetCall(callID)
	if !found {
		writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "call not found")
		return
	}

	resp := CallStatusResponse{
		CallID:      call.CallID,
		UserID:      call.UserID,
		Type:        string(call.Type),
		Status:      string(call.Status),
		EndedReason: call.EndReason,
	}

	if call.Answer != nil {
		resp.Answer = &RTCSessionDescription{
			Type: call.Answer.Type,
			SDP:  call.Answer.SDP,
		}
	}

	if call.StartedAt != nil {
		resp.StartedAt = call.StartedAt
	}
	if call.EndedAt != nil {
		resp.EndedAt = call.EndedAt
		if call.StartedAt != nil {
			duration := int64(call.EndedAt.Sub(*call.StartedAt).Seconds())
			resp.Duration = &duration
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
