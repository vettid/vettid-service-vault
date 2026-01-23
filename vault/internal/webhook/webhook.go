// Package webhook provides webhook delivery with retry logic for VettID Service Vault.
//
// Webhooks are used to notify service backends about events asynchronously,
// such as when a user responds to an auth or authz request.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Common errors
var (
	ErrInvalidURL        = errors.New("invalid webhook URL")
	ErrMaxRetriesReached = errors.New("maximum retries reached")
	ErrDeliveryFailed    = errors.New("webhook delivery failed")
	ErrContextCancelled  = errors.New("context cancelled")
)

// Event represents a webhook event to be delivered.
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// Delivery represents a webhook delivery attempt.
type Delivery struct {
	EventID      string
	URL          string
	Payload      []byte
	Signature    string
	Status       DeliveryStatus
	StatusCode   int
	ResponseBody string
	Attempts     int
	LastAttempt  time.Time
	NextRetry    *time.Time
	Error        string
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

// DeliveryStatus represents the status of a webhook delivery.
type DeliveryStatus string

const (
	StatusPending   DeliveryStatus = "pending"
	StatusSuccess   DeliveryStatus = "success"
	StatusRetrying  DeliveryStatus = "retrying"
	StatusFailed    DeliveryStatus = "failed"
	StatusCancelled DeliveryStatus = "cancelled"
)

// Dispatcher handles webhook delivery with retries.
type Dispatcher struct {
	mu            sync.RWMutex
	client        *http.Client
	secret        []byte
	maxRetries    int
	retryDelays   []time.Duration
	pending       map[string]*Delivery
	history       []*Delivery
	maxHistory    int
	workerCount   int
	queue         chan *Delivery
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// DispatcherConfig holds configuration for the webhook dispatcher.
type DispatcherConfig struct {
	// Secret for signing webhooks (HMAC-SHA256)
	Secret []byte

	// MaxRetries is the maximum number of retry attempts
	MaxRetries int

	// RetryDelays specifies the delay between each retry attempt
	// If not provided, uses exponential backoff: 5s, 30s, 2m, 10m, 1h
	RetryDelays []time.Duration

	// HTTPTimeout is the timeout for each HTTP request
	HTTPTimeout time.Duration

	// WorkerCount is the number of concurrent workers
	WorkerCount int

	// MaxHistory is the maximum number of deliveries to keep in history
	MaxHistory int
}

// DefaultRetryDelays provides sensible retry delays with exponential backoff.
var DefaultRetryDelays = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	1 * time.Hour,
}

// NewDispatcher creates a new webhook dispatcher.
func NewDispatcher(cfg DispatcherConfig) *Dispatcher {
	retryDelays := cfg.RetryDelays
	if len(retryDelays) == 0 {
		retryDelays = DefaultRetryDelays
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = len(retryDelays)
	}

	httpTimeout := cfg.HTTPTimeout
	if httpTimeout == 0 {
		httpTimeout = 30 * time.Second
	}

	workerCount := cfg.WorkerCount
	if workerCount == 0 {
		workerCount = 5
	}

	maxHistory := cfg.MaxHistory
	if maxHistory == 0 {
		maxHistory = 1000
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &Dispatcher{
		client: &http.Client{
			Timeout: httpTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		secret:      cfg.Secret,
		maxRetries:  maxRetries,
		retryDelays: retryDelays,
		pending:     make(map[string]*Delivery),
		history:     make([]*Delivery, 0, maxHistory),
		maxHistory:  maxHistory,
		workerCount: workerCount,
		queue:       make(chan *Delivery, 1000),
		ctx:         ctx,
		cancel:      cancel,
	}

	return d
}

// Start begins processing webhook deliveries.
func (d *Dispatcher) Start() {
	for i := 0; i < d.workerCount; i++ {
		d.wg.Add(1)
		go d.worker()
	}
}

// Stop gracefully stops the dispatcher.
func (d *Dispatcher) Stop() {
	d.cancel()
	close(d.queue)
	d.wg.Wait()
}

// worker processes deliveries from the queue.
func (d *Dispatcher) worker() {
	defer d.wg.Done()

	for delivery := range d.queue {
		select {
		case <-d.ctx.Done():
			return
		default:
			d.deliver(delivery)
		}
	}
}

// Dispatch queues an event for delivery.
func (d *Dispatcher) Dispatch(url string, event Event) (*Delivery, error) {
	if url == "" {
		return nil, ErrInvalidURL
	}

	// Serialize the event
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshaling event: %w", err)
	}

	// Sign the payload
	signature := d.sign(payload)

	delivery := &Delivery{
		EventID:   event.ID,
		URL:       url,
		Payload:   payload,
		Signature: signature,
		Status:    StatusPending,
		Attempts:  0,
		CreatedAt: time.Now().UTC(),
	}

	// Store in pending
	d.mu.Lock()
	d.pending[event.ID] = delivery
	d.mu.Unlock()

	// Queue for delivery
	select {
	case d.queue <- delivery:
	default:
		// Queue is full, deliver synchronously
		go d.deliver(delivery)
	}

	return delivery, nil
}

// DispatchAsync dispatches an event without waiting for the result.
func (d *Dispatcher) DispatchAsync(url string, event Event) error {
	_, err := d.Dispatch(url, event)
	return err
}

// deliver attempts to deliver a webhook.
func (d *Dispatcher) deliver(delivery *Delivery) {
	for {
		delivery.Attempts++
		delivery.LastAttempt = time.Now().UTC()

		err := d.sendRequest(delivery)
		if err == nil {
			// Success
			delivery.Status = StatusSuccess
			d.complete(delivery)
			return
		}

		// Check if we should retry
		if delivery.Attempts >= d.maxRetries {
			delivery.Status = StatusFailed
			delivery.Error = fmt.Sprintf("max retries reached: %v", err)
			d.complete(delivery)
			return
		}

		// Schedule retry
		delivery.Status = StatusRetrying
		retryDelay := d.getRetryDelay(delivery.Attempts)
		nextRetry := time.Now().Add(retryDelay)
		delivery.NextRetry = &nextRetry
		delivery.Error = err.Error()

		// Wait for retry or cancellation
		select {
		case <-d.ctx.Done():
			delivery.Status = StatusCancelled
			d.complete(delivery)
			return
		case <-time.After(retryDelay):
			// Continue with retry
		}
	}
}

// sendRequest sends the HTTP request.
func (d *Dispatcher) sendRequest(delivery *Delivery) error {
	req, err := http.NewRequestWithContext(d.ctx, "POST", delivery.URL, bytes.NewReader(delivery.Payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VettID-ServiceVault/1.0")
	req.Header.Set("X-VettID-Event-ID", delivery.EventID)
	req.Header.Set("X-VettID-Signature", delivery.Signature)
	req.Header.Set("X-VettID-Timestamp", delivery.CreatedAt.Format(time.RFC3339))

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	delivery.StatusCode = resp.StatusCode

	// Read response body (limited)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	delivery.ResponseBody = string(body)

	// Check for success (2xx status codes)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, delivery.ResponseBody)
}

// sign generates an HMAC-SHA256 signature for the payload.
func (d *Dispatcher) sign(payload []byte) string {
	if len(d.secret) == 0 {
		return ""
	}

	h := hmac.New(sha256.New, d.secret)
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// getRetryDelay returns the delay for the given attempt number.
func (d *Dispatcher) getRetryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return d.retryDelays[0]
	}
	if attempt > len(d.retryDelays) {
		return d.retryDelays[len(d.retryDelays)-1]
	}
	return d.retryDelays[attempt-1]
}

// complete moves a delivery from pending to history.
func (d *Dispatcher) complete(delivery *Delivery) {
	now := time.Now().UTC()
	delivery.CompletedAt = &now

	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.pending, delivery.EventID)

	// Add to history
	d.history = append(d.history, delivery)

	// Trim history if needed
	if len(d.history) > d.maxHistory {
		d.history = d.history[len(d.history)-d.maxHistory:]
	}
}

// GetDelivery returns a delivery by event ID.
func (d *Dispatcher) GetDelivery(eventID string) (*Delivery, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check pending first
	if delivery, ok := d.pending[eventID]; ok {
		return delivery, true
	}

	// Check history
	for i := len(d.history) - 1; i >= 0; i-- {
		if d.history[i].EventID == eventID {
			return d.history[i], true
		}
	}

	return nil, false
}

// GetPendingCount returns the number of pending deliveries.
func (d *Dispatcher) GetPendingCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.pending)
}

// GetRecentDeliveries returns recent deliveries from history.
func (d *Dispatcher) GetRecentDeliveries(limit int) []*Delivery {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 || limit > len(d.history) {
		limit = len(d.history)
	}

	result := make([]*Delivery, limit)
	copy(result, d.history[len(d.history)-limit:])
	return result
}

// VerifySignature verifies a webhook signature.
// This is useful for services receiving webhooks.
func VerifySignature(payload []byte, signature string, secret []byte) bool {
	expected := "sha256=" + hex.EncodeToString(hmacSHA256(payload, secret))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func hmacSHA256(data, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
