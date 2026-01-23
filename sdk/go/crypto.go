package vettid

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"sort"

	"github.com/mr-tron/base58"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// GenerateServiceIdentity generates a new service identity with cryptographic keys.
func GenerateServiceIdentity(serviceName string, serviceType ServiceType) (*ServiceIdentity, *ServiceKeyPair, error) {
	// Generate Ed25519 signing keypair
	signingPub, signingPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Generate X25519 encryption keypair
	var encPriv, encPub [32]byte
	if _, err := rand.Read(encPriv[:]); err != nil {
		return nil, nil, err
	}
	curve25519.ScalarBaseMult(&encPub, &encPriv)

	// Derive service ID: base58(sha256(signing_public_key)[0:20])
	hash := sha256.Sum256(signingPub)
	serviceID := base58.Encode(hash[:20])

	keypair := &ServiceKeyPair{
		SigningPrivateKey:    signingPriv,
		SigningPublicKey:     signingPub,
		EncryptionPrivateKey: encPriv,
		EncryptionPublicKey:  encPub,
	}

	identity := &ServiceIdentity{
		ServiceID:           serviceID,
		ServiceName:         serviceName,
		ServiceType:         serviceType,
		SigningPublicKey:    base64.StdEncoding.EncodeToString(signingPub),
		EncryptionPublicKey: base64.StdEncoding.EncodeToString(encPub[:]),
	}

	return identity, keypair, nil
}

// DeriveServiceID derives a service ID from a public key.
func DeriveServiceID(publicKey []byte) string {
	hash := sha256.Sum256(publicKey)
	return base58.Encode(hash[:20])
}

// Sign signs data with Ed25519.
func Sign(data []byte, privateKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(privateKey, data)
}

// Verify verifies an Ed25519 signature.
func Verify(data, signature []byte, publicKey ed25519.PublicKey) bool {
	return ed25519.Verify(publicKey, data, signature)
}

// EncryptedMessage represents an encrypted message.
type EncryptedMessage struct {
	EphemeralPublicKey [32]byte
	Nonce              [24]byte
	Ciphertext         []byte
}

// Encrypt encrypts a message for a recipient using X25519 + XSalsa20-Poly1305.
// If senderPrivateKey is nil, an ephemeral keypair is generated.
func Encrypt(message []byte, recipientPublicKey [32]byte, senderPrivateKey *[32]byte) (*EncryptedMessage, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}

	if senderPrivateKey != nil {
		// Use provided sender key
		ciphertext := box.Seal(nil, message, &nonce, &recipientPublicKey, senderPrivateKey)
		return &EncryptedMessage{
			Nonce:      nonce,
			Ciphertext: ciphertext,
		}, nil
	}

	// Generate ephemeral keypair
	ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	ciphertext := box.Seal(nil, message, &nonce, &recipientPublicKey, ephemeralPriv)
	return &EncryptedMessage{
		EphemeralPublicKey: *ephemeralPub,
		Nonce:              nonce,
		Ciphertext:         ciphertext,
	}, nil
}

// Decrypt decrypts a message using X25519 + XSalsa20-Poly1305.
func Decrypt(msg *EncryptedMessage, senderPublicKey, recipientPrivateKey [32]byte) ([]byte, bool) {
	return box.Open(nil, msg.Ciphertext, &msg.Nonce, &senderPublicKey, &recipientPrivateKey)
}

// Canonicalize converts an object to canonical JSON for signing.
func Canonicalize(v interface{}) ([]byte, error) {
	// First marshal to get the structure
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	// Unmarshal to a map
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		// If not an object, just return as-is
		return data, nil
	}

	// Re-marshal with sorted keys
	return canonicalMarshal(m)
}

func canonicalMarshal(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		// Sort keys
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		result := []byte("{")
		for i, k := range keys {
			if i > 0 {
				result = append(result, ',')
			}
			keyJSON, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			result = append(result, keyJSON...)
			result = append(result, ':')
			valJSON, err := canonicalMarshal(val[k])
			if err != nil {
				return nil, err
			}
			result = append(result, valJSON...)
		}
		result = append(result, '}')
		return result, nil

	case []interface{}:
		result := []byte("[")
		for i, item := range val {
			if i > 0 {
				result = append(result, ',')
			}
			itemJSON, err := canonicalMarshal(item)
			if err != nil {
				return nil, err
			}
			result = append(result, itemJSON...)
		}
		result = append(result, ']')
		return result, nil

	default:
		return json.Marshal(v)
	}
}

// SignObject signs a canonical JSON object and returns a base64 signature.
func SignObject(obj interface{}, privateKey ed25519.PrivateKey) (string, error) {
	data, err := Canonicalize(obj)
	if err != nil {
		return "", err
	}
	sig := Sign(data, privateKey)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyObject verifies a signature over a canonical JSON object.
func VerifyObject(obj interface{}, signature string, publicKey ed25519.PublicKey) (bool, error) {
	data, err := Canonicalize(obj)
	if err != nil {
		return false, err
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false, err
	}
	return Verify(data, sig, publicKey), nil
}
