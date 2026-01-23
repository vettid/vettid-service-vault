package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/curve25519"
)

func generateX25519KeyPair(t *testing.T) (privateKey, publicKey [32]byte) {
	_, err := io.ReadFull(rand.Reader, privateKey[:])
	require.NoError(t, err)

	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	curve25519.ScalarBaseMult(&publicKey, &privateKey)
	return
}

func TestEncryptDecrypt(t *testing.T) {
	// Generate recipient key pair
	recipientPrivate, recipientPublic := generateX25519KeyPair(t)

	// Test message
	plaintext := []byte("Hello, World! This is a test message.")

	// Encrypt
	encrypted, err := Encrypt(plaintext, recipientPublic)
	require.NoError(t, err)

	// Decrypt
	decrypted, err := Decrypt(encrypted, recipientPrivate)
	require.NoError(t, err)

	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecryptEmpty(t *testing.T) {
	recipientPrivate, recipientPublic := generateX25519KeyPair(t)

	// Empty message
	encrypted, err := Encrypt([]byte{}, recipientPublic)
	require.NoError(t, err)

	decrypted, err := Decrypt(encrypted, recipientPrivate)
	require.NoError(t, err)

	assert.Empty(t, decrypted)
}

func TestEncryptDecryptLarge(t *testing.T) {
	recipientPrivate, recipientPublic := generateX25519KeyPair(t)

	// Large message (1MB)
	plaintext := make([]byte, 1024*1024)
	_, err := rand.Read(plaintext)
	require.NoError(t, err)

	encrypted, err := Encrypt(plaintext, recipientPublic)
	require.NoError(t, err)

	decrypted, err := Decrypt(encrypted, recipientPrivate)
	require.NoError(t, err)

	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptWrongKey(t *testing.T) {
	_, recipientPublic := generateX25519KeyPair(t)
	wrongPrivate, _ := generateX25519KeyPair(t)

	plaintext := []byte("Secret message")

	encrypted, err := Encrypt(plaintext, recipientPublic)
	require.NoError(t, err)

	// Decrypting with wrong key should fail
	_, err = Decrypt(encrypted, wrongPrivate)
	assert.Error(t, err)
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	recipientPrivate, recipientPublic := generateX25519KeyPair(t)

	plaintext := []byte("Secret message")

	encrypted, err := Encrypt(plaintext, recipientPublic)
	require.NoError(t, err)

	// Tamper with ciphertext
	encrypted.Ciphertext[0] ^= 0xFF

	// Decryption should fail due to authentication
	_, err = Decrypt(encrypted, recipientPrivate)
	assert.Error(t, err)
}

func TestEncryptedMessageJSON(t *testing.T) {
	recipientPrivate, recipientPublic := generateX25519KeyPair(t)

	plaintext := []byte("Test message for JSON")

	encrypted, err := Encrypt(plaintext, recipientPublic)
	require.NoError(t, err)

	// Marshal to JSON
	data, err := json.Marshal(encrypted)
	require.NoError(t, err)

	// Unmarshal
	var parsed EncryptedMessage
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Should still decrypt correctly
	decrypted, err := Decrypt(&parsed, recipientPrivate)
	require.NoError(t, err)

	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecryptJSON(t *testing.T) {
	recipientPrivate, recipientPublic := generateX25519KeyPair(t)

	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
		Tags  []string `json:"tags"`
	}

	original := TestData{
		Name:  "Test",
		Value: 42,
		Tags:  []string{"a", "b", "c"},
	}

	encrypted, err := EncryptJSON(original, recipientPublic)
	require.NoError(t, err)

	var decrypted TestData
	err = DecryptJSON(encrypted, recipientPrivate, &decrypted)
	require.NoError(t, err)

	assert.Equal(t, original, decrypted)
}

func TestDeriveSharedSecret(t *testing.T) {
	private1, public1 := generateX25519KeyPair(t)
	private2, public2 := generateX25519KeyPair(t)

	// Both parties should derive the same shared secret
	secret1, err := DeriveSharedSecret(private1, public2)
	require.NoError(t, err)

	secret2, err := DeriveSharedSecret(private2, public1)
	require.NoError(t, err)

	assert.Equal(t, secret1, secret2)
}

func TestSign(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	message := []byte("Message to sign")

	signature := Sign(privateKey, message)
	assert.NotEmpty(t, signature)

	// Verify signature
	assert.True(t, Verify(publicKey, message, signature))
}

func TestSignWithMetadata(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	message := []byte("Message to sign")

	sig := SignWithMetadata(privateKey, message)

	assert.Equal(t, "ed25519", sig.Algorithm)
	assert.NotEmpty(t, sig.PublicKey)
	assert.NotEmpty(t, sig.Signature)
	assert.False(t, sig.Timestamp.IsZero())

	// Verify
	err = VerifySignature(sig, message)
	assert.NoError(t, err)
}

func TestVerifySignatureWrongMessage(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	sig := SignWithMetadata(privateKey, []byte("Original message"))

	// Verification with different message should fail
	err = VerifySignature(sig, []byte("Different message"))
	assert.Error(t, err)
}

func TestCanonicalizeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "simple object",
			input:    map[string]interface{}{"b": 2, "a": 1},
			expected: `{"a":1,"b":2}`,
		},
		{
			name:     "nested object",
			input:    map[string]interface{}{"z": map[string]interface{}{"b": 2, "a": 1}, "a": 1},
			expected: `{"a":1,"z":{"a":1,"b":2}}`,
		},
		{
			name:     "array",
			input:    []interface{}{3, 1, 2},
			expected: `[3,1,2]`, // Arrays preserve order
		},
		{
			name:     "string",
			input:    "hello",
			expected: `"hello"`,
		},
		{
			name:     "null",
			input:    nil,
			expected: `null`,
		},
		{
			name:     "bool true",
			input:    true,
			expected: `true`,
		},
		{
			name:     "bool false",
			input:    false,
			expected: `false`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CanonicalizeJSON(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

func TestCanonicalizeJSONDeterministic(t *testing.T) {
	// Same data should always produce same output
	data := map[string]interface{}{
		"z": "last",
		"a": "first",
		"m": "middle",
		"nested": map[string]interface{}{
			"x": 1,
			"b": 2,
		},
	}

	var results [][]byte
	for i := 0; i < 10; i++ {
		result, err := CanonicalizeJSON(data)
		require.NoError(t, err)
		results = append(results, result)
	}

	for i := 1; i < len(results); i++ {
		assert.True(t, bytes.Equal(results[0], results[i]), "Canonicalization not deterministic")
	}
}

func TestSignJSON(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	data := map[string]interface{}{
		"name":  "Test",
		"value": 42,
	}

	sig, err := SignJSON(privateKey, data)
	require.NoError(t, err)

	// Verify with same data (possibly different field order)
	reorderedData := map[string]interface{}{
		"value": 42,
		"name":  "Test",
	}

	err = VerifyJSONSignature(sig, reorderedData)
	assert.NoError(t, err)
}

func TestGetPublicKeyFromSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	sig := SignWithMetadata(privateKey, []byte("test"))

	extracted, err := GetPublicKeyFromSignature(sig)
	require.NoError(t, err)

	assert.Equal(t, publicKey, extracted)
}
