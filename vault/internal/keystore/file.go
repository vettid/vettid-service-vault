package keystore

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileKeyStore is a file-based keystore implementation for development and testing.
// Keys are persisted to disk in JSON format.
//
// WARNING: This implementation stores keys in plaintext and is NOT suitable
// for production use. Use KMSKeyStore for production deployments.
type FileKeyStore struct {
	mu      sync.RWMutex
	path    string
	data    *fileStoreData
	changed bool
}

type fileStoreData struct {
	SigningKeys    map[string][]byte `json:"signing_keys"`
	EncryptionKeys map[string][]byte `json:"encryption_keys"`
}

// NewFileKeyStore creates a new file-based keystore.
// If the file exists, it will be loaded; otherwise, a new store is created.
func NewFileKeyStore(path string) (*FileKeyStore, error) {
	store := &FileKeyStore{
		path: path,
		data: &fileStoreData{
			SigningKeys:    make(map[string][]byte),
			EncryptionKeys: make(map[string][]byte),
		},
	}

	// Try to load existing data
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading keystore file: %w", err)
		}
		if err := json.Unmarshal(data, store.data); err != nil {
			return nil, fmt.Errorf("parsing keystore file: %w", err)
		}
	}

	return store, nil
}

// save persists the keystore to disk.
func (f *FileKeyStore) save() error {
	if !f.changed {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating keystore directory: %w", err)
	}

	data, err := json.MarshalIndent(f.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling keystore: %w", err)
	}

	// Write atomically by writing to temp file first
	tempPath := f.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("writing keystore file: %w", err)
	}

	if err := os.Rename(tempPath, f.path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("renaming keystore file: %w", err)
	}

	f.changed = false
	return nil
}

// StoreSigningKey stores an Ed25519 private key to file.
func (f *FileKeyStore) StoreSigningKey(keyID string, privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return ErrInvalidKeySize
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.data.SigningKeys[keyID]; exists {
		return ErrKeyExists
	}

	// Copy the key
	keyCopy := make([]byte, len(privateKey))
	copy(keyCopy, privateKey)
	f.data.SigningKeys[keyID] = keyCopy
	f.changed = true

	return f.save()
}

// GetSigningKey retrieves an Ed25519 private key from file.
func (f *FileKeyStore) GetSigningKey(keyID string) (ed25519.PrivateKey, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	key, exists := f.data.SigningKeys[keyID]
	if !exists {
		return nil, ErrKeyNotFound
	}

	// Return a copy
	keyCopy := make(ed25519.PrivateKey, len(key))
	copy(keyCopy, key)
	return keyCopy, nil
}

// StoreEncryptionKey stores an X25519 private key to file.
func (f *FileKeyStore) StoreEncryptionKey(keyID string, privateKey [32]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.data.EncryptionKeys[keyID]; exists {
		return ErrKeyExists
	}

	keyCopy := make([]byte, 32)
	copy(keyCopy, privateKey[:])
	f.data.EncryptionKeys[keyID] = keyCopy
	f.changed = true

	return f.save()
}

// GetEncryptionKey retrieves an X25519 private key from file.
func (f *FileKeyStore) GetEncryptionKey(keyID string) ([32]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	key, exists := f.data.EncryptionKeys[keyID]
	if !exists {
		return [32]byte{}, ErrKeyNotFound
	}

	var result [32]byte
	copy(result[:], key)
	return result, nil
}

// Sign produces a signature using the stored signing key.
func (f *FileKeyStore) Sign(keyID string, data []byte) ([]byte, error) {
	key, err := f.GetSigningKey(keyID)
	if err != nil {
		return nil, err
	}

	return ed25519.Sign(key, data), nil
}

// DeleteKey removes a key from the file store.
func (f *FileKeyStore) DeleteKey(keyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	_, hasSigning := f.data.SigningKeys[keyID]
	_, hasEncryption := f.data.EncryptionKeys[keyID]

	if !hasSigning && !hasEncryption {
		return ErrKeyNotFound
	}

	delete(f.data.SigningKeys, keyID)
	delete(f.data.EncryptionKeys, keyID)
	f.changed = true

	return f.save()
}

// KeyExists checks if a key with the given ID exists.
func (f *FileKeyStore) KeyExists(keyID string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	_, hasSigning := f.data.SigningKeys[keyID]
	_, hasEncryption := f.data.EncryptionKeys[keyID]

	return hasSigning || hasEncryption
}

// ListKeys returns all key IDs in the store.
func (f *FileKeyStore) ListKeys() ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	keys := make(map[string]struct{})
	for k := range f.data.SigningKeys {
		keys[k] = struct{}{}
	}
	for k := range f.data.EncryptionKeys {
		keys[k] = struct{}{}
	}

	result := make([]string, 0, len(keys))
	for k := range keys {
		result = append(result, k)
	}

	return result, nil
}

// Close persists any pending changes and releases resources.
func (f *FileKeyStore) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.save()
}
