package vettid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AuthEvent represents an authentication webhook event.
type AuthEvent struct {
	RequestID  string        `json:"request_id"`
	UserID     string        `json:"user_id"`
	Status     RequestStatus `json:"status"`
	Timestamp  time.Time     `json:"timestamp"`
	SessionKey string        `json:"session_key,omitempty"`
}

// AuthzEvent represents an authorization webhook event.
type AuthzEvent struct {
	RequestID string        `json:"request_id"`
	UserID    string        `json:"user_id"`
	Action    string        `json:"action"`
	Resource  string        `json:"resource"`
	Status    RequestStatus `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	ExpiresAt *time.Time    `json:"expires_at,omitempty"`
}

// ContractEvent represents a contract webhook event.
type ContractEvent struct {
	ContractID string    `json:"contract_id"`
	UserID     string    `json:"user_id"`
	ServiceID  string    `json:"service_id"`
	OfferingID string    `json:"offering_id"`
	EventType  string    `json:"contract_event"` // "accepted", "cancelled", "expired"
	Timestamp  time.Time `json:"timestamp"`
}

// CallEvent represents a call webhook event.
type CallEvent struct {
	CallID    string                 `json:"call_id"`
	RequestID string                 `json:"request_id"`
	UserID    string                 `json:"user_id"`
	Status    CallStatus             `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Answer    map[string]interface{} `json:"answer,omitempty"`
	Duration  int                    `json:"duration_seconds,omitempty"`
}

// PaymentEvent represents a payment webhook event.
type PaymentEvent struct {
	RequestID      string        `json:"request_id"`
	UserID         string        `json:"user_id"`
	Status         PaymentStatus `json:"status"`
	Timestamp      time.Time     `json:"timestamp"`
	PaymentID      string        `json:"payment_id,omitempty"`
	TransactionRef string        `json:"transaction_id,omitempty"`
	FailureReason  string        `json:"failure_reason,omitempty"`
}

// SecretEvent represents a secret operation webhook event.
type SecretEvent struct {
	RequestID string        `json:"request_id"`
	UserID    string        `json:"user_id"`
	SecretID  string        `json:"secret_id,omitempty"`
	Operation string        `json:"operation"` // "stored", "retrieved", "deleted"
	Status    RequestStatus `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Data      []byte        `json:"-"` // Decoded from base64
}

// Handler function types
type (
	AuthHandler     func(event *AuthEvent) error
	AuthzHandler    func(event *AuthzEvent) error
	ContractHandler func(event *ContractEvent) error
	CallHandler     func(event *CallEvent) error
	PaymentHandler  func(event *PaymentEvent) error
	SecretHandler   func(event *SecretEvent) error
)

// WebhookRouter routes webhook events to handlers.
type WebhookRouter struct {
	signingSecret    string
	authHandlers     []AuthHandler
	authzHandlers    []AuthzHandler
	contractHandlers []ContractHandler
	callHandlers     []CallHandler
	paymentHandlers  []PaymentHandler
	secretHandlers   []SecretHandler
}

// NewWebhookRouter creates a new webhook router.
func NewWebhookRouter(signingSecret string) *WebhookRouter {
	return &WebhookRouter{
		signingSecret: signingSecret,
	}
}

// OnAuth registers an authentication event handler.
func (r *WebhookRouter) OnAuth(handler AuthHandler) *WebhookRouter {
	r.authHandlers = append(r.authHandlers, handler)
	return r
}

// OnAuthz registers an authorization event handler.
func (r *WebhookRouter) OnAuthz(handler AuthzHandler) *WebhookRouter {
	r.authzHandlers = append(r.authzHandlers, handler)
	return r
}

// OnContract registers a contract event handler.
func (r *WebhookRouter) OnContract(handler ContractHandler) *WebhookRouter {
	r.contractHandlers = append(r.contractHandlers, handler)
	return r
}

// OnCall registers a call event handler.
func (r *WebhookRouter) OnCall(handler CallHandler) *WebhookRouter {
	r.callHandlers = append(r.callHandlers, handler)
	return r
}

// OnPayment registers a payment event handler.
func (r *WebhookRouter) OnPayment(handler PaymentHandler) *WebhookRouter {
	r.paymentHandlers = append(r.paymentHandlers, handler)
	return r
}

// OnSecret registers a secret operation event handler.
func (r *WebhookRouter) OnSecret(handler SecretHandler) *WebhookRouter {
	r.secretHandlers = append(r.secretHandlers, handler)
	return r
}

// VerifySignature verifies the webhook signature.
func (r *WebhookRouter) VerifySignature(payload []byte, signature string) bool {
	if r.signingSecret == "" {
		return true // No verification if secret not configured
	}

	mac := hmac.New(sha256.New, []byte(r.signingSecret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// Handle processes an incoming webhook.
func (r *WebhookRouter) Handle(payload []byte, signature string) error {
	// Verify signature if configured
	if r.signingSecret != "" && signature != "" {
		if !r.VerifySignature(payload, signature) {
			return fmt.Errorf("invalid webhook signature")
		}
	}

	// Parse payload
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return fmt.Errorf("invalid JSON payload: %w", err)
	}

	eventType, ok := data["event_type"].(string)
	if !ok || eventType == "" {
		return fmt.Errorf("missing event_type in payload")
	}

	switch eventType {
	case "auth":
		return r.handleAuth(data)
	case "authz":
		return r.handleAuthz(data)
	case "contract":
		return r.handleContract(data)
	case "call":
		return r.handleCall(data)
	case "payment":
		return r.handlePayment(data)
	case "secret":
		return r.handleSecret(data)
	default:
		return fmt.Errorf("unknown event type: %s", eventType)
	}
}

func (r *WebhookRouter) handleAuth(data map[string]interface{}) error {
	timestamp, _ := time.Parse(time.RFC3339, data["timestamp"].(string))
	event := &AuthEvent{
		RequestID:  data["request_id"].(string),
		UserID:     data["user_id"].(string),
		Status:     RequestStatus(data["status"].(string)),
		Timestamp:  timestamp,
		SessionKey: getStringOr(data, "session_key", ""),
	}

	for _, handler := range r.authHandlers {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *WebhookRouter) handleAuthz(data map[string]interface{}) error {
	timestamp, _ := time.Parse(time.RFC3339, data["timestamp"].(string))
	event := &AuthzEvent{
		RequestID: data["request_id"].(string),
		UserID:    data["user_id"].(string),
		Action:    data["action"].(string),
		Resource:  data["resource"].(string),
		Status:    RequestStatus(data["status"].(string)),
		Timestamp: timestamp,
	}
	if expiresAt, ok := data["expires_at"].(string); ok && expiresAt != "" {
		t, _ := time.Parse(time.RFC3339, expiresAt)
		event.ExpiresAt = &t
	}

	for _, handler := range r.authzHandlers {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *WebhookRouter) handleContract(data map[string]interface{}) error {
	timestamp, _ := time.Parse(time.RFC3339, data["timestamp"].(string))
	event := &ContractEvent{
		ContractID: data["contract_id"].(string),
		UserID:     data["user_id"].(string),
		ServiceID:  data["service_id"].(string),
		OfferingID: data["offering_id"].(string),
		EventType:  data["contract_event"].(string),
		Timestamp:  timestamp,
	}

	for _, handler := range r.contractHandlers {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *WebhookRouter) handleCall(data map[string]interface{}) error {
	timestamp, _ := time.Parse(time.RFC3339, data["timestamp"].(string))
	event := &CallEvent{
		CallID:    data["call_id"].(string),
		RequestID: getStringOr(data, "request_id", ""),
		UserID:    data["user_id"].(string),
		Status:    CallStatus(data["status"].(string)),
		Timestamp: timestamp,
	}
	if answer, ok := data["answer"].(map[string]interface{}); ok {
		event.Answer = answer
	}
	if duration, ok := data["duration_seconds"].(float64); ok {
		event.Duration = int(duration)
	}

	for _, handler := range r.callHandlers {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *WebhookRouter) handlePayment(data map[string]interface{}) error {
	timestamp, _ := time.Parse(time.RFC3339, data["timestamp"].(string))
	event := &PaymentEvent{
		RequestID:      data["request_id"].(string),
		UserID:         data["user_id"].(string),
		Status:         PaymentStatus(data["status"].(string)),
		Timestamp:      timestamp,
		PaymentID:      getStringOr(data, "payment_id", ""),
		TransactionRef: getStringOr(data, "transaction_id", ""),
		FailureReason:  getStringOr(data, "failure_reason", ""),
	}

	for _, handler := range r.paymentHandlers {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *WebhookRouter) handleSecret(data map[string]interface{}) error {
	timestamp, _ := time.Parse(time.RFC3339, data["timestamp"].(string))
	event := &SecretEvent{
		RequestID: data["request_id"].(string),
		UserID:    data["user_id"].(string),
		SecretID:  getStringOr(data, "secret_id", ""),
		Operation: data["operation"].(string),
		Status:    RequestStatus(data["status"].(string)),
		Timestamp: timestamp,
	}

	// Decode base64 data if present
	if dataStr, ok := data["data"].(string); ok && dataStr != "" {
		// Remove potential "sha256=" prefix for signature verification compatibility
		if strings.HasPrefix(dataStr, "sha256=") {
			dataStr = strings.TrimPrefix(dataStr, "sha256=")
		}
		decoded, err := base64.StdEncoding.DecodeString(dataStr)
		if err == nil {
			event.Data = decoded
		}
	}

	for _, handler := range r.secretHandlers {
		if err := handler(event); err != nil {
			return err
		}
	}
	return nil
}
