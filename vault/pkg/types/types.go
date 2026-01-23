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
	CapabilityBrowseData   CapabilityType = "browse_data"
	CapabilityRequestData  CapabilityType = "request_data"
	CapabilityNotify       CapabilityType = "notify"
	CapabilityCall         CapabilityType = "call"
	CapabilityPayment      CapabilityType = "payment"
	CapabilitySecrets      CapabilityType = "secrets"
)

// Capability aliases for handler package
type Capability = CapabilityType

const (
	CapabilityBrowse = CapabilityBrowseData
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

// DataRequest represents a request for user data.
type DataRequest struct {
	RequestID    string                 `json:"request_id"`
	UserID       string                 `json:"user_id"`
	RequestType  string                 `json:"request_type"` // browse_metadata or request_data
	DataTypes    []string               `json:"data_types,omitempty"`
	DataPaths    []string               `json:"data_paths,omitempty"`
	Purpose      string                 `json:"purpose"`
	Context      map[string]interface{} `json:"context,omitempty"`
	ExpiresAt    time.Time              `json:"expires_at"`
	OfflineGrace time.Duration          `json:"offline_grace,omitempty"`
	CallbackURL  string                 `json:"callback_url,omitempty"`
}

// DataResponse represents a user's response to a data request.
type DataResponse struct {
	RequestID   string                 `json:"request_id"`
	Status      RequestStatus          `json:"status"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Metadata    []DataTypeMetadata     `json:"metadata,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Signature   *Signature             `json:"signature,omitempty"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
}

// DataTypeMetadata describes available data types.
type DataTypeMetadata struct {
	DataType    string `json:"data_type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Available   bool   `json:"available"`
}

// Notification represents a notification sent to a user.
type Notification struct {
	NotificationID string                 `json:"notification_id"`
	Title          string                 `json:"title"`
	Body           string                 `json:"body"`
	Category       string                 `json:"category"`
	Priority       string                 `json:"priority"`
	Data           map[string]interface{} `json:"data,omitempty"`
	ActionURL      string                 `json:"action_url,omitempty"`
	ImageURL       string                 `json:"image_url,omitempty"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ContractAmendment represents a proposed change to an existing contract.
type ContractAmendment struct {
	AmendmentID       string            `json:"amendment_id"`
	ContractID        string            `json:"contract_id"`
	Type              AmendmentType     `json:"type"`
	Capabilities      []CapabilityGrant `json:"capabilities,omitempty"`       // For add/remove capabilities
	NewOfferingID     string            `json:"new_offering_id,omitempty"`    // For upgrade/downgrade
	Reason            string            `json:"reason,omitempty"`
	ProposedBy        string            `json:"proposed_by"`                  // user_id or service_id
	ProposedAt        time.Time         `json:"proposed_at"`
	UserSignature     *Signature        `json:"user_signature,omitempty"`
	ServiceSignature  *Signature        `json:"service_signature,omitempty"`
	Status            AmendmentStatus   `json:"status"`
	AppliedAt         *time.Time        `json:"applied_at,omitempty"`
}

// AmendmentType indicates the type of contract amendment.
type AmendmentType string

const (
	AmendmentAddCapabilities    AmendmentType = "add_capabilities"
	AmendmentRemoveCapabilities AmendmentType = "remove_capabilities"
	AmendmentUpgrade            AmendmentType = "upgrade"
	AmendmentDowngrade          AmendmentType = "downgrade"
	AmendmentExtend             AmendmentType = "extend"
)

// AmendmentStatus indicates the status of an amendment.
type AmendmentStatus string

const (
	AmendmentStatusPending  AmendmentStatus = "pending"
	AmendmentStatusApproved AmendmentStatus = "approved"
	AmendmentStatusRejected AmendmentStatus = "rejected"
	AmendmentStatusApplied  AmendmentStatus = "applied"
	AmendmentStatusExpired  AmendmentStatus = "expired"
)

// Error codes for API responses.
const (
	ErrCodeInvalidRequest     = "invalid_request"
	ErrCodeUnauthorized       = "unauthorized"
	ErrCodeForbidden          = "forbidden"
	ErrCodeNotFound           = "not_found"
	ErrCodeConflict           = "conflict"
	ErrCodeTimeout            = "timeout"
	ErrCodeRateLimited        = "rate_limited"
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

// ============================================================================
// Call Types (Phase 3)
// ============================================================================

// CallRequest represents a request to initiate a call with a user.
type CallRequest struct {
	RequestID   string                 `json:"request_id"`
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"` // voice, video
	Purpose     string                 `json:"purpose,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	ICEServers  []ICEServer            `json:"ice_servers,omitempty"`
	Offer       *RTCSessionDescription `json:"offer,omitempty"`
	ExpiresAt   time.Time              `json:"expires_at"`
}

// ICEServer represents a STUN/TURN server for WebRTC.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// RTCSessionDescription represents an SDP offer or answer.
type RTCSessionDescription struct {
	Type string `json:"type"` // offer, answer
	SDP  string `json:"sdp"`
}

// ============================================================================
// Payment Types (Phase 3)
// ============================================================================

// PaymentRequest represents a payment request to a user.
type PaymentRequest struct {
	RequestID      string           `json:"request_id"`
	UserID         string           `json:"user_id"`
	Amount         Money            `json:"amount"`
	Description    string           `json:"description"`
	MerchantInfo   MerchantInfo     `json:"merchant_info"`
	Items          []PaymentItem    `json:"items,omitempty"`
	AllowedMethods []string         `json:"allowed_methods,omitempty"`
	RecurringInfo  *RecurringInfo   `json:"recurring_info,omitempty"`
	ExpiresAt      time.Time        `json:"expires_at"`
}

// Money represents a monetary amount.
type Money struct {
	Amount   int64  `json:"amount"`   // Amount in smallest currency unit
	Currency string `json:"currency"` // ISO 4217 currency code
}

// MerchantInfo contains merchant identification.
type MerchantInfo struct {
	MerchantID   string `json:"merchant_id"`
	MerchantName string `json:"merchant_name"`
	MerchantURL  string `json:"merchant_url,omitempty"`
	MerchantLogo string `json:"merchant_logo,omitempty"`
	Category     string `json:"category,omitempty"`
}

// PaymentItem represents a line item in a payment.
type PaymentItem struct {
	ItemID      string `json:"item_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Quantity    int    `json:"quantity"`
	UnitPrice   Money  `json:"unit_price"`
	TotalPrice  Money  `json:"total_price"`
	ImageURL    string `json:"image_url,omitempty"`
}

// RecurringInfo describes recurring payment terms.
type RecurringInfo struct {
	Interval      string     `json:"interval"` // day, week, month, year
	IntervalCount int        `json:"interval_count"`
	StartDate     time.Time  `json:"start_date"`
	EndDate       *time.Time `json:"end_date,omitempty"`
	TrialDays     int        `json:"trial_days,omitempty"`
}

// ============================================================================
// Secrets Types (Phase 3)
// ============================================================================

// SecretRequest represents a request to store/retrieve/manage secrets.
type SecretRequest struct {
	RequestID   string                 `json:"request_id"`
	UserID      string                 `json:"user_id"`
	Operation   string                 `json:"operation"` // store, retrieve, delete, list, update
	SecretType  string                 `json:"secret_type,omitempty"` // minor, critical, user_owned
	SecretID    string                 `json:"secret_id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Data        []byte                 `json:"data,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ExpiresAt   time.Time              `json:"expires_at"`
}
