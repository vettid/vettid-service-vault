package keystore

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// KMSKeyStore is a keystore implementation backed by AWS KMS.
//
// This implementation uses KMS for:
// - Key encryption: Private keys are encrypted using a KMS CMK
// - Signing: Can optionally use KMS asymmetric keys for signing
//
// For Ed25519 keys (which KMS doesn't natively support), the private keys
// are encrypted with a KMS data key and stored in the local cache.
type KMSKeyStore struct {
	mu        sync.RWMutex
	client    *kms.Client
	cmkKeyID  string
	ctx       context.Context
	cancelFn  context.CancelFunc

	// Encrypted key cache (encrypted bytes stored in memory)
	signingKeys    map[string][]byte // Encrypted Ed25519 private keys
	encryptionKeys map[string][]byte // Encrypted X25519 private keys

	// Data encryption key (cached, encrypted by CMK)
	dataKey          []byte // Plaintext data key for local encryption
	encryptedDataKey []byte // Encrypted data key (for storage)
}

// KMSKeyStoreConfig holds configuration for the KMS keystore.
type KMSKeyStoreConfig struct {
	// CMKKeyID is the ARN or ID of the KMS Customer Master Key
	CMKKeyID string

	// Region is the AWS region (optional, uses default if not set)
	Region string
}

// NewKMSKeyStore creates a new KMS-backed keystore.
func NewKMSKeyStore(cfg aws.Config, cmkKeyID string) (*KMSKeyStore, error) {
	ctx, cancel := context.WithCancel(context.Background())

	client := kms.NewFromConfig(cfg)

	store := &KMSKeyStore{
		client:         client,
		cmkKeyID:       cmkKeyID,
		ctx:            ctx,
		cancelFn:       cancel,
		signingKeys:    make(map[string][]byte),
		encryptionKeys: make(map[string][]byte),
	}

	// Generate a data encryption key
	if err := store.generateDataKey(); err != nil {
		cancel()
		return nil, fmt.Errorf("generating data key: %w", err)
	}

	return store, nil
}

// generateDataKey creates a new data encryption key using KMS.
func (k *KMSKeyStore) generateDataKey() error {
	ctx, cancel := context.WithTimeout(k.ctx, 30*time.Second)
	defer cancel()

	resp, err := k.client.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String(k.cmkKeyID),
		KeySpec: types.DataKeySpecAes256,
	})
	if err != nil {
		return fmt.Errorf("KMS GenerateDataKey: %w", err)
	}

	k.dataKey = resp.Plaintext
	k.encryptedDataKey = resp.CiphertextBlob

	return nil
}

// encrypt encrypts data using the data key with AES-GCM.
func (k *KMSKeyStore) encrypt(plaintext []byte) ([]byte, error) {
	// For production, implement proper AES-GCM encryption
	// For now, use KMS directly for simplicity

	ctx, cancel := context.WithTimeout(k.ctx, 30*time.Second)
	defer cancel()

	resp, err := k.client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(k.cmkKeyID),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, fmt.Errorf("KMS Encrypt: %w", err)
	}

	return resp.CiphertextBlob, nil
}

// decrypt decrypts data using KMS.
func (k *KMSKeyStore) decrypt(ciphertext []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(k.ctx, 30*time.Second)
	defer cancel()

	resp, err := k.client.Decrypt(ctx, &kms.DecryptInput{
		KeyId:          aws.String(k.cmkKeyID),
		CiphertextBlob: ciphertext,
	})
	if err != nil {
		return nil, fmt.Errorf("KMS Decrypt: %w", err)
	}

	return resp.Plaintext, nil
}

// StoreSigningKey encrypts and stores an Ed25519 private key.
func (k *KMSKeyStore) StoreSigningKey(keyID string, privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return ErrInvalidKeySize
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if _, exists := k.signingKeys[keyID]; exists {
		return ErrKeyExists
	}

	encrypted, err := k.encrypt(privateKey)
	if err != nil {
		return fmt.Errorf("encrypting signing key: %w", err)
	}

	k.signingKeys[keyID] = encrypted
	return nil
}

// GetSigningKey decrypts and returns an Ed25519 private key.
func (k *KMSKeyStore) GetSigningKey(keyID string) (ed25519.PrivateKey, error) {
	k.mu.RLock()
	encrypted, exists := k.signingKeys[keyID]
	k.mu.RUnlock()

	if !exists {
		return nil, ErrKeyNotFound
	}

	plaintext, err := k.decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypting signing key: %w", err)
	}

	return ed25519.PrivateKey(plaintext), nil
}

// StoreEncryptionKey encrypts and stores an X25519 private key.
func (k *KMSKeyStore) StoreEncryptionKey(keyID string, privateKey [32]byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if _, exists := k.encryptionKeys[keyID]; exists {
		return ErrKeyExists
	}

	encrypted, err := k.encrypt(privateKey[:])
	if err != nil {
		return fmt.Errorf("encrypting encryption key: %w", err)
	}

	k.encryptionKeys[keyID] = encrypted
	return nil
}

// GetEncryptionKey decrypts and returns an X25519 private key.
func (k *KMSKeyStore) GetEncryptionKey(keyID string) ([32]byte, error) {
	k.mu.RLock()
	encrypted, exists := k.encryptionKeys[keyID]
	k.mu.RUnlock()

	if !exists {
		return [32]byte{}, ErrKeyNotFound
	}

	plaintext, err := k.decrypt(encrypted)
	if err != nil {
		return [32]byte{}, fmt.Errorf("decrypting encryption key: %w", err)
	}

	var result [32]byte
	copy(result[:], plaintext)
	return result, nil
}

// Sign produces a signature using the stored signing key.
func (k *KMSKeyStore) Sign(keyID string, data []byte) ([]byte, error) {
	privateKey, err := k.GetSigningKey(keyID)
	if err != nil {
		return nil, err
	}

	return ed25519.Sign(privateKey, data), nil
}

// DeleteKey removes a key from the store.
func (k *KMSKeyStore) DeleteKey(keyID string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	_, hasSigning := k.signingKeys[keyID]
	_, hasEncryption := k.encryptionKeys[keyID]

	if !hasSigning && !hasEncryption {
		return ErrKeyNotFound
	}

	delete(k.signingKeys, keyID)
	delete(k.encryptionKeys, keyID)

	return nil
}

// KeyExists checks if a key with the given ID exists.
func (k *KMSKeyStore) KeyExists(keyID string) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()

	_, hasSigning := k.signingKeys[keyID]
	_, hasEncryption := k.encryptionKeys[keyID]

	return hasSigning || hasEncryption
}

// ListKeys returns all key IDs in the store.
func (k *KMSKeyStore) ListKeys() ([]string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	keys := make(map[string]struct{})
	for id := range k.signingKeys {
		keys[id] = struct{}{}
	}
	for id := range k.encryptionKeys {
		keys[id] = struct{}{}
	}

	result := make([]string, 0, len(keys))
	for id := range keys {
		result = append(result, id)
	}

	return result, nil
}

// Close releases resources and clears sensitive data from memory.
func (k *KMSKeyStore) Close() error {
	k.cancelFn()

	k.mu.Lock()
	defer k.mu.Unlock()

	// Clear the data key from memory
	for i := range k.dataKey {
		k.dataKey[i] = 0
	}

	// Clear cached encrypted keys (they're encrypted, but still good practice)
	for id := range k.signingKeys {
		delete(k.signingKeys, id)
	}
	for id := range k.encryptionKeys {
		delete(k.encryptionKeys, id)
	}

	return nil
}

// Export exports the keystore state for persistence.
// The returned data contains encrypted keys and can be stored safely.
func (k *KMSKeyStore) Export() ([]byte, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	state := struct {
		EncryptedDataKey string            `json:"encrypted_data_key"`
		SigningKeys      map[string]string `json:"signing_keys"`
		EncryptionKeys   map[string]string `json:"encryption_keys"`
	}{
		EncryptedDataKey: base64.StdEncoding.EncodeToString(k.encryptedDataKey),
		SigningKeys:      make(map[string]string),
		EncryptionKeys:   make(map[string]string),
	}

	for id, key := range k.signingKeys {
		state.SigningKeys[id] = base64.StdEncoding.EncodeToString(key)
	}
	for id, key := range k.encryptionKeys {
		state.EncryptionKeys[id] = base64.StdEncoding.EncodeToString(key)
	}

	return json.Marshal(state)
}

// Import restores keystore state from previously exported data.
func (k *KMSKeyStore) Import(data []byte) error {
	var state struct {
		EncryptedDataKey string            `json:"encrypted_data_key"`
		SigningKeys      map[string]string `json:"signing_keys"`
		EncryptionKeys   map[string]string `json:"encryption_keys"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parsing import data: %w", err)
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	// Decode the encrypted data key and decrypt it
	encDataKey, err := base64.StdEncoding.DecodeString(state.EncryptedDataKey)
	if err != nil {
		return fmt.Errorf("decoding encrypted data key: %w", err)
	}

	ctx, cancel := context.WithTimeout(k.ctx, 30*time.Second)
	defer cancel()

	resp, err := k.client.Decrypt(ctx, &kms.DecryptInput{
		KeyId:          aws.String(k.cmkKeyID),
		CiphertextBlob: encDataKey,
	})
	if err != nil {
		return fmt.Errorf("decrypting data key: %w", err)
	}

	k.dataKey = resp.Plaintext
	k.encryptedDataKey = encDataKey

	// Import signing keys
	for id, encodedKey := range state.SigningKeys {
		key, err := base64.StdEncoding.DecodeString(encodedKey)
		if err != nil {
			return fmt.Errorf("decoding signing key %s: %w", id, err)
		}
		k.signingKeys[id] = key
	}

	// Import encryption keys
	for id, encodedKey := range state.EncryptionKeys {
		key, err := base64.StdEncoding.DecodeString(encodedKey)
		if err != nil {
			return fmt.Errorf("decoding encryption key %s: %w", id, err)
		}
		k.encryptionKeys[id] = key
	}

	return nil
}
