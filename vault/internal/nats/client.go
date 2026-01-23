// Package nats provides NATS messaging clients for VettID Service Vault.
//
// VettID uses a split NATS topology:
// - ServiceSpace: Service's own NATS cluster (user → service messages)
// - MessageSpace: VettID's hosted NATS (service → user messages after contract)
//
// Services are self-sovereign and operate their own NATS infrastructure.
package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/vettid/vettid-service-vault/vault/internal/crypto"
)

// Common errors
var (
	ErrNotConnected    = errors.New("not connected to NATS")
	ErrAlreadyConnected = errors.New("already connected to NATS")
	ErrSubscribeFailed = errors.New("subscription failed")
	ErrPublishFailed   = errors.New("publish failed")
	ErrInvalidMessage  = errors.New("invalid message format")
	ErrTopicNotAllowed = errors.New("topic not allowed by permissions")
)

// MessageHandler handles incoming messages from users.
type MessageHandler func(ctx context.Context, msg *Message) error

// Message represents a decrypted message from a user or service.
type Message struct {
	// Subject is the NATS subject the message was received on
	Subject string

	// UserID is the sender's user ID (extracted from topic)
	UserID string

	// ServiceID is the service ID (for messages to services)
	ServiceID string

	// EventType is the type of event (e.g., "auth.response", "authz.response")
	EventType string

	// Payload is the decrypted message payload
	Payload []byte

	// Timestamp is when the message was sent
	Timestamp time.Time

	// EncryptedPayload is the original encrypted payload (for verification)
	EncryptedPayload *crypto.EncryptedMessage

	// Raw is the original NATS message (for acking)
	Raw *nats.Msg
}

// ClientConfig holds configuration for a NATS client.
type ClientConfig struct {
	// Endpoint is the NATS server URL (e.g., "nats://localhost:4222")
	Endpoint string

	// Credentials is the NATS credentials (JWT + seed) for authentication
	Credentials []byte

	// ServiceID is this service's identifier
	ServiceID string

	// Name is a human-readable name for this connection
	Name string

	// ReconnectWait is how long to wait between reconnection attempts
	ReconnectWait time.Duration

	// MaxReconnects is the maximum number of reconnection attempts (-1 for unlimited)
	MaxReconnects int

	// Timeout is the connection timeout
	Timeout time.Duration
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		ReconnectWait: 2 * time.Second,
		MaxReconnects: -1, // Unlimited
		Timeout:       10 * time.Second,
	}
}

// BaseClient provides common NATS functionality for both ServiceSpace and MessageSpace.
type BaseClient struct {
	mu       sync.RWMutex
	conn     *nats.Conn
	js       jetstream.JetStream
	config   ClientConfig
	handlers map[string]MessageHandler
	subs     []*nats.Subscription
	ctx      context.Context
	cancel   context.CancelFunc
}

// newBaseClient creates a new base client.
func newBaseClient(cfg ClientConfig) *BaseClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &BaseClient{
		config:   cfg,
		handlers: make(map[string]MessageHandler),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Connect establishes a connection to the NATS server.
func (c *BaseClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil && c.conn.IsConnected() {
		return ErrAlreadyConnected
	}

	opts := []nats.Option{
		nats.Name(c.config.Name),
		nats.ReconnectWait(c.config.ReconnectWait),
		nats.MaxReconnects(c.config.MaxReconnects),
		nats.Timeout(c.config.Timeout),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				// Log disconnection
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			// Log reconnection
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			// Log errors
		}),
	}

	// Add credentials if provided
	// Note: For credentials from bytes, the caller should write to a temp file
	// or use nats.UserJWTAndSeed() with parsed values

	conn, err := nats.Connect(c.config.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}

	c.conn = conn

	// Initialize JetStream if available
	js, err := jetstream.New(conn)
	if err != nil {
		// JetStream may not be available, continue without it
		c.js = nil
	} else {
		c.js = js
	}

	return nil
}

// IsConnected returns true if connected to NATS.
func (c *BaseClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && c.conn.IsConnected()
}

// Close gracefully closes the NATS connection.
func (c *BaseClient) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Unsubscribe all subscriptions
	for _, sub := range c.subs {
		sub.Unsubscribe()
	}
	c.subs = nil

	if c.conn != nil {
		c.conn.Drain()
		c.conn.Close()
		c.conn = nil
	}

	return nil
}

// Publish publishes a message to a subject.
func (c *BaseClient) Publish(subject string, data []byte) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return ErrNotConnected
	}

	return conn.Publish(subject, data)
}

// Subscribe subscribes to a subject with a handler.
func (c *BaseClient) Subscribe(subject string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || !c.conn.IsConnected() {
		return nil, ErrNotConnected
	}

	sub, err := c.conn.Subscribe(subject, handler)
	if err != nil {
		return nil, fmt.Errorf("subscribing to %s: %w", subject, err)
	}

	c.subs = append(c.subs, sub)
	return sub, nil
}

// QueueSubscribe subscribes to a subject with a queue group.
func (c *BaseClient) QueueSubscribe(subject, queue string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || !c.conn.IsConnected() {
		return nil, ErrNotConnected
	}

	sub, err := c.conn.QueueSubscribe(subject, queue, handler)
	if err != nil {
		return nil, fmt.Errorf("queue subscribing to %s: %w", subject, err)
	}

	c.subs = append(c.subs, sub)
	return sub, nil
}

// Request sends a request and waits for a response.
func (c *BaseClient) Request(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return nil, ErrNotConnected
	}

	return conn.Request(subject, data, timeout)
}
