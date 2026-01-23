package contract

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/crypto"
	"github.com/vettid/vettid-service-vault/vault/internal/identity"
	"github.com/vettid/vettid-service-vault/vault/internal/keystore"
	"github.com/vettid/vettid-service-vault/vault/internal/nats"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
	"golang.org/x/crypto/curve25519"
)

// Negotiator handles contract negotiation between users and services.
type Negotiator struct {
	store            Store
	keystore         keystore.KeyStore
	serviceIdentity  *identity.ServiceIdentity
	signingKeyID     string
	encryptionKeyID  string
	credentialIssuer *nats.CredentialIssuer
	offerings        []ContractOffering
}

// NegotiatorConfig holds configuration for the negotiator.
type NegotiatorConfig struct {
	Store            Store
	Keystore         keystore.KeyStore
	ServiceIdentity  *identity.ServiceIdentity
	SigningKeyID     string
	EncryptionKeyID  string
	CredentialIssuer *nats.CredentialIssuer
	Offerings        []ContractOffering
}

// NewNegotiator creates a new contract negotiator.
func NewNegotiator(cfg NegotiatorConfig) (*Negotiator, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("contract store is required")
	}
	if cfg.Keystore == nil {
		return nil, fmt.Errorf("keystore is required")
	}
	if cfg.ServiceIdentity == nil {
		return nil, fmt.Errorf("service identity is required")
	}

	return &Negotiator{
		store:            cfg.Store,
		keystore:         cfg.Keystore,
		serviceIdentity:  cfg.ServiceIdentity,
		signingKeyID:     cfg.SigningKeyID,
		encryptionKeyID:  cfg.EncryptionKeyID,
		credentialIssuer: cfg.CredentialIssuer,
		offerings:        cfg.Offerings,
	}, nil
}

// GetOffer returns the service's current contract offer.
func (n *Negotiator) GetOffer() *ServiceContractOffer {
	return &ServiceContractOffer{
		ServiceID:            n.serviceIdentity.ServiceID,
		ServiceName:          n.serviceIdentity.ServiceName,
		ServicePublicKey:     base64.StdEncoding.EncodeToString(n.serviceIdentity.SigningKey),
		ServiceEncryptionKey: base64.StdEncoding.EncodeToString(n.serviceIdentity.EncryptionKey[:]),
		ServiceNATSEndpoint:  n.serviceIdentity.NATSEndpoint,
		Domain:               n.serviceIdentity.Domain,
		DomainVerified:       n.serviceIdentity.DomainVerified,
		Offerings:            n.offerings,
		OfferVersion:         "1.0",
	}
}

// HandleOfferRequest generates an offer for a requesting user.
func (n *Negotiator) HandleOfferRequest(ctx context.Context, userID string) (*ServiceContractOffer, error) {
	// For now, return the standard offer
	// In the future, this could customize offers based on the user
	return n.GetOffer(), nil
}

// HandleContractSubmission processes a user-signed contract submission.
func (n *Negotiator) HandleContractSubmission(ctx context.Context, contract *SignedConnectionContract) error {
	// Verify the contract data
	if err := n.validateContractSubmission(contract); err != nil {
		return fmt.Errorf("validating contract: %w", err)
	}

	// Verify the user's signature
	if err := VerifyUserSignature(contract); err != nil {
		return fmt.Errorf("verifying user signature: %w", err)
	}

	// Set contract status to pending
	contract.Status = types.ContractStatusPending
	contract.ServiceID = n.serviceIdentity.ServiceID

	// Generate connection-specific encryption key for this user
	// This provides forward secrecy - each connection has unique keys
	connectionPrivKey, connectionPubKey, err := generateConnectionKeys()
	if err != nil {
		return fmt.Errorf("generating connection keys: %w", err)
	}

	contract.ServiceConnectionKey = base64.StdEncoding.EncodeToString(connectionPubKey[:])

	// Store the connection private key (associated with this contract)
	if err := n.keystore.StoreEncryptionKey(contract.ContractID, connectionPrivKey); err != nil {
		return fmt.Errorf("storing connection key: %w", err)
	}

	// Save the contract
	if err := n.store.SaveContract(ctx, contract); err != nil {
		return fmt.Errorf("saving contract: %w", err)
	}

	return nil
}

// AcceptContract counter-signs and activates a pending contract.
func (n *Negotiator) AcceptContract(ctx context.Context, contractID string) (*SignedConnectionContract, error) {
	// Get the pending contract
	contract, err := n.store.GetContract(ctx, contractID)
	if err != nil {
		return nil, fmt.Errorf("getting contract: %w", err)
	}

	if contract.Status != types.ContractStatusPending {
		return nil, fmt.Errorf("contract is not pending: %s", contract.Status)
	}

	// Get the signing key
	signingKey, err := n.keystore.GetSigningKey(n.signingKeyID)
	if err != nil {
		return nil, fmt.Errorf("getting signing key: %w", err)
	}

	// Sign the contract
	signature, err := crypto.SignJSON(signingKey, contract.ToUnsigned())
	if err != nil {
		return nil, fmt.Errorf("signing contract: %w", err)
	}
	contract.ServiceSignature = signature

	// Issue NATS credentials for the user
	if n.credentialIssuer != nil {
		userConnKey, err := base64.StdEncoding.DecodeString(contract.UserConnectionKey)
		if err != nil {
			return nil, fmt.Errorf("decoding user connection key: %w", err)
		}
		var connKeyArr [32]byte
		copy(connKeyArr[:], userConnKey)

		creds, err := n.credentialIssuer.IssueUserCredentials(contract.UserID, connKeyArr)
		if err != nil {
			return nil, fmt.Errorf("issuing credentials: %w", err)
		}
		contract.ServiceNATSCreds = creds
	}

	// Activate the contract
	now := time.Now().UTC()
	contract.Status = types.ContractStatusActive
	contract.ActivatedAt = &now

	// Update the contract
	if err := n.store.UpdateContract(ctx, contract); err != nil {
		return nil, fmt.Errorf("updating contract: %w", err)
	}

	return contract, nil
}

// RejectContract rejects a pending contract.
func (n *Negotiator) RejectContract(ctx context.Context, contractID, reason string) error {
	contract, err := n.store.GetContract(ctx, contractID)
	if err != nil {
		return fmt.Errorf("getting contract: %w", err)
	}

	if contract.Status != types.ContractStatusPending {
		return fmt.Errorf("contract is not pending: %s", contract.Status)
	}

	now := time.Now().UTC()
	contract.Status = types.ContractStatusCancelled
	contract.CancelledAt = &now
	contract.CancelledBy = n.serviceIdentity.ServiceID
	contract.CancellationReason = reason

	// Clean up the connection key
	n.keystore.DeleteKey(contractID)

	return n.store.UpdateContract(ctx, contract)
}

// CancelContract cancels an active contract.
func (n *Negotiator) CancelContract(ctx context.Context, contractID, reason string) error {
	contract, err := n.store.GetContract(ctx, contractID)
	if err != nil {
		return fmt.Errorf("getting contract: %w", err)
	}

	if contract.Status != types.ContractStatusActive && contract.Status != types.ContractStatusPaused {
		return fmt.Errorf("contract cannot be cancelled: %s", contract.Status)
	}

	now := time.Now().UTC()
	contract.Status = types.ContractStatusCancelled
	contract.CancelledAt = &now
	contract.CancelledBy = n.serviceIdentity.ServiceID
	contract.CancellationReason = reason

	// Revoke NATS credentials
	if n.credentialIssuer != nil {
		n.credentialIssuer.RevokeUserCredentials(contract.UserID)
	}

	// Clean up the connection key
	n.keystore.DeleteKey(contractID)

	return n.store.UpdateContract(ctx, contract)
}

// validateContractSubmission validates a submitted contract.
func (n *Negotiator) validateContractSubmission(contract *SignedConnectionContract) error {
	if contract.ContractID == "" {
		return fmt.Errorf("contract_id is required")
	}
	if contract.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if contract.OfferingID == "" {
		return fmt.Errorf("offering_id is required")
	}
	if contract.UserConnectionKey == "" {
		return fmt.Errorf("user_connection_key is required")
	}
	if contract.UserSignature == nil {
		return fmt.Errorf("user_signature is required")
	}

	// Verify the offering exists
	found := false
	for _, o := range n.offerings {
		if o.OfferingID == contract.OfferingID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown offering_id: %s", contract.OfferingID)
	}

	return nil
}

// CreateInvite generates a contract invite.
func (n *Negotiator) CreateInvite(ctx context.Context, offeringID string, maxUses int, ttl time.Duration) (*ContractInvite, error) {
	// Generate invite ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("generating invite ID: %w", err)
	}
	inviteID := base64.RawURLEncoding.EncodeToString(idBytes)

	invite := &ContractInvite{
		InviteID:   inviteID,
		ServiceID:  n.serviceIdentity.ServiceID,
		OfferingID: offeringID,
		ExpiresAt:  time.Now().Add(ttl),
		MaxUses:    maxUses,
		Uses:       0,
		CreatedAt:  time.Now().UTC(),
	}

	if err := n.store.SaveInvite(ctx, invite); err != nil {
		return nil, fmt.Errorf("saving invite: %w", err)
	}

	return invite, nil
}

// generateConnectionKeys generates a fresh X25519 key pair for a connection.
func generateConnectionKeys() ([32]byte, [32]byte, error) {
	var private, public [32]byte

	if _, err := rand.Read(private[:]); err != nil {
		return private, public, err
	}

	// Clamp for X25519
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64

	curve25519.ScalarBaseMult(&public, &private)

	return private, public, nil
}

// GenerateContractID generates a unique contract ID.
func GenerateContractID() string {
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	return base64.RawURLEncoding.EncodeToString(idBytes)
}

// VerifyUserSignature verifies the user's signature on a contract.
// The user's public key is derived from the user_id.
func VerifyUserSignature(contract *SignedConnectionContract) error {
	if contract.UserSignature == nil {
		return fmt.Errorf("user signature is missing")
	}

	// Get the public key from the signature
	pubKey, err := crypto.GetPublicKeyFromSignature(contract.UserSignature)
	if err != nil {
		return fmt.Errorf("extracting public key: %w", err)
	}

	// Verify user_id matches the public key
	derivedUserID := identity.DeriveServiceID(ed25519.PublicKey(pubKey))
	if derivedUserID != contract.UserID {
		return fmt.Errorf("user_id does not match signing key")
	}

	// Verify the signature over the unsigned contract data
	return crypto.VerifyJSONSignature(contract.UserSignature, contract.ToUnsigned())
}

// ProposeAmendment creates a new amendment proposal for a contract.
// The amendment must be signed by both parties before it can be applied.
func (n *Negotiator) ProposeAmendment(ctx context.Context, contractID string, amendmentType types.AmendmentType, opts ...AmendmentOption) (*ContractAmendment, error) {
	// Get the contract
	contract, err := n.store.GetContract(ctx, contractID)
	if err != nil {
		return nil, fmt.Errorf("getting contract: %w", err)
	}

	if contract.Status != types.ContractStatusActive {
		return nil, fmt.Errorf("contract is not active: %s", contract.Status)
	}

	// Generate amendment ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("generating amendment ID: %w", err)
	}
	amendmentID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	amendment := &ContractAmendment{
		AmendmentID: amendmentID,
		ContractID:  contractID,
		Type:        amendmentType,
		ProposedBy:  n.serviceIdentity.ServiceID,
		ProposedAt:  now,
		ExpiresAt:   now.Add(24 * time.Hour), // Default 24h expiration
		Status:      types.AmendmentStatusPending,
	}

	// Apply options
	for _, opt := range opts {
		opt(amendment)
	}

	// Sign the amendment
	signingKey, err := n.keystore.GetSigningKey(n.signingKeyID)
	if err != nil {
		return nil, fmt.Errorf("getting signing key: %w", err)
	}

	signature, err := crypto.SignJSON(signingKey, amendment.ToUnsigned())
	if err != nil {
		return nil, fmt.Errorf("signing amendment: %w", err)
	}
	amendment.ServiceSignature = signature

	// Save the amendment
	if err := n.store.SaveAmendment(ctx, amendment); err != nil {
		return nil, fmt.Errorf("saving amendment: %w", err)
	}

	return amendment, nil
}

// HandleAmendmentResponse processes a user's response to an amendment proposal.
func (n *Negotiator) HandleAmendmentResponse(ctx context.Context, amendmentID string, userSignature *crypto.Signature, approved bool) error {
	// Get the amendment
	amendment, err := n.store.GetAmendment(ctx, amendmentID)
	if err != nil {
		return fmt.Errorf("getting amendment: %w", err)
	}

	if amendment.Status != types.AmendmentStatusPending {
		return fmt.Errorf("amendment is not pending: %s", amendment.Status)
	}

	if time.Now().After(amendment.ExpiresAt) {
		amendment.Status = types.AmendmentStatusExpired
		n.store.UpdateAmendment(ctx, amendment)
		return fmt.Errorf("amendment has expired")
	}

	// Get the contract to verify user
	contract, err := n.store.GetContract(ctx, amendment.ContractID)
	if err != nil {
		return fmt.Errorf("getting contract: %w", err)
	}

	if !approved {
		// User rejected the amendment
		now := time.Now().UTC()
		amendment.Status = types.AmendmentStatusRejected
		amendment.RejectedAt = &now
		amendment.RejectedBy = contract.UserID
		return n.store.UpdateAmendment(ctx, amendment)
	}

	// Verify the user's signature
	if err := verifyAmendmentSignature(amendment, userSignature, contract.UserID); err != nil {
		return fmt.Errorf("verifying user signature: %w", err)
	}

	amendment.UserSignature = userSignature
	amendment.Status = types.AmendmentStatusApproved

	// Apply the amendment
	return n.ApplyAmendment(ctx, amendment, contract)
}

// ApplyAmendment applies an approved amendment to a contract.
func (n *Negotiator) ApplyAmendment(ctx context.Context, amendment *ContractAmendment, contract *SignedConnectionContract) error {
	// Both signatures required
	if amendment.UserSignature == nil || amendment.ServiceSignature == nil {
		return fmt.Errorf("both signatures required to apply amendment")
	}

	// Apply the amendment based on type
	switch amendment.Type {
	case types.AmendmentAddCapabilities:
		contract.OfferingSnapshot.Capabilities = append(
			contract.OfferingSnapshot.Capabilities,
			amendment.AddCapabilities...,
		)

	case types.AmendmentRemoveCapabilities:
		filtered := make([]types.CapabilityGrant, 0, len(contract.OfferingSnapshot.Capabilities))
		for _, cap := range contract.OfferingSnapshot.Capabilities {
			remove := false
			for _, removeCap := range amendment.RemoveCapabilities {
				if cap.Capability == removeCap {
					remove = true
					break
				}
			}
			if !remove {
				filtered = append(filtered, cap)
			}
		}
		contract.OfferingSnapshot.Capabilities = filtered

	case types.AmendmentExtend:
		if amendment.NewExpiration != nil {
			contract.ExpiresAt = amendment.NewExpiration
		}

	case types.AmendmentUpgrade, types.AmendmentDowngrade:
		// Find the new offering
		var newOffering *ContractOffering
		for _, o := range n.offerings {
			if o.OfferingID == amendment.NewOfferingID {
				newOffering = &o
				break
			}
		}
		if newOffering == nil {
			return fmt.Errorf("offering not found: %s", amendment.NewOfferingID)
		}
		contract.OfferingID = newOffering.OfferingID
		contract.OfferingSnapshot = *newOffering
	}

	// Update the contract
	if err := n.store.UpdateContract(ctx, contract); err != nil {
		return fmt.Errorf("updating contract: %w", err)
	}

	// Mark amendment as applied
	now := time.Now().UTC()
	amendment.Status = types.AmendmentStatusApplied
	amendment.AppliedAt = &now

	return n.store.UpdateAmendment(ctx, amendment)
}

// GetAmendment retrieves an amendment by ID.
func (n *Negotiator) GetAmendment(ctx context.Context, amendmentID string) (*ContractAmendment, error) {
	return n.store.GetAmendment(ctx, amendmentID)
}

// ListAmendments lists amendments for a contract.
func (n *Negotiator) ListAmendments(ctx context.Context, contractID string, status *types.AmendmentStatus) ([]*ContractAmendment, error) {
	return n.store.ListAmendments(ctx, AmendmentFilter{
		ContractID: contractID,
		Status:     status,
	})
}

// verifyAmendmentSignature verifies a signature on an amendment.
func verifyAmendmentSignature(amendment *ContractAmendment, signature *crypto.Signature, expectedUserID string) error {
	if signature == nil {
		return fmt.Errorf("signature is missing")
	}

	// Get the public key from the signature
	pubKey, err := crypto.GetPublicKeyFromSignature(signature)
	if err != nil {
		return fmt.Errorf("extracting public key: %w", err)
	}

	// Verify user_id matches the public key
	derivedUserID := identity.DeriveServiceID(ed25519.PublicKey(pubKey))
	if derivedUserID != expectedUserID {
		return fmt.Errorf("user_id does not match signing key")
	}

	// Verify the signature over the unsigned amendment data
	return crypto.VerifyJSONSignature(signature, amendment.ToUnsigned())
}

// AmendmentOption configures an amendment.
type AmendmentOption func(*ContractAmendment)

// WithAddCapabilities sets capabilities to add.
func WithAddCapabilities(caps []types.CapabilityGrant) AmendmentOption {
	return func(a *ContractAmendment) {
		a.AddCapabilities = caps
	}
}

// WithRemoveCapabilities sets capabilities to remove.
func WithRemoveCapabilities(caps []types.CapabilityType) AmendmentOption {
	return func(a *ContractAmendment) {
		a.RemoveCapabilities = caps
	}
}

// WithNewOffering sets a new offering for upgrade/downgrade.
func WithNewOffering(offeringID string) AmendmentOption {
	return func(a *ContractAmendment) {
		a.NewOfferingID = offeringID
	}
}

// WithNewExpiration sets a new expiration for extension.
func WithNewExpiration(exp time.Time) AmendmentOption {
	return func(a *ContractAmendment) {
		a.NewExpiration = &exp
	}
}

// WithAmendmentReason sets the reason for the amendment.
func WithAmendmentReason(reason string) AmendmentOption {
	return func(a *ContractAmendment) {
		a.Reason = reason
	}
}

// WithAmendmentExpiration sets when the amendment proposal expires.
func WithAmendmentExpiration(exp time.Time) AmendmentOption {
	return func(a *ContractAmendment) {
		a.ExpiresAt = exp
	}
}
