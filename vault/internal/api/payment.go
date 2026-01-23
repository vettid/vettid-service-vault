// Package api provides the REST API for the VettID Service Vault.
package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// PaymentRequestBody is the request body for creating a payment request.
type PaymentRequestBody struct {
	UserID         string                    `json:"user_id"`
	Amount         MoneyRequest              `json:"amount"`
	Description    string                    `json:"description"`
	MerchantInfo   MerchantInfoRequest       `json:"merchant_info,omitempty"`
	Items          []PaymentItemRequest      `json:"items,omitempty"`
	AllowedMethods []string                  `json:"allowed_methods,omitempty"`
	RecurringInfo  *RecurringInfoRequest     `json:"recurring_info,omitempty"`
	CallbackURL    string                    `json:"callback_url,omitempty"`
	ExpiresIn      int                       `json:"expires_in,omitempty"` // Seconds
	Metadata       map[string]interface{}    `json:"metadata,omitempty"`
}

// MoneyRequest represents a monetary amount in requests.
type MoneyRequest struct {
	Amount   int64  `json:"amount"`   // Amount in smallest currency unit (cents)
	Currency string `json:"currency"` // ISO 4217 currency code (e.g., "USD")
}

// MerchantInfoRequest contains merchant details for the payment.
type MerchantInfoRequest struct {
	MerchantID   string `json:"merchant_id,omitempty"`
	MerchantName string `json:"merchant_name,omitempty"`
	MerchantURL  string `json:"merchant_url,omitempty"`
	MerchantLogo string `json:"merchant_logo,omitempty"`
	Category     string `json:"category,omitempty"`
}

// PaymentItemRequest represents a line item in a payment.
type PaymentItemRequest struct {
	ItemID      string       `json:"item_id,omitempty"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Quantity    int          `json:"quantity"`
	UnitPrice   MoneyRequest `json:"unit_price"`
	ImageURL    string       `json:"image_url,omitempty"`
}

// RecurringInfoRequest describes recurring payment terms.
type RecurringInfoRequest struct {
	Interval      string `json:"interval"` // day, week, month, year
	IntervalCount int    `json:"interval_count"`
	TrialDays     int    `json:"trial_days,omitempty"`
}

// PaymentRequestResponse is returned when a payment request is created.
type PaymentRequestResponse struct {
	RequestID   string    `json:"request_id"`
	Status      string    `json:"status"`
	Amount      int64     `json:"amount"`
	Currency    string    `json:"currency"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// PaymentStatusResponse is the response for getting payment status.
type PaymentStatusResponse struct {
	RequestID       string                 `json:"request_id"`
	UserID          string                 `json:"user_id"`
	Status          string                 `json:"status"`
	Amount          int64                  `json:"amount"`
	Currency        string                 `json:"currency"`
	Description     string                 `json:"description"`
	PaymentMethodID string                 `json:"payment_method_id,omitempty"`
	TransactionID   string                 `json:"transaction_id,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	FailedAt        *time.Time             `json:"failed_at,omitempty"`
	FailureReason   string                 `json:"failure_reason,omitempty"`
	RefundedAt      *time.Time             `json:"refunded_at,omitempty"`
	RefundAmount    *int64                 `json:"refund_amount,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// handlePaymentRequest creates a new payment request.
// POST /api/v1/payment/request
func (s *Server) handlePaymentRequest(w http.ResponseWriter, r *http.Request) {
	if s.paymentHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "payment handler not configured")
		return
	}

	var req PaymentRequestBody
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "user_id is required")
		return
	}
	if req.Amount.Amount <= 0 {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "amount must be positive")
		return
	}
	if req.Amount.Currency == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "currency is required")
		return
	}
	if req.Description == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "description is required")
		return
	}

	// Convert to handler request
	expiresIn := time.Duration(req.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 15 * time.Minute // Default 15 minute timeout
	}

	// Build options
	var opts []handler.PaymentOption

	// Convert items
	if len(req.Items) > 0 {
		items := make([]handler.PaymentItem, len(req.Items))
		for i, item := range req.Items {
			items[i] = handler.PaymentItem{
				ItemID:      item.ItemID,
				Name:        item.Name,
				Description: item.Description,
				Quantity:    item.Quantity,
				UnitPrice: handler.Money{
					Amount:   item.UnitPrice.Amount,
					Currency: item.UnitPrice.Currency,
				},
				ImageURL: item.ImageURL,
			}
		}
		opts = append(opts, handler.WithPaymentItems(items))
	}

	// Convert recurring info
	if req.RecurringInfo != nil {
		recurringInfo := &handler.RecurringInfo{
			Interval:      req.RecurringInfo.Interval,
			IntervalCount: req.RecurringInfo.IntervalCount,
			TrialDays:     req.RecurringInfo.TrialDays,
			StartDate:     time.Now(),
		}
		opts = append(opts, handler.WithPaymentRecurring(recurringInfo))
	}

	if req.CallbackURL != "" {
		opts = append(opts, handler.WithPaymentCallback(req.CallbackURL))
	}
	if req.Metadata != nil {
		opts = append(opts, handler.WithPaymentMetadata(req.Metadata))
	}
	if expiresIn != 15*time.Minute {
		opts = append(opts, handler.WithPaymentTimeout(expiresIn))
	}

	amount := handler.Money{
		Amount:   req.Amount.Amount,
		Currency: req.Amount.Currency,
	}
	merchant := handler.MerchantInfo{
		MerchantID:   req.MerchantInfo.MerchantID,
		MerchantName: req.MerchantInfo.MerchantName,
		MerchantURL:  req.MerchantInfo.MerchantURL,
		MerchantLogo: req.MerchantInfo.MerchantLogo,
		Category:     req.MerchantInfo.Category,
	}

	paymentReq, err := s.paymentHandler.RequestPayment(r.Context(), req.UserID, amount, req.Description, merchant, opts...)
	if err != nil {
		s.logger.Error("failed to create payment request", "error", err, "user_id", req.UserID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, PaymentRequestResponse{
		RequestID: paymentReq.RequestID,
		Status:    "pending",
		Amount:    req.Amount.Amount,
		Currency:  req.Amount.Currency,
		ExpiresAt: paymentReq.ExpiresAt,
	})
}

// getPaymentRequest returns the status of a payment request.
// GET /api/v1/payment/request/{requestID}
func (s *Server) getPaymentRequest(w http.ResponseWriter, r *http.Request) {
	if s.paymentHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "payment handler not configured")
		return
	}

	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "request_id is required")
		return
	}

	// First check pending requests
	paymentReq, found := s.paymentHandler.GetPaymentRequest(requestID)
	if found {
		resp := PaymentStatusResponse{
			RequestID: paymentReq.RequestID,
			UserID:    paymentReq.UserID,
			Status:    string(paymentReq.Status),
			Amount:    paymentReq.Amount.Amount,
			Currency:  paymentReq.Amount.Currency,
			Description: paymentReq.Description,
			Metadata:  paymentReq.Metadata,
		}
		if paymentReq.Payment != nil {
			resp.TransactionID = paymentReq.Payment.TransactionRef
			resp.CompletedAt = paymentReq.Payment.CompletedAt
			resp.FailedAt = paymentReq.Payment.FailedAt
			resp.FailureReason = paymentReq.Payment.FailureReason
			if paymentReq.Payment.RefundedAmount != nil {
				refundAmt := paymentReq.Payment.RefundedAmount.Amount
				resp.RefundAmount = &refundAmt
			}
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "payment request not found")
}

// handlePaymentComplete marks a payment as completed.
// POST /api/v1/payment/request/{requestID}/complete
func (s *Server) handlePaymentComplete(w http.ResponseWriter, r *http.Request) {
	if s.paymentHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "payment handler not configured")
		return
	}

	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "request_id is required")
		return
	}

	var body struct {
		TransactionID string `json:"transaction_id"`
		ReceiptURL    string `json:"receipt_url,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "invalid request body")
		return
	}

	// Get the payment request to find the payment ID
	paymentReq, found := s.paymentHandler.GetPaymentRequest(requestID)
	if !found || paymentReq.Payment == nil {
		writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "payment not found or not yet approved")
		return
	}

	_, err := s.paymentHandler.CompletePayment(r.Context(), paymentReq.Payment.PaymentID, body.TransactionID, body.ReceiptURL)
	if err != nil {
		s.logger.Error("failed to complete payment", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// handlePaymentFail marks a payment as failed.
// POST /api/v1/payment/request/{requestID}/fail
func (s *Server) handlePaymentFail(w http.ResponseWriter, r *http.Request) {
	if s.paymentHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "payment handler not configured")
		return
	}

	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "request_id is required")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "invalid request body")
		return
	}

	// Get the payment request to find the payment ID
	paymentReq, found := s.paymentHandler.GetPaymentRequest(requestID)
	if !found || paymentReq.Payment == nil {
		writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "payment not found or not yet approved")
		return
	}

	_, err := s.paymentHandler.FailPayment(r.Context(), paymentReq.Payment.PaymentID, body.Reason)
	if err != nil {
		s.logger.Error("failed to mark payment as failed", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "failed"})
}

// handlePaymentRefund issues a refund for a completed payment.
// POST /api/v1/payment/request/{requestID}/refund
func (s *Server) handlePaymentRefund(w http.ResponseWriter, r *http.Request) {
	if s.paymentHandler == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "payment handler not configured")
		return
	}

	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "request_id is required")
		return
	}

	var body struct {
		Amount   int64  `json:"amount,omitempty"`   // Partial refund amount in cents, or 0 for full refund
		Currency string `json:"currency,omitempty"` // Currency for partial refund
		Reason   string `json:"reason,omitempty"`
	}
	readJSON(r, &body) // Optional body

	// Get the payment request to find the payment ID
	paymentReq, found := s.paymentHandler.GetPaymentRequest(requestID)
	if !found || paymentReq.Payment == nil {
		writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "payment not found or not yet approved")
		return
	}

	// Convert amount to Money if provided
	var refundAmount *handler.Money
	if body.Amount > 0 {
		currency := body.Currency
		if currency == "" {
			currency = paymentReq.Payment.Amount.Currency
		}
		refundAmount = &handler.Money{
			Amount:   body.Amount,
			Currency: currency,
		}
	}

	_, err := s.paymentHandler.RefundPayment(r.Context(), paymentReq.Payment.PaymentID, refundAmount, body.Reason)
	if err != nil {
		s.logger.Error("failed to refund payment", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "refunded"})
}
