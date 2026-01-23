package contract

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/crypto"
	"github.com/vettid/vettid-service-vault/vault/internal/identity"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// VerifyContract performs comprehensive verification of a contract.
func VerifyContract(contract *SignedConnectionContract) error {
	// Check required fields
	if contract.ContractID == "" {
		return fmt.Errorf("contract_id is required")
	}
	if contract.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if contract.ServiceID == "" {
		return fmt.Errorf("service_id is required")
	}

	// Verify user signature
	if err := VerifyUserSignature(contract); err != nil {
		return fmt.Errorf("invalid user signature: %w", err)
	}

	// If contract is active, verify service signature
	if contract.Status == types.ContractStatusActive {
		if err := VerifyServiceSignature(contract); err != nil {
			return fmt.Errorf("invalid service signature: %w", err)
		}
	}

	return nil
}

// VerifyServiceSignature verifies the service's counter-signature.
func VerifyServiceSignature(contract *SignedConnectionContract) error {
	if contract.ServiceSignature == nil {
		return fmt.Errorf("service signature is missing")
	}

	// Get the public key from the signature
	pubKey, err := crypto.GetPublicKeyFromSignature(contract.ServiceSignature)
	if err != nil {
		return fmt.Errorf("extracting public key: %w", err)
	}

	// Verify service_id matches the public key
	derivedServiceID := identity.DeriveServiceID(ed25519.PublicKey(pubKey))
	if derivedServiceID != contract.ServiceID {
		return fmt.Errorf("service_id does not match signing key")
	}

	// Verify the signature over the unsigned contract data
	return crypto.VerifyJSONSignature(contract.ServiceSignature, contract.ToUnsigned())
}

// DeriveUserID derives a user ID from their Ed25519 public key.
// This uses the same algorithm as service ID derivation.
func DeriveUserID(publicKey ed25519.PublicKey) string {
	return identity.DeriveServiceID(publicKey)
}

// VerifyUserID checks if a user_id matches the given public key.
func VerifyUserID(userID string, publicKey ed25519.PublicKey) bool {
	return identity.VerifyServiceID(userID, publicKey)
}

// VerifyCapability checks if a contract grants a specific capability.
func VerifyCapability(contract *SignedConnectionContract, capability types.CapabilityType) bool {
	if contract.Status != types.ContractStatusActive {
		return false
	}

	for _, grant := range contract.OfferingSnapshot.Capabilities {
		if grant.Capability == capability {
			// Check expiration if set
			if grant.ExpiresAt != nil && grant.ExpiresAt.Before(timeNow()) {
				return false
			}
			return true
		}
	}

	return false
}

// VerifyCapabilityWithScope checks if a contract grants a capability with a specific scope.
func VerifyCapabilityWithScope(contract *SignedConnectionContract, capability types.CapabilityType, scope string) bool {
	if contract.Status != types.ContractStatusActive {
		return false
	}

	for _, grant := range contract.OfferingSnapshot.Capabilities {
		if grant.Capability == capability {
			// Check scope match (empty scope in grant means all scopes)
			if grant.Scope != "" && grant.Scope != scope {
				continue
			}

			// Check expiration
			if grant.ExpiresAt != nil && grant.ExpiresAt.Before(timeNow()) {
				continue
			}

			return true
		}
	}

	return false
}

// VerifyContractActive checks if a contract is in active state.
func VerifyContractActive(contract *SignedConnectionContract) error {
	switch contract.Status {
	case types.ContractStatusActive:
		// Check expiration
		if contract.ExpiresAt != nil && contract.ExpiresAt.Before(timeNow()) {
			return fmt.Errorf("contract has expired")
		}
		return nil

	case types.ContractStatusPending:
		return fmt.Errorf("contract is pending activation")

	case types.ContractStatusPaused:
		return fmt.Errorf("contract is paused")

	case types.ContractStatusCancelled:
		return fmt.Errorf("contract has been cancelled")

	case types.ContractStatusExpired:
		return fmt.Errorf("contract has expired")

	default:
		return fmt.Errorf("unknown contract status: %s", contract.Status)
	}
}

// ContractSummary provides a summary view of a contract.
type ContractSummary struct {
	ContractID   string               `json:"contract_id"`
	UserID       string               `json:"user_id"`
	ServiceID    string               `json:"service_id"`
	OfferingName string               `json:"offering_name"`
	Status       types.ContractStatus `json:"status"`
	Capabilities []string             `json:"capabilities"`
	CreatedAt    string               `json:"created_at"`
	ActivatedAt  string               `json:"activated_at,omitempty"`
}

// Summarize creates a summary of a contract.
func Summarize(contract *SignedConnectionContract) ContractSummary {
	caps := make([]string, 0, len(contract.OfferingSnapshot.Capabilities))
	for _, c := range contract.OfferingSnapshot.Capabilities {
		caps = append(caps, string(c.Capability))
	}

	summary := ContractSummary{
		ContractID:   contract.ContractID,
		UserID:       contract.UserID,
		ServiceID:    contract.ServiceID,
		OfferingName: contract.OfferingSnapshot.Name,
		Status:       contract.Status,
		Capabilities: caps,
		CreatedAt:    contract.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if contract.ActivatedAt != nil {
		summary.ActivatedAt = contract.ActivatedAt.Format("2006-01-02T15:04:05Z")
	}

	return summary
}

// timeNow is a function that returns the current time.
// It can be replaced in tests.
var timeNow = func() time.Time {
	return time.Now().UTC()
}
