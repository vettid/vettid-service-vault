// Package crypto provides cryptographic primitives for the VettID Service Vault.
//
// Encryption uses X25519 key exchange with XChaCha20-Poly1305 AEAD.
// This provides forward secrecy through ephemeral keys and authenticated
// encryption to prevent tampering.
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

// EncryptedMessage represents an encrypted message payload.
// It contains all the information needed to decrypt the message.
type EncryptedMessage struct {
	// EphemeralPublicKey is the sender's ephemeral X25519 public key
	EphemeralPublicKey [32]byte

	// Nonce is the 24-byte XChaCha20-Poly1305 nonce
	Nonce [24]byte

	// Ciphertext is the encrypted data with authentication tag
	Ciphertext []byte
}

// RandReader is the random source used for cryptographic operations.
// Can be replaced for testing.
var RandReader io.Reader = rand.Reader

// Encrypt encrypts plaintext for a recipient using their X25519 public key.
//
// The encryption process:
// 1. Generate an ephemeral X25519 key pair
// 2. Perform ECDH to derive a shared secret
// 3. Use the shared secret as the key for XChaCha20-Poly1305
// 4. Encrypt the plaintext with a random nonce
//
// The ephemeral public key is included in the output so the recipient
// can derive the same shared secret using their private key.
func Encrypt(plaintext []byte, recipientPubKey [32]byte) (*EncryptedMessage, error) {
	// Generate ephemeral X25519 key pair
	var ephPrivate, ephPublic [32]byte
	if _, err := io.ReadFull(RandReader, ephPrivate[:]); err != nil {
		return nil, fmt.Errorf("generating ephemeral key: %w", err)
	}

	// Clamp the private key for X25519
	ephPrivate[0] &= 248
	ephPrivate[31] &= 127
	ephPrivate[31] |= 64

	curve25519.ScalarBaseMult(&ephPublic, &ephPrivate)

	// Derive shared secret
	sharedSecret, err := DeriveSharedSecret(ephPrivate, recipientPubKey)
	if err != nil {
		return nil, fmt.Errorf("deriving shared secret: %w", err)
	}

	// Create XChaCha20-Poly1305 AEAD cipher
	aead, err := chacha20poly1305.NewX(sharedSecret[:])
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	// Generate random nonce
	var nonce [24]byte
	if _, err := io.ReadFull(RandReader, nonce[:]); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Encrypt (ciphertext includes authentication tag)
	ciphertext := aead.Seal(nil, nonce[:], plaintext, nil)

	return &EncryptedMessage{
		EphemeralPublicKey: ephPublic,
		Nonce:              nonce,
		Ciphertext:         ciphertext,
	}, nil
}

// Decrypt decrypts an encrypted message using the recipient's private key.
func Decrypt(msg *EncryptedMessage, recipientPrivKey [32]byte) ([]byte, error) {
	// Derive shared secret from ephemeral public key and recipient's private key
	sharedSecret, err := DeriveSharedSecret(recipientPrivKey, msg.EphemeralPublicKey)
	if err != nil {
		return nil, fmt.Errorf("deriving shared secret: %w", err)
	}

	// Create XChaCha20-Poly1305 AEAD cipher
	aead, err := chacha20poly1305.NewX(sharedSecret[:])
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	// Decrypt and verify
	plaintext, err := aead.Open(nil, msg.Nonce[:], msg.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// DeriveSharedSecret performs X25519 key exchange to derive a shared secret.
func DeriveSharedSecret(privateKey, publicKey [32]byte) ([32]byte, error) {
	var sharedSecret [32]byte

	result, err := curve25519.X25519(privateKey[:], publicKey[:])
	if err != nil {
		return sharedSecret, fmt.Errorf("X25519 key exchange: %w", err)
	}

	copy(sharedSecret[:], result)

	// Check for low-order point (all zeros result)
	var zero [32]byte
	if sharedSecret == zero {
		return sharedSecret, fmt.Errorf("invalid public key: low-order point")
	}

	return sharedSecret, nil
}

// MarshalJSON implements json.Marshaler for EncryptedMessage.
func (m *EncryptedMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		EphemeralPublicKey string `json:"ephemeral_public_key"`
		Nonce              string `json:"nonce"`
		Ciphertext         string `json:"ciphertext"`
	}{
		EphemeralPublicKey: base64.StdEncoding.EncodeToString(m.EphemeralPublicKey[:]),
		Nonce:              base64.StdEncoding.EncodeToString(m.Nonce[:]),
		Ciphertext:         base64.StdEncoding.EncodeToString(m.Ciphertext),
	})
}

// UnmarshalJSON implements json.Unmarshaler for EncryptedMessage.
func (m *EncryptedMessage) UnmarshalJSON(data []byte) error {
	var encoded struct {
		EphemeralPublicKey string `json:"ephemeral_public_key"`
		Nonce              string `json:"nonce"`
		Ciphertext         string `json:"ciphertext"`
	}

	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}

	ephKey, err := base64.StdEncoding.DecodeString(encoded.EphemeralPublicKey)
	if err != nil {
		return fmt.Errorf("decoding ephemeral key: %w", err)
	}
	if len(ephKey) != 32 {
		return fmt.Errorf("invalid ephemeral key size: %d", len(ephKey))
	}

	nonce, err := base64.StdEncoding.DecodeString(encoded.Nonce)
	if err != nil {
		return fmt.Errorf("decoding nonce: %w", err)
	}
	if len(nonce) != 24 {
		return fmt.Errorf("invalid nonce size: %d", len(nonce))
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded.Ciphertext)
	if err != nil {
		return fmt.Errorf("decoding ciphertext: %w", err)
	}

	copy(m.EphemeralPublicKey[:], ephKey)
	copy(m.Nonce[:], nonce)
	m.Ciphertext = ciphertext

	return nil
}

// EncryptJSON encrypts a JSON-serializable value.
func EncryptJSON(v interface{}, recipientPubKey [32]byte) (*EncryptedMessage, error) {
	plaintext, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON: %w", err)
	}

	return Encrypt(plaintext, recipientPubKey)
}

// DecryptJSON decrypts an encrypted message and unmarshals the JSON content.
func DecryptJSON(msg *EncryptedMessage, recipientPrivKey [32]byte, v interface{}) error {
	plaintext, err := Decrypt(msg, recipientPrivKey)
	if err != nil {
		return err
	}

	return json.Unmarshal(plaintext, v)
}
