// Package handler provides the event handler engine for VettID Service Vault.
//
// The handler engine dispatches incoming messages from users to appropriate
// handlers based on event type. It manages the lifecycle of auth/authz requests
// and provides a unified interface for the service to interact with users.
package handler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/internal/keystore"
	"github.com/vettid/vettid-service-vault/vault/internal/nats"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// EventHandler handles a specific type of event from users.
type EventHandler interface {
	// HandleRequest processes an incoming request from a user.
	HandleRequest(ctx context.Context, req *Request) (*Response, error)

	// EventType returns the event type this handler processes.
	EventType() string
}

// Request represents an incoming request from a user.
type Request struct {
	// RequestID is the unique identifier for this request
	RequestID string

	// UserID is the user who sent the request
	UserID string

	// EventType is the type of event
	EventType string

	// Payload is the decrypted request payload
	Payload []byte

	// Contract is the user's active contract (if any)
	Contract *contract.SignedConnectionContract

	// Timestamp is when the request was received
	Timestamp time.Time

	// Metadata contains additional context
	Metadata map[string]interface{}
}

// Response represents the handler's response.
type Response struct {
	// Status indicates the outcome
	Status string

	// Payload is the response payload (will be encrypted)
	Payload []byte

	// Data contains structured response data (alternative to Payload)
	Data map[string]interface{}

	// Error contains error details if status is error
	Error *types.APIError
}

// Engine dispatches events to registered handlers.
type Engine struct {
	mu             sync.RWMutex
	handlers       map[string]EventHandler
	contracts      contract.Store
	keystore       keystore.KeyStore
	router         *nats.Router
	serviceSpace   *nats.ServiceSpaceClient
	messageSpace   *nats.MessageSpaceClient
	serviceID      string
	defaultTimeout time.Duration
}

// EngineConfig holds configuration for the handler engine.
type EngineConfig struct {
	Contracts      contract.Store
	Keystore       keystore.KeyStore
	ServiceSpace   *nats.ServiceSpaceClient
	MessageSpace   *nats.MessageSpaceClient
	ServiceID      string
	DefaultTimeout time.Duration
}

// NewEngine creates a new handler engine.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.Contracts == nil {
		return nil, fmt.Errorf("contract store is required")
	}
	if cfg.ServiceID == "" {
		return nil, fmt.Errorf("service ID is required")
	}

	timeout := cfg.DefaultTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	e := &Engine{
		handlers:       make(map[string]EventHandler),
		contracts:      cfg.Contracts,
		keystore:       cfg.Keystore,
		serviceSpace:   cfg.ServiceSpace,
		messageSpace:   cfg.MessageSpace,
		serviceID:      cfg.ServiceID,
		defaultTimeout: timeout,
	}

	// Create router if we have a ServiceSpace client
	if cfg.ServiceSpace != nil {
		e.router = nats.NewRouter()
		e.router.SetServiceSpace(cfg.ServiceSpace)
	}

	return e, nil
}

// RegisterHandler registers a handler for an event type.
func (e *Engine) RegisterHandler(h EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()

	eventType := h.EventType()
	e.handlers[eventType] = h

	// Register with the router if available
	if e.router != nil {
		e.router.RegisterHandler(eventType, e.createMessageHandler(h))
	}
}

// createMessageHandler wraps an EventHandler for the NATS router.
func (e *Engine) createMessageHandler(h EventHandler) nats.MessageHandler {
	return func(ctx context.Context, msg *nats.Message) error {
		// Get the user's contract
		userContract, err := e.contracts.GetContractByUser(ctx, msg.UserID)
		if err != nil {
			// No contract - reject unless this is a contract-related event
			if msg.EventType != "contract.submit" && msg.EventType != "contract.request" {
				return fmt.Errorf("no active contract for user: %s", msg.UserID)
			}
		}

		// Create request
		req := &Request{
			RequestID: fmt.Sprintf("%s-%d", msg.UserID, time.Now().UnixNano()),
			UserID:    msg.UserID,
			EventType: msg.EventType,
			Payload:   msg.Payload,
			Contract:  userContract,
			Timestamp: msg.Timestamp,
			Metadata:  make(map[string]interface{}),
		}

		// Call handler
		resp, err := h.HandleRequest(ctx, req)
		if err != nil {
			return err
		}

		// If there's a response to send, we'd do it here
		// For now, the handler is responsible for sending responses
		_ = resp

		return nil
	}
}

// Start begins processing events.
func (e *Engine) Start() error {
	if e.router == nil {
		return fmt.Errorf("no router configured (ServiceSpace client required)")
	}

	return e.router.Start()
}

// Handler returns a registered handler by event type.
func (e *Engine) Handler(eventType string) (EventHandler, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	h, ok := e.handlers[eventType]
	return h, ok
}

// EventTypes returns all registered event types.
func (e *Engine) EventTypes() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	types := make([]string, 0, len(e.handlers))
	for t := range e.handlers {
		types = append(types, t)
	}
	return types
}

// SendToUser sends a message to a user via MessageSpace.
func (e *Engine) SendToUser(ctx context.Context, userID string, eventType string, payload []byte) error {
	if e.messageSpace == nil {
		return fmt.Errorf("MessageSpace client not configured")
	}

	// Get the user's contract for their connection key
	_, err := e.contracts.GetContractByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user contract: %w", err)
	}

	// Decode user's connection key
	var userPubKey [32]byte
	// TODO: In production, decode from userContract.UserConnectionKey

	return e.messageSpace.SendToUser(userID, eventType, payload, userPubKey)
}

// GetContract returns a user's active contract.
func (e *Engine) GetContract(ctx context.Context, userID string) (*contract.SignedConnectionContract, error) {
	return e.contracts.GetContractByUser(ctx, userID)
}

// Contracts returns the contract store.
func (e *Engine) Contracts() contract.Store {
	return e.contracts
}

// ServiceID returns the service identifier.
func (e *Engine) ServiceID() string {
	return e.serviceID
}

// hasCapability checks if a user's contract grants the specified capability.
func (e *Engine) hasCapability(ctx context.Context, userID string, capability types.CapabilityType) bool {
	userContract, err := e.contracts.GetContractByUser(ctx, userID)
	if err != nil || userContract == nil {
		return false
	}

	// Check if the contract is active
	if userContract.Status != types.ContractStatusActive {
		return false
	}

	// Check capabilities in the offering snapshot
	for _, grant := range userContract.OfferingSnapshot.Capabilities {
		if grant.Capability == capability {
			// Check if capability has expired
			if grant.ExpiresAt != nil && grant.ExpiresAt.Before(time.Now()) {
				continue
			}
			return true
		}
	}

	return false
}

// callWebhook sends an HTTP POST to a webhook URL with the given event and payload.
// This is used to notify services of async events like auth responses, call accepted, etc.
// Note: This method creates its own context with a reasonable timeout for webhook calls.
func (e *Engine) callWebhook(webhookURL, eventType string, payload interface{}) error {
	if webhookURL == "" {
		return nil // No webhook configured, silently skip
	}

	// Create a context with timeout for the webhook call
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return callWebhookHTTP(ctx, webhookURL, eventType, payload)
}
