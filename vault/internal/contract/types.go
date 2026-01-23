// Package contract provides contract management for VettID Service Vault.
//
// Contracts are the foundation of user-service relationships in VettID.
// They establish what capabilities a service has, what data it can access,
// and under what terms the relationship operates.
//
// The contract flow:
// 1. Service publishes contract offerings
// 2. User requests an offer
// 3. User reviews and signs the offer
// 4. Service counter-signs, creating an active contract
// 5. Service issues NATS credentials to the user
package contract

import (
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/crypto"
	"github.com/vettid/vettid-service-vault/vault/internal/nats"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// ServiceContractOffer describes what a service offers to users.
// This is the public-facing description of the service that users see
// before establishing a connection.
type ServiceContractOffer struct {
	// ServiceID is the service's unique identifier
	ServiceID string `json:"service_id"`

	// ServiceName is a human-readable name for the service
	ServiceName string `json:"service_name"`

	// ServicePublicKey is the Ed25519 public key (base58)
	ServicePublicKey string `json:"service_public_key"`

	// ServiceEncryptionKey is the X25519 public key (base58)
	ServiceEncryptionKey string `json:"service_encryption_key"`

	// ServiceNATSEndpoint is where users connect to communicate
	ServiceNATSEndpoint string `json:"service_nats_endpoint"`

	// Domain is the verified domain for the service
	Domain string `json:"domain,omitempty"`

	// DomainVerified indicates if the domain has been verified
	DomainVerified bool `json:"domain_verified"`

	// Attestations are verifications from the VettID registry
	Attestations []types.RegistryAttestation `json:"attestations,omitempty"`

	// Offerings are the available contract options
	Offerings []ContractOffering `json:"offerings"`

	// OfferVersion is the version of this offer (for updates)
	OfferVersion string `json:"offer_version"`

	// ValidUntil is when this offer expires (optional)
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// ContractOffering describes a specific offering from a service.
// A service may have multiple offerings (e.g., free tier, premium tier).
type ContractOffering struct {
	// OfferingID uniquely identifies this offering
	OfferingID string `json:"offering_id"`

	// Name is a human-readable name
	Name string `json:"name"`

	// Description explains what this offering provides
	Description string `json:"description"`

	// Capabilities lists what the service can do under this offering
	Capabilities []types.CapabilityGrant `json:"capabilities"`

	// RequiredData lists data the service needs from the user
	RequiredData []types.DataRequirement `json:"required_data,omitempty"`

	// Pricing describes the cost (if any)
	Pricing *types.Pricing `json:"pricing,omitempty"`

	// TermsURL links to the full terms of service
	TermsURL string `json:"terms_url,omitempty"`

	// TermsHash is the hash of the terms document
	TermsHash string `json:"terms_hash,omitempty"`
}

// SignedConnectionContract is the binding agreement between user and service.
// Both parties sign this contract to establish the connection.
type SignedConnectionContract struct {
	// ContractID uniquely identifies this contract
	ContractID string `json:"contract_id"`

	// UserID is the user's identifier (derived from their public key)
	UserID string `json:"user_id"`

	// ServiceID is the service's identifier
	ServiceID string `json:"service_id"`

	// OfferingID identifies which offering this contract is for
	OfferingID string `json:"offering_id"`

	// OfferingSnapshot captures the offering at contract creation time
	OfferingSnapshot ContractOffering `json:"offering_snapshot"`

	// UserConnectionKey is the user's X25519 public key for this connection
	UserConnectionKey string `json:"user_connection_key"`

	// ServiceConnectionKey is the service's X25519 public key for this connection
	ServiceConnectionKey string `json:"service_connection_key"`

	// ServiceNATSCreds are the NATS credentials for the user (encrypted)
	ServiceNATSCreds *nats.UserCredentials `json:"service_nats_creds,omitempty"`

	// UserSignature is the user's signature on the contract
	UserSignature *crypto.Signature `json:"user_signature"`

	// ServiceSignature is the service's counter-signature
	ServiceSignature *crypto.Signature `json:"service_signature,omitempty"`

	// Status is the current contract status
	Status types.ContractStatus `json:"status"`

	// CreatedAt is when the contract was created
	CreatedAt time.Time `json:"created_at"`

	// ActivatedAt is when the contract became active
	ActivatedAt *time.Time `json:"activated_at,omitempty"`

	// ExpiresAt is when the contract expires (if applicable)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// CancelledAt is when the contract was cancelled
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	// CancelledBy identifies who cancelled (user_id or service_id)
	CancelledBy string `json:"cancelled_by,omitempty"`

	// CancellationReason explains why the contract was cancelled
	CancellationReason string `json:"cancellation_reason,omitempty"`
}

// UnsignedContract is the contract data that gets signed.
// This excludes signatures and post-creation fields.
type UnsignedContract struct {
	ContractID           string           `json:"contract_id"`
	UserID               string           `json:"user_id"`
	ServiceID            string           `json:"service_id"`
	OfferingID           string           `json:"offering_id"`
	OfferingSnapshot     ContractOffering `json:"offering_snapshot"`
	UserConnectionKey    string           `json:"user_connection_key"`
	ServiceConnectionKey string           `json:"service_connection_key"`
	CreatedAt            time.Time        `json:"created_at"`
}

// ToUnsigned extracts the signable portion of a contract.
func (c *SignedConnectionContract) ToUnsigned() UnsignedContract {
	return UnsignedContract{
		ContractID:           c.ContractID,
		UserID:               c.UserID,
		ServiceID:            c.ServiceID,
		OfferingID:           c.OfferingID,
		OfferingSnapshot:     c.OfferingSnapshot,
		UserConnectionKey:    c.UserConnectionKey,
		ServiceConnectionKey: c.ServiceConnectionKey,
		CreatedAt:            c.CreatedAt,
	}
}

// ContractInvite is a short-lived invitation for a user to connect.
type ContractInvite struct {
	// InviteID is a unique identifier for this invite
	InviteID string `json:"invite_id"`

	// ServiceID is the service offering the invite
	ServiceID string `json:"service_id"`

	// OfferingID identifies which offering this invite is for
	OfferingID string `json:"offering_id"`

	// ExpiresAt is when this invite expires
	ExpiresAt time.Time `json:"expires_at"`

	// MaxUses is the maximum number of times this invite can be used
	MaxUses int `json:"max_uses"`

	// Uses is the current number of times this invite has been used
	Uses int `json:"uses"`

	// Metadata is optional additional data
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is when the invite was created
	CreatedAt time.Time `json:"created_at"`
}

// ContractFilter specifies criteria for querying contracts.
type ContractFilter struct {
	// ServiceID filters by service
	ServiceID string

	// UserID filters by user
	UserID string

	// Status filters by contract status
	Status *types.ContractStatus

	// OfferingID filters by offering
	OfferingID string

	// CreatedAfter filters contracts created after this time
	CreatedAfter *time.Time

	// CreatedBefore filters contracts created before this time
	CreatedBefore *time.Time

	// Limit is the maximum number of results
	Limit int

	// Cursor is for pagination
	Cursor string
}

// ContractListResult is the result of listing contracts.
type ContractListResult struct {
	// Contracts are the matching contracts
	Contracts []*SignedConnectionContract `json:"contracts"`

	// NextCursor is the cursor for the next page (empty if no more)
	NextCursor string `json:"next_cursor,omitempty"`

	// TotalCount is the total number of matching contracts (if available)
	TotalCount int `json:"total_count,omitempty"`
}

// ContractAmendment represents a proposed change to an existing contract.
// Amendments allow modifying contracts without full re-negotiation.
type ContractAmendment struct {
	// AmendmentID uniquely identifies this amendment
	AmendmentID string `json:"amendment_id"`

	// ContractID is the contract being amended
	ContractID string `json:"contract_id"`

	// Type indicates what kind of change this is
	Type types.AmendmentType `json:"type"`

	// AddCapabilities are capabilities to add (for add_capabilities type)
	AddCapabilities []types.CapabilityGrant `json:"add_capabilities,omitempty"`

	// RemoveCapabilities are capabilities to remove (for remove_capabilities type)
	RemoveCapabilities []types.CapabilityType `json:"remove_capabilities,omitempty"`

	// NewOfferingID is the new offering (for upgrade/downgrade types)
	NewOfferingID string `json:"new_offering_id,omitempty"`

	// NewExpiration extends the contract (for extend type)
	NewExpiration *time.Time `json:"new_expiration,omitempty"`

	// Reason explains why the amendment is proposed
	Reason string `json:"reason,omitempty"`

	// ProposedBy identifies who proposed this (user_id or service_id)
	ProposedBy string `json:"proposed_by"`

	// ProposedAt is when the amendment was proposed
	ProposedAt time.Time `json:"proposed_at"`

	// ExpiresAt is when this amendment proposal expires
	ExpiresAt time.Time `json:"expires_at"`

	// UserSignature is the user's approval signature
	UserSignature *crypto.Signature `json:"user_signature,omitempty"`

	// ServiceSignature is the service's approval signature
	ServiceSignature *crypto.Signature `json:"service_signature,omitempty"`

	// Status is the current amendment status
	Status types.AmendmentStatus `json:"status"`

	// AppliedAt is when the amendment was applied
	AppliedAt *time.Time `json:"applied_at,omitempty"`

	// RejectedAt is when the amendment was rejected
	RejectedAt *time.Time `json:"rejected_at,omitempty"`

	// RejectedBy identifies who rejected
	RejectedBy string `json:"rejected_by,omitempty"`

	// RejectionReason explains why it was rejected
	RejectionReason string `json:"rejection_reason,omitempty"`
}

// UnsignedAmendment is the amendment data that gets signed.
type UnsignedAmendment struct {
	AmendmentID        string                  `json:"amendment_id"`
	ContractID         string                  `json:"contract_id"`
	Type               types.AmendmentType     `json:"type"`
	AddCapabilities    []types.CapabilityGrant `json:"add_capabilities,omitempty"`
	RemoveCapabilities []types.CapabilityType  `json:"remove_capabilities,omitempty"`
	NewOfferingID      string                  `json:"new_offering_id,omitempty"`
	NewExpiration      *time.Time              `json:"new_expiration,omitempty"`
	Reason             string                  `json:"reason,omitempty"`
	ProposedBy         string                  `json:"proposed_by"`
	ProposedAt         time.Time               `json:"proposed_at"`
}

// ToUnsigned extracts the signable portion of an amendment.
func (a *ContractAmendment) ToUnsigned() UnsignedAmendment {
	return UnsignedAmendment{
		AmendmentID:        a.AmendmentID,
		ContractID:         a.ContractID,
		Type:               a.Type,
		AddCapabilities:    a.AddCapabilities,
		RemoveCapabilities: a.RemoveCapabilities,
		NewOfferingID:      a.NewOfferingID,
		NewExpiration:      a.NewExpiration,
		Reason:             a.Reason,
		ProposedBy:         a.ProposedBy,
		ProposedAt:         a.ProposedAt,
	}
}

// AmendmentFilter specifies criteria for querying amendments.
type AmendmentFilter struct {
	ContractID string
	Status     *types.AmendmentStatus
	ProposedBy string
	Limit      int
}
