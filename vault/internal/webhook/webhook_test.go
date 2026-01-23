package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatcher_Dispatch_Success(t *testing.T) {
	// Create a test server that returns 200
	var received atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("X-VettID-Event-ID"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewDispatcher(DispatcherConfig{
		Secret:     []byte("test-secret"),
		MaxRetries: 3,
	})
	d.Start()
	defer d.Stop()

	event := Event{
		ID:        "test-event-1",
		Type:      "auth.response",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"status": "approved",
		},
	}

	delivery, err := d.Dispatch(server.URL, event)
	require.NoError(t, err)
	assert.NotNil(t, delivery)

	// Wait for delivery
	time.Sleep(100 * time.Millisecond)

	assert.True(t, received.Load())

	// Check delivery status
	updated, found := d.GetDelivery(event.ID)
	require.True(t, found)
	assert.Equal(t, StatusSuccess, updated.Status)
	assert.Equal(t, 200, updated.StatusCode)
}

func TestDispatcher_Dispatch_Retry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewDispatcher(DispatcherConfig{
		MaxRetries:  5,
		RetryDelays: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond},
	})
	d.Start()
	defer d.Stop()

	event := Event{
		ID:        "retry-test",
		Type:      "test",
		Timestamp: time.Now(),
		Data:      map[string]interface{}{},
	}

	_, err := d.Dispatch(server.URL, event)
	require.NoError(t, err)

	// Wait for retries
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int32(3), attempts.Load())

	delivery, found := d.GetDelivery(event.ID)
	require.True(t, found)
	assert.Equal(t, StatusSuccess, delivery.Status)
	assert.Equal(t, 3, delivery.Attempts)
}

func TestDispatcher_Dispatch_MaxRetriesReached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := NewDispatcher(DispatcherConfig{
		MaxRetries:  2,
		RetryDelays: []time.Duration{10 * time.Millisecond},
	})
	d.Start()
	defer d.Stop()

	event := Event{
		ID:        "fail-test",
		Type:      "test",
		Timestamp: time.Now(),
		Data:      map[string]interface{}{},
	}

	_, err := d.Dispatch(server.URL, event)
	require.NoError(t, err)

	// Wait for retries
	time.Sleep(200 * time.Millisecond)

	delivery, found := d.GetDelivery(event.ID)
	require.True(t, found)
	assert.Equal(t, StatusFailed, delivery.Status)
	assert.Equal(t, 2, delivery.Attempts)
}

func TestDispatcher_Signature(t *testing.T) {
	var receivedSig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-VettID-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	secret := []byte("webhook-secret")
	d := NewDispatcher(DispatcherConfig{
		Secret: secret,
	})
	d.Start()
	defer d.Stop()

	event := Event{
		ID:        "sig-test",
		Type:      "test",
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"key": "value"},
	}

	delivery, err := d.Dispatch(server.URL, event)
	require.NoError(t, err)

	// Wait for delivery
	time.Sleep(100 * time.Millisecond)

	assert.NotEmpty(t, receivedSig)
	assert.True(t, VerifySignature(delivery.Payload, receivedSig, secret))
}

func TestVerifySignature(t *testing.T) {
	secret := []byte("test-secret")
	payload := []byte(`{"event":"test"}`)

	// Generate signature
	d := NewDispatcher(DispatcherConfig{Secret: secret})
	signature := d.sign(payload)

	// Verify valid signature
	assert.True(t, VerifySignature(payload, signature, secret))

	// Verify invalid signature
	assert.False(t, VerifySignature(payload, "sha256=invalid", secret))

	// Verify wrong secret
	assert.False(t, VerifySignature(payload, signature, []byte("wrong-secret")))

	// Verify modified payload
	assert.False(t, VerifySignature([]byte(`{"event":"modified"}`), signature, secret))
}

func TestDispatcher_InvalidURL(t *testing.T) {
	d := NewDispatcher(DispatcherConfig{})

	event := Event{
		ID:   "test",
		Type: "test",
	}

	_, err := d.Dispatch("", event)
	assert.ErrorIs(t, err, ErrInvalidURL)
}

func TestDispatcher_GetPendingCount(t *testing.T) {
	d := NewDispatcher(DispatcherConfig{})
	assert.Equal(t, 0, d.GetPendingCount())
}

func TestDispatcher_GetRecentDeliveries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewDispatcher(DispatcherConfig{})
	d.Start()
	defer d.Stop()

	// Send a few events
	for i := 0; i < 5; i++ {
		event := Event{
			ID:        "event-" + string(rune('a'+i)),
			Type:      "test",
			Timestamp: time.Now(),
		}
		d.Dispatch(server.URL, event)
	}

	// Wait for deliveries
	time.Sleep(200 * time.Millisecond)

	deliveries := d.GetRecentDeliveries(3)
	assert.Len(t, deliveries, 3)
}

func TestEvent_JSON(t *testing.T) {
	event := Event{
		ID:        "event-123",
		Type:      "auth.response",
		Timestamp: time.Now().UTC(),
		Data: map[string]interface{}{
			"status":     "approved",
			"user_id":    "user123",
			"request_id": "req456",
		},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var parsed Event
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, event.ID, parsed.ID)
	assert.Equal(t, event.Type, parsed.Type)
	assert.Equal(t, event.Data["status"], parsed.Data["status"])
}
