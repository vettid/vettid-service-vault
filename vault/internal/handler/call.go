package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// CallHandler handles voice/video call signaling between services and users.
// It manages WebRTC session negotiation through the MessageSpace.
type CallHandler struct {
	mu           sync.RWMutex
	engine       *Engine
	activeCalls  map[string]*Call
	pending      map[string]*CallRequest
	callbacks    map[string]CallCallback
	iceServers   []ICEServer
	timeout      time.Duration
	maxDuration  time.Duration
}

// Call represents an active or completed call session.
type Call struct {
	CallID       string                 `json:"call_id"`
	RequestID    string                 `json:"request_id"`
	UserID       string                 `json:"user_id"`
	Type         CallType               `json:"type"`
	Direction    CallDirection          `json:"direction"`
	Status       CallStatus             `json:"status"`
	ICEServers   []ICEServer            `json:"ice_servers,omitempty"`
	Offer        *RTCSessionDescription `json:"offer,omitempty"`
	Answer       *RTCSessionDescription `json:"answer,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	ConnectedAt  *time.Time             `json:"connected_at,omitempty"`
	EndedAt      *time.Time             `json:"ended_at,omitempty"`
	EndReason    string                 `json:"end_reason,omitempty"`
	Duration     time.Duration          `json:"duration,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// CallRequest represents a pending call request.
type CallRequest struct {
	RequestID   string                 `json:"request_id"`
	UserID      string                 `json:"user_id"`
	Type        CallType               `json:"type"`
	Purpose     string                 `json:"purpose"`
	Context     map[string]interface{} `json:"context,omitempty"`
	ICEServers  []ICEServer            `json:"ice_servers,omitempty"`
	Offer       *RTCSessionDescription `json:"offer,omitempty"`
	ExpiresAt   time.Time              `json:"expires_at"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	Status      types.RequestStatus    `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	Call        *Call                  `json:"call,omitempty"`
}

// CallType indicates the type of call.
type CallType string

const (
	CallTypeVoice CallType = "voice"
	CallTypeVideo CallType = "video"
)

// CallDirection indicates who initiated the call.
type CallDirection string

const (
	CallDirectionOutgoing CallDirection = "outgoing" // Service to user
	CallDirectionIncoming CallDirection = "incoming" // User to service
)

// CallStatus indicates the current state of a call.
type CallStatus string

const (
	CallStatusInitiating CallStatus = "initiating"
	CallStatusRinging    CallStatus = "ringing"
	CallStatusConnecting CallStatus = "connecting"
	CallStatusConnected  CallStatus = "connected"
	CallStatusEnded      CallStatus = "ended"
	CallStatusFailed     CallStatus = "failed"
	CallStatusRejected   CallStatus = "rejected"
	CallStatusMissed     CallStatus = "missed"
	CallStatusBusy       CallStatus = "busy"
)

// RTCSessionDescription represents an SDP offer or answer.
type RTCSessionDescription struct {
	Type string `json:"type"` // "offer" or "answer"
	SDP  string `json:"sdp"`
}

// ICEServer represents a STUN/TURN server configuration.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// ICECandidate represents an ICE candidate for connectivity.
type ICECandidate struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid"`
	SDPMLineIndex int    `json:"sdpMLineIndex"`
}

// CallCallback is called when a call event occurs.
type CallCallback func(call *Call)

// CallHandlerConfig holds configuration for the call handler.
type CallHandlerConfig struct {
	Engine      *Engine
	ICEServers  []ICEServer
	Timeout     time.Duration // Ring timeout
	MaxDuration time.Duration // Maximum call duration
}

// NewCallHandler creates a new call handler.
func NewCallHandler(cfg CallHandlerConfig) *CallHandler {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second // 60 second ring timeout
	}

	maxDuration := cfg.MaxDuration
	if maxDuration == 0 {
		maxDuration = 4 * time.Hour // 4 hour max call
	}

	// Default ICE servers if none provided
	iceServers := cfg.ICEServers
	if len(iceServers) == 0 {
		iceServers = []ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
		}
	}

	return &CallHandler{
		engine:      cfg.Engine,
		activeCalls: make(map[string]*Call),
		pending:     make(map[string]*CallRequest),
		callbacks:   make(map[string]CallCallback),
		iceServers:  iceServers,
		timeout:     timeout,
		maxDuration: maxDuration,
	}
}

// EventType returns the event type this handler processes.
func (h *CallHandler) EventType() string {
	return "call.signal"
}

// HandleRequest processes incoming call signaling from users.
func (h *CallHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	var signal struct {
		Type      string                 `json:"type"` // answer, candidate, hangup
		CallID    string                 `json:"call_id"`
		RequestID string                 `json:"request_id,omitempty"`
		Answer    *RTCSessionDescription `json:"answer,omitempty"`
		Candidate *ICECandidate          `json:"candidate,omitempty"`
		Reason    string                 `json:"reason,omitempty"`
	}

	if err := json.Unmarshal(req.Payload, &signal); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: "invalid call signal format",
			},
		}, nil
	}

	switch signal.Type {
	case "answer":
		return h.handleAnswer(ctx, req.UserID, signal.CallID, signal.Answer)
	case "candidate":
		return h.handleCandidate(ctx, req.UserID, signal.CallID, signal.Candidate)
	case "hangup":
		return h.handleHangup(ctx, req.UserID, signal.CallID, signal.Reason)
	case "accept":
		return h.handleAccept(ctx, req.UserID, signal.RequestID)
	case "reject":
		return h.handleReject(ctx, req.UserID, signal.RequestID, signal.Reason)
	default:
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: fmt.Sprintf("unknown signal type: %s", signal.Type),
			},
		}, nil
	}
}

// InitiateCall starts a call to a user.
func (h *CallHandler) InitiateCall(ctx context.Context, userID string, callType CallType, offer *RTCSessionDescription, opts ...CallOption) (*CallRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows calls
	if !contract.VerifyCapability(userContract, types.CapabilityCall) {
		return nil, fmt.Errorf("contract does not grant call capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &CallRequest{
		RequestID:  requestID,
		UserID:     userID,
		Type:       callType,
		ICEServers: h.iceServers,
		Offer:      offer,
		ExpiresAt:  now.Add(h.timeout),
		Status:     types.RequestStatusPending,
		CreatedAt:  now,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	// Store pending request
	h.mu.Lock()
	h.pending[requestID] = request
	h.mu.Unlock()

	// Send call request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.CallRequest{
			RequestID:  requestID,
			UserID:     userID,
			Type:       string(callType),
			Purpose:    request.Purpose,
			Context:    request.Context,
			ICEServers: toTypesICEServers(h.iceServers),
			Offer:      toTypesSessionDesc(offer),
			ExpiresAt:  request.ExpiresAt,
		})

		// Get user's public key from contract
		var userPubKey [32]byte
		// In production, decode from userContract.UserConnectionKey

		if err := h.engine.messageSpace.SendToUser(userID, "call.request", payload, userPubKey); err != nil {
			// Log error but don't fail - request is created
		}
	}

	// Start timeout goroutine
	go h.watchCallTimeout(requestID)

	return request, nil
}

// handleAccept processes a user accepting a call.
func (h *CallHandler) handleAccept(ctx context.Context, userID, requestID string) (*Response, error) {
	h.mu.Lock()
	request, exists := h.pending[requestID]
	if !exists {
		h.mu.Unlock()
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeNotFound,
				Message: "call request not found",
			},
		}, nil
	}

	if request.UserID != userID {
		h.mu.Unlock()
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeForbidden,
				Message: "call request belongs to different user",
			},
		}, nil
	}

	if time.Now().After(request.ExpiresAt) {
		request.Status = types.RequestStatusExpired
		h.mu.Unlock()
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeTimeout,
				Message: "call request has expired",
			},
		}, nil
	}

	// Create call
	callID := generateCallID()
	now := time.Now().UTC()
	call := &Call{
		CallID:    callID,
		RequestID: requestID,
		UserID:    userID,
		Type:      request.Type,
		Direction: CallDirectionOutgoing,
		Status:    CallStatusConnecting,
		ICEServers: request.ICEServers,
		Offer:     request.Offer,
		Metadata:  request.Context,
		StartedAt: &now,
		CreatedAt: now,
	}

	h.activeCalls[callID] = call
	request.Status = types.RequestStatusApproved
	request.Call = call
	delete(h.pending, requestID)

	callback := h.callbacks[requestID]
	delete(h.callbacks, requestID)
	h.mu.Unlock()

	// Execute callback
	if callback != nil {
		callback(call)
	}

	// Notify service of call acceptance
	if request.CallbackURL != "" {
		go h.engine.callWebhook(request.CallbackURL, "call.accepted", map[string]interface{}{
			"request_id": requestID,
			"call_id":    callID,
			"user_id":    userID,
			"type":       request.Type,
		})
	}

	return &Response{
		Status: "ok",
		Data: map[string]interface{}{
			"call_id":     callID,
			"ice_servers": request.ICEServers,
			"offer":       request.Offer,
		},
	}, nil
}

// handleReject processes a user rejecting a call.
func (h *CallHandler) handleReject(ctx context.Context, userID, requestID, reason string) (*Response, error) {
	h.mu.Lock()
	request, exists := h.pending[requestID]
	if !exists {
		h.mu.Unlock()
		return &Response{Status: "ok"}, nil // Idempotent
	}

	if request.UserID != userID {
		h.mu.Unlock()
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeForbidden,
				Message: "call request belongs to different user",
			},
		}, nil
	}

	request.Status = types.RequestStatusDenied
	delete(h.pending, requestID)

	callback := h.callbacks[requestID]
	delete(h.callbacks, requestID)
	h.mu.Unlock()

	// Execute callback with nil call
	if callback != nil {
		callback(&Call{
			RequestID: requestID,
			UserID:    userID,
			Status:    CallStatusRejected,
			EndReason: reason,
		})
	}

	// Notify service
	if request.CallbackURL != "" {
		go h.engine.callWebhook(request.CallbackURL, "call.rejected", map[string]interface{}{
			"request_id": requestID,
			"user_id":    userID,
			"reason":     reason,
		})
	}

	return &Response{Status: "ok"}, nil
}

// handleAnswer processes an SDP answer from the user.
func (h *CallHandler) handleAnswer(ctx context.Context, userID, callID string, answer *RTCSessionDescription) (*Response, error) {
	h.mu.Lock()
	call, exists := h.activeCalls[callID]
	if !exists {
		h.mu.Unlock()
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeNotFound,
				Message: "call not found",
			},
		}, nil
	}

	if call.UserID != userID {
		h.mu.Unlock()
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeForbidden,
				Message: "call belongs to different user",
			},
		}, nil
	}

	call.Answer = answer
	call.Status = CallStatusConnected
	now := time.Now().UTC()
	call.ConnectedAt = &now
	h.mu.Unlock()

	return &Response{Status: "ok"}, nil
}

// handleCandidate processes an ICE candidate from the user.
func (h *CallHandler) handleCandidate(ctx context.Context, userID, callID string, candidate *ICECandidate) (*Response, error) {
	h.mu.RLock()
	call, exists := h.activeCalls[callID]
	h.mu.RUnlock()

	if !exists {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeNotFound,
				Message: "call not found",
			},
		}, nil
	}

	if call.UserID != userID {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeForbidden,
				Message: "call belongs to different user",
			},
		}, nil
	}

	// Forward candidate to service (via webhook or callback)
	// In a real implementation, this would use WebSocket or similar

	return &Response{Status: "ok"}, nil
}

// handleHangup processes a call hangup from the user.
func (h *CallHandler) handleHangup(ctx context.Context, userID, callID, reason string) (*Response, error) {
	h.mu.Lock()
	call, exists := h.activeCalls[callID]
	if !exists {
		h.mu.Unlock()
		return &Response{Status: "ok"}, nil // Idempotent
	}

	if call.UserID != userID {
		h.mu.Unlock()
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeForbidden,
				Message: "call belongs to different user",
			},
		}, nil
	}

	now := time.Now().UTC()
	call.Status = CallStatusEnded
	call.EndedAt = &now
	call.EndReason = reason

	if call.ConnectedAt != nil {
		call.Duration = now.Sub(*call.ConnectedAt)
	}

	delete(h.activeCalls, callID)
	h.mu.Unlock()

	return &Response{Status: "ok"}, nil
}

// EndCall ends an active call from the service side.
func (h *CallHandler) EndCall(ctx context.Context, callID, reason string) error {
	h.mu.Lock()
	call, exists := h.activeCalls[callID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("call not found")
	}

	now := time.Now().UTC()
	call.Status = CallStatusEnded
	call.EndedAt = &now
	call.EndReason = reason

	if call.ConnectedAt != nil {
		call.Duration = now.Sub(*call.ConnectedAt)
	}

	delete(h.activeCalls, callID)
	h.mu.Unlock()

	// Send hangup signal to user
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":    "hangup",
			"call_id": callID,
			"reason":  reason,
		})

		var userPubKey [32]byte
		h.engine.messageSpace.SendToUser(call.UserID, "call.signal", payload, userPubKey)
	}

	return nil
}

// GetCall returns an active call by ID.
func (h *CallHandler) GetCall(callID string) (*Call, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	call, ok := h.activeCalls[callID]
	return call, ok
}

// GetCallRequest returns a pending call request by ID.
func (h *CallHandler) GetCallRequest(requestID string) (*CallRequest, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	req, ok := h.pending[requestID]
	return req, ok
}

// SetCallback registers a callback for when a call request gets a response.
func (h *CallHandler) SetCallback(requestID string, callback CallCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks[requestID] = callback
}

// watchCallTimeout monitors call request timeout.
func (h *CallHandler) watchCallTimeout(requestID string) {
	h.mu.RLock()
	request, exists := h.pending[requestID]
	if !exists {
		h.mu.RUnlock()
		return
	}
	expiresAt := request.ExpiresAt
	h.mu.RUnlock()

	time.Sleep(time.Until(expiresAt))

	h.mu.Lock()
	request, exists = h.pending[requestID]
	if !exists {
		h.mu.Unlock()
		return
	}

	request.Status = types.RequestStatusExpired
	delete(h.pending, requestID)

	callback := h.callbacks[requestID]
	delete(h.callbacks, requestID)
	h.mu.Unlock()

	// Execute callback with missed call
	if callback != nil {
		callback(&Call{
			RequestID: requestID,
			UserID:    request.UserID,
			Status:    CallStatusMissed,
			EndReason: "timeout",
		})
	}

	// Notify service
	if request.CallbackURL != "" {
		h.engine.callWebhook(request.CallbackURL, "call.missed", map[string]interface{}{
			"request_id": requestID,
			"user_id":    request.UserID,
		})
	}
}

// Helper functions

func generateCallID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func toTypesICEServers(servers []ICEServer) []types.ICEServer {
	result := make([]types.ICEServer, len(servers))
	for i, s := range servers {
		result[i] = types.ICEServer{
			URLs:       s.URLs,
			Username:   s.Username,
			Credential: s.Credential,
		}
	}
	return result
}

func toTypesSessionDesc(desc *RTCSessionDescription) *types.RTCSessionDescription {
	if desc == nil {
		return nil
	}
	return &types.RTCSessionDescription{
		Type: desc.Type,
		SDP:  desc.SDP,
	}
}

// CallOption configures a call request.
type CallOption func(*CallRequest)

// WithCallPurpose sets the purpose of the call.
func WithCallPurpose(purpose string) CallOption {
	return func(r *CallRequest) {
		r.Purpose = purpose
	}
}

// WithCallContext adds context to the call request.
func WithCallContext(ctx map[string]interface{}) CallOption {
	return func(r *CallRequest) {
		r.Context = ctx
	}
}

// WithCallTimeout sets a custom ring timeout.
func WithCallTimeout(timeout time.Duration) CallOption {
	return func(r *CallRequest) {
		r.ExpiresAt = r.CreatedAt.Add(timeout)
	}
}

// WithCallCallback sets the webhook callback URL.
func WithCallCallback(url string) CallOption {
	return func(r *CallRequest) {
		r.CallbackURL = url
	}
}
