package crypto

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/mr-tron/base58"
)

// Signature represents a cryptographic signature with metadata.
type Signature struct {
	// Algorithm identifies the signature algorithm (always "ed25519")
	Algorithm string `json:"algorithm"`

	// PublicKey is the signer's public key (base58 encoded)
	PublicKey string `json:"public_key"`

	// Signature is the Ed25519 signature (base64 encoded)
	Signature string `json:"signature"`

	// Timestamp is when the signature was created
	Timestamp time.Time `json:"timestamp"`
}

// Sign creates an Ed25519 signature over the given data.
func Sign(privateKey ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(privateKey, data)
}

// Verify checks an Ed25519 signature.
func Verify(publicKey ed25519.PublicKey, data, signature []byte) bool {
	return ed25519.Verify(publicKey, data, signature)
}

// SignWithMetadata creates a Signature with full metadata.
func SignWithMetadata(privateKey ed25519.PrivateKey, data []byte) *Signature {
	publicKey := privateKey.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(privateKey, data)

	return &Signature{
		Algorithm: "ed25519",
		PublicKey: base58.Encode(publicKey),
		Signature: base64.StdEncoding.EncodeToString(sig),
		Timestamp: time.Now().UTC(),
	}
}

// VerifySignature verifies a Signature struct against the given data.
func VerifySignature(sig *Signature, data []byte) error {
	if sig.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported algorithm: %s", sig.Algorithm)
	}

	publicKey, err := base58.Decode(sig.PublicKey)
	if err != nil {
		return fmt.Errorf("decoding public key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size: %d", len(publicKey))
	}

	signature, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil {
		return fmt.Errorf("decoding signature: %w", err)
	}

	if !ed25519.Verify(publicKey, data, signature) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// CanonicalizeJSON converts a value to canonical JSON for signing.
//
// Canonical JSON ensures deterministic serialization:
// - Object keys are sorted alphabetically
// - No unnecessary whitespace
// - Numbers are serialized consistently
//
// This is critical for signature verification since any byte difference
// will cause verification to fail.
func CanonicalizeJSON(v interface{}) ([]byte, error) {
	// First marshal to get a map representation
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling value: %w", err)
	}

	// Unmarshal into a generic structure to normalize
	var normalized interface{}
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("unmarshaling for normalization: %w", err)
	}

	// Recursively sort and serialize
	return canonicalMarshal(normalized)
}

// canonicalMarshal recursively marshals a value to canonical JSON.
func canonicalMarshal(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case nil:
		return []byte("null"), nil

	case bool:
		if val {
			return []byte("true"), nil
		}
		return []byte("false"), nil

	case float64:
		// JSON numbers are always float64 from unmarshal
		return json.Marshal(val)

	case string:
		return json.Marshal(val)

	case []interface{}:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, elem := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			elemBytes, err := canonicalMarshal(elem)
			if err != nil {
				return nil, err
			}
			buf.Write(elemBytes)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil

	case map[string]interface{}:
		// Sort keys
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}

			// Write key
			keyBytes, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf.Write(keyBytes)
			buf.WriteByte(':')

			// Write value
			valBytes, err := canonicalMarshal(val[k])
			if err != nil {
				return nil, err
			}
			buf.Write(valBytes)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil

	default:
		// Fallback for unknown types
		return json.Marshal(val)
	}
}

// SignJSON signs a JSON-serializable value after canonicalizing it.
func SignJSON(privateKey ed25519.PrivateKey, v interface{}) (*Signature, error) {
	canonical, err := CanonicalizeJSON(v)
	if err != nil {
		return nil, fmt.Errorf("canonicalizing JSON: %w", err)
	}

	return SignWithMetadata(privateKey, canonical), nil
}

// VerifyJSONSignature verifies a signature over a JSON-serializable value.
func VerifyJSONSignature(sig *Signature, v interface{}) error {
	canonical, err := CanonicalizeJSON(v)
	if err != nil {
		return fmt.Errorf("canonicalizing JSON: %w", err)
	}

	return VerifySignature(sig, canonical)
}

// GetPublicKeyFromSignature extracts the public key from a signature.
func GetPublicKeyFromSignature(sig *Signature) (ed25519.PublicKey, error) {
	publicKey, err := base58.Decode(sig.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decoding public key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: %d", len(publicKey))
	}
	return ed25519.PublicKey(publicKey), nil
}
