// Package vettid provides a client SDK for VettID Service Vault.
package vettid

import (
	"crypto/ed25519"
	"time"
)

// ServiceType represents the category of a service.
type ServiceType string

const (
	ServiceTypeGeneric    ServiceType = "generic"
	ServiceTypePayment    ServiceType = "payment"
	ServiceTypeIdentity   ServiceType = "identity"
	ServiceTypeCommerce   ServiceType = "commerce"
	ServiceTypeHealthcare ServiceType = "healthcare"
	ServiceTypeFinance    ServiceType = "finance"
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
	RequestStatusPending         RequestStatus = "pending"
	RequestStatusApproved        RequestStatus = "approved"
	RequestStatusDenied          RequestStatus = "denied"
	RequestStatusExpired         RequestStatus = "expired"
	RequestStatusOfflineApproved RequestStatus = "offline_approved"
)

// CapabilityType represents a capability that can be granted.
type CapabilityType string

const (
	CapabilityAuthenticate CapabilityType = "authenticate"
	CapabilityAuthorize    CapabilityType = "authorize"
	CapabilityReadData     CapabilityType = "read_data"
	CapabilityWriteData    CapabilityType = "write_data"
	CapabilitySign         CapabilityType = "sign"
	CapabilityBrowseData   CapabilityType = "browse_data"
	CapabilityRequestData  CapabilityType = "request_data"
	CapabilityNotify       CapabilityType = "notify"
	CapabilityCall         CapabilityType = "call"
	CapabilityPayment      CapabilityType = "payment"
	CapabilitySecrets      CapabilityType = "secrets"
)

// CallType represents the type of call.
type CallType string

const (
	CallTypeVoice CallType = "voice"
	CallTypeVideo CallType = "video"
)

// CallStatus represents the status of a call.
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

// PaymentStatus represents the status of a payment.
type PaymentStatus string

const (
	PaymentStatusPending       PaymentStatus = "pending"
	PaymentStatusProcessing    PaymentStatus = "processing"
	PaymentStatusCompleted     PaymentStatus = "completed"
	PaymentStatusFailed        PaymentStatus = "failed"
	PaymentStatusCancelled     PaymentStatus = "cancelled"
	PaymentStatusRefunded      PaymentStatus = "refunded"
	PaymentStatusPartialRefund PaymentStatus = "partial_refund"
)

// SecretType represents the security level of a secret.
type SecretType string

const (
	SecretTypeMinor     SecretType = "minor"
	SecretTypeCritical  SecretType = "critical"
	SecretTypeUserOwned SecretType = "user_owned"
)

// ServiceIdentity represents a service's cryptographic identity.
type ServiceIdentity struct {
	ServiceID           string      `json:"service_id"`
	ServiceName         string      `json:"service_name"`
	ServiceType         ServiceType `json:"service_type"`
	SigningPublicKey    string      `json:"signing_public_key"`    // Base64 Ed25519
	EncryptionPublicKey string      `json:"encryption_public_key"` // Base64 X25519
	Domain              string      `json:"domain,omitempty"`
	DomainVerified      bool        `json:"domain_verified,omitempty"`
	NATSEndpoint        string      `json:"nats_endpoint,omitempty"`
}

// ServiceKeyPair contains the private and public keys for a service.
type ServiceKeyPair struct {
	SigningPrivateKey    ed25519.PrivateKey
	SigningPublicKey     ed25519.PublicKey
	EncryptionPrivateKey [32]byte // X25519 private key
	EncryptionPublicKey  [32]byte // X25519 public key
}

// CapabilityGrant represents a capability granted to a service.
type CapabilityGrant struct {
	Capability  CapabilityType         `json:"capability"`
	Scope       string                 `json:"scope,omitempty"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
}

// Money represents a monetary amount.
type Money struct {
	Amount   int64  `json:"amount"`   // In smallest currency unit (cents)
	Currency string `json:"currency"` // ISO 4217
}

// ConnectionContract represents a signed connection contract.
type ConnectionContract struct {
	ContractID   string         `json:"contract_id"`
	UserID       string         `json:"user_id"`
	ServiceID    string         `json:"service_id"`
	OfferingID   string         `json:"offering_id"`
	Status       ContractStatus `json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	ActivatedAt  *time.Time     `json:"activated_at,omitempty"`
	CancelledAt  *time.Time     `json:"cancelled_at,omitempty"`
}

// AuthResponse represents an authentication response.
type AuthResponse struct {
	RequestID  string        `json:"request_id"`
	Status     RequestStatus `json:"status"`
	UserID     string        `json:"user_id"`
	Timestamp  time.Time     `json:"timestamp"`
	SessionKey string        `json:"session_key,omitempty"`
}

// AuthzResponse represents an authorization response.
type AuthzResponse struct {
	RequestID string        `json:"request_id"`
	Status    RequestStatus `json:"status"`
	UserID    string        `json:"user_id"`
	Action    string        `json:"action"`
	Resource  string        `json:"resource"`
	Timestamp time.Time     `json:"timestamp"`
	ExpiresAt *time.Time    `json:"expires_at,omitempty"`
}

// CallResult represents the result of a call operation.
type CallResult struct {
	CallID      string                 `json:"call_id"`
	RequestID   string                 `json:"request_id"`
	Status      CallStatus             `json:"status"`
	Answer      map[string]interface{} `json:"answer,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	ConnectedAt *time.Time             `json:"connected_at,omitempty"`
	EndedAt     *time.Time             `json:"ended_at,omitempty"`
	Duration    int                    `json:"duration_seconds,omitempty"`
}

// PaymentResult represents the result of a payment operation.
type PaymentResult struct {
	RequestID      string        `json:"request_id"`
	Status         PaymentStatus `json:"status"`
	PaymentID      string        `json:"payment_id,omitempty"`
	TransactionRef string        `json:"transaction_id,omitempty"`
	ReceiptURL     string        `json:"receipt_url,omitempty"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
	FailedAt       *time.Time    `json:"failed_at,omitempty"`
	FailureReason  string        `json:"failure_reason,omitempty"`
}

// SecretResult represents the result of a secret operation.
type SecretResult struct {
	RequestID string        `json:"request_id"`
	Status    RequestStatus `json:"status"`
	SecretID  string        `json:"secret_id,omitempty"`
	Data      []byte        `json:"data,omitempty"`
}

// VaultConfig holds the SDK configuration.
type VaultConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	Retries int
	Headers map[string]string
}

// APIError represents an error from the Service Vault API.
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}
