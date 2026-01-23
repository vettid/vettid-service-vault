// Package types provides shared type definitions for the VettID Service Vault
// that can be reused by SDKs.
package types

import (
	"time"
)

// ServiceType represents the type of service.
type ServiceType string

const (
	ServiceTypeGeneric     ServiceType = "generic"
	ServiceTypePayment     ServiceType = "payment"
	ServiceTypeIdentity    ServiceType = "identity"
	ServiceTypeCommerce    ServiceType = "commerce"
	ServiceTypeHealthcare  ServiceType = "healthcare"
	ServiceTypeFinance     ServiceType = "finance"
)

// ContractStatus represents the status of a connection contract.
type ContractStatus string

const (
	ContractStatusPending   ContractStatus = "pending"
	ContractStatusActive    ContractStatus = "active"
	ContractStatusPaused    ContractStatus = "paused"
	ContractStatusCancelled ContractStatus = "cancelled"
	ContractStatusExpired   ContractStatus = "expired"
)

// RequestStatus represents the status of an auth/authz request.
type RequestStatus string

const (
	RequestStatusPending  RequestStatus = "pending"
	RequestStatusApproved RequestStatus = "approved"
	RequestStatusDenied   RequestStatus = "denied"
	RequestStatusExpired  RequestStatus = "expired"
	RequestStatusOffline  RequestStatus = "offline_approved" // Approved within offline grace period
)

// CapabilityType represents a type of capability that can be granted.
type CapabilityType string

const (
	CapabilityAuthenticate CapabilityType = "authenticate"
	CapabilityAuthorize    CapabilityType = "authorize"
	CapabilityReadData     CapabilityType = "read_data"
	CapabilityWriteData    CapabilityType = "write_data"
	CapabilitySign         CapabilityType = "sign"
)

// CapabilityGrant represents a specific capability granted to a service.
type CapabilityGrant struct {
	Capability  CapabilityType         `json:"capability"`
	Scope       string                 `json:"scope,omitempty"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
}

// DataRequirement specifies data the service requires from the user.
type DataRequirement struct {
	DataType    string `json:"data_type"`
	Required    bool   `json:"required"`
	Purpose     string `json:"purpose"`
	Retention   string `json:"retention,omitempty"`   // Duration or "session"
	ThirdParty  bool   `json:"third_party,omitempty"` // Will data be shared?
}

// Pricing describes the pricing for a service offering.
type Pricing struct {
	Type     string `json:"type"` // free, one_time, subscription
	Amount   int64  `json:"amount,omitempty"`
	Currency string `json:"currency,omitempty"`
	Interval string `json:"interval,omitempty"` // For subscriptions
}

// Signature represents a cryptographic signature with metadata.
type Signature struct {
	Algorithm string    `json:"algorithm"` // ed25519
	PublicKey string    `json:"public_key"`
	Signature string    `json:"signature"` // base64 encoded
	Timestamp time.Time `json:"timestamp"`
}

// RegistryAttestation represents verification from the VettID registry.
type RegistryAttestation struct {
	Type      string    `json:"type"`
	Issuer    string    `json:"issuer"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Signature string    `json:"signature"`
}

// EncryptedPayload represents an encrypted message payload.
type EncryptedPayload struct {
	EphemeralPublicKey string `json:"ephemeral_public_key"` // Base64 X25519 public key
	Nonce              string `json:"nonce"`                // Base64 24-byte nonce
	Ciphertext         string `json:"ciphertext"`           // Base64 encrypted data
}

// EventMessage represents a message sent over NATS.
type EventMessage struct {
	EventID   string           `json:"event_id"`
	EventType string           `json:"event_type"`
	Timestamp time.Time        `json:"timestamp"`
	Sender    string           `json:"sender"`    // service_id or user_id
	Recipient string           `json:"recipient"` // service_id or user_id
	Payload   EncryptedPayload `json:"payload"`
}

// AuthRequest represents a request for user authentication.
type AuthRequest struct {
	RequestID    string                 `json:"request_id"`
	UserID       string                 `json:"user_id"`
	Purpose      string                 `json:"purpose"`
	Context      map[string]interface{} `json:"context,omitempty"`
	ExpiresAt    time.Time              `json:"expires_at"`
	OfflineGrace time.Duration          `json:"offline_grace,omitempty"`
	CallbackURL  string                 `json:"callback_url,omitempty"`
}

// AuthResponse represents a response to an authentication request.
type AuthResponse struct {
	RequestID  string        `json:"request_id"`
	Status     RequestStatus `json:"status"`
	UserID     string        `json:"user_id"`
	Timestamp  time.Time     `json:"timestamp"`
	Signature  Signature     `json:"signature"`
	SessionKey string        `json:"session_key,omitempty"` // Optional session-specific key
}

// AuthzRequest represents a request for user authorization.
type AuthzRequest struct {
	RequestID    string                 `json:"request_id"`
	UserID       string                 `json:"user_id"`
	Action       string                 `json:"action"`
	Resource     string                 `json:"resource"`
	Context      map[string]interface{} `json:"context,omitempty"`
	ExpiresAt    time.Time              `json:"expires_at"`
	OfflineGrace time.Duration          `json:"offline_grace,omitempty"`
	CallbackURL  string                 `json:"callback_url,omitempty"`
}

// AuthzResponse represents a response to an authorization request.
type AuthzResponse struct {
	RequestID  string        `json:"request_id"`
	Status     RequestStatus `json:"status"`
	UserID     string        `json:"user_id"`
	Action     string        `json:"action"`
	Resource   string        `json:"resource"`
	Timestamp  time.Time     `json:"timestamp"`
	Signature  Signature     `json:"signature"`
	ExpiresAt  *time.Time    `json:"expires_at,omitempty"` // When approval expires
}

// Error codes for API responses.
const (
	ErrCodeInvalidRequest     = "invalid_request"
	ErrCodeUnauthorized       = "unauthorized"
	ErrCodeForbidden          = "forbidden"
	ErrCodeNotFound           = "not_found"
	ErrCodeConflict           = "conflict"
	ErrCodeTimeout            = "timeout"
	ErrCodeUserOffline        = "user_offline"
	ErrCodeContractRequired   = "contract_required"
	ErrCodeCapabilityDenied   = "capability_denied"
	ErrCodeSignatureInvalid   = "signature_invalid"
	ErrCodeInternal           = "internal_error"
)

// APIError represents an error response from the API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e APIError) Error() string {
	return e.Message
}
