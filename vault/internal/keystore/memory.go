package keystore

import (
	"crypto/ed25519"
	"sync"
)

// MemoryKeyStore is an in-memory keystore implementation for development and testing.
// Keys are not persisted and are lost when the process exits.
//
// WARNING: This implementation is NOT suitable for production use.
// Private keys are stored in plain memory without protection.
type MemoryKeyStore struct {
	mu             sync.RWMutex
	signingKeys    map[string]ed25519.PrivateKey
	encryptionKeys map[string][32]byte
}

// NewMemoryKeyStore creates a new in-memory keystore.
func NewMemoryKeyStore() *MemoryKeyStore {
	return &MemoryKeyStore{
		signingKeys:    make(map[string]ed25519.PrivateKey),
		encryptionKeys: make(map[string][32]byte),
	}
}

// StoreSigningKey stores an Ed25519 private key in memory.
func (m *MemoryKeyStore) StoreSigningKey(keyID string, privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return ErrInvalidKeySize
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.signingKeys[keyID]; exists {
		return ErrKeyExists
	}

	// Copy the key to prevent external modification
	keyCopy := make(ed25519.PrivateKey, len(privateKey))
	copy(keyCopy, privateKey)
	m.signingKeys[keyID] = keyCopy

	return nil
}

// GetSigningKey retrieves an Ed25519 private key from memory.
func (m *MemoryKeyStore) GetSigningKey(keyID string) (ed25519.PrivateKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key, exists := m.signingKeys[keyID]
	if !exists {
		return nil, ErrKeyNotFound
	}

	// Return a copy to prevent external modification
	keyCopy := make(ed25519.PrivateKey, len(key))
	copy(keyCopy, key)
	return keyCopy, nil
}

// StoreEncryptionKey stores an X25519 private key in memory.
func (m *MemoryKeyStore) StoreEncryptionKey(keyID string, privateKey [32]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.encryptionKeys[keyID]; exists {
		return ErrKeyExists
	}

	m.encryptionKeys[keyID] = privateKey
	return nil
}

// GetEncryptionKey retrieves an X25519 private key from memory.
func (m *MemoryKeyStore) GetEncryptionKey(keyID string) ([32]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key, exists := m.encryptionKeys[keyID]
	if !exists {
		return [32]byte{}, ErrKeyNotFound
	}

	return key, nil
}

// Sign produces a signature using the stored signing key.
func (m *MemoryKeyStore) Sign(keyID string, data []byte) ([]byte, error) {
	key, err := m.GetSigningKey(keyID)
	if err != nil {
		return nil, err
	}

	return ed25519.Sign(key, data), nil
}

// DeleteKey removes a key from memory.
func (m *MemoryKeyStore) DeleteKey(keyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, hasSigning := m.signingKeys[keyID]
	_, hasEncryption := m.encryptionKeys[keyID]

	if !hasSigning && !hasEncryption {
		return ErrKeyNotFound
	}

	delete(m.signingKeys, keyID)
	delete(m.encryptionKeys, keyID)

	return nil
}

// KeyExists checks if a key with the given ID exists.
func (m *MemoryKeyStore) KeyExists(keyID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, hasSigning := m.signingKeys[keyID]
	_, hasEncryption := m.encryptionKeys[keyID]

	return hasSigning || hasEncryption
}

// ListKeys returns all key IDs in the store.
func (m *MemoryKeyStore) ListKeys() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make(map[string]struct{})
	for k := range m.signingKeys {
		keys[k] = struct{}{}
	}
	for k := range m.encryptionKeys {
		keys[k] = struct{}{}
	}

	result := make([]string, 0, len(keys))
	for k := range keys {
		result = append(result, k)
	}

	return result, nil
}

// Close is a no-op for the memory keystore.
func (m *MemoryKeyStore) Close() error {
	// Clear all keys from memory
	m.mu.Lock()
	defer m.mu.Unlock()

	// Zero out signing keys
	for id, key := range m.signingKeys {
		for i := range key {
			key[i] = 0
		}
		delete(m.signingKeys, id)
	}

	// Zero out encryption keys
	for id, key := range m.encryptionKeys {
		for i := range key {
			key[i] = 0
		}
		delete(m.encryptionKeys, id)
	}

	return nil
}
