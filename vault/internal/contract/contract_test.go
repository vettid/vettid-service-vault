package contract

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vettid/vettid-service-vault/vault/internal/crypto"
	"github.com/vettid/vettid-service-vault/vault/internal/identity"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

func TestSignedConnectionContractToUnsigned(t *testing.T) {
	contract := &SignedConnectionContract{
		ContractID:           "test-contract-123",
		UserID:               "user-abc",
		ServiceID:            "service-xyz",
		OfferingID:           "offering-1",
		UserConnectionKey:    "user-key",
		ServiceConnectionKey: "service-key",
		CreatedAt:            time.Now().UTC(),
		Status:               types.ContractStatusActive,
	}

	unsigned := contract.ToUnsigned()

	assert.Equal(t, contract.ContractID, unsigned.ContractID)
	assert.Equal(t, contract.UserID, unsigned.UserID)
	assert.Equal(t, contract.ServiceID, unsigned.ServiceID)
	assert.Equal(t, contract.OfferingID, unsigned.OfferingID)
	assert.Equal(t, contract.UserConnectionKey, unsigned.UserConnectionKey)
	assert.Equal(t, contract.ServiceConnectionKey, unsigned.ServiceConnectionKey)
	assert.Equal(t, contract.CreatedAt, unsigned.CreatedAt)
}

func TestVerifyCapability(t *testing.T) {
	contract := &SignedConnectionContract{
		Status: types.ContractStatusActive,
		OfferingSnapshot: ContractOffering{
			Capabilities: []types.CapabilityGrant{
				{Capability: types.CapabilityAuthenticate},
				{Capability: types.CapabilityAuthorize},
			},
		},
	}

	assert.True(t, VerifyCapability(contract, types.CapabilityAuthenticate))
	assert.True(t, VerifyCapability(contract, types.CapabilityAuthorize))
	assert.False(t, VerifyCapability(contract, types.CapabilitySign))
}

func TestVerifyCapabilityInactiveContract(t *testing.T) {
	contract := &SignedConnectionContract{
		Status: types.ContractStatusPending,
		OfferingSnapshot: ContractOffering{
			Capabilities: []types.CapabilityGrant{
				{Capability: types.CapabilityAuthenticate},
			},
		},
	}

	assert.False(t, VerifyCapability(contract, types.CapabilityAuthenticate))
}

func TestVerifyCapabilityWithScope(t *testing.T) {
	contract := &SignedConnectionContract{
		Status: types.ContractStatusActive,
		OfferingSnapshot: ContractOffering{
			Capabilities: []types.CapabilityGrant{
				{Capability: types.CapabilityReadData, Scope: "profile"},
				{Capability: types.CapabilityReadData, Scope: "email"},
			},
		},
	}

	assert.True(t, VerifyCapabilityWithScope(contract, types.CapabilityReadData, "profile"))
	assert.True(t, VerifyCapabilityWithScope(contract, types.CapabilityReadData, "email"))
	assert.False(t, VerifyCapabilityWithScope(contract, types.CapabilityReadData, "phone"))
}

func TestVerifyContractActive(t *testing.T) {
	tests := []struct {
		name    string
		status  types.ContractStatus
		wantErr bool
	}{
		{"active", types.ContractStatusActive, false},
		{"pending", types.ContractStatusPending, true},
		{"paused", types.ContractStatusPaused, true},
		{"cancelled", types.ContractStatusCancelled, true},
		{"expired", types.ContractStatusExpired, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contract := &SignedConnectionContract{Status: tc.status}
			err := VerifyContractActive(contract)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	now := time.Now().UTC()
	activated := now.Add(time.Hour)

	contract := &SignedConnectionContract{
		ContractID: "contract-123",
		UserID:     "user-456",
		ServiceID:  "service-789",
		Status:     types.ContractStatusActive,
		OfferingSnapshot: ContractOffering{
			Name: "Premium Plan",
			Capabilities: []types.CapabilityGrant{
				{Capability: types.CapabilityAuthenticate},
				{Capability: types.CapabilityAuthorize},
			},
		},
		CreatedAt:   now,
		ActivatedAt: &activated,
	}

	summary := Summarize(contract)

	assert.Equal(t, "contract-123", summary.ContractID)
	assert.Equal(t, "user-456", summary.UserID)
	assert.Equal(t, "service-789", summary.ServiceID)
	assert.Equal(t, "Premium Plan", summary.OfferingName)
	assert.Equal(t, types.ContractStatusActive, summary.Status)
	assert.Len(t, summary.Capabilities, 2)
	assert.Contains(t, summary.Capabilities, "authenticate")
	assert.Contains(t, summary.Capabilities, "authorize")
	assert.NotEmpty(t, summary.CreatedAt)
	assert.NotEmpty(t, summary.ActivatedAt)
}

func TestDeriveUserID(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	userID := DeriveUserID(pub)
	assert.NotEmpty(t, userID)
	assert.True(t, VerifyUserID(userID, pub))
}

func TestVerifyUserSignature(t *testing.T) {
	// Generate user keys
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	userID := identity.DeriveServiceID(pub)

	// Create contract
	contract := &SignedConnectionContract{
		ContractID:           GenerateContractID(),
		UserID:               userID,
		ServiceID:            "test-service",
		OfferingID:           "offering-1",
		UserConnectionKey:    "user-conn-key",
		ServiceConnectionKey: "service-conn-key",
		CreatedAt:            time.Now().UTC(),
	}

	// Sign the contract
	sig, err := crypto.SignJSON(priv, contract.ToUnsigned())
	require.NoError(t, err)
	contract.UserSignature = sig

	// Verify
	err = VerifyUserSignature(contract)
	assert.NoError(t, err)
}

func TestVerifyUserSignatureMismatch(t *testing.T) {
	// Generate user keys
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create contract with wrong user ID
	contract := &SignedConnectionContract{
		ContractID:           GenerateContractID(),
		UserID:               "wrong-user-id",
		ServiceID:            "test-service",
		OfferingID:           "offering-1",
		UserConnectionKey:    "user-conn-key",
		ServiceConnectionKey: "service-conn-key",
		CreatedAt:            time.Now().UTC(),
	}

	// Sign the contract
	sig, err := crypto.SignJSON(priv, contract.ToUnsigned())
	require.NoError(t, err)
	contract.UserSignature = sig

	// Verification should fail due to ID mismatch
	err = VerifyUserSignature(contract)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestGenerateContractID(t *testing.T) {
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id := GenerateContractID()
		assert.NotEmpty(t, id)
		assert.False(t, ids[id], "Duplicate contract ID generated")
		ids[id] = true
	}
}

func TestContractOffering(t *testing.T) {
	offering := ContractOffering{
		OfferingID:  "premium-1",
		Name:        "Premium Plan",
		Description: "Full access to all features",
		Capabilities: []types.CapabilityGrant{
			{Capability: types.CapabilityAuthenticate},
			{Capability: types.CapabilityAuthorize},
			{Capability: types.CapabilityReadData, Scope: "profile"},
		},
		RequiredData: []types.DataRequirement{
			{DataType: "email", Required: true, Purpose: "Account identification"},
		},
		Pricing: &types.Pricing{
			Type:     "subscription",
			Amount:   999,
			Currency: "USD",
			Interval: "month",
		},
		TermsURL:  "https://example.com/terms",
		TermsHash: "sha256:abc123",
	}

	assert.Equal(t, "premium-1", offering.OfferingID)
	assert.Equal(t, "Premium Plan", offering.Name)
	assert.Len(t, offering.Capabilities, 3)
	assert.Len(t, offering.RequiredData, 1)
	assert.NotNil(t, offering.Pricing)
	assert.Equal(t, "subscription", offering.Pricing.Type)
}

func TestContractFilter(t *testing.T) {
	active := types.ContractStatusActive
	now := time.Now()

	filter := ContractFilter{
		ServiceID:     "service-123",
		UserID:        "user-456",
		Status:        &active,
		OfferingID:    "offering-1",
		CreatedAfter:  &now,
		CreatedBefore: nil,
		Limit:         50,
		Cursor:        "cursor-abc",
	}

	assert.Equal(t, "service-123", filter.ServiceID)
	assert.Equal(t, "user-456", filter.UserID)
	assert.Equal(t, types.ContractStatusActive, *filter.Status)
	assert.Equal(t, 50, filter.Limit)
}
