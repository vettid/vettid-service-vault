package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// AuthHandler handles authentication requests from the service API
// and responses from users.
type AuthHandler struct {
	mu          sync.RWMutex
	engine      *Engine
	pending     map[string]*AuthRequest
	callbacks   map[string]AuthCallback
	timeout     time.Duration
	offlineGrace time.Duration
}

// AuthRequest represents a pending authentication request.
type AuthRequest struct {
	RequestID    string                 `json:"request_id"`
	UserID       string                 `json:"user_id"`
	Purpose      string                 `json:"purpose"`
	Context      map[string]interface{} `json:"context,omitempty"`
	ExpiresAt    time.Time              `json:"expires_at"`
	OfflineGrace time.Duration          `json:"offline_grace,omitempty"`
	CallbackURL  string                 `json:"callback_url,omitempty"`
	Status       types.RequestStatus    `json:"status"`
	CreatedAt    time.Time              `json:"created_at"`
	RespondedAt  *time.Time             `json:"responded_at,omitempty"`
	Response     *AuthResponse          `json:"response,omitempty"`
}

// AuthResponse represents a user's response to an auth request.
type AuthResponse struct {
	RequestID string              `json:"request_id"`
	Status    types.RequestStatus `json:"status"`
	Timestamp time.Time           `json:"timestamp"`
	Signature *types.Signature    `json:"signature,omitempty"`
}

// AuthCallback is called when an auth request receives a response.
type AuthCallback func(request *AuthRequest, response *AuthResponse)

// AuthHandlerConfig holds configuration for the auth handler.
type AuthHandlerConfig struct {
	Engine       *Engine
	Timeout      time.Duration
	OfflineGrace time.Duration
}

// NewAuthHandler creates a new authentication handler.
func NewAuthHandler(cfg AuthHandlerConfig) *AuthHandler {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	offlineGrace := cfg.OfflineGrace
	if offlineGrace == 0 {
		offlineGrace = 24 * time.Hour
	}

	h := &AuthHandler{
		engine:      cfg.Engine,
		pending:     make(map[string]*AuthRequest),
		callbacks:   make(map[string]AuthCallback),
		timeout:     timeout,
		offlineGrace: offlineGrace,
	}

	return h
}

// EventType returns the event type this handler processes.
func (h *AuthHandler) EventType() string {
	return "auth.response"
}

// HandleRequest processes an incoming auth response from a user.
func (h *AuthHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	// Parse the response
	var authResp AuthResponse
	if err := json.Unmarshal(req.Payload, &authResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: "invalid auth response format",
			},
		}, nil
	}

	// Process the response
	if err := h.HandleResponse(req.UserID, authResp.RequestID, &authResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: err.Error(),
			},
		}, nil
	}

	return &Response{
		Status: "ok",
	}, nil
}

// RequestAuth creates a new authentication request.
func (h *AuthHandler) RequestAuth(ctx context.Context, userID string, purpose string, opts ...AuthOption) (*AuthRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows authentication
	if !contract.VerifyCapability(userContract, types.CapabilityAuthenticate) {
		return nil, fmt.Errorf("contract does not grant authenticate capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &AuthRequest{
		RequestID:    requestID,
		UserID:       userID,
		Purpose:      purpose,
		ExpiresAt:    now.Add(h.timeout),
		OfflineGrace: h.offlineGrace,
		Status:       types.RequestStatusPending,
		CreatedAt:    now,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	// Store pending request
	h.mu.Lock()
	h.pending[requestID] = request
	h.mu.Unlock()

	// Send request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.AuthRequest{
			RequestID:    requestID,
			UserID:       userID,
			Purpose:      purpose,
			Context:      request.Context,
			ExpiresAt:    request.ExpiresAt,
			OfflineGrace: request.OfflineGrace,
			CallbackURL:  request.CallbackURL,
		})

		// Get user's public key from contract
		var userPubKey [32]byte
		// In production, decode from userContract.UserConnectionKey

		if err := h.engine.messageSpace.SendAuthRequest(userID, payload, userPubKey); err != nil {
			// Log error but don't fail - request is created
		}
	}

	return request, nil
}

// HandleResponse processes a user's response to an auth request.
func (h *AuthHandler) HandleResponse(userID string, requestID string, response *AuthResponse) error {
	h.mu.Lock()
	request, exists := h.pending[requestID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("unknown request ID: %s", requestID)
	}

	// Verify request belongs to this user
	if request.UserID != userID {
		h.mu.Unlock()
		return fmt.Errorf("request belongs to different user")
	}

	// Check if expired
	if time.Now().After(request.ExpiresAt) {
		// Check offline grace period
		graceEnd := request.ExpiresAt.Add(request.OfflineGrace)
		if time.Now().After(graceEnd) {
			request.Status = types.RequestStatusExpired
			h.mu.Unlock()
			return fmt.Errorf("request has expired")
		}
		// Within offline grace - mark as offline approved if approved
		if response.Status == types.RequestStatusApproved {
			response.Status = types.RequestStatusOffline
		}
	}

	// Update request
	now := time.Now().UTC()
	request.Status = response.Status
	request.RespondedAt = &now
	request.Response = response

	// Get callback if registered
	callback := h.callbacks[requestID]
	delete(h.callbacks, requestID)
	delete(h.pending, requestID)
	h.mu.Unlock()

	// Execute callback
	if callback != nil {
		callback(request, response)
	}

	// Call webhook if configured
	if request.CallbackURL != "" {
		go h.callWebhook(request, response)
	}

	return nil
}

// GetRequest returns a pending auth request by ID.
func (h *AuthHandler) GetRequest(requestID string) (*AuthRequest, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	req, ok := h.pending[requestID]
	return req, ok
}

// SetCallback registers a callback for when a request receives a response.
func (h *AuthHandler) SetCallback(requestID string, callback AuthCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks[requestID] = callback
}

// callWebhook sends the auth result to a webhook URL.
func (h *AuthHandler) callWebhook(request *AuthRequest, response *AuthResponse) {
	payload, _ := json.Marshal(map[string]interface{}{
		"type":       "auth.response",
		"request_id": request.RequestID,
		"user_id":    request.UserID,
		"status":     response.Status,
		"timestamp":  response.Timestamp,
	})

	req, err := http.NewRequest("POST", request.CallbackURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_ = payload // Use in actual implementation
}

// Cleanup removes expired pending requests.
func (h *AuthHandler) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for id, req := range h.pending {
		graceEnd := req.ExpiresAt.Add(req.OfflineGrace)
		if now.After(graceEnd) {
			delete(h.pending, id)
			delete(h.callbacks, id)
		}
	}
}

// AuthOption configures an auth request.
type AuthOption func(*AuthRequest)

// WithAuthContext adds context to the auth request.
func WithAuthContext(ctx map[string]interface{}) AuthOption {
	return func(r *AuthRequest) {
		r.Context = ctx
	}
}

// WithAuthTimeout sets a custom timeout for the auth request.
func WithAuthTimeout(timeout time.Duration) AuthOption {
	return func(r *AuthRequest) {
		r.ExpiresAt = r.CreatedAt.Add(timeout)
	}
}

// WithAuthCallback sets the webhook callback URL.
func WithAuthCallback(url string) AuthOption {
	return func(r *AuthRequest) {
		r.CallbackURL = url
	}
}

// WithOfflineGrace sets the offline grace period.
func WithOfflineGrace(grace time.Duration) AuthOption {
	return func(r *AuthRequest) {
		r.OfflineGrace = grace
	}
}
