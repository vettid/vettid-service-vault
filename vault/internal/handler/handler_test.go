package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// mockContractStore is a simple mock for testing.
type mockContractStore struct{}

func (m *mockContractStore) SaveContract(ctx context.Context, c interface{}) error { return nil }
func (m *mockContractStore) GetContract(ctx context.Context, id string) (interface{}, error) {
	return nil, nil
}
func (m *mockContractStore) GetContractByUser(ctx context.Context, userID string) (interface{}, error) {
	return nil, nil
}
func (m *mockContractStore) UpdateContract(ctx context.Context, c interface{}) error { return nil }
func (m *mockContractStore) UpdateContractStatus(ctx context.Context, id string, status interface{}) error {
	return nil
}
func (m *mockContractStore) ListContracts(ctx context.Context, filter interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockContractStore) DeleteContract(ctx context.Context, id string) error { return nil }
func (m *mockContractStore) SaveInvite(ctx context.Context, invite interface{}) error { return nil }
func (m *mockContractStore) GetInvite(ctx context.Context, id string) (interface{}, error) {
	return nil, nil
}
func (m *mockContractStore) UseInvite(ctx context.Context, id string) error         { return nil }
func (m *mockContractStore) DeleteInvite(ctx context.Context, id string) error      { return nil }
func (m *mockContractStore) CleanupExpired(ctx context.Context) (int, error)        { return 0, nil }

func TestAuthHandler(t *testing.T) {
	h := NewAuthHandler(AuthHandlerConfig{
		Timeout:      5 * time.Minute,
		OfflineGrace: 1 * time.Hour,
	})

	assert.Equal(t, "auth.response", h.EventType())
}

func TestAuthzHandler(t *testing.T) {
	h := NewAuthzHandler(AuthzHandlerConfig{
		Timeout:      5 * time.Minute,
		OfflineGrace: 1 * time.Hour,
	})

	assert.Equal(t, "authz.response", h.EventType())
}

func TestAuthRequestOptions(t *testing.T) {
	now := time.Now().UTC()
	req := &AuthRequest{
		CreatedAt: now,
	}

	// Apply options
	WithAuthContext(map[string]interface{}{"key": "value"})(req)
	WithAuthTimeout(10 * time.Minute)(req)
	WithAuthCallback("https://example.com/callback")(req)
	WithOfflineGrace(2 * time.Hour)(req)

	assert.Equal(t, "value", req.Context["key"])
	assert.Equal(t, now.Add(10*time.Minute), req.ExpiresAt)
	assert.Equal(t, "https://example.com/callback", req.CallbackURL)
	assert.Equal(t, 2*time.Hour, req.OfflineGrace)
}

func TestAuthzRequestOptions(t *testing.T) {
	now := time.Now().UTC()
	req := &AuthzRequest{
		CreatedAt: now,
	}

	// Apply options
	WithAuthzContext(map[string]interface{}{"action": "test"})(req)
	WithAuthzTimeout(15 * time.Minute)(req)
	WithAuthzCallback("https://example.com/authz-callback")(req)
	WithAuthzOfflineGrace(4 * time.Hour)(req)

	assert.Equal(t, "test", req.Context["action"])
	assert.Equal(t, now.Add(15*time.Minute), req.ExpiresAt)
	assert.Equal(t, "https://example.com/authz-callback", req.CallbackURL)
	assert.Equal(t, 4*time.Hour, req.OfflineGrace)
}

func TestAuthHandlerGetRequest(t *testing.T) {
	h := NewAuthHandler(AuthHandlerConfig{})

	// No request should exist initially
	_, ok := h.GetRequest("nonexistent")
	assert.False(t, ok)

	// Add a request manually
	h.mu.Lock()
	h.pending["test-123"] = &AuthRequest{
		RequestID: "test-123",
		UserID:    "user-456",
		Status:    types.RequestStatusPending,
	}
	h.mu.Unlock()

	// Should find it now
	req, ok := h.GetRequest("test-123")
	assert.True(t, ok)
	assert.Equal(t, "test-123", req.RequestID)
	assert.Equal(t, "user-456", req.UserID)
}

func TestAuthzHandlerGetRequest(t *testing.T) {
	h := NewAuthzHandler(AuthzHandlerConfig{})

	// No request should exist initially
	_, ok := h.GetRequest("nonexistent")
	assert.False(t, ok)

	// Add a request manually
	h.mu.Lock()
	h.pending["test-789"] = &AuthzRequest{
		RequestID: "test-789",
		UserID:    "user-abc",
		Action:    "read",
		Resource:  "documents",
		Status:    types.RequestStatusPending,
	}
	h.mu.Unlock()

	// Should find it now
	req, ok := h.GetRequest("test-789")
	assert.True(t, ok)
	assert.Equal(t, "test-789", req.RequestID)
	assert.Equal(t, "read", req.Action)
	assert.Equal(t, "documents", req.Resource)
}

func TestAuthHandlerCallback(t *testing.T) {
	h := NewAuthHandler(AuthHandlerConfig{})

	callbackCalled := false
	var receivedRequest *AuthRequest
	var receivedResponse *AuthResponse

	// Add a pending request
	h.mu.Lock()
	h.pending["callback-test"] = &AuthRequest{
		RequestID:    "callback-test",
		UserID:       "user-123",
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		OfflineGrace: 1 * time.Hour,
		Status:       types.RequestStatusPending,
	}
	h.mu.Unlock()

	// Register callback
	h.SetCallback("callback-test", func(req *AuthRequest, resp *AuthResponse) {
		callbackCalled = true
		receivedRequest = req
		receivedResponse = resp
	})

	// Handle response
	err := h.HandleResponse("user-123", "callback-test", &AuthResponse{
		RequestID: "callback-test",
		Status:    types.RequestStatusApproved,
		Timestamp: time.Now(),
	})

	require.NoError(t, err)
	assert.True(t, callbackCalled)
	assert.NotNil(t, receivedRequest)
	assert.NotNil(t, receivedResponse)
	assert.Equal(t, types.RequestStatusApproved, receivedResponse.Status)
}

func TestAuthHandlerExpiredRequest(t *testing.T) {
	h := NewAuthHandler(AuthHandlerConfig{})

	// Add an expired request with no offline grace
	h.mu.Lock()
	h.pending["expired-test"] = &AuthRequest{
		RequestID:    "expired-test",
		UserID:       "user-123",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		OfflineGrace: 0,
		Status:       types.RequestStatusPending,
	}
	h.mu.Unlock()

	// Handle response should fail
	err := h.HandleResponse("user-123", "expired-test", &AuthResponse{
		RequestID: "expired-test",
		Status:    types.RequestStatusApproved,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestAuthHandlerWrongUser(t *testing.T) {
	h := NewAuthHandler(AuthHandlerConfig{})

	// Add a request
	h.mu.Lock()
	h.pending["wrong-user-test"] = &AuthRequest{
		RequestID:    "wrong-user-test",
		UserID:       "user-123",
		ExpiresAt:    time.Now().Add(5 * time.Minute),
		OfflineGrace: 1 * time.Hour,
		Status:       types.RequestStatusPending,
	}
	h.mu.Unlock()

	// Handle response from wrong user
	err := h.HandleResponse("different-user", "wrong-user-test", &AuthResponse{
		RequestID: "wrong-user-test",
		Status:    types.RequestStatusApproved,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "different user")
}

func TestAuthHandlerCleanup(t *testing.T) {
	h := NewAuthHandler(AuthHandlerConfig{})

	// Add expired and valid requests
	h.mu.Lock()
	h.pending["expired"] = &AuthRequest{
		RequestID:    "expired",
		ExpiresAt:    time.Now().Add(-2 * time.Hour),
		OfflineGrace: 1 * time.Hour,
	}
	h.pending["valid"] = &AuthRequest{
		RequestID:    "valid",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		OfflineGrace: 1 * time.Hour,
	}
	h.mu.Unlock()

	// Run cleanup
	h.Cleanup()

	// Expired should be removed
	_, ok := h.GetRequest("expired")
	assert.False(t, ok)

	// Valid should remain
	_, ok = h.GetRequest("valid")
	assert.True(t, ok)
}

func TestAuthzHandlerCheckAuthorization(t *testing.T) {
	h := NewAuthzHandler(AuthzHandlerConfig{})

	// No authorization initially
	assert.False(t, h.CheckAuthorization("user-123", "read", "documents"))

	// Add an approved authorization
	expiresAt := time.Now().Add(1 * time.Hour)
	h.mu.Lock()
	h.pending["authz-1"] = &AuthzRequest{
		RequestID: "authz-1",
		UserID:    "user-123",
		Action:    "read",
		Resource:  "documents",
		Response: &AuthzResponse{
			Status:    types.RequestStatusApproved,
			Action:    "read",
			Resource:  "documents",
			ExpiresAt: &expiresAt,
		},
	}
	h.mu.Unlock()

	// Should find the authorization
	assert.True(t, h.CheckAuthorization("user-123", "read", "documents"))

	// Different action should not match
	assert.False(t, h.CheckAuthorization("user-123", "write", "documents"))

	// Different user should not match
	assert.False(t, h.CheckAuthorization("user-456", "read", "documents"))
}
