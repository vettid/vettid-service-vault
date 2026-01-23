package nats

import (
	"encoding/json"
	"fmt"

	"github.com/vettid/vettid-service-vault/vault/internal/crypto"
)

// MessageSpaceClient manages connections to VettID's hosted NATS cluster.
// This is where services send messages TO users after contract activation.
//
// Topic structure:
// - MessageSpace.<user_id>.fromService.<service_id>.> - Service → User messages
//
// Services publish to MessageSpace; users subscribe there.
// This topology ensures users only receive messages from services they've contracted with.
type MessageSpaceClient struct {
	*BaseClient
	serviceID     string
	encryptionKey [32]byte // Service's encryption private key
}

// NewMessageSpaceClient creates a new MessageSpace client.
func NewMessageSpaceClient(cfg ClientConfig, encryptionKey [32]byte) *MessageSpaceClient {
	base := newBaseClient(cfg)
	return &MessageSpaceClient{
		BaseClient:    base,
		serviceID:     cfg.ServiceID,
		encryptionKey: encryptionKey,
	}
}

// SendToUser sends an encrypted message to a user via MessageSpace.
// This is the primary method for service → user communication after contract activation.
func (c *MessageSpaceClient) SendToUser(userID string, eventType string, payload []byte, userPubKey [32]byte) error {
	// Encrypt the payload for the user
	encMsg, err := crypto.Encrypt(payload, userPubKey)
	if err != nil {
		return fmt.Errorf("encrypting message: %w", err)
	}

	data, err := json.Marshal(encMsg)
	if err != nil {
		return fmt.Errorf("marshaling encrypted message: %w", err)
	}

	// MessageSpace topic: MessageSpace.<user_id>.fromService.<service_id>.<event_type>
	subject := fmt.Sprintf("MessageSpace.%s.fromService.%s.%s", userID, c.serviceID, eventType)
	return c.Publish(subject, data)
}

// SendAuthRequest sends an authentication request to a user.
func (c *MessageSpaceClient) SendAuthRequest(userID string, request interface{}, userPubKey [32]byte) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshaling auth request: %w", err)
	}

	return c.SendToUser(userID, "auth.request", payload, userPubKey)
}

// SendAuthzRequest sends an authorization request to a user.
func (c *MessageSpaceClient) SendAuthzRequest(userID string, request interface{}, userPubKey [32]byte) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshaling authz request: %w", err)
	}

	return c.SendToUser(userID, "authz.request", payload, userPubKey)
}

// SendContractOffer sends a contract offer to a user.
func (c *MessageSpaceClient) SendContractOffer(userID string, offer interface{}, userPubKey [32]byte) error {
	payload, err := json.Marshal(offer)
	if err != nil {
		return fmt.Errorf("marshaling contract offer: %w", err)
	}

	return c.SendToUser(userID, "contract.offer", payload, userPubKey)
}

// SendNotification sends a notification to a user.
func (c *MessageSpaceClient) SendNotification(userID string, notification interface{}, userPubKey [32]byte) error {
	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("marshaling notification: %w", err)
	}

	return c.SendToUser(userID, "notification", payload, userPubKey)
}

// Broadcast sends a message to multiple users.
// This is useful for announcements or updates that apply to many users.
func (c *MessageSpaceClient) Broadcast(userIDs []string, eventType string, payload []byte, userPubKeys map[string][32]byte) []error {
	var errs []error

	for _, userID := range userIDs {
		pubKey, ok := userPubKeys[userID]
		if !ok {
			errs = append(errs, fmt.Errorf("no public key for user %s", userID))
			continue
		}

		if err := c.SendToUser(userID, eventType, payload, pubKey); err != nil {
			errs = append(errs, fmt.Errorf("failed to send to %s: %w", userID, err))
		}
	}

	return errs
}
