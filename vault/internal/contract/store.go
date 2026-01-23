package contract

import (
	"context"
	"errors"

	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// Common errors
var (
	ErrContractNotFound    = errors.New("contract not found")
	ErrContractExists      = errors.New("contract already exists")
	ErrInviteNotFound      = errors.New("invite not found")
	ErrInviteExpired       = errors.New("invite has expired")
	ErrInviteExhausted     = errors.New("invite has reached maximum uses")
	ErrInvalidContractData = errors.New("invalid contract data")
	ErrAmendmentNotFound   = errors.New("amendment not found")
	ErrAmendmentExists     = errors.New("amendment already exists")
)

// Store defines the interface for contract persistence.
// Implementations must ensure tenant isolation by including service_id
// in all queries and operations.
type Store interface {
	// SaveContract stores a new contract.
	// Returns ErrContractExists if a contract with the same ID exists.
	SaveContract(ctx context.Context, contract *SignedConnectionContract) error

	// GetContract retrieves a contract by ID.
	// Returns ErrContractNotFound if the contract doesn't exist.
	GetContract(ctx context.Context, contractID string) (*SignedConnectionContract, error)

	// GetContractByUser retrieves a user's active contract.
	// Returns ErrContractNotFound if no active contract exists.
	GetContractByUser(ctx context.Context, userID string) (*SignedConnectionContract, error)

	// UpdateContract updates an existing contract.
	// Only the following fields can be updated:
	// - Status
	// - ServiceSignature (for activation)
	// - ActivatedAt
	// - CancelledAt, CancelledBy, CancellationReason (for cancellation)
	UpdateContract(ctx context.Context, contract *SignedConnectionContract) error

	// UpdateContractStatus updates only the status of a contract.
	UpdateContractStatus(ctx context.Context, contractID string, status types.ContractStatus) error

	// ListContracts retrieves contracts matching the filter.
	ListContracts(ctx context.Context, filter ContractFilter) (*ContractListResult, error)

	// DeleteContract permanently removes a contract.
	// This should only be used for cancelled/expired contracts.
	DeleteContract(ctx context.Context, contractID string) error

	// SaveInvite stores a new contract invite.
	SaveInvite(ctx context.Context, invite *ContractInvite) error

	// GetInvite retrieves an invite by ID.
	GetInvite(ctx context.Context, inviteID string) (*ContractInvite, error)

	// UseInvite increments the use count of an invite.
	// Returns ErrInviteExhausted if max uses reached.
	UseInvite(ctx context.Context, inviteID string) error

	// DeleteInvite removes an invite.
	DeleteInvite(ctx context.Context, inviteID string) error

	// CleanupExpired removes expired contracts and invites.
	CleanupExpired(ctx context.Context) (int, error)

	// Amendment operations

	// SaveAmendment stores a new contract amendment.
	SaveAmendment(ctx context.Context, amendment *ContractAmendment) error

	// GetAmendment retrieves an amendment by ID.
	GetAmendment(ctx context.Context, amendmentID string) (*ContractAmendment, error)

	// UpdateAmendment updates an existing amendment.
	UpdateAmendment(ctx context.Context, amendment *ContractAmendment) error

	// ListAmendments retrieves amendments matching the filter.
	ListAmendments(ctx context.Context, filter AmendmentFilter) ([]*ContractAmendment, error)

	// DeleteAmendment removes an amendment.
	DeleteAmendment(ctx context.Context, amendmentID string) error
}
