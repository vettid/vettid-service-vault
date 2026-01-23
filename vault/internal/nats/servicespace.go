package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/vettid/vettid-service-vault/vault/internal/crypto"
)

// ServiceSpaceClient manages connections to the service's own NATS cluster.
// This is where users send messages TO the service.
//
// Topic structure:
// - ServiceSpace.<service_id>.fromUser.<user_id>.> - User → Service messages
// - ServiceSpace.<service_id>.toUser.<user_id>.>   - Service → User (setup only)
type ServiceSpaceClient struct {
	*BaseClient
	serviceID      string
	encryptionKey  [32]byte // Service's encryption private key for decryption
	router         *Router
	streamName     string
}

// NewServiceSpaceClient creates a new ServiceSpace client.
func NewServiceSpaceClient(cfg ClientConfig, encryptionKey [32]byte) *ServiceSpaceClient {
	base := newBaseClient(cfg)
	return &ServiceSpaceClient{
		BaseClient:    base,
		serviceID:     cfg.ServiceID,
		encryptionKey: encryptionKey,
		streamName:    "SERVICESPACE_" + cfg.ServiceID,
	}
}

// Connect establishes the connection and sets up JetStream.
func (c *ServiceSpaceClient) Connect() error {
	if err := c.BaseClient.Connect(); err != nil {
		return err
	}

	// Set up JetStream stream for message persistence
	if c.js != nil {
		if err := c.setupStream(); err != nil {
			// Non-fatal, continue without persistence
		}
	}

	return nil
}

// setupStream creates the JetStream stream for message persistence.
func (c *ServiceSpaceClient) setupStream() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	streamCfg := jetstream.StreamConfig{
		Name:        c.streamName,
		Description: "VettID ServiceSpace messages for " + c.serviceID,
		Subjects: []string{
			fmt.Sprintf("ServiceSpace.%s.fromUser.>", c.serviceID),
		},
		Retention:    jetstream.WorkQueuePolicy,
		MaxAge:       24 * time.Hour, // Messages expire after 24h
		MaxMsgs:      1000000,
		MaxBytes:     1024 * 1024 * 1024, // 1GB
		Discard:      jetstream.DiscardOld,
		MaxMsgSize:   1024 * 1024, // 1MB per message
		Duplicates:   5 * time.Minute,
		Replicas:     1,
	}

	_, err := c.js.CreateOrUpdateStream(ctx, streamCfg)
	return err
}

// SubscribeToUser subscribes to messages from a specific user.
func (c *ServiceSpaceClient) SubscribeToUser(userID string, handler MessageHandler) error {
	subject := fmt.Sprintf("ServiceSpace.%s.fromUser.%s.>", c.serviceID, userID)

	_, err := c.Subscribe(subject, func(msg *nats.Msg) {
		c.handleMessage(userID, msg, handler)
	})

	return err
}

// SubscribeToAllUsers subscribes to messages from all connected users.
func (c *ServiceSpaceClient) SubscribeToAllUsers(handler MessageHandler) error {
	subject := fmt.Sprintf("ServiceSpace.%s.fromUser.>", c.serviceID)

	_, err := c.QueueSubscribe(subject, "service-handlers", func(msg *nats.Msg) {
		// Extract user ID from subject
		userID := extractUserID(msg.Subject, c.serviceID)
		c.handleMessage(userID, msg, handler)
	})

	return err
}

// handleMessage decrypts and processes an incoming message.
func (c *ServiceSpaceClient) handleMessage(userID string, natsMsg *nats.Msg, handler MessageHandler) {
	ctx := context.Background()

	// Parse the encrypted message
	var encMsg crypto.EncryptedMessage
	if err := json.Unmarshal(natsMsg.Data, &encMsg); err != nil {
		// Log error and acknowledge to prevent redelivery
		natsMsg.Ack()
		return
	}

	// Decrypt the message
	plaintext, err := crypto.Decrypt(&encMsg, c.encryptionKey)
	if err != nil {
		// Log decryption error
		natsMsg.Ack()
		return
	}

	// Extract event type from subject
	eventType := extractEventType(natsMsg.Subject)

	msg := &Message{
		Subject:          natsMsg.Subject,
		UserID:           userID,
		ServiceID:        c.serviceID,
		EventType:        eventType,
		Payload:          plaintext,
		Timestamp:        time.Now(),
		EncryptedPayload: &encMsg,
		Raw:              natsMsg,
	}

	// Call handler
	if err := handler(ctx, msg); err != nil {
		// Handler failed, message may be redelivered
		natsMsg.Nak()
		return
	}

	natsMsg.Ack()
}

// PublishToUser sends a message to a specific user (during setup).
// After contract activation, use MessageSpaceClient for user messages.
func (c *ServiceSpaceClient) PublishToUser(userID string, eventType string, payload []byte, userPubKey [32]byte) error {
	// Encrypt the payload for the user
	encMsg, err := crypto.Encrypt(payload, userPubKey)
	if err != nil {
		return fmt.Errorf("encrypting message: %w", err)
	}

	data, err := json.Marshal(encMsg)
	if err != nil {
		return fmt.Errorf("marshaling encrypted message: %w", err)
	}

	subject := fmt.Sprintf("ServiceSpace.%s.toUser.%s.%s", c.serviceID, userID, eventType)
	return c.Publish(subject, data)
}

// RequestFromUser sends a request to a user and waits for response.
func (c *ServiceSpaceClient) RequestFromUser(userID string, eventType string, payload []byte, userPubKey [32]byte, timeout time.Duration) ([]byte, error) {
	// Encrypt the request
	encMsg, err := crypto.Encrypt(payload, userPubKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting request: %w", err)
	}

	data, err := json.Marshal(encMsg)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	subject := fmt.Sprintf("ServiceSpace.%s.toUser.%s.%s", c.serviceID, userID, eventType)
	resp, err := c.Request(subject, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Decrypt the response
	var respEncMsg crypto.EncryptedMessage
	if err := json.Unmarshal(resp.Data, &respEncMsg); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	plaintext, err := crypto.Decrypt(&respEncMsg, c.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting response: %w", err)
	}

	return plaintext, nil
}

// SetRouter sets the message router for this client.
func (c *ServiceSpaceClient) SetRouter(r *Router) {
	c.router = r
}

// extractUserID extracts the user ID from a ServiceSpace subject.
// Subject format: ServiceSpace.<service_id>.fromUser.<user_id>.<event_type>
func extractUserID(subject, serviceID string) string {
	prefix := fmt.Sprintf("ServiceSpace.%s.fromUser.", serviceID)
	if !strings.HasPrefix(subject, prefix) {
		return ""
	}

	remainder := subject[len(prefix):]
	parts := strings.SplitN(remainder, ".", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// extractEventType extracts the event type from a subject.
func extractEventType(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
