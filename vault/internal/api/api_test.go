package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vettid/vettid-service-vault/vault/internal/config"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/internal/identity"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

func newTestServer(t *testing.T) *Server {
	cfg := config.DefaultConfig()
	cfg.Environment = "development" // Skip auth

	identity, _, err := identity.GenerateIdentity("Test Service", types.ServiceTypeGeneric, "nats://localhost:4222")
	require.NoError(t, err)

	authHandler := handler.NewAuthHandler(handler.AuthHandlerConfig{})
	authzHandler := handler.NewAuthzHandler(handler.AuthzHandlerConfig{})

	server, err := NewServer(ServerConfig{
		Config:       cfg,
		Identity:     identity,
		AuthHandler:  authHandler,
		AuthzHandler: authzHandler,
	})
	require.NoError(t, err)

	return server
}

func TestHealthCheck(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}

func TestReadyCheck(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ready", resp["status"])
}

func TestGetServiceInfo(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/info", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp["service_id"])
	assert.Equal(t, "Test Service", resp["service_name"])
}

func TestAuthRequestValidation(t *testing.T) {
	server := newTestServer(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing user_id",
			body:       `{"purpose": "login"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   types.ErrCodeInvalidRequest,
		},
		{
			name:       "missing purpose",
			body:       `{"user_id": "user123"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   types.ErrCodeInvalidRequest,
		},
		{
			name:       "invalid json",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   types.ErrCodeInvalidRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/request", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.Router().ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)

			var resp types.APIError
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, resp.Code)
		})
	}
}

func TestAuthzRequestValidation(t *testing.T) {
	server := newTestServer(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing user_id",
			body:       `{"action": "read", "resource": "documents"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   types.ErrCodeInvalidRequest,
		},
		{
			name:       "missing action",
			body:       `{"user_id": "user123", "resource": "documents"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   types.ErrCodeInvalidRequest,
		},
		{
			name:       "missing resource",
			body:       `{"user_id": "user123", "action": "read"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   types.ErrCodeInvalidRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/authz/request", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.Router().ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)

			var resp types.APIError
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, resp.Code)
		})
	}
}

func TestGetAuthRequestNotFound(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/auth/request/nonexistent", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp types.APIError
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, types.ErrCodeNotFound, resp.Code)
}

func TestGetAuthzRequestNotFound(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/authz/request/nonexistent", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp types.APIError
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, types.ErrCodeNotFound, resp.Code)
}

func TestListContractsNoStore(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/contracts", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestGetContractNoStore(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/contracts/test-contract", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestGenerateInviteNoNegotiator(t *testing.T) {
	server := newTestServer(t)

	body := `{"offering_id": "test-offering"}`
	req := httptest.NewRequest("POST", "/api/v1/contracts/invite", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestCancelContractNoNegotiator(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/v1/contracts/test-contract", nil)
	w := httptest.NewRecorder()

	server.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()

	data := map[string]string{"key": "value"}
	writeJSON(w, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp map[string]string
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "value", resp["key"])
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "test error")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp types.APIError
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, types.ErrCodeInvalidRequest, resp.Code)
	assert.Equal(t, "test error", resp.Message)
}

func TestCORSMiddleware(t *testing.T) {
	handler := CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test preflight
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))

	// Test regular request
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}
