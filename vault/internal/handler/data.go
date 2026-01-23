package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// DataHandler handles data requests from the service API
// and responses from users.
type DataHandler struct {
	mu           sync.RWMutex
	engine       *Engine
	pending      map[string]*DataRequest
	callbacks    map[string]DataCallback
	timeout      time.Duration
	offlineGrace time.Duration
}

// DataRequest represents a pending data request.
type DataRequest struct {
	RequestID    string                 `json:"request_id"`
	UserID       string                 `json:"user_id"`
	RequestType  DataRequestType        `json:"request_type"`
	DataTypes    []string               `json:"data_types,omitempty"`    // For browse_metadata
	DataPaths    []string               `json:"data_paths,omitempty"`    // For request_data
	Purpose      string                 `json:"purpose"`
	Context      map[string]interface{} `json:"context,omitempty"`
	ExpiresAt    time.Time              `json:"expires_at"`
	OfflineGrace time.Duration          `json:"offline_grace,omitempty"`
	CallbackURL  string                 `json:"callback_url,omitempty"`
	Status       types.RequestStatus    `json:"status"`
	CreatedAt    time.Time              `json:"created_at"`
	RespondedAt  *time.Time             `json:"responded_at,omitempty"`
	Response     *DataResponse          `json:"response,omitempty"`
}

// DataRequestType indicates the type of data request.
type DataRequestType string

const (
	// DataRequestBrowse requests metadata about available data.
	DataRequestBrowse DataRequestType = "browse_metadata"
	// DataRequestData requests specific data fields.
	DataRequestData DataRequestType = "request_data"
)

// DataResponse represents a user's response to a data request.
type DataResponse struct {
	RequestID   string                 `json:"request_id"`
	Status      types.RequestStatus    `json:"status"`
	Data        map[string]interface{} `json:"data,omitempty"`         // Returned data
	Metadata    []DataTypeMetadata     `json:"metadata,omitempty"`     // For browse responses
	Timestamp   time.Time              `json:"timestamp"`
	Signature   *types.Signature       `json:"signature,omitempty"`
	Constraints map[string]interface{} `json:"constraints,omitempty"` // User-imposed constraints
}

// DataTypeMetadata describes a type of data the user can share.
type DataTypeMetadata struct {
	DataType    string `json:"data_type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Available   bool   `json:"available"`
}

// DataCallback is called when a data request receives a response.
type DataCallback func(request *DataRequest, response *DataResponse)

// DataHandlerConfig holds configuration for the data handler.
type DataHandlerConfig struct {
	Engine       *Engine
	Timeout      time.Duration
	OfflineGrace time.Duration
}

// NewDataHandler creates a new data handler.
func NewDataHandler(cfg DataHandlerConfig) *DataHandler {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	offlineGrace := cfg.OfflineGrace
	if offlineGrace == 0 {
		offlineGrace = 24 * time.Hour
	}

	h := &DataHandler{
		engine:       cfg.Engine,
		pending:      make(map[string]*DataRequest),
		callbacks:    make(map[string]DataCallback),
		timeout:      timeout,
		offlineGrace: offlineGrace,
	}

	return h
}

// EventType returns the event type this handler processes.
func (h *DataHandler) EventType() string {
	return "data.response"
}

// HandleRequest processes an incoming data response from a user.
func (h *DataHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	// Parse the response
	var dataResp DataResponse
	if err := json.Unmarshal(req.Payload, &dataResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: "invalid data response format",
			},
		}, nil
	}

	// Process the response
	if err := h.HandleResponse(req.UserID, dataResp.RequestID, &dataResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: err.Error(),
			},
		}, nil
	}

	return &Response{
		Status: "ok",
	}, nil
}

// BrowseMetadata requests metadata about available data types from a user.
func (h *DataHandler) BrowseMetadata(ctx context.Context, userID string, dataTypes []string, opts ...DataOption) (*DataRequest, error) {
	return h.createRequest(ctx, userID, DataRequestBrowse, dataTypes, nil, opts...)
}

// RequestData requests specific data from a user.
func (h *DataHandler) RequestData(ctx context.Context, userID string, dataPaths []string, purpose string, opts ...DataOption) (*DataRequest, error) {
	opts = append(opts, WithDataPurpose(purpose))
	return h.createRequest(ctx, userID, DataRequestData, nil, dataPaths, opts...)
}

// createRequest creates a new data request.
func (h *DataHandler) createRequest(ctx context.Context, userID string, reqType DataRequestType, dataTypes, dataPaths []string, opts ...DataOption) (*DataRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Determine required capability based on request type
	var requiredCap types.Capability
	switch reqType {
	case DataRequestBrowse:
		requiredCap = types.CapabilityBrowseData
	case DataRequestData:
		requiredCap = types.CapabilityRequestData
	}

	// Verify contract allows the requested capability
	if !contract.VerifyCapability(userContract, requiredCap) {
		return nil, fmt.Errorf("contract does not grant %s capability", requiredCap)
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &DataRequest{
		RequestID:    requestID,
		UserID:       userID,
		RequestType:  reqType,
		DataTypes:    dataTypes,
		DataPaths:    dataPaths,
		ExpiresAt:    now.Add(h.timeout),
		OfflineGrace: h.offlineGrace,
		Status:       types.RequestStatusPending,
		CreatedAt:    now,
	}

	// Apply options
	for _, opt := range opts {
		opt(request)
	}

	// Store pending request
	h.mu.Lock()
	h.pending[requestID] = request
	h.mu.Unlock()

	// Send request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.DataRequest{
			RequestID:    requestID,
			UserID:       userID,
			RequestType:  string(reqType),
			DataTypes:    dataTypes,
			DataPaths:    dataPaths,
			Purpose:      request.Purpose,
			Context:      request.Context,
			ExpiresAt:    request.ExpiresAt,
			OfflineGrace: request.OfflineGrace,
			CallbackURL:  request.CallbackURL,
		})

		// Get user's public key from contract
		var userPubKey [32]byte
		// In production, decode from userContract.UserConnectionKey

		if err := h.engine.messageSpace.SendToUser(userID, "data.request", payload, userPubKey); err != nil {
			// Log error but don't fail - request is created
		}
	}

	return request, nil
}

// HandleResponse processes a user's response to a data request.
func (h *DataHandler) HandleResponse(userID string, requestID string, response *DataResponse) error {
	h.mu.Lock()
	request, exists := h.pending[requestID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("unknown request ID: %s", requestID)
	}

	// Verify request belongs to this user
	if request.UserID != userID {
		h.mu.Unlock()
		return fmt.Errorf("request belongs to different user")
	}

	// Check if expired
	if time.Now().After(request.ExpiresAt) {
		graceEnd := request.ExpiresAt.Add(request.OfflineGrace)
		if time.Now().After(graceEnd) {
			request.Status = types.RequestStatusExpired
			h.mu.Unlock()
			return fmt.Errorf("request has expired")
		}
		if response.Status == types.RequestStatusApproved {
			response.Status = types.RequestStatusOffline
		}
	}

	// Update request
	now := time.Now().UTC()
	request.Status = response.Status
	request.RespondedAt = &now
	request.Response = response

	// Get callback if registered
	callback := h.callbacks[requestID]
	delete(h.callbacks, requestID)
	delete(h.pending, requestID)
	h.mu.Unlock()

	// Execute callback
	if callback != nil {
		callback(request, response)
	}

	// Call webhook if configured
	if request.CallbackURL != "" {
		go h.callWebhook(request, response)
	}

	return nil
}

// GetRequest returns a pending data request by ID.
func (h *DataHandler) GetRequest(requestID string) (*DataRequest, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	req, ok := h.pending[requestID]
	return req, ok
}

// SetCallback registers a callback for when a request receives a response.
func (h *DataHandler) SetCallback(requestID string, callback DataCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks[requestID] = callback
}

// callWebhook sends the data result to a webhook URL.
func (h *DataHandler) callWebhook(request *DataRequest, response *DataResponse) {
	payload, _ := json.Marshal(map[string]interface{}{
		"type":         "data.response",
		"request_id":   request.RequestID,
		"user_id":      request.UserID,
		"request_type": request.RequestType,
		"status":       response.Status,
		"data":         response.Data,
		"metadata":     response.Metadata,
		"timestamp":    response.Timestamp,
	})

	req, err := http.NewRequest("POST", request.CallbackURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_ = payload // Use in actual implementation
}

// Cleanup removes expired pending requests.
func (h *DataHandler) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for id, req := range h.pending {
		graceEnd := req.ExpiresAt.Add(req.OfflineGrace)
		if now.After(graceEnd) {
			delete(h.pending, id)
			delete(h.callbacks, id)
		}
	}
}

// DataOption configures a data request.
type DataOption func(*DataRequest)

// WithDataContext adds context to the data request.
func WithDataContext(ctx map[string]interface{}) DataOption {
	return func(r *DataRequest) {
		r.Context = ctx
	}
}

// WithDataPurpose sets the purpose of the data request.
func WithDataPurpose(purpose string) DataOption {
	return func(r *DataRequest) {
		r.Purpose = purpose
	}
}

// WithDataTimeout sets a custom timeout for the data request.
func WithDataTimeout(timeout time.Duration) DataOption {
	return func(r *DataRequest) {
		r.ExpiresAt = r.CreatedAt.Add(timeout)
	}
}

// WithDataCallback sets the webhook callback URL.
func WithDataCallback(url string) DataOption {
	return func(r *DataRequest) {
		r.CallbackURL = url
	}
}

// WithDataOfflineGrace sets the offline grace period.
func WithDataOfflineGrace(grace time.Duration) DataOption {
	return func(r *DataRequest) {
		r.OfflineGrace = grace
	}
}
