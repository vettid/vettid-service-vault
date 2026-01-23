// Package handler provides the event handler engine for VettID Service Vault.
package handler

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookPayload represents the standard webhook payload format.
type WebhookPayload struct {
	EventType string      `json:"event_type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// WebhookConfig holds configuration for webhook calls.
type WebhookConfig struct {
	// Timeout for webhook HTTP calls
	Timeout time.Duration

	// MaxRetries is the maximum number of retry attempts
	MaxRetries int

	// RetryDelay is the base delay between retries (uses exponential backoff)
	RetryDelay time.Duration

	// InsecureSkipVerify disables TLS verification (for development only)
	InsecureSkipVerify bool
}

// DefaultWebhookConfig returns sensible defaults for webhook configuration.
func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		Timeout:            10 * time.Second,
		MaxRetries:         3,
		RetryDelay:         500 * time.Millisecond,
		InsecureSkipVerify: false,
	}
}

var webhookConfig = DefaultWebhookConfig()

// SetWebhookConfig updates the global webhook configuration.
func SetWebhookConfig(cfg WebhookConfig) {
	webhookConfig = cfg
}

// callWebhookHTTP sends an HTTP POST to a webhook URL.
func callWebhookHTTP(ctx context.Context, webhookURL, eventType string, data interface{}) error {
	payload := WebhookPayload{
		EventType: eventType,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling webhook payload: %w", err)
	}

	client := &http.Client{
		Timeout: webhookConfig.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: webhookConfig.InsecureSkipVerify,
			},
		},
	}

	var lastErr error
	for attempt := 0; attempt <= webhookConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := webhookConfig.RetryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating webhook request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "VettID-ServiceVault/1.0")
		req.Header.Set("X-VettID-Event", eventType)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook request failed: %w", err)
			continue
		}

		// Close body immediately - we don't need the response content
		resp.Body.Close()

		// Success on 2xx status codes
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		// Don't retry on 4xx errors (client errors)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("webhook returned client error: %d", resp.StatusCode)
		}

		// Retry on 5xx errors
		lastErr = fmt.Errorf("webhook returned server error: %d", resp.StatusCode)
	}

	return lastErr
}
