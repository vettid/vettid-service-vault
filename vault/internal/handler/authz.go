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

// AuthzHandler handles authorization requests from the service API
// and responses from users.
type AuthzHandler struct {
	mu           sync.RWMutex
	engine       *Engine
	pending      map[string]*AuthzRequest
	callbacks    map[string]AuthzCallback
	timeout      time.Duration
	offlineGrace time.Duration
}

// AuthzRequest represents a pending authorization request.
type AuthzRequest struct {
	RequestID    string                 `json:"request_id"`
	UserID       string                 `json:"user_id"`
	Action       string                 `json:"action"`
	Resource     string                 `json:"resource"`
	Context      map[string]interface{} `json:"context,omitempty"`
	ExpiresAt    time.Time              `json:"expires_at"`
	OfflineGrace time.Duration          `json:"offline_grace,omitempty"`
	CallbackURL  string                 `json:"callback_url,omitempty"`
	Status       types.RequestStatus    `json:"status"`
	CreatedAt    time.Time              `json:"created_at"`
	RespondedAt  *time.Time             `json:"responded_at,omitempty"`
	Response     *AuthzResponse         `json:"response,omitempty"`
}

// AuthzResponse represents a user's response to an authz request.
type AuthzResponse struct {
	RequestID    string              `json:"request_id"`
	Status       types.RequestStatus `json:"status"`
	Action       string              `json:"action"`
	Resource     string              `json:"resource"`
	Timestamp    time.Time           `json:"timestamp"`
	ExpiresAt    *time.Time          `json:"expires_at,omitempty"` // When approval expires
	Signature    *types.Signature    `json:"signature,omitempty"`
	Constraints  map[string]interface{} `json:"constraints,omitempty"` // User-imposed constraints
}

// AuthzCallback is called when an authz request receives a response.
type AuthzCallback func(request *AuthzRequest, response *AuthzResponse)

// AuthzHandlerConfig holds configuration for the authz handler.
type AuthzHandlerConfig struct {
	Engine       *Engine
	Timeout      time.Duration
	OfflineGrace time.Duration
}

// NewAuthzHandler creates a new authorization handler.
func NewAuthzHandler(cfg AuthzHandlerConfig) *AuthzHandler {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	offlineGrace := cfg.OfflineGrace
	if offlineGrace == 0 {
		offlineGrace = 24 * time.Hour
	}

	h := &AuthzHandler{
		engine:      cfg.Engine,
		pending:     make(map[string]*AuthzRequest),
		callbacks:   make(map[string]AuthzCallback),
		timeout:     timeout,
		offlineGrace: offlineGrace,
	}

	return h
}

// EventType returns the event type this handler processes.
func (h *AuthzHandler) EventType() string {
	return "authz.response"
}

// HandleRequest processes an incoming authz response from a user.
func (h *AuthzHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	// Parse the response
	var authzResp AuthzResponse
	if err := json.Unmarshal(req.Payload, &authzResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: "invalid authz response format",
			},
		}, nil
	}

	// Process the response
	if err := h.HandleResponse(req.UserID, authzResp.RequestID, &authzResp); err != nil {
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

// RequestAuthz creates a new authorization request.
func (h *AuthzHandler) RequestAuthz(ctx context.Context, userID string, action string, resource string, opts ...AuthzOption) (*AuthzRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows authorization
	if !contract.VerifyCapability(userContract, types.CapabilityAuthorize) {
		return nil, fmt.Errorf("contract does not grant authorize capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &AuthzRequest{
		RequestID:    requestID,
		UserID:       userID,
		Action:       action,
		Resource:     resource,
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
		payload, _ := json.Marshal(types.AuthzRequest{
			RequestID:    requestID,
			UserID:       userID,
			Action:       action,
			Resource:     resource,
			Context:      request.Context,
			ExpiresAt:    request.ExpiresAt,
			OfflineGrace: request.OfflineGrace,
			CallbackURL:  request.CallbackURL,
		})

		// Get user's public key from contract
		var userPubKey [32]byte
		// In production, decode from userContract.UserConnectionKey

		if err := h.engine.messageSpace.SendAuthzRequest(userID, payload, userPubKey); err != nil {
			// Log error but don't fail - request is created
		}
	}

	return request, nil
}

// HandleResponse processes a user's response to an authz request.
func (h *AuthzHandler) HandleResponse(userID string, requestID string, response *AuthzResponse) error {
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

	// Verify action and resource match
	if request.Action != response.Action || request.Resource != response.Resource {
		h.mu.Unlock()
		return fmt.Errorf("response action/resource mismatch")
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

// GetRequest returns a pending authz request by ID.
func (h *AuthzHandler) GetRequest(requestID string) (*AuthzRequest, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	req, ok := h.pending[requestID]
	return req, ok
}

// SetCallback registers a callback for when a request receives a response.
func (h *AuthzHandler) SetCallback(requestID string, callback AuthzCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks[requestID] = callback
}

// callWebhook sends the authz result to a webhook URL.
func (h *AuthzHandler) callWebhook(request *AuthzRequest, response *AuthzResponse) {
	payload, _ := json.Marshal(map[string]interface{}{
		"type":       "authz.response",
		"request_id": request.RequestID,
		"user_id":    request.UserID,
		"action":     request.Action,
		"resource":   request.Resource,
		"status":     response.Status,
		"timestamp":  response.Timestamp,
		"expires_at": response.ExpiresAt,
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
func (h *AuthzHandler) Cleanup() {
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

// CheckAuthorization checks if a previous authorization is still valid.
// Returns true if the user has an unexpired approval for the action/resource.
func (h *AuthzHandler) CheckAuthorization(userID string, action string, resource string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Look for a matching approved request
	for _, req := range h.pending {
		if req.UserID != userID || req.Action != action || req.Resource != resource {
			continue
		}
		if req.Response == nil || req.Response.Status != types.RequestStatusApproved {
			continue
		}
		if req.Response.ExpiresAt != nil && time.Now().After(*req.Response.ExpiresAt) {
			continue
		}
		return true
	}

	return false
}

// AuthzOption configures an authz request.
type AuthzOption func(*AuthzRequest)

// WithAuthzContext adds context to the authz request.
func WithAuthzContext(ctx map[string]interface{}) AuthzOption {
	return func(r *AuthzRequest) {
		r.Context = ctx
	}
}

// WithAuthzTimeout sets a custom timeout for the authz request.
func WithAuthzTimeout(timeout time.Duration) AuthzOption {
	return func(r *AuthzRequest) {
		r.ExpiresAt = r.CreatedAt.Add(timeout)
	}
}

// WithAuthzCallback sets the webhook callback URL.
func WithAuthzCallback(url string) AuthzOption {
	return func(r *AuthzRequest) {
		r.CallbackURL = url
	}
}

// WithAuthzOfflineGrace sets the offline grace period.
func WithAuthzOfflineGrace(grace time.Duration) AuthzOption {
	return func(r *AuthzRequest) {
		r.OfflineGrace = grace
	}
}
