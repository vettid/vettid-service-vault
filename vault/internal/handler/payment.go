package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// PaymentHandler handles payment requests and processing.
type PaymentHandler struct {
	mu           sync.RWMutex
	engine       *Engine
	pending      map[string]*PaymentRequest
	completed    map[string]*Payment
	callbacks    map[string]PaymentCallback
	timeout      time.Duration
	maxHistory   int
}

// PaymentRequest represents a pending payment request.
type PaymentRequest struct {
	RequestID      string                 `json:"request_id"`
	UserID         string                 `json:"user_id"`
	Amount         Money                  `json:"amount"`
	Description    string                 `json:"description"`
	MerchantInfo   MerchantInfo           `json:"merchant_info"`
	Items          []PaymentItem          `json:"items,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	AllowedMethods []PaymentMethodType    `json:"allowed_methods,omitempty"`
	RecurringInfo  *RecurringInfo         `json:"recurring_info,omitempty"`
	ExpiresAt      time.Time              `json:"expires_at"`
	CallbackURL    string                 `json:"callback_url,omitempty"`
	ReturnURL      string                 `json:"return_url,omitempty"`
	Status         types.RequestStatus    `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	Payment        *Payment               `json:"payment,omitempty"`
}

// Payment represents a completed or in-progress payment.
type Payment struct {
	PaymentID       string                 `json:"payment_id"`
	RequestID       string                 `json:"request_id"`
	UserID          string                 `json:"user_id"`
	Amount          Money                  `json:"amount"`
	Status          PaymentStatus          `json:"status"`
	Method          PaymentMethodType      `json:"method"`
	MethodDetails   map[string]interface{} `json:"method_details,omitempty"`
	TransactionRef  string                 `json:"transaction_ref,omitempty"`
	ReceiptURL      string                 `json:"receipt_url,omitempty"`
	RefundedAmount  *Money                 `json:"refunded_amount,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	UserSignature   *types.Signature       `json:"user_signature,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	FailedAt        *time.Time             `json:"failed_at,omitempty"`
	FailureReason   string                 `json:"failure_reason,omitempty"`
}

// Money represents a monetary amount.
type Money struct {
	Amount   int64  `json:"amount"`   // Amount in smallest currency unit (cents, etc.)
	Currency string `json:"currency"` // ISO 4217 currency code
}

// MerchantInfo contains merchant identification.
type MerchantInfo struct {
	MerchantID   string `json:"merchant_id"`
	MerchantName string `json:"merchant_name"`
	MerchantURL  string `json:"merchant_url,omitempty"`
	MerchantLogo string `json:"merchant_logo,omitempty"`
	Category     string `json:"category,omitempty"`
}

// PaymentItem represents a line item in a payment.
type PaymentItem struct {
	ItemID      string `json:"item_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Quantity    int    `json:"quantity"`
	UnitPrice   Money  `json:"unit_price"`
	TotalPrice  Money  `json:"total_price"`
	ImageURL    string `json:"image_url,omitempty"`
}

// RecurringInfo describes recurring payment terms.
type RecurringInfo struct {
	Interval      string     `json:"interval"` // day, week, month, year
	IntervalCount int        `json:"interval_count"`
	StartDate     time.Time  `json:"start_date"`
	EndDate       *time.Time `json:"end_date,omitempty"`
	TrialDays     int        `json:"trial_days,omitempty"`
}

// PaymentMethodType indicates the payment method.
type PaymentMethodType string

const (
	PaymentMethodCard         PaymentMethodType = "card"
	PaymentMethodBankTransfer PaymentMethodType = "bank_transfer"
	PaymentMethodWallet       PaymentMethodType = "wallet"
	PaymentMethodCrypto       PaymentMethodType = "crypto"
)

// PaymentStatus indicates the state of a payment.
type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusProcessing PaymentStatus = "processing"
	PaymentStatusCompleted  PaymentStatus = "completed"
	PaymentStatusFailed     PaymentStatus = "failed"
	PaymentStatusCancelled  PaymentStatus = "cancelled"
	PaymentStatusRefunded   PaymentStatus = "refunded"
	PaymentStatusPartial    PaymentStatus = "partial_refund"
)

// PaymentCallback is called when a payment request receives a response.
type PaymentCallback func(request *PaymentRequest, payment *Payment)

// PaymentHandlerConfig holds configuration for the payment handler.
type PaymentHandlerConfig struct {
	Engine     *Engine
	Timeout    time.Duration
	MaxHistory int
}

// NewPaymentHandler creates a new payment handler.
func NewPaymentHandler(cfg PaymentHandlerConfig) *PaymentHandler {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}

	maxHistory := cfg.MaxHistory
	if maxHistory == 0 {
		maxHistory = 1000
	}

	return &PaymentHandler{
		engine:     cfg.Engine,
		pending:    make(map[string]*PaymentRequest),
		completed:  make(map[string]*Payment),
		callbacks:  make(map[string]PaymentCallback),
		timeout:    timeout,
		maxHistory: maxHistory,
	}
}

// EventType returns the event type this handler processes.
func (h *PaymentHandler) EventType() string {
	return "payment.response"
}

// HandleRequest processes an incoming payment response from a user.
func (h *PaymentHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	var paymentResp struct {
		RequestID     string                 `json:"request_id"`
		Status        string                 `json:"status"` // approved, denied
		Method        PaymentMethodType      `json:"method,omitempty"`
		MethodDetails map[string]interface{} `json:"method_details,omitempty"`
		Signature     *types.Signature       `json:"signature,omitempty"`
	}

	if err := json.Unmarshal(req.Payload, &paymentResp); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: "invalid payment response format",
			},
		}, nil
	}

	if err := h.HandleResponse(req.UserID, paymentResp.RequestID, paymentResp.Status, paymentResp.Method, paymentResp.MethodDetails, paymentResp.Signature); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: err.Error(),
			},
		}, nil
	}

	return &Response{Status: "ok"}, nil
}

// RequestPayment sends a payment request to a user.
func (h *PaymentHandler) RequestPayment(ctx context.Context, userID string, amount Money, description string, merchant MerchantInfo, opts ...PaymentOption) (*PaymentRequest, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows payments
	if !contract.VerifyCapability(userContract, types.CapabilityPayment) {
		return nil, fmt.Errorf("contract does not grant payment capability")
	}

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	request := &PaymentRequest{
		RequestID:    requestID,
		UserID:       userID,
		Amount:       amount,
		Description:  description,
		MerchantInfo: merchant,
		ExpiresAt:    now.Add(h.timeout),
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

	// Send payment request to user via MessageSpace
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(types.PaymentRequest{
			RequestID:      requestID,
			UserID:         userID,
			Amount:         types.Money{Amount: amount.Amount, Currency: amount.Currency},
			Description:    description,
			MerchantInfo:   toTypesMerchantInfo(merchant),
			Items:          toTypesPaymentItems(request.Items),
			AllowedMethods: toTypesPaymentMethods(request.AllowedMethods),
			RecurringInfo:  toTypesRecurringInfo(request.RecurringInfo),
			ExpiresAt:      request.ExpiresAt,
		})

		// Get user's public key from contract
		var userPubKey [32]byte
		// In production, decode from userContract.UserConnectionKey

		if err := h.engine.messageSpace.SendToUser(userID, "payment.request", payload, userPubKey); err != nil {
			// Log error but don't fail - request is created
		}
	}

	return request, nil
}

// HandleResponse processes a user's response to a payment request.
func (h *PaymentHandler) HandleResponse(userID, requestID, status string, method PaymentMethodType, methodDetails map[string]interface{}, signature *types.Signature) error {
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
		request.Status = types.RequestStatusExpired
		h.mu.Unlock()
		return fmt.Errorf("request has expired")
	}

	var payment *Payment

	switch status {
	case "approved":
		request.Status = types.RequestStatusApproved

		// Create payment record
		paymentID := generatePaymentID()
		now := time.Now().UTC()
		payment = &Payment{
			PaymentID:     paymentID,
			RequestID:     requestID,
			UserID:        userID,
			Amount:        request.Amount,
			Status:        PaymentStatusProcessing,
			Method:        method,
			MethodDetails: methodDetails,
			Metadata:      request.Metadata,
			UserSignature: signature,
			CreatedAt:     now,
		}

		request.Payment = payment
		h.completed[paymentID] = payment

	case "denied":
		request.Status = types.RequestStatusDenied

	default:
		h.mu.Unlock()
		return fmt.Errorf("invalid status: %s", status)
	}

	// Get callback
	callback := h.callbacks[requestID]
	delete(h.callbacks, requestID)
	delete(h.pending, requestID)
	h.mu.Unlock()

	// Execute callback
	if callback != nil {
		callback(request, payment)
	}

	// Call webhook if configured
	if request.CallbackURL != "" {
		go h.callPaymentWebhook(request, payment)
	}

	return nil
}

// CompletePayment marks a payment as completed (after processing).
func (h *PaymentHandler) CompletePayment(ctx context.Context, paymentID, transactionRef, receiptURL string) (*Payment, error) {
	h.mu.Lock()
	payment, exists := h.completed[paymentID]
	if !exists {
		h.mu.Unlock()
		return nil, fmt.Errorf("payment not found")
	}

	if payment.Status != PaymentStatusProcessing {
		h.mu.Unlock()
		return nil, fmt.Errorf("payment is not processing: %s", payment.Status)
	}

	now := time.Now().UTC()
	payment.Status = PaymentStatusCompleted
	payment.CompletedAt = &now
	payment.TransactionRef = transactionRef
	payment.ReceiptURL = receiptURL
	h.mu.Unlock()

	// Notify user of completion
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":            "payment.completed",
			"payment_id":      paymentID,
			"transaction_ref": transactionRef,
			"receipt_url":     receiptURL,
		})

		var userPubKey [32]byte
		h.engine.messageSpace.SendToUser(payment.UserID, "payment.notification", payload, userPubKey)
	}

	return payment, nil
}

// FailPayment marks a payment as failed.
func (h *PaymentHandler) FailPayment(ctx context.Context, paymentID, reason string) (*Payment, error) {
	h.mu.Lock()
	payment, exists := h.completed[paymentID]
	if !exists {
		h.mu.Unlock()
		return nil, fmt.Errorf("payment not found")
	}

	if payment.Status != PaymentStatusProcessing {
		h.mu.Unlock()
		return nil, fmt.Errorf("payment is not processing: %s", payment.Status)
	}

	now := time.Now().UTC()
	payment.Status = PaymentStatusFailed
	payment.FailedAt = &now
	payment.FailureReason = reason
	h.mu.Unlock()

	// Notify user of failure
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":       "payment.failed",
			"payment_id": paymentID,
			"reason":     reason,
		})

		var userPubKey [32]byte
		h.engine.messageSpace.SendToUser(payment.UserID, "payment.notification", payload, userPubKey)
	}

	return payment, nil
}

// RefundPayment processes a refund for a completed payment.
func (h *PaymentHandler) RefundPayment(ctx context.Context, paymentID string, amount *Money, reason string) (*Payment, error) {
	h.mu.Lock()
	payment, exists := h.completed[paymentID]
	if !exists {
		h.mu.Unlock()
		return nil, fmt.Errorf("payment not found")
	}

	if payment.Status != PaymentStatusCompleted && payment.Status != PaymentStatusPartial {
		h.mu.Unlock()
		return nil, fmt.Errorf("payment cannot be refunded: %s", payment.Status)
	}

	// Default to full refund
	refundAmount := payment.Amount
	if amount != nil {
		refundAmount = *amount
	}

	// Check if partial or full
	if refundAmount.Amount >= payment.Amount.Amount {
		payment.Status = PaymentStatusRefunded
	} else {
		payment.Status = PaymentStatusPartial
	}
	payment.RefundedAmount = &refundAmount
	h.mu.Unlock()

	// Notify user of refund
	if h.engine.messageSpace != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":       "payment.refunded",
			"payment_id": paymentID,
			"amount":     refundAmount,
			"reason":     reason,
		})

		var userPubKey [32]byte
		h.engine.messageSpace.SendToUser(payment.UserID, "payment.notification", payload, userPubKey)
	}

	return payment, nil
}

// GetPaymentRequest returns a pending payment request by ID.
func (h *PaymentHandler) GetPaymentRequest(requestID string) (*PaymentRequest, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	req, ok := h.pending[requestID]
	return req, ok
}

// GetPayment returns a payment by ID.
func (h *PaymentHandler) GetPayment(paymentID string) (*Payment, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	payment, ok := h.completed[paymentID]
	return payment, ok
}

// SetCallback registers a callback for when a payment request receives a response.
func (h *PaymentHandler) SetCallback(requestID string, callback PaymentCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callbacks[requestID] = callback
}

// callPaymentWebhook sends the payment result to a webhook URL.
func (h *PaymentHandler) callPaymentWebhook(request *PaymentRequest, payment *Payment) {
	payload := map[string]interface{}{
		"type":        "payment.response",
		"request_id":  request.RequestID,
		"user_id":     request.UserID,
		"status":      request.Status,
		"amount":      request.Amount,
		"description": request.Description,
	}

	if payment != nil {
		payload["payment_id"] = payment.PaymentID
		payload["payment_status"] = payment.Status
		payload["method"] = payment.Method
	}

	h.engine.callWebhook(request.CallbackURL, "payment.response", payload)
}

// Cleanup removes expired pending requests.
func (h *PaymentHandler) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for id, req := range h.pending {
		if now.After(req.ExpiresAt) {
			delete(h.pending, id)
			delete(h.callbacks, id)
		}
	}

	// Trim completed history if needed
	if len(h.completed) > h.maxHistory {
		// Find oldest to remove (simple approach)
		oldest := make([]*Payment, 0, len(h.completed))
		for _, p := range h.completed {
			oldest = append(oldest, p)
		}

		// Sort by creation time
		for i := range oldest {
			for j := i + 1; j < len(oldest); j++ {
				if oldest[i].CreatedAt.After(oldest[j].CreatedAt) {
					oldest[i], oldest[j] = oldest[j], oldest[i]
				}
			}
		}

		// Remove oldest
		toRemove := len(h.completed) - h.maxHistory
		for i := 0; i < toRemove && i < len(oldest); i++ {
			delete(h.completed, oldest[i].PaymentID)
		}
	}
}

// Helper functions

func generatePaymentID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "pay_" + base64.RawURLEncoding.EncodeToString(b)
}

func toTypesMerchantInfo(m MerchantInfo) types.MerchantInfo {
	return types.MerchantInfo{
		MerchantID:   m.MerchantID,
		MerchantName: m.MerchantName,
		MerchantURL:  m.MerchantURL,
		MerchantLogo: m.MerchantLogo,
		Category:     m.Category,
	}
}

func toTypesPaymentItems(items []PaymentItem) []types.PaymentItem {
	if items == nil {
		return nil
	}
	result := make([]types.PaymentItem, len(items))
	for i, item := range items {
		result[i] = types.PaymentItem{
			ItemID:      item.ItemID,
			Name:        item.Name,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   types.Money{Amount: item.UnitPrice.Amount, Currency: item.UnitPrice.Currency},
			TotalPrice:  types.Money{Amount: item.TotalPrice.Amount, Currency: item.TotalPrice.Currency},
			ImageURL:    item.ImageURL,
		}
	}
	return result
}

func toTypesPaymentMethods(methods []PaymentMethodType) []string {
	if methods == nil {
		return nil
	}
	result := make([]string, len(methods))
	for i, m := range methods {
		result[i] = string(m)
	}
	return result
}

func toTypesRecurringInfo(r *RecurringInfo) *types.RecurringInfo {
	if r == nil {
		return nil
	}
	return &types.RecurringInfo{
		Interval:      r.Interval,
		IntervalCount: r.IntervalCount,
		StartDate:     r.StartDate,
		EndDate:       r.EndDate,
		TrialDays:     r.TrialDays,
	}
}

// PaymentOption configures a payment request.
type PaymentOption func(*PaymentRequest)

// WithPaymentItems sets the line items.
func WithPaymentItems(items []PaymentItem) PaymentOption {
	return func(r *PaymentRequest) {
		r.Items = items
	}
}

// WithPaymentMethods sets the allowed payment methods.
func WithPaymentMethods(methods []PaymentMethodType) PaymentOption {
	return func(r *PaymentRequest) {
		r.AllowedMethods = methods
	}
}

// WithPaymentMetadata sets custom metadata.
func WithPaymentMetadata(metadata map[string]interface{}) PaymentOption {
	return func(r *PaymentRequest) {
		r.Metadata = metadata
	}
}

// WithPaymentRecurring sets recurring payment info.
func WithPaymentRecurring(info *RecurringInfo) PaymentOption {
	return func(r *PaymentRequest) {
		r.RecurringInfo = info
	}
}

// WithPaymentCallback sets the webhook callback URL.
func WithPaymentCallback(url string) PaymentOption {
	return func(r *PaymentRequest) {
		r.CallbackURL = url
	}
}

// WithPaymentReturnURL sets the return URL after payment.
func WithPaymentReturnURL(url string) PaymentOption {
	return func(r *PaymentRequest) {
		r.ReturnURL = url
	}
}

// WithPaymentTimeout sets a custom timeout.
func WithPaymentTimeout(timeout time.Duration) PaymentOption {
	return func(r *PaymentRequest) {
		r.ExpiresAt = r.CreatedAt.Add(timeout)
	}
}
