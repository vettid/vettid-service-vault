package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockResolver is a mock DNS resolver for testing.
type MockResolver struct {
	records map[string][]string
	err     error
}

func (m *MockResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	records, ok := m.records[name]
	if !ok {
		return nil, &mockDNSError{isNotFound: true}
	}
	return records, nil
}

type mockDNSError struct {
	isNotFound bool
}

func (e *mockDNSError) Error() string    { return "mock DNS error" }
func (e *mockDNSError) Timeout() bool    { return false }
func (e *mockDNSError) Temporary() bool  { return false }
func (e *mockDNSError) IsNotFound() bool { return e.isNotFound }

func TestValidate_Success(t *testing.T) {
	resolver := &MockResolver{
		records: map[string][]string{
			"_vettid-service.example.com": {"vettid-service-id=3Kj9mNxPqRsT5vWy"},
		},
	}

	v := NewValidator(ValidatorConfig{Resolver: resolver})
	result, err := v.Validate(context.Background(), "example.com", "3Kj9mNxPqRsT5vWy")

	require.NoError(t, err)
	assert.True(t, result.Verified)
	assert.Equal(t, "example.com", result.Domain)
	assert.Equal(t, "3Kj9mNxPqRsT5vWy", result.ServiceID)
	assert.NotZero(t, result.VerifiedAt)
}

func TestValidate_NoTXTRecord(t *testing.T) {
	resolver := &MockResolver{
		records: map[string][]string{},
	}

	v := NewValidator(ValidatorConfig{Resolver: resolver})
	result, err := v.Validate(context.Background(), "example.com", "3Kj9mNxPqRsT5vWy")

	assert.ErrorIs(t, err, ErrNoTXTRecord)
	assert.False(t, result.Verified)
}

func TestValidate_ServiceIDMismatch(t *testing.T) {
	resolver := &MockResolver{
		records: map[string][]string{
			"_vettid-service.example.com": {"vettid-service-id=DifferentServiceID"},
		},
	}

	v := NewValidator(ValidatorConfig{Resolver: resolver})
	result, err := v.Validate(context.Background(), "example.com", "3Kj9mNxPqRsT5vWy")

	assert.ErrorIs(t, err, ErrServiceIDMismatch)
	assert.False(t, result.Verified)
}

func TestValidate_EmptyDomain(t *testing.T) {
	v := NewValidator(ValidatorConfig{})
	_, err := v.Validate(context.Background(), "", "3Kj9mNxPqRsT5vWy")

	assert.ErrorIs(t, err, ErrDomainEmpty)
}

func TestValidate_NormalizesDomain(t *testing.T) {
	resolver := &MockResolver{
		records: map[string][]string{
			"_vettid-service.example.com": {"vettid-service-id=test123"},
		},
	}

	v := NewValidator(ValidatorConfig{Resolver: resolver})

	tests := []string{
		"EXAMPLE.COM",
		"Example.Com",
		"example.com.",
		"EXAMPLE.COM.",
	}

	for _, domain := range tests {
		t.Run(domain, func(t *testing.T) {
			result, err := v.Validate(context.Background(), domain, "test123")
			require.NoError(t, err)
			assert.True(t, result.Verified)
			assert.Equal(t, "example.com", result.Domain)
		})
	}
}

func TestValidate_MultipleRecords(t *testing.T) {
	resolver := &MockResolver{
		records: map[string][]string{
			"_vettid-service.example.com": {
				"some-other-record",
				"vettid-service-id=correct-id",
				"another-record",
			},
		},
	}

	v := NewValidator(ValidatorConfig{Resolver: resolver})
	result, err := v.Validate(context.Background(), "example.com", "correct-id")

	require.NoError(t, err)
	assert.True(t, result.Verified)
}

func TestValidate_LookupError(t *testing.T) {
	resolver := &MockResolver{
		err: errors.New("network error"),
	}

	v := NewValidator(ValidatorConfig{Resolver: resolver})
	result, err := v.Validate(context.Background(), "example.com", "test123")

	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLookupFailed)
	assert.False(t, result.Verified)
}

func TestGenerateTXTRecord(t *testing.T) {
	record := GenerateTXTRecord("3Kj9mNxPqRsT5vWy")
	assert.Equal(t, "vettid-service-id=3Kj9mNxPqRsT5vWy", record)
}

func TestGenerateTXTRecordName(t *testing.T) {
	tests := []struct {
		domain   string
		expected string
	}{
		{"example.com", "_vettid-service.example.com"},
		{"EXAMPLE.COM", "_vettid-service.example.com"},
		{"example.com.", "_vettid-service.example.com"},
		{"sub.example.com", "_vettid-service.sub.example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			result := GenerateTXTRecordName(tc.domain)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestInstructions(t *testing.T) {
	instructions := Instructions("example.com", "test123")

	assert.Contains(t, instructions, "example.com")
	assert.Contains(t, instructions, "test123")
	assert.Contains(t, instructions, "_vettid-service.example.com")
	assert.Contains(t, instructions, "vettid-service-id=test123")
}

func TestValidateAsync(t *testing.T) {
	resolver := &MockResolver{
		records: map[string][]string{
			"_vettid-service.example.com": {"vettid-service-id=async-test"},
		},
	}

	v := NewValidator(ValidatorConfig{Resolver: resolver})
	ch := v.ValidateAsync(context.Background(), "example.com", "async-test")

	result := <-ch
	assert.True(t, result.Verified)
}
