# VettID Auth Service Example

This example demonstrates how to integrate VettID Service Vault for user authentication in a web application.

## Overview

This example implements:

1. **User login with VettID** - Passwordless authentication
2. **Transaction authorization** - Approve sensitive actions
3. **Webhook handling** - Process user responses
4. **Session management** - Track authenticated sessions

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│  Web Frontend   │────►│  Auth Service   │────►│  Service Vault  │
│   (React/Vue)   │     │   (This Code)   │     │                 │
│                 │     │                 │     │                 │
└─────────────────┘     └────────┬────────┘     └────────┬────────┘
                                 │                       │
                                 │ Webhook               │ NATS
                                 │                       │
                                 ▼                       ▼
                        ┌─────────────────┐     ┌─────────────────┐
                        │    Database     │     │   VettID User   │
                        │    (Sessions)   │     │     (Mobile)    │
                        └─────────────────┘     └─────────────────┘
```

## Quick Start

### 1. Prerequisites

- Go 1.23+
- Running VettID Service Vault
- PostgreSQL or SQLite (for sessions)

### 2. Configuration

Create a `.env` file:

```env
# Service Vault connection
VAULT_ENDPOINT=http://localhost:8080
VAULT_API_KEY=your-api-key
WEBHOOK_SECRET=your-webhook-secret

# Server
PORT=3000
BASE_URL=http://localhost:3000

# Database
DATABASE_URL=sqlite:./sessions.db

# Session
SESSION_SECRET=your-session-secret
SESSION_DURATION=24h
```

### 3. Run the Example

```bash
go run main.go
```

### 4. Test the Flow

1. Open `http://localhost:3000`
2. Click "Login with VettID"
3. Scan the QR code with your VettID app
4. Approve the connection request
5. You're logged in!

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `GET /` | GET | Home page |
| `GET /login` | GET | Login page with QR code |
| `POST /api/login/start` | POST | Start login flow |
| `GET /api/login/status/:id` | GET | Check login status |
| `POST /api/webhooks/vettid` | POST | Webhook handler |
| `GET /api/me` | GET | Get current user |
| `POST /api/logout` | POST | Logout |

## Code Structure

```
auth-service/
├── main.go              # Application entry point
├── handlers/
│   ├── auth.go          # Authentication handlers
│   ├── webhooks.go      # Webhook handlers
│   └── middleware.go    # Auth middleware
├── vault/
│   └── client.go        # Service Vault client
├── store/
│   └── sessions.go      # Session storage
└── templates/
    ├── index.html       # Home page
    └── login.html       # Login page
```

## Integration Details

### Starting a Login

```go
func (h *AuthHandler) StartLogin(w http.ResponseWriter, r *http.Request) {
    // Generate a unique login session
    loginID := generateID()

    // Request auth from vault
    resp, err := h.vault.RequestAuth(&vault.AuthRequest{
        UserID:  "",  // Unknown until user scans
        Purpose: "Login to Example App",
        Context: map[string]interface{}{
            "login_id": loginID,
            "ip":       r.RemoteAddr,
        },
        TimeoutSeconds:   300,
        OfflineGraceHours: 0,  // No offline approval for login
        CallbackURL:      h.baseURL + "/api/webhooks/vettid",
    })

    // Store pending login
    h.store.CreatePendingLogin(loginID, resp.RequestID)

    // Return QR code URL
    json.NewEncoder(w).Encode(map[string]string{
        "login_id": loginID,
        "qr_url":   resp.QRURL,
    })
}
```

### Handling Webhooks

```go
func (h *WebhookHandler) HandleVettID(w http.ResponseWriter, r *http.Request) {
    // Verify signature
    signature := r.Header.Get("X-VettID-Signature")
    body, _ := io.ReadAll(r.Body)

    if !h.vault.VerifySignature(body, signature) {
        http.Error(w, "Invalid signature", http.StatusUnauthorized)
        return
    }

    var event WebhookEvent
    json.Unmarshal(body, &event)

    switch event.Type {
    case "auth.response":
        h.handleAuthResponse(event)
    case "contract.created":
        h.handleContractCreated(event)
    }

    w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) handleAuthResponse(event WebhookEvent) {
    if event.Status != "approved" {
        h.store.FailLogin(event.RequestID, event.Status)
        return
    }

    // Create session
    sessionID := h.store.CreateSession(event.UserID)
    h.store.CompleteLogin(event.RequestID, sessionID)
}
```

### Checking Login Status (Polling)

```go
func (h *AuthHandler) LoginStatus(w http.ResponseWriter, r *http.Request) {
    loginID := chi.URLParam(r, "id")

    login, err := h.store.GetLogin(loginID)
    if err != nil {
        http.Error(w, "Not found", http.StatusNotFound)
        return
    }

    response := map[string]interface{}{
        "status": login.Status,
    }

    if login.Status == "completed" {
        response["session_token"] = login.SessionID
    }

    json.NewEncoder(w).Encode(response)
}
```

## Security Considerations

### 1. Webhook Verification

Always verify webhook signatures to prevent spoofing:

```go
func (c *VaultClient) VerifySignature(payload []byte, signature string) bool {
    expected := "sha256=" + hex.EncodeToString(
        hmac.New(sha256.New, []byte(c.webhookSecret)).
            Sum(payload),
    )
    return hmac.Equal([]byte(expected), []byte(signature))
}
```

### 2. CSRF Protection

Use CSRF tokens for login initiation:

```go
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
    token := csrf.Token(r)
    // Include token in form
}
```

### 3. Rate Limiting

Implement rate limiting on login endpoints:

```go
var limiter = rate.NewLimiter(rate.Every(time.Second), 10)

func RateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            http.Error(w, "Too many requests", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

### 4. Secure Sessions

Use secure, httpOnly cookies:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "session",
    Value:    sessionID,
    HttpOnly: true,
    Secure:   true,
    SameSite: http.SameSiteStrictMode,
    Path:     "/",
    MaxAge:   int(24 * time.Hour / time.Second),
})
```

## Testing

### Unit Tests

```bash
go test ./...
```

### Integration Tests

```bash
# Start vault in test mode
export VAULT_ENDPOINT=http://localhost:8080

go test -tags=integration ./...
```

### Manual Testing

1. Start the service
2. Use the VettID development app
3. Or use curl to simulate webhooks:

```bash
# Simulate auth response
curl -X POST http://localhost:3000/api/webhooks/vettid \
  -H "Content-Type: application/json" \
  -H "X-VettID-Signature: sha256=..." \
  -d '{
    "type": "auth.response",
    "request_id": "auth_123",
    "user_id": "usr_abc",
    "status": "approved"
  }'
```

## Production Deployment

### Environment Variables

```env
# Production settings
VAULT_ENDPOINT=https://vault.yourservice.com
VAULT_API_KEY=${VAULT_API_KEY}
WEBHOOK_SECRET=${WEBHOOK_SECRET}
BASE_URL=https://yourservice.com
DATABASE_URL=postgres://user:pass@host/db
SESSION_SECRET=${SESSION_SECRET}
```

### Docker

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o auth-service .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/auth-service .
EXPOSE 3000
CMD ["./auth-service"]
```

## Related Documentation

- [SDK Integration Guide](../../docs/SDK-INTEGRATION-GUIDE.md)
- [Deployment Guide](../../docs/DEPLOYMENT-GUIDE.md)
- [Security Model](../../docs/SECURITY-MODEL.md)
