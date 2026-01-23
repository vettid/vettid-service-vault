package nats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractUserID(t *testing.T) {
	tests := []struct {
		name      string
		subject   string
		serviceID string
		expected  string
	}{
		{
			name:      "valid subject",
			subject:   "ServiceSpace.svc123.fromUser.user456.auth.response",
			serviceID: "svc123",
			expected:  "user456",
		},
		{
			name:      "subject with simple event",
			subject:   "ServiceSpace.svc123.fromUser.user789.event",
			serviceID: "svc123",
			expected:  "user789",
		},
		{
			name:      "wrong service ID",
			subject:   "ServiceSpace.other.fromUser.user456.event",
			serviceID: "svc123",
			expected:  "",
		},
		{
			name:      "invalid format",
			subject:   "InvalidSubject",
			serviceID: "svc123",
			expected:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractUserID(tc.subject, tc.serviceID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExtractEventType(t *testing.T) {
	tests := []struct {
		subject  string
		expected string
	}{
		{
			subject:  "ServiceSpace.svc.fromUser.user.auth.response",
			expected: "response",
		},
		{
			subject:  "MessageSpace.user.fromService.svc.notification",
			expected: "notification",
		},
		{
			subject:  "simple",
			expected: "simple",
		},
	}

	for _, tc := range tests {
		t.Run(tc.subject, func(t *testing.T) {
			result := extractEventType(tc.subject)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestNewRouter(t *testing.T) {
	router := NewRouter()
	assert.NotNil(t, router)
	assert.Empty(t, router.EventTypes())
}

func TestRouterRegisterHandler(t *testing.T) {
	router := NewRouter()

	handler := func(ctx context.Context, msg *Message) error {
		return nil
	}

	router.RegisterHandler("auth.response", handler)
	router.RegisterHandler("authz.response", handler)

	types := router.EventTypes()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "auth.response")
	assert.Contains(t, types, "authz.response")
}

func TestRouterRoute(t *testing.T) {
	router := NewRouter()

	var handledMessage *Message
	handler := func(ctx context.Context, msg *Message) error {
		handledMessage = msg
		return nil
	}

	router.RegisterHandler("test.event", handler)

	msg := &Message{
		UserID:    "user123",
		EventType: "test.event",
		Payload:   []byte("test payload"),
	}

	err := router.Route(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, msg, handledMessage)
}

func TestRouterDefaultHandler(t *testing.T) {
	router := NewRouter()

	var handledUnknown bool
	router.SetDefaultHandler(func(ctx context.Context, msg *Message) error {
		handledUnknown = true
		return nil
	})

	msg := &Message{
		EventType: "unknown.event",
	}

	err := router.Route(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, handledUnknown)
}

func TestRouterNoHandler(t *testing.T) {
	router := NewRouter()

	msg := &Message{
		EventType: "unknown.event",
	}

	err := router.Route(context.Background(), msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no handler")
}

func TestRouterMiddleware(t *testing.T) {
	router := NewRouter()

	var order []string

	router.Use(func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg *Message) error {
			order = append(order, "mw1-before")
			err := next(ctx, msg)
			order = append(order, "mw1-after")
			return err
		}
	})

	router.Use(func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg *Message) error {
			order = append(order, "mw2-before")
			err := next(ctx, msg)
			order = append(order, "mw2-after")
			return err
		}
	})

	router.RegisterHandler("test", func(ctx context.Context, msg *Message) error {
		order = append(order, "handler")
		return nil
	})

	err := router.Route(context.Background(), &Message{EventType: "test"})
	require.NoError(t, err)

	// Middleware should wrap in order: mw1(mw2(handler))
	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	assert.Equal(t, expected, order)
}

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	assert.NotZero(t, cfg.ReconnectWait)
	assert.Equal(t, -1, cfg.MaxReconnects) // Unlimited
	assert.NotZero(t, cfg.Timeout)
}

func TestGenerateAccountKeyPair(t *testing.T) {
	pub, seed, err := GenerateAccountKeyPair()
	require.NoError(t, err)

	assert.NotEmpty(t, pub)
	assert.NotEmpty(t, seed)
	assert.True(t, pub[0] == 'A') // Account public keys start with A
	assert.True(t, seed[0] == 'S' && seed[1] == 'A') // Account seeds start with SA
}

func TestGenerateOperatorKeyPair(t *testing.T) {
	pub, seed, err := GenerateOperatorKeyPair()
	require.NoError(t, err)

	assert.NotEmpty(t, pub)
	assert.NotEmpty(t, seed)
	assert.True(t, pub[0] == 'O') // Operator public keys start with O
	assert.True(t, seed[0] == 'S' && seed[1] == 'O') // Operator seeds start with SO
}
