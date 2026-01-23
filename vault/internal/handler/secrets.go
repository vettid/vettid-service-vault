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

// SecretsHandler manages secrets stored in users' protean credentials.
//
// Three types of secrets:
// - Minor: Automatically accessible after initial approval (e.g., preferences)
// - Critical: Requires password + approval for each access (e.g., signing keys)
// - UserOwned: Keys that become the user's property (e.g., crypto wallet keys)
type SecretsHandler struct {
	mu           sync.RWMutex
	engine       *Engine
	pending      map[string]*SecretRequest
	callbacks    map[string]SecretCallback
	timeout      time.Duration
}

// SecretRequest represents a pending request to store or retrieve a secret.
type SecretRequest struct {
	RequestID     string                 `json:"request_id"`
	UserID        string                 `json:"user_id"`
	Operation     SecretOperation        `json:"operation"`
	SecretType    SecretType             `json:"secret_type"`
	SecretID      string                 `json:"secret_id,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Data          []byte                 `json:"data,omitempty"` // Encrypted for store ops
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	ExpiresAt     time.Time              `json:"expires_at"`
	CallbackURL   string                 `json:"callback_url,omitempty"`
	Status        types.RequestStatus    `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	Response      *SecretResponse        `json:"response,omitempty"`
}

// SecretResponse represents a user's response to a secret request.
type SecretResponse struct {
	RequestID   string           `json:"request_id"`
	Status      string           `json:"status"` // approved, denied
	SecretID    string           `json:"secret_id,omitempty"`
	Data        []byte           `json:"data,omitempty"` // For retrieve operations
	Timestamp   time.Time        `json:"timestamp"`
	Signature   *types.Signature `json:"signature,omitempty"`
}

// SecretOperation indicates the type of secret operation.
type SecretOperation string

const (
	SecretOpStore    SecretOperation = "store"
	SecretOpRetrieve SecretOperation = "retrieve"
	SecretOpDelete   SecretOperation = "delete"
	SecretOpList     SecretOperation = "list"
	SecretOpUpdate   SecretOperation = "update"
)

// SecretType indicates the security level of the secret.
type SecretType string

const (
	// SecretTypeMinor - Automatic access after initial approval.
	// Good for: preferences, non-sensitive settings, cached tokens.
	SecretTypeMinor SecretType = "minor"

	// SecretTypeCritical - Requires password + approval each time.
	// Good for: signing keys, 2FA seeds, recovery codes.
	SecretTypeCritical SecretType = "critical"

	// SecretTypeUserOwned - Keys that become user's property.
	// Good for: crypto wallet keys, decryption keys, certificates.
	SecretTypeUserOwned SecretType = "user_owned"
)

// SecretMetadata describes a stored secret (without the actual data).
type SecretMetadata struct {
	SecretID    string                 `json:"secret_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Type        SecretType             `json:"type"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   *time.Time             `json:"updated_at,omitempty"`
	LastAccess  *time.Time             `json:"last_access,omitempty"`
	AccessCount int                    `json:"access_count"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// SecretCallback is called when a secret request receives a response.
type SecretCallback func(request *SecretRequest, response *SecretResponse)

// SecretsHandlerConfig holds configuration for the secrets handler.
type SecretsHandlerConfig struct {
	Engine  *Engine
	Timeout time.Duration
}

// NewSecretsHandler creates a new secrets handler.
func NewSecretsHandler(cfg SecretsHandlerConfig) *SecretsHandler {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	return &SecretsHandler{
		engine:    cfg.Engine,
		pending:   make(map[string]*SecretRequest),
		callbacks: make(map[string]SecretCallback),
		timeout:   timeout,
	}
}

// EventType returns the event type this handler processes.
func (h *SecretsHandler) EventType() string {
	return "secret.response"
}

// HandleRequest processes an incoming secret response from a user.
func (h *SecretsHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	var secretResp SecretResponse
	if err := json.Unmarshal(req.Payload, &secretResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: "invalid secret response format",
			},
		}, nil
	}

	if err := h.HandleResponse(req.UserID, &secretResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: err.Error(),
			},
		}, nil
	}

	return &Response{Status: "ok"}, nil
}

// StoreSecret requests to store a secret in the user's vault.
func (h *SecretsHandler) StoreSecret(ctx context.Context, userID string, name string, data []byte, secretType SecretType, opts ...SecretOption) (*SecretRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows secrets
	if !contract.VerifyCapability(userContract, types.CapabilitySecrets) {
		return nil, fmt.Errorf("contract does not grant secrets capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &SecretRequest{
		RequestID:  requestID,
		UserID:     userID,
		Operation:  SecretOpStore,
		SecretType: secretType,
		Name:       name,
		Data:       data,
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

	// Send secret request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.SecretRequest{
			RequestID:   requestID,
			UserID:      userID,
			Operation:   string(SecretOpStore),
			SecretType:  string(secretType),
			Name:        name,
			Description: request.Description,
			Data:        data,
			Metadata:    request.Metadata,
			ExpiresAt:   request.ExpiresAt,
		})

		// Get user's public key from contract
		var userPubKey [32]byte
		// In production, decode from userContract.UserConnectionKey

		if err := h.engine.messageSpace.SendToUser(userID, "secret.request", payload, userPubKey); err != nil {
			// Log error but don't fail - request is created
		}
	}

	return request, nil
}

// RetrieveSecret requests to retrieve a secret from the user's vault.
func (h *SecretsHandler) RetrieveSecret(ctx context.Context, userID string, secretID string, opts ...SecretOption) (*SecretRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows secrets
	if !contract.VerifyCapability(userContract, types.CapabilitySecrets) {
		return nil, fmt.Errorf("contract does not grant secrets capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &SecretRequest{
		RequestID: requestID,
		UserID:    userID,
		Operation: SecretOpRetrieve,
		SecretID:  secretID,
		ExpiresAt: now.Add(h.timeout),
		Status:    types.RequestStatusPending,
		CreatedAt: now,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	// Store pending request
	h.mu.Lock()
	h.pending[requestID] = request
	h.mu.Unlock()

	// Send secret request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.SecretRequest{
			RequestID: requestID,
			UserID:    userID,
			Operation: string(SecretOpRetrieve),
			SecretID:  secretID,
			ExpiresAt: request.ExpiresAt,
		})

		var userPubKey [32]byte
		if err := h.engine.messageSpace.SendToUser(userID, "secret.request", payload, userPubKey); err != nil {
			// Log error but don't fail
		}
	}

	return request, nil
}

// DeleteSecret requests to delete a secret from the user's vault.
func (h *SecretsHandler) DeleteSecret(ctx context.Context, userID string, secretID string, opts ...SecretOption) (*SecretRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows secrets
	if !contract.VerifyCapability(userContract, types.CapabilitySecrets) {
		return nil, fmt.Errorf("contract does not grant secrets capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &SecretRequest{
		RequestID: requestID,
		UserID:    userID,
		Operation: SecretOpDelete,
		SecretID:  secretID,
		ExpiresAt: now.Add(h.timeout),
		Status:    types.RequestStatusPending,
		CreatedAt: now,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	// Store pending request
	h.mu.Lock()
	h.pending[requestID] = request
	h.mu.Unlock()

	// Send secret request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.SecretRequest{
			RequestID: requestID,
			UserID:    userID,
			Operation: string(SecretOpDelete),
			SecretID:  secretID,
			ExpiresAt: request.ExpiresAt,
		})

		var userPubKey [32]byte
		h.engine.messageSpace.SendToUser(userID, "secret.request", payload, userPubKey)
	}

	return request, nil
}

// ListSecrets requests a list of secrets stored for this service.
func (h *SecretsHandler) ListSecrets(ctx context.Context, userID string, opts ...SecretOption) (*SecretRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows secrets
	if !contract.VerifyCapability(userContract, types.CapabilitySecrets) {
		return nil, fmt.Errorf("contract does not grant secrets capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &SecretRequest{
		RequestID: requestID,
		UserID:    userID,
		Operation: SecretOpList,
		ExpiresAt: now.Add(h.timeout),
		Status:    types.RequestStatusPending,
		CreatedAt: now,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	// Store pending request
	h.mu.Lock()
	h.pending[requestID] = request
	h.mu.Unlock()

	// Send secret request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.SecretRequest{
			RequestID: requestID,
			UserID:    userID,
			Operation: string(SecretOpList),
			ExpiresAt: request.ExpiresAt,
		})

		var userPubKey [32]byte
		h.engine.messageSpace.SendToUser(userID, "secret.request", payload, userPubKey)
	}

	return request, nil
}

// UpdateSecret requests to update an existing secret.
func (h *SecretsHandler) UpdateSecret(ctx context.Context, userID string, secretID string, data []byte, opts ...SecretOption) (*SecretRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows secrets
	if !contract.VerifyCapability(userContract, types.CapabilitySecrets) {
		return nil, fmt.Errorf("contract does not grant secrets capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &SecretRequest{
		RequestID: requestID,
		UserID:    userID,
		Operation: SecretOpUpdate,
		SecretID:  secretID,
		Data:      data,
		ExpiresAt: now.Add(h.timeout),
		Status:    types.RequestStatusPending,
		CreatedAt: now,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	// Store pending request
	h.mu.Lock()
	h.pending[requestID] = request
	h.mu.Unlock()

	// Send secret request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.SecretRequest{
			RequestID: requestID,
			UserID:    userID,
			Operation: string(SecretOpUpdate),
			SecretID:  secretID,
			Data:      data,
			Metadata:  request.Metadata,
			ExpiresAt: request.ExpiresAt,
		})

		var userPubKey [32]byte
		h.engine.messageSpace.SendToUser(userID, "secret.request", payload, userPubKey)
	}

	return request, nil
}

// HandleResponse processes a user's response to a secret request.
func (h *SecretsHandler) HandleResponse(userID string, response *SecretResponse) error {
	h.mu.Lock()
	request, exists := h.pending[response.RequestID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("unknown request ID: %s", response.RequestID)
	}

	// Verify request belongs to this user
	if request.UserID != userID {
		h.mu.Unlock()
		return fmt.Errorf("request belongs to different user")
	}

	// Check if expired
	if time.Now().After(request.ExpiresAt) {
		request.Status = types.RequestStatusExpired
		h.mu.Unlock()
		return fmt.Errorf("request has expired")
	}

	// Update request
	switch response.Status {
	case "approved":
		request.Status = types.RequestStatusApproved
	case "denied":
		request.Status = types.RequestStatusDenied
	default:
		h.mu.Unlock()
		return fmt.Errorf("invalid status: %s", response.Status)
	}

	request.Response = response

	// Get callback
	callback := h.callbacks[response.RequestID]
	delete(h.callbacks, response.RequestID)
	delete(h.pending, response.RequestID)
	h.mu.Unlock()

	// Execute callback
	if callback != nil {
		callback(request, response)
	}

	// Call webhook if configured
	if request.CallbackURL != "" {
		go h.callSecretWebhook(request, response)
	}

	return nil
}

// GetSecretRequest returns a pending secret request by ID.
func (h *SecretsHandler) GetSecretRequest(requestID string) (*SecretRequest, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	req, ok := h.pending[requestID]
	return req, ok
}

// SetCallback registers a callback for when a secret request receives a response.
func (h *SecretsHandler) SetCallback(requestID string, callback SecretCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks[requestID] = callback
}

// callSecretWebhook sends the secret result to a webhook URL.
func (h *SecretsHandler) callSecretWebhook(request *SecretRequest, response *SecretResponse) {
	payload := map[string]interface{}{
		"type":        "secret.response",
		"request_id":  request.RequestID,
		"user_id":     request.UserID,
		"operation":   request.Operation,
		"status":      request.Status,
	}

	if response.SecretID != "" {
		payload["secret_id"] = response.SecretID
	}

	// Note: Don't include actual secret data in webhook for security
	// The service should fetch via the response object

	h.engine.callWebhook(request.CallbackURL, "secret.response", payload)
}

// Cleanup removes expired pending requests.
func (h *SecretsHandler) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for id, req := range h.pending {
		if now.After(req.ExpiresAt) {
			delete(h.pending, id)
			delete(h.callbacks, id)
		}
	}
}

// SecretOption configures a secret request.
type SecretOption func(*SecretRequest)

// WithSecretDescription sets the secret description.
func WithSecretDescription(description string) SecretOption {
	return func(r *SecretRequest) {
		r.Description = description
	}
}

// WithSecretMetadata sets custom metadata.
func WithSecretMetadata(metadata map[string]interface{}) SecretOption {
	return func(r *SecretRequest) {
		r.Metadata = metadata
	}
}

// WithSecretCallback sets the webhook callback URL.
func WithSecretCallback(url string) SecretOption {
	return func(r *SecretRequest) {
		r.CallbackURL = url
	}
}

// WithSecretTimeout sets a custom timeout.
func WithSecretTimeout(timeout time.Duration) SecretOption {
	return func(r *SecretRequest) {
		r.ExpiresAt = r.CreatedAt.Add(timeout)
	}
}

// ParseSecretType parses a string into a SecretType.
func ParseSecretType(s string) (SecretType, error) {
	switch s {
	case "minor":
		return SecretTypeMinor, nil
	case "critical":
		return SecretTypeCritical, nil
	case "user_owned":
		return SecretTypeUserOwned, nil
	default:
		return "", fmt.Errorf("invalid secret type: %s (must be minor, critical, or user_owned)", s)
	}
}
