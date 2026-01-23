package identity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

func TestGenerateIdentity(t *testing.T) {
	identity, keyPair, err := GenerateIdentity("Test Service", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)

	// Verify identity fields
	assert.NotEmpty(t, identity.ServiceID)
	assert.NotNil(t, identity.SigningKey)
	assert.NotEmpty(t, identity.EncryptionKey)
	assert.Equal(t, "Test Service", identity.ServiceName)
	assert.Equal(t, types.ServiceTypeGeneric, identity.ServiceType)
	assert.Equal(t, "nats://localhost:4222", identity.NATSEndpoint)

	// Verify key pair
	assert.NotNil(t, keyPair.SigningPrivateKey)
	assert.NotEmpty(t, keyPair.EncryptionPrivateKey)

	// Verify service ID derivation is correct
	assert.True(t, VerifyServiceID(identity.ServiceID, identity.SigningKey))
}

func TestDeriveServiceID(t *testing.T) {
	identity, _, err := GenerateIdentity("Test", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)

	// Derive again and compare
	derivedID := DeriveServiceID(identity.SigningKey)
	assert.Equal(t, identity.ServiceID, derivedID)

	// Service ID should be base58 encoded and non-empty
	assert.NotEmpty(t, derivedID)
	assert.Greater(t, len(derivedID), 10) // Base58 encoding of 20 bytes should be > 10 chars
}

func TestVerifyServiceID(t *testing.T) {
	identity, _, err := GenerateIdentity("Test", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)

	// Correct ID should verify
	assert.True(t, VerifyServiceID(identity.ServiceID, identity.SigningKey))

	// Wrong ID should not verify
	assert.False(t, VerifyServiceID("wrongid", identity.SigningKey))

	// Different key should not match
	identity2, _, err := GenerateIdentity("Test2", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)
	assert.False(t, VerifyServiceID(identity.ServiceID, identity2.SigningKey))
}

func TestServiceIdentityJSON(t *testing.T) {
	identity, _, err := GenerateIdentity("Test Service", types.ServiceTypePayment, "nats://localhost:4222")
	require.NoError(t, err)

	identity.Domain = "example.com"
	identity.DomainVerified = true

	// Marshal to JSON
	data, err := json.Marshal(identity)
	require.NoError(t, err)

	// Unmarshal back
	var parsed ServiceIdentity
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Compare fields
	assert.Equal(t, identity.ServiceID, parsed.ServiceID)
	assert.Equal(t, identity.SigningKey, parsed.SigningKey)
	assert.Equal(t, identity.EncryptionKey, parsed.EncryptionKey)
	assert.Equal(t, identity.ServiceName, parsed.ServiceName)
	assert.Equal(t, identity.ServiceType, parsed.ServiceType)
	assert.Equal(t, identity.NATSEndpoint, parsed.NATSEndpoint)
	assert.Equal(t, identity.Domain, parsed.Domain)
	assert.Equal(t, identity.DomainVerified, parsed.DomainVerified)
}

func TestServiceIdentityJSONValidation(t *testing.T) {
	// Create an identity
	identity, _, err := GenerateIdentity("Test", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)

	// Marshal to JSON
	data, err := json.Marshal(identity)
	require.NoError(t, err)

	// Modify the service_id to create a mismatch
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	raw["service_id"] = "tampered_service_id"

	tamperedData, err := json.Marshal(raw)
	require.NoError(t, err)

	// Unmarshaling should fail due to ID mismatch
	var parsed ServiceIdentity
	err = json.Unmarshal(tamperedData, &parsed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service_id does not match")
}

func TestParseIdentity(t *testing.T) {
	identity, _, err := GenerateIdentity("Test", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)

	data, err := json.Marshal(identity)
	require.NoError(t, err)

	parsed, err := ParseIdentity(data)
	require.NoError(t, err)

	assert.Equal(t, identity.ServiceID, parsed.ServiceID)
	assert.Equal(t, identity.ServiceName, parsed.ServiceName)
}

func TestFromKeys(t *testing.T) {
	// Generate keys first
	identity, _, err := GenerateIdentity("Original", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)

	// Create new identity from existing keys
	newIdentity := FromKeys(
		identity.SigningKey,
		identity.EncryptionKey,
		"New Name",
		types.ServiceTypePayment,
		"nats://new:4222",
	)

	// Service ID should be the same (derived from same signing key)
	assert.Equal(t, identity.ServiceID, newIdentity.ServiceID)

	// Other fields should be updated
	assert.Equal(t, "New Name", newIdentity.ServiceName)
	assert.Equal(t, types.ServiceTypePayment, newIdentity.ServiceType)
	assert.Equal(t, "nats://new:4222", newIdentity.NATSEndpoint)
}

func TestMultipleIdentitiesUnique(t *testing.T) {
	ids := make(map[string]bool)

	// Generate multiple identities and ensure all are unique
	for i := 0; i < 10; i++ {
		identity, _, err := GenerateIdentity("Test", types.ServiceTypeGeneric, "nats://localhost:4222")
		require.NoError(t, err)

		assert.False(t, ids[identity.ServiceID], "Duplicate service ID generated")
		ids[identity.ServiceID] = true
	}
}
