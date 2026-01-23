// Package keystore provides secure storage interfaces for cryptographic keys.
//
// The keystore abstraction allows the Service Vault to work with different
// key storage backends: in-memory (development), file-based (testing),
// and AWS KMS (production).
package keystore

import (
	"crypto/ed25519"
	"errors"
)

// Common errors returned by keystore implementations.
var (
	ErrKeyNotFound     = errors.New("key not found")
	ErrKeyExists       = errors.New("key already exists")
	ErrInvalidKeyType  = errors.New("invalid key type")
	ErrInvalidKeySize  = errors.New("invalid key size")
	ErrStorageFailed   = errors.New("storage operation failed")
	ErrSigningFailed   = errors.New("signing operation failed")
	ErrDecryptFailed   = errors.New("decryption operation failed")
)

// KeyType identifies the type of cryptographic key.
type KeyType string

const (
	KeyTypeSigning    KeyType = "signing"    // Ed25519 signing key
	KeyTypeEncryption KeyType = "encryption" // X25519 encryption key
)

// KeyStore defines the interface for secure key storage and cryptographic operations.
//
// Implementations may store keys locally (file, memory) or delegate to
// hardware security modules (HSM) or cloud KMS services.
type KeyStore interface {
	// StoreSigningKey stores an Ed25519 private key.
	// Returns ErrKeyExists if a key with the same ID already exists.
	StoreSigningKey(keyID string, privateKey ed25519.PrivateKey) error

	// GetSigningKey retrieves an Ed25519 private key by ID.
	// Returns ErrKeyNotFound if the key does not exist.
	GetSigningKey(keyID string) (ed25519.PrivateKey, error)

	// StoreEncryptionKey stores an X25519 private key.
	// Returns ErrKeyExists if a key with the same ID already exists.
	StoreEncryptionKey(keyID string, privateKey [32]byte) error

	// GetEncryptionKey retrieves an X25519 private key by ID.
	// Returns ErrKeyNotFound if the key does not exist.
	GetEncryptionKey(keyID string) ([32]byte, error)

	// Sign produces a signature using the stored signing key.
	// This allows HSM/KMS implementations to sign without exposing the private key.
	// Returns ErrKeyNotFound if the key does not exist.
	Sign(keyID string, data []byte) ([]byte, error)

	// DeleteKey removes a key from storage.
	// Returns ErrKeyNotFound if the key does not exist.
	DeleteKey(keyID string) error

	// KeyExists checks if a key with the given ID exists.
	KeyExists(keyID string) bool

	// ListKeys returns all key IDs in the store.
	ListKeys() ([]string, error)

	// Close releases any resources held by the keystore.
	Close() error
}

// KeyMetadata holds metadata about a stored key.
type KeyMetadata struct {
	KeyID     string
	KeyType   KeyType
	CreatedAt int64 // Unix timestamp
}
