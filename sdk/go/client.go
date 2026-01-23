package vettid

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ServiceVaultClient is the main client for interacting with Service Vault.
type ServiceVaultClient struct {
	config     *VaultConfig
	httpClient *http.Client
}

// NewClient creates a new Service Vault client.
func NewClient(baseURL, apiKey string) *ServiceVaultClient {
	return NewClientWithConfig(&VaultConfig{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
		Timeout: 30 * time.Second,
		Retries: 3,
		Headers: make(map[string]string),
	})
}

// NewClientWithConfig creates a new Service Vault client with custom configuration.
func NewClientWithConfig(config *VaultConfig) *ServiceVaultClient {
	return &ServiceVaultClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// ============================================================================
// Authentication
// ============================================================================

// AuthRequestOptions contains options for an authentication request.
type AuthRequestOptions struct {
	Context      map[string]interface{}
	ExpiresIn    int    // Seconds until expiry
	OfflineGrace int    // Offline grace period in ms
	CallbackURL  string
}

// RequestAuth requests authentication from a user.
func (c *ServiceVaultClient) RequestAuth(ctx context.Context, userID, purpose string, opts *AuthRequestOptions) (string, error) {
	body := map[string]interface{}{
		"user_id": userID,
		"purpose": purpose,
	}
	if opts != nil {
		if opts.Context != nil {
			body["context"] = opts.Context
		}
		if opts.ExpiresIn > 0 {
			body["expires_in"] = opts.ExpiresIn
		}
		if opts.OfflineGrace > 0 {
			body["offline_grace"] = opts.OfflineGrace
		}
		if opts.CallbackURL != "" {
			body["callback_url"] = opts.CallbackURL
		}
	}

	resp, err := c.post(ctx, "/api/v1/auth/request", body)
	if err != nil {
		return "", err
	}
	return resp["request_id"].(string), nil
}

// GetAuthRequest gets the status of an authentication request.
func (c *ServiceVaultClient) GetAuthRequest(ctx context.Context, requestID string) (*AuthResponse, error) {
	resp, err := c.get(ctx, "/api/v1/auth/request/"+requestID, nil)
	if err != nil {
		return nil, err
	}

	timestamp, _ := time.Parse(time.RFC3339, resp["timestamp"].(string))
	result := &AuthResponse{
		RequestID:  resp["request_id"].(string),
		Status:     RequestStatus(resp["status"].(string)),
		UserID:     resp["user_id"].(string),
		Timestamp:  timestamp,
		SessionKey: getStringOr(resp, "session_key", ""),
	}
	return result, nil
}

// ============================================================================
// Authorization
// ============================================================================

// AuthzRequestOptions contains options for an authorization request.
type AuthzRequestOptions struct {
	Context      map[string]interface{}
	ExpiresIn    int
	OfflineGrace int
	CallbackURL  string
}

// RequestAuthz requests authorization for an action from a user.
func (c *ServiceVaultClient) RequestAuthz(ctx context.Context, userID, action, resource string, opts *AuthzRequestOptions) (string, error) {
	body := map[string]interface{}{
		"user_id":  userID,
		"action":   action,
		"resource": resource,
	}
	if opts != nil {
		if opts.Context != nil {
			body["context"] = opts.Context
		}
		if opts.ExpiresIn > 0 {
			body["expires_in"] = opts.ExpiresIn
		}
		if opts.OfflineGrace > 0 {
			body["offline_grace"] = opts.OfflineGrace
		}
		if opts.CallbackURL != "" {
			body["callback_url"] = opts.CallbackURL
		}
	}

	resp, err := c.post(ctx, "/api/v1/authz/request", body)
	if err != nil {
		return "", err
	}
	return resp["request_id"].(string), nil
}

// GetAuthzRequest gets the status of an authorization request.
func (c *ServiceVaultClient) GetAuthzRequest(ctx context.Context, requestID string) (*AuthzResponse, error) {
	resp, err := c.get(ctx, "/api/v1/authz/request/"+requestID, nil)
	if err != nil {
		return nil, err
	}

	timestamp, _ := time.Parse(time.RFC3339, resp["timestamp"].(string))
	result := &AuthzResponse{
		RequestID: resp["request_id"].(string),
		Status:    RequestStatus(resp["status"].(string)),
		UserID:    resp["user_id"].(string),
		Action:    resp["action"].(string),
		Resource:  resp["resource"].(string),
		Timestamp: timestamp,
	}
	if expiresAt, ok := resp["expires_at"].(string); ok && expiresAt != "" {
		t, _ := time.Parse(time.RFC3339, expiresAt)
		result.ExpiresAt = &t
	}
	return result, nil
}

// ============================================================================
// Contracts
// ============================================================================

// ListContractsOptions contains options for listing contracts.
type ListContractsOptions struct {
	Status string
	Limit  int
	Offset int
}

// ListContracts lists all contracts.
func (c *ServiceVaultClient) ListContracts(ctx context.Context, opts *ListContractsOptions) ([]*ConnectionContract, error) {
	params := url.Values{}
	if opts != nil {
		if opts.Status != "" {
			params.Set("status", opts.Status)
		}
		if opts.Limit > 0 {
			params.Set("limit", fmt.Sprintf("%d", opts.Limit))
		}
		if opts.Offset > 0 {
			params.Set("offset", fmt.Sprintf("%d", opts.Offset))
		}
	}

	resp, err := c.get(ctx, "/api/v1/contracts", params)
	if err != nil {
		return nil, err
	}

	contractsData, _ := resp["contracts"].([]interface{})
	contracts := make([]*ConnectionContract, len(contractsData))
	for i, cd := range contractsData {
		contracts[i] = mapContract(cd.(map[string]interface{}))
	}
	return contracts, nil
}

// GetContract gets a specific contract.
func (c *ServiceVaultClient) GetContract(ctx context.Context, contractID string) (*ConnectionContract, error) {
	resp, err := c.get(ctx, "/api/v1/contracts/"+contractID, nil)
	if err != nil {
		return nil, err
	}
	return mapContract(resp), nil
}

// InviteResult contains the result of generating an invite.
type InviteResult struct {
	InviteURL  string
	InviteCode string
	ExpiresAt  time.Time
}

// GenerateInvite generates a connection invite.
func (c *ServiceVaultClient) GenerateInvite(ctx context.Context, offeringID string, expiresIn int) (*InviteResult, error) {
	body := map[string]interface{}{}
	if offeringID != "" {
		body["offering_id"] = offeringID
	}
	if expiresIn > 0 {
		body["expires_in"] = expiresIn
	}

	resp, err := c.post(ctx, "/api/v1/contracts/invite", body)
	if err != nil {
		return nil, err
	}

	expiresAt, _ := time.Parse(time.RFC3339, resp["expires_at"].(string))
	return &InviteResult{
		InviteURL:  resp["invite_url"].(string),
		InviteCode: resp["invite_code"].(string),
		ExpiresAt:  expiresAt,
	}, nil
}

// CancelContract cancels a contract.
func (c *ServiceVaultClient) CancelContract(ctx context.Context, contractID, reason string) error {
	body := map[string]interface{}{}
	if reason != "" {
		body["reason"] = reason
	}
	_, err := c.delete(ctx, "/api/v1/contracts/"+contractID, body)
	return err
}

// ============================================================================
// Calls
// ============================================================================

// CallOptions contains options for initiating a call.
type CallOptions struct {
	Purpose     string
	Context     map[string]interface{}
	ICEServers  []map[string]interface{}
	Offer       map[string]interface{}
	ExpiresIn   int
	CallbackURL string
}

// CallInitResult contains the result of initiating a call.
type CallInitResult struct {
	CallID    string
	ExpiresAt time.Time
}

// InitiateCall initiates a call to a user.
func (c *ServiceVaultClient) InitiateCall(ctx context.Context, userID string, callType CallType, opts *CallOptions) (*CallInitResult, error) {
	body := map[string]interface{}{
		"user_id": userID,
		"type":    string(callType),
	}
	if opts != nil {
		if opts.Purpose != "" {
			body["purpose"] = opts.Purpose
		}
		if opts.Context != nil {
			body["context"] = opts.Context
		}
		if opts.ICEServers != nil {
			body["ice_servers"] = opts.ICEServers
		}
		if opts.Offer != nil {
			body["offer"] = opts.Offer
		}
		if opts.ExpiresIn > 0 {
			body["expires_in"] = opts.ExpiresIn
		}
		if opts.CallbackURL != "" {
			body["callback_url"] = opts.CallbackURL
		}
	}

	resp, err := c.post(ctx, "/api/v1/call/initiate", body)
	if err != nil {
		return nil, err
	}

	expiresAt, _ := time.Parse(time.RFC3339, resp["expires_at"].(string))
	return &CallInitResult{
		CallID:    resp["call_id"].(string),
		ExpiresAt: expiresAt,
	}, nil
}

// GetCallStatus gets the status of a call.
func (c *ServiceVaultClient) GetCallStatus(ctx context.Context, callID string) (*CallResult, error) {
	resp, err := c.get(ctx, "/api/v1/call/"+callID, nil)
	if err != nil {
		return nil, err
	}

	result := &CallResult{
		CallID:    resp["call_id"].(string),
		RequestID: getStringOr(resp, "request_id", ""),
		Status:    CallStatus(resp["status"].(string)),
	}
	if answer, ok := resp["answer"].(map[string]interface{}); ok {
		result.Answer = answer
	}
	if startedAt, ok := resp["started_at"].(string); ok && startedAt != "" {
		t, _ := time.Parse(time.RFC3339, startedAt)
		result.StartedAt = &t
	}
	if connectedAt, ok := resp["connected_at"].(string); ok && connectedAt != "" {
		t, _ := time.Parse(time.RFC3339, connectedAt)
		result.ConnectedAt = &t
	}
	if endedAt, ok := resp["ended_at"].(string); ok && endedAt != "" {
		t, _ := time.Parse(time.RFC3339, endedAt)
		result.EndedAt = &t
	}
	if duration, ok := resp["duration_seconds"].(float64); ok {
		result.Duration = int(duration)
	}
	return result, nil
}

// EndCall ends an active call.
func (c *ServiceVaultClient) EndCall(ctx context.Context, callID, reason string) error {
	body := map[string]interface{}{}
	if reason != "" {
		body["reason"] = reason
	}
	_, err := c.post(ctx, "/api/v1/call/"+callID+"/end", body)
	return err
}

// ============================================================================
// Payments
// ============================================================================

// PaymentOptions contains options for requesting a payment.
type PaymentOptions struct {
	MerchantInfo   map[string]interface{}
	Items          []map[string]interface{}
	AllowedMethods []string
	RecurringInfo  map[string]interface{}
	CallbackURL    string
	ExpiresIn      int
	Metadata       map[string]interface{}
}

// PaymentRequestResult contains the result of requesting a payment.
type PaymentRequestResult struct {
	RequestID string
	ExpiresAt time.Time
}

// RequestPayment requests a payment from a user.
func (c *ServiceVaultClient) RequestPayment(ctx context.Context, userID string, amount Money, description string, opts *PaymentOptions) (*PaymentRequestResult, error) {
	body := map[string]interface{}{
		"user_id":     userID,
		"amount":      map[string]interface{}{"amount": amount.Amount, "currency": amount.Currency},
		"description": description,
	}
	if opts != nil {
		if opts.MerchantInfo != nil {
			body["merchant_info"] = opts.MerchantInfo
		}
		if opts.Items != nil {
			body["items"] = opts.Items
		}
		if opts.AllowedMethods != nil {
			body["allowed_methods"] = opts.AllowedMethods
		}
		if opts.RecurringInfo != nil {
			body["recurring_info"] = opts.RecurringInfo
		}
		if opts.CallbackURL != "" {
			body["callback_url"] = opts.CallbackURL
		}
		if opts.ExpiresIn > 0 {
			body["expires_in"] = opts.ExpiresIn
		}
		if opts.Metadata != nil {
			body["metadata"] = opts.Metadata
		}
	}

	resp, err := c.post(ctx, "/api/v1/payment/request", body)
	if err != nil {
		return nil, err
	}

	expiresAt, _ := time.Parse(time.RFC3339, resp["expires_at"].(string))
	return &PaymentRequestResult{
		RequestID: resp["request_id"].(string),
		ExpiresAt: expiresAt,
	}, nil
}

// GetPaymentStatus gets the status of a payment request.
func (c *ServiceVaultClient) GetPaymentStatus(ctx context.Context, requestID string) (*PaymentResult, error) {
	resp, err := c.get(ctx, "/api/v1/payment/request/"+requestID, nil)
	if err != nil {
		return nil, err
	}

	result := &PaymentResult{
		RequestID:      resp["request_id"].(string),
		Status:         PaymentStatus(resp["status"].(string)),
		PaymentID:      getStringOr(resp, "payment_id", ""),
		TransactionRef: getStringOr(resp, "transaction_id", ""),
		ReceiptURL:     getStringOr(resp, "receipt_url", ""),
		FailureReason:  getStringOr(resp, "failure_reason", ""),
	}
	if completedAt, ok := resp["completed_at"].(string); ok && completedAt != "" {
		t, _ := time.Parse(time.RFC3339, completedAt)
		result.CompletedAt = &t
	}
	if failedAt, ok := resp["failed_at"].(string); ok && failedAt != "" {
		t, _ := time.Parse(time.RFC3339, failedAt)
		result.FailedAt = &t
	}
	return result, nil
}

// CompletePayment marks a payment as completed.
func (c *ServiceVaultClient) CompletePayment(ctx context.Context, requestID, transactionID, receiptURL string) error {
	body := map[string]interface{}{
		"transaction_id": transactionID,
	}
	if receiptURL != "" {
		body["receipt_url"] = receiptURL
	}
	_, err := c.post(ctx, "/api/v1/payment/request/"+requestID+"/complete", body)
	return err
}

// FailPayment marks a payment as failed.
func (c *ServiceVaultClient) FailPayment(ctx context.Context, requestID, reason string) error {
	body := map[string]interface{}{
		"reason": reason,
	}
	_, err := c.post(ctx, "/api/v1/payment/request/"+requestID+"/fail", body)
	return err
}

// RefundPayment refunds a payment.
func (c *ServiceVaultClient) RefundPayment(ctx context.Context, requestID string, amount int64, reason string) error {
	body := map[string]interface{}{}
	if amount > 0 {
		body["amount"] = amount
	}
	if reason != "" {
		body["reason"] = reason
	}
	_, err := c.post(ctx, "/api/v1/payment/request/"+requestID+"/refund", body)
	return err
}

// ============================================================================
// Secrets
// ============================================================================

// StoreSecretOptions contains options for storing a secret.
type StoreSecretOptions struct {
	Description string
	Metadata    map[string]interface{}
	CallbackURL string
	ExpiresIn   int
}

// SecretRequestResult contains the result of a secret operation.
type SecretRequestResult struct {
	RequestID string
	ExpiresAt time.Time
}

// StoreSecret stores a secret in the user's vault.
func (c *ServiceVaultClient) StoreSecret(ctx context.Context, userID, secretType, name string, data []byte, opts *StoreSecretOptions) (*SecretRequestResult, error) {
	body := map[string]interface{}{
		"user_id":     userID,
		"secret_type": secretType,
		"name":        name,
		"data":        base64.StdEncoding.EncodeToString(data),
	}
	if opts != nil {
		if opts.Description != "" {
			body["description"] = opts.Description
		}
		if opts.Metadata != nil {
			body["metadata"] = opts.Metadata
		}
		if opts.CallbackURL != "" {
			body["callback_url"] = opts.CallbackURL
		}
		if opts.ExpiresIn > 0 {
			body["expires_in"] = opts.ExpiresIn
		}
	}

	resp, err := c.post(ctx, "/api/v1/secrets/store", body)
	if err != nil {
		return nil, err
	}

	expiresAt, _ := time.Parse(time.RFC3339, resp["expires_at"].(string))
	return &SecretRequestResult{
		RequestID: resp["request_id"].(string),
		ExpiresAt: expiresAt,
	}, nil
}

// RetrieveSecretOptions contains options for retrieving a secret.
type RetrieveSecretOptions struct {
	Purpose     string
	CallbackURL string
	ExpiresIn   int
}

// RetrieveSecret retrieves a secret from the user's vault.
func (c *ServiceVaultClient) RetrieveSecret(ctx context.Context, userID, secretID string, opts *RetrieveSecretOptions) (*SecretRequestResult, error) {
	body := map[string]interface{}{
		"user_id":   userID,
		"secret_id": secretID,
	}
	if opts != nil {
		if opts.Purpose != "" {
			body["purpose"] = opts.Purpose
		}
		if opts.CallbackURL != "" {
			body["callback_url"] = opts.CallbackURL
		}
		if opts.ExpiresIn > 0 {
			body["expires_in"] = opts.ExpiresIn
		}
	}

	resp, err := c.post(ctx, "/api/v1/secrets/retrieve", body)
	if err != nil {
		return nil, err
	}

	expiresAt, _ := time.Parse(time.RFC3339, resp["expires_at"].(string))
	return &SecretRequestResult{
		RequestID: resp["request_id"].(string),
		ExpiresAt: expiresAt,
	}, nil
}

// DeleteSecret deletes a secret from the user's vault.
func (c *ServiceVaultClient) DeleteSecret(ctx context.Context, userID, secretID, callbackURL string) (*SecretRequestResult, error) {
	params := url.Values{}
	params.Set("user_id", userID)
	if callbackURL != "" {
		params.Set("callback_url", callbackURL)
	}

	resp, err := c.deleteWithParams(ctx, "/api/v1/secrets/"+secretID, params)
	if err != nil {
		return nil, err
	}

	expiresAt, _ := time.Parse(time.RFC3339, resp["expires_at"].(string))
	return &SecretRequestResult{
		RequestID: resp["request_id"].(string),
		ExpiresAt: expiresAt,
	}, nil
}

// ============================================================================
// Health
// ============================================================================

// Health checks if the vault is healthy.
func (c *ServiceVaultClient) Health(ctx context.Context) bool {
	resp, err := c.get(ctx, "/health", nil)
	if err != nil {
		return false
	}
	status, _ := resp["status"].(string)
	return status == "ok"
}

// ============================================================================
// Private HTTP methods
// ============================================================================

func (c *ServiceVaultClient) get(ctx context.Context, path string, params url.Values) (map[string]interface{}, error) {
	urlStr := c.config.BaseURL + path
	if params != nil && len(params) > 0 {
		urlStr += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	return c.doRequest(req)
}

func (c *ServiceVaultClient) post(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	return c.doRequest(req)
}

func (c *ServiceVaultClient) delete(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil && len(body) > 0 {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.config.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	return c.doRequest(req)
}

func (c *ServiceVaultClient) deleteWithParams(ctx context.Context, path string, params url.Values) (map[string]interface{}, error) {
	urlStr := c.config.BaseURL + path
	if params != nil && len(params) > 0 {
		urlStr += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, urlStr, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	return c.doRequest(req)
}

func (c *ServiceVaultClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}
}

func (c *ServiceVaultClient) doRequest(req *http.Request) (map[string]interface{}, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(body, &apiErr); err != nil {
			apiErr = APIError{
				Code:    "internal_error",
				Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
			}
		}
		return nil, &apiErr
	}

	var result map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ============================================================================
// Helpers
// ============================================================================

func mapContract(data map[string]interface{}) *ConnectionContract {
	createdAt, _ := time.Parse(time.RFC3339, data["created_at"].(string))
	contract := &ConnectionContract{
		ContractID: data["contract_id"].(string),
		UserID:     data["user_id"].(string),
		ServiceID:  data["service_id"].(string),
		OfferingID: data["offering_id"].(string),
		Status:     ContractStatus(data["status"].(string)),
		CreatedAt:  createdAt,
	}
	if activatedAt, ok := data["activated_at"].(string); ok && activatedAt != "" {
		t, _ := time.Parse(time.RFC3339, activatedAt)
		contract.ActivatedAt = &t
	}
	if cancelledAt, ok := data["cancelled_at"].(string); ok && cancelledAt != "" {
		t, _ := time.Parse(time.RFC3339, cancelledAt)
		contract.CancelledAt = &t
	}
	return contract
}

func getStringOr(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}
