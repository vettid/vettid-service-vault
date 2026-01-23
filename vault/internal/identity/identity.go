// Package identity provides service identity management for VettID Service Vault.
//
// Service identity is self-sovereign - derived from cryptographic keys controlled
// by the service provider. The service_id is deterministically computed from the
// Ed25519 signing public key: service_id = base58(sha256(public_key)[0:20])
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/mr-tron/base58"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
	"golang.org/x/crypto/curve25519"
)

// ServiceIdentity represents a service's public identity information.
// This is the information that can be shared with users and the registry.
type ServiceIdentity struct {
	// ServiceID is the unique identifier derived from the signing public key.
	// Format: base58(sha256(signing_public_key)[0:20])
	ServiceID string `json:"service_id"`

	// SigningKey is the Ed25519 public key for signature verification.
	SigningKey ed25519.PublicKey `json:"signing_key"`

	// EncryptionKey is the X25519 public key for message encryption.
	EncryptionKey [32]byte `json:"encryption_key"`

	// ServiceName is a human-readable name for the service.
	ServiceName string `json:"service_name"`

	// ServiceType categorizes the service (payment, identity, etc.).
	ServiceType types.ServiceType `json:"service_type"`

	// NATSEndpoint is the service's NATS server address for user connections.
	NATSEndpoint string `json:"nats_endpoint"`

	// Domain is the optional verified domain for the service.
	Domain string `json:"domain,omitempty"`

	// DomainVerified indicates if the domain has been verified via DNS.
	DomainVerified bool `json:"domain_verified"`

	// CreatedAt is when this identity was created.
	CreatedAt time.Time `json:"created_at"`
}

// ServiceKeyPair holds both public and private keys for a service.
// The private keys should be stored securely using the KeyStore interface.
type ServiceKeyPair struct {
	// SigningPrivateKey is the Ed25519 private key for signing.
	SigningPrivateKey ed25519.PrivateKey

	// EncryptionPrivateKey is the X25519 private key for decryption.
	EncryptionPrivateKey [32]byte
}

// RandReader is the random source used for key generation.
// Can be replaced for testing.
var RandReader io.Reader = rand.Reader

// GenerateIdentity creates a new service identity with fresh cryptographic keys.
func GenerateIdentity(name string, serviceType types.ServiceType, natsEndpoint string) (*ServiceIdentity, *ServiceKeyPair, error) {
	// Generate Ed25519 signing key pair
	publicKey, privateKey, err := ed25519.GenerateKey(RandReader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating signing key: %w", err)
	}

	// Generate X25519 encryption key pair
	var encPrivateKey [32]byte
	if _, err := io.ReadFull(RandReader, encPrivateKey[:]); err != nil {
		return nil, nil, fmt.Errorf("generating encryption key: %w", err)
	}

	// Clamp the private key for X25519 (per RFC 7748)
	encPrivateKey[0] &= 248
	encPrivateKey[31] &= 127
	encPrivateKey[31] |= 64

	// Derive public key
	var encPublicKey [32]byte
	curve25519.ScalarBaseMult(&encPublicKey, &encPrivateKey)

	// Derive service ID from signing public key
	serviceID := DeriveServiceID(publicKey)

	identity := &ServiceIdentity{
		ServiceID:     serviceID,
		SigningKey:    publicKey,
		EncryptionKey: encPublicKey,
		ServiceName:   name,
		ServiceType:   serviceType,
		NATSEndpoint:  natsEndpoint,
		CreatedAt:     time.Now().UTC(),
	}

	keyPair := &ServiceKeyPair{
		SigningPrivateKey:    privateKey,
		EncryptionPrivateKey: encPrivateKey,
	}

	return identity, keyPair, nil
}

// DeriveServiceID computes the service_id from an Ed25519 public key.
// The derivation is: base58(sha256(public_key)[0:20])
func DeriveServiceID(publicKey ed25519.PublicKey) string {
	hash := sha256.Sum256(publicKey)
	truncated := hash[:20]
	return base58.Encode(truncated)
}

// VerifyServiceID checks if a service_id matches the given public key.
func VerifyServiceID(serviceID string, publicKey ed25519.PublicKey) bool {
	derived := DeriveServiceID(publicKey)
	return serviceID == derived
}

// FromKeys creates a ServiceIdentity from existing public keys.
func FromKeys(signingKey ed25519.PublicKey, encryptionKey [32]byte, name string, serviceType types.ServiceType, natsEndpoint string) *ServiceIdentity {
	return &ServiceIdentity{
		ServiceID:     DeriveServiceID(signingKey),
		SigningKey:    signingKey,
		EncryptionKey: encryptionKey,
		ServiceName:   name,
		ServiceType:   serviceType,
		NATSEndpoint:  natsEndpoint,
		CreatedAt:     time.Now().UTC(),
	}
}

// FromPrivateKeys creates a ServiceIdentity from private keys.
// This is useful when loading keys from a keystore.
func FromPrivateKeys(signingPrivKey ed25519.PrivateKey, encryptionPrivKey [32]byte, name string, serviceType types.ServiceType, natsEndpoint string) *ServiceIdentity {
	// Extract public signing key
	signingPubKey := signingPrivKey.Public().(ed25519.PublicKey)

	// Derive public encryption key from private key
	var encPubKey [32]byte
	curve25519.ScalarBaseMult(&encPubKey, &encryptionPrivKey)

	return &ServiceIdentity{
		ServiceID:     DeriveServiceID(signingPubKey),
		SigningKey:    signingPubKey,
		EncryptionKey: encPubKey,
		ServiceName:   name,
		ServiceType:   serviceType,
		NATSEndpoint:  natsEndpoint,
		CreatedAt:     time.Now().UTC(),
	}
}

// MarshalJSON implements json.Marshaler for ServiceIdentity.
// Keys are encoded as base64 for JSON transport.
func (i *ServiceIdentity) MarshalJSON() ([]byte, error) {
	type alias struct {
		ServiceID      string            `json:"service_id"`
		SigningKey     string            `json:"signing_key"`
		EncryptionKey  string            `json:"encryption_key"`
		ServiceName    string            `json:"service_name"`
		ServiceType    types.ServiceType `json:"service_type"`
		NATSEndpoint   string            `json:"nats_endpoint"`
		Domain         string            `json:"domain,omitempty"`
		DomainVerified bool              `json:"domain_verified"`
		CreatedAt      time.Time         `json:"created_at"`
	}

	return json.Marshal(alias{
		ServiceID:      i.ServiceID,
		SigningKey:     base58.Encode(i.SigningKey),
		EncryptionKey:  base58.Encode(i.EncryptionKey[:]),
		ServiceName:    i.ServiceName,
		ServiceType:    i.ServiceType,
		NATSEndpoint:   i.NATSEndpoint,
		Domain:         i.Domain,
		DomainVerified: i.DomainVerified,
		CreatedAt:      i.CreatedAt,
	})
}

// UnmarshalJSON implements json.Unmarshaler for ServiceIdentity.
func (i *ServiceIdentity) UnmarshalJSON(data []byte) error {
	type alias struct {
		ServiceID      string            `json:"service_id"`
		SigningKey     string            `json:"signing_key"`
		EncryptionKey  string            `json:"encryption_key"`
		ServiceName    string            `json:"service_name"`
		ServiceType    types.ServiceType `json:"service_type"`
		NATSEndpoint   string            `json:"nats_endpoint"`
		Domain         string            `json:"domain,omitempty"`
		DomainVerified bool              `json:"domain_verified"`
		CreatedAt      time.Time         `json:"created_at"`
	}

	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}

	signingKey, err := base58.Decode(a.SigningKey)
	if err != nil {
		return fmt.Errorf("decoding signing key: %w", err)
	}
	if len(signingKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid signing key size: %d", len(signingKey))
	}

	encryptionKey, err := base58.Decode(a.EncryptionKey)
	if err != nil {
		return fmt.Errorf("decoding encryption key: %w", err)
	}
	if len(encryptionKey) != 32 {
		return fmt.Errorf("invalid encryption key size: %d", len(encryptionKey))
	}

	i.ServiceID = a.ServiceID
	i.SigningKey = signingKey
	copy(i.EncryptionKey[:], encryptionKey)
	i.ServiceName = a.ServiceName
	i.ServiceType = a.ServiceType
	i.NATSEndpoint = a.NATSEndpoint
	i.Domain = a.Domain
	i.DomainVerified = a.DomainVerified
	i.CreatedAt = a.CreatedAt

	// Verify service ID matches public key
	if !VerifyServiceID(i.ServiceID, i.SigningKey) {
		return fmt.Errorf("service_id does not match signing key")
	}

	return nil
}

// ParseIdentity parses a ServiceIdentity from JSON.
func ParseIdentity(data []byte) (*ServiceIdentity, error) {
	var identity ServiceIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, err
	}
	return &identity, nil
}

