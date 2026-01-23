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

// NotificationHandler handles sending notifications to users via MessageSpace.
type NotificationHandler struct {
	mu           sync.RWMutex
	engine       *Engine
	sent         map[string]*Notification
	maxHistory   int
	retryCount   int
	retryDelay   time.Duration
}

// Notification represents a notification sent to a user.
type Notification struct {
	NotificationID string                 `json:"notification_id"`
	UserID         string                 `json:"user_id"`
	Title          string                 `json:"title"`
	Body           string                 `json:"body"`
	Category       NotificationCategory   `json:"category"`
	Priority       NotificationPriority   `json:"priority"`
	Data           map[string]interface{} `json:"data,omitempty"`
	ActionURL      string                 `json:"action_url,omitempty"`
	ImageURL       string                 `json:"image_url,omitempty"`
	Status         NotificationStatus     `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	SentAt         *time.Time             `json:"sent_at,omitempty"`
	DeliveredAt    *time.Time             `json:"delivered_at,omitempty"`
	ReadAt         *time.Time             `json:"read_at,omitempty"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
	RetryCount     int                    `json:"retry_count"`
}

// NotificationCategory categorizes notifications.
type NotificationCategory string

const (
	CategoryInfo      NotificationCategory = "info"
	CategoryAlert     NotificationCategory = "alert"
	CategoryAction    NotificationCategory = "action"
	CategoryPromo     NotificationCategory = "promo"
	CategoryTransact  NotificationCategory = "transaction"
	CategorySecurity  NotificationCategory = "security"
)

// NotificationPriority indicates urgency.
type NotificationPriority string

const (
	PriorityLow    NotificationPriority = "low"
	PriorityNormal NotificationPriority = "normal"
	PriorityHigh   NotificationPriority = "high"
	PriorityUrgent NotificationPriority = "urgent"
)

// NotificationStatus tracks delivery state.
type NotificationStatus string

const (
	StatusPending   NotificationStatus = "pending"
	StatusSent      NotificationStatus = "sent"
	StatusDelivered NotificationStatus = "delivered"
	StatusRead      NotificationStatus = "read"
	StatusFailed    NotificationStatus = "failed"
	StatusExpired   NotificationStatus = "expired"
)

// NotificationHandlerConfig holds configuration for the notification handler.
type NotificationHandlerConfig struct {
	Engine     *Engine
	MaxHistory int           // Maximum notifications to keep in history
	RetryCount int           // Number of retry attempts
	RetryDelay time.Duration // Delay between retries
}

// NewNotificationHandler creates a new notification handler.
func NewNotificationHandler(cfg NotificationHandlerConfig) *NotificationHandler {
	maxHistory := cfg.MaxHistory
	if maxHistory == 0 {
		maxHistory = 1000
	}

	retryCount := cfg.RetryCount
	if retryCount == 0 {
		retryCount = 3
	}

	retryDelay := cfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = 5 * time.Second
	}

	return &NotificationHandler{
		engine:     cfg.Engine,
		sent:       make(map[string]*Notification),
		maxHistory: maxHistory,
		retryCount: retryCount,
		retryDelay: retryDelay,
	}
}

// EventType returns the event type this handler processes.
func (h *NotificationHandler) EventType() string {
	return "notification.ack"
}

// HandleRequest processes notification acknowledgments from users.
func (h *NotificationHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	// Parse the acknowledgment
	var ack struct {
		NotificationID string    `json:"notification_id"`
		Status         string    `json:"status"` // "delivered" or "read"
		Timestamp      time.Time `json:"timestamp"`
	}

	if err := json.Unmarshal(req.Payload, &ack); err != nil {
		return &Response{
			Status: "error",
			Error: &types.APIError{
				Code:    types.ErrCodeInvalidRequest,
				Message: "invalid notification ack format",
			},
		}, nil
	}

	// Update notification status
	h.mu.Lock()
	notif, exists := h.sent[ack.NotificationID]
	if exists && notif.UserID == req.UserID {
		switch ack.Status {
		case "delivered":
			notif.Status = StatusDelivered
			notif.DeliveredAt = &ack.Timestamp
		case "read":
			notif.Status = StatusRead
			notif.ReadAt = &ack.Timestamp
		}
	}
	h.mu.Unlock()

	return &Response{Status: "ok"}, nil
}

// Send sends a notification to a user.
func (h *NotificationHandler) Send(ctx context.Context, userID string, title, body string, opts ...NotificationOption) (*Notification, error) {
	// Verify user has an active contract
	userContract, err := h.engine.GetContract(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no active contract: %w", err)
	}

	// Verify contract allows notifications
	if !contract.VerifyCapability(userContract, types.CapabilityNotify) {
		return nil, fmt.Errorf("contract does not grant notify capability")
	}

	// Generate notification ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	notificationID := base64.RawURLEncoding.EncodeToString(idBytes)

	now := time.Now().UTC()
	notif := &Notification{
		NotificationID: notificationID,
		UserID:         userID,
		Title:          title,
		Body:           body,
		Category:       CategoryInfo,
		Priority:       PriorityNormal,
		Status:         StatusPending,
		CreatedAt:      now,
	}

	// Apply options
	for _, opt := range opts {
		opt(notif)
	}

	// Send via MessageSpace
	if err := h.sendToUser(ctx, notif, userContract); err != nil {
		notif.Status = StatusFailed
		return notif, fmt.Errorf("failed to send notification: %w", err)
	}

	sentAt := time.Now().UTC()
	notif.SentAt = &sentAt
	notif.Status = StatusSent

	// Store in history
	h.mu.Lock()
	h.sent[notificationID] = notif
	h.trimHistory()
	h.mu.Unlock()

	return notif, nil
}

// SendBatch sends notifications to multiple users.
func (h *NotificationHandler) SendBatch(ctx context.Context, userIDs []string, title, body string, opts ...NotificationOption) ([]*Notification, []error) {
	notifications := make([]*Notification, 0, len(userIDs))
	errs := make([]error, 0)

	for _, userID := range userIDs {
		notif, err := h.Send(ctx, userID, title, body, opts...)
		notifications = append(notifications, notif)
		if err != nil {
			errs = append(errs, fmt.Errorf("user %s: %w", userID, err))
		}
	}

	return notifications, errs
}

// sendToUser sends the notification via MessageSpace.
func (h *NotificationHandler) sendToUser(ctx context.Context, notif *Notification, userContract *contract.SignedConnectionContract) error {
	if h.engine.messageSpace == nil {
		return fmt.Errorf("MessageSpace client not configured")
	}

	payload, err := json.Marshal(types.Notification{
		NotificationID: notif.NotificationID,
		Title:          notif.Title,
		Body:           notif.Body,
		Category:       string(notif.Category),
		Priority:       string(notif.Priority),
		Data:           notif.Data,
		ActionURL:      notif.ActionURL,
		ImageURL:       notif.ImageURL,
		ExpiresAt:      notif.ExpiresAt,
		CreatedAt:      notif.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("marshaling notification: %w", err)
	}

	// Get user's public key from contract
	var userPubKey [32]byte
	// In production, decode from userContract.UserConnectionKey

	return h.engine.messageSpace.SendNotification(notif.UserID, payload, userPubKey)
}

// GetNotification returns a sent notification by ID.
func (h *NotificationHandler) GetNotification(notificationID string) (*Notification, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	notif, ok := h.sent[notificationID]
	return notif, ok
}

// GetUserNotifications returns all notifications for a user.
func (h *NotificationHandler) GetUserNotifications(userID string, limit int) []*Notification {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var notifications []*Notification
	for _, notif := range h.sent {
		if notif.UserID == userID {
			notifications = append(notifications, notif)
			if limit > 0 && len(notifications) >= limit {
				break
			}
		}
	}
	return notifications
}

// trimHistory removes old notifications when exceeding maxHistory.
func (h *NotificationHandler) trimHistory() {
	if len(h.sent) <= h.maxHistory {
		return
	}

	// Find oldest notifications to remove
	var oldest []*Notification
	for _, notif := range h.sent {
		oldest = append(oldest, notif)
	}

	// Sort by creation time (simple bubble sort for small lists)
	for i := range oldest {
		for j := i + 1; j < len(oldest); j++ {
			if oldest[i].CreatedAt.After(oldest[j].CreatedAt) {
				oldest[i], oldest[j] = oldest[j], oldest[i]
			}
		}
	}

	// Remove oldest entries
	toRemove := len(h.sent) - h.maxHistory
	for i := 0; i < toRemove && i < len(oldest); i++ {
		delete(h.sent, oldest[i].NotificationID)
	}
}

// Cleanup removes expired notifications.
func (h *NotificationHandler) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for id, notif := range h.sent {
		if notif.ExpiresAt != nil && now.After(*notif.ExpiresAt) {
			notif.Status = StatusExpired
			delete(h.sent, id)
		}
	}
}

// NotificationOption configures a notification.
type NotificationOption func(*Notification)

// WithCategory sets the notification category.
func WithCategory(category NotificationCategory) NotificationOption {
	return func(n *Notification) {
		n.Category = category
	}
}

// WithPriority sets the notification priority.
func WithPriority(priority NotificationPriority) NotificationOption {
	return func(n *Notification) {
		n.Priority = priority
	}
}

// WithNotificationData adds custom data to the notification.
func WithNotificationData(data map[string]interface{}) NotificationOption {
	return func(n *Notification) {
		n.Data = data
	}
}

// WithActionURL sets a URL to open when the notification is tapped.
func WithActionURL(url string) NotificationOption {
	return func(n *Notification) {
		n.ActionURL = url
	}
}

// WithImageURL sets an image to display with the notification.
func WithImageURL(url string) NotificationOption {
	return func(n *Notification) {
		n.ImageURL = url
	}
}

// WithExpiration sets when the notification expires.
func WithExpiration(expiresAt time.Time) NotificationOption {
	return func(n *Notification) {
		n.ExpiresAt = &expiresAt
	}
}

// WithTTL sets the notification's time-to-live.
func WithTTL(ttl time.Duration) NotificationOption {
	return func(n *Notification) {
		exp := n.CreatedAt.Add(ttl)
		n.ExpiresAt = &exp
	}
}
