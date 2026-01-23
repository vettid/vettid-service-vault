# VettID Service Vault SDK Integration Guide

This guide explains how to integrate the VettID Service Vault with your backend services.

## Table of Contents

1. [Overview](#overview)
2. [Authentication](#authentication)
3. [Core Workflows](#core-workflows)
4. [API Reference](#api-reference)
5. [Error Handling](#error-handling)
6. [Best Practices](#best-practices)
7. [Examples](#examples)

---

## Overview

The Service Vault provides a REST API for your backend to:

- **Manage user connections** via contracts
- **Request authentication** from users
- **Request authorization** for specific actions
- **Request data** from users (with consent)
- **Send notifications** to users
- **Receive webhooks** when users respond

### Integration Architecture

```
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│                 │         │                 │         │                 │
│  Your Backend   │◄───────►│  Service Vault  │◄───────►│   VettID User   │
│                 │  REST   │                 │  NATS   │    (Mobile)     │
│                 │   API   │                 │         │                 │
└─────────────────┘         └─────────────────┘         └─────────────────┘
        │                           │
        │                           │
        ▼                           ▼
   Webhooks ◄───────────────── Callbacks
```

---

## Authentication

All API requests must include an API key in the `Authorization` header:

```http
Authorization: Bearer your-api-key
```

### Example (cURL)

```bash
curl -X GET https://your-vault.example.com/api/v1/contracts \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json"
```

### Example (Node.js)

```javascript
const response = await fetch('https://your-vault.example.com/api/v1/contracts', {
  headers: {
    'Authorization': `Bearer ${process.env.VAULT_API_KEY}`,
    'Content-Type': 'application/json'
  }
});
```

---

## Core Workflows

### 1. User Connection (Contract Flow)

Before interacting with a user, you need an active contract:

```
1. Generate invite link → 2. User scans QR/clicks link → 3. User reviews & signs
        ↓                                                        ↓
4. Service receives contract ← 5. Service counter-signs ← 6. Contract active
```

#### Step 1: Generate an Invite

```bash
curl -X POST https://vault.example.com/api/v1/contracts/invite \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "offering_id": "premium",
    "max_uses": 1,
    "ttl_hours": 24,
    "metadata": {
      "campaign": "signup-flow"
    }
  }'
```

Response:
```json
{
  "invite_id": "abc123",
  "invite_url": "vettid://connect?invite=abc123&service=XyZ...",
  "qr_code": "data:image/png;base64,...",
  "expires_at": "2024-01-02T12:00:00Z"
}
```

#### Step 2: Wait for Contract (Webhook or Polling)

**Webhook approach (recommended):**
Configure your webhook URL in the vault settings. You'll receive:

```json
{
  "type": "contract.created",
  "contract_id": "con_123",
  "user_id": "usr_abc",
  "offering_id": "premium",
  "status": "pending"
}
```

**Polling approach:**
```bash
curl https://vault.example.com/api/v1/contracts?status=pending \
  -H "Authorization: Bearer $API_KEY"
```

#### Step 3: Accept the Contract

```bash
curl -X POST https://vault.example.com/api/v1/contracts/con_123/accept \
  -H "Authorization: Bearer $API_KEY"
```

Response:
```json
{
  "contract_id": "con_123",
  "user_id": "usr_abc",
  "status": "active",
  "activated_at": "2024-01-01T12:00:00Z",
  "capabilities": ["authenticate", "authorize"]
}
```

---

### 2. Authentication Flow

Request the user to authenticate (e.g., for login):

```
1. Request auth → 2. User receives prompt → 3. User approves/denies
        ↓                                            ↓
4. Receive response ← ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
```

#### Request Authentication

```bash
curl -X POST https://vault.example.com/api/v1/auth/request \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "usr_abc",
    "purpose": "Login to your account",
    "context": {
      "ip_address": "203.0.113.42",
      "device": "Chrome on macOS",
      "location": "San Francisco, CA"
    },
    "timeout_seconds": 300,
    "offline_grace_hours": 24,
    "callback_url": "https://your-backend.com/webhooks/vettid"
  }'
```

Response:
```json
{
  "request_id": "auth_789",
  "status": "pending",
  "expires_at": "2024-01-01T12:05:00Z"
}
```

#### Receive Response (Webhook)

```json
{
  "type": "auth.response",
  "request_id": "auth_789",
  "user_id": "usr_abc",
  "status": "approved",
  "timestamp": "2024-01-01T12:01:30Z",
  "signature": {
    "algorithm": "ed25519",
    "public_key": "...",
    "signature": "...",
    "timestamp": "2024-01-01T12:01:30Z"
  }
}
```

#### Poll for Response (Alternative)

```bash
curl https://vault.example.com/api/v1/auth/request/auth_789 \
  -H "Authorization: Bearer $API_KEY"
```

---

### 3. Authorization Flow

Request permission for a specific action:

```bash
curl -X POST https://vault.example.com/api/v1/authz/request \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "usr_abc",
    "action": "transfer_funds",
    "resource": "account/checking",
    "context": {
      "amount": 500.00,
      "currency": "USD",
      "recipient": "John Doe"
    },
    "timeout_seconds": 120,
    "callback_url": "https://your-backend.com/webhooks/vettid"
  }'
```

Response:
```json
{
  "request_id": "authz_456",
  "status": "pending",
  "expires_at": "2024-01-01T12:02:00Z"
}
```

---

### 4. Data Request Flow

Request data from the user (with their consent):

#### Browse Available Data

```bash
curl -X POST https://vault.example.com/api/v1/data/browse \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "usr_abc",
    "data_types": ["identity.name", "identity.email", "identity.phone"],
    "callback_url": "https://your-backend.com/webhooks/vettid"
  }'
```

#### Request Specific Data

```bash
curl -X POST https://vault.example.com/api/v1/data/request \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "usr_abc",
    "data_paths": ["identity.email", "identity.phone"],
    "purpose": "Account verification",
    "timeout_seconds": 3600,
    "callback_url": "https://your-backend.com/webhooks/vettid"
  }'
```

Response (via webhook):
```json
{
  "type": "data.response",
  "request_id": "data_321",
  "user_id": "usr_abc",
  "status": "approved",
  "data": {
    "identity.email": "user@example.com",
    "identity.phone": "+1-555-0123"
  },
  "constraints": {
    "retention": "30 days",
    "third_party": false
  }
}
```

---

### 5. Notifications

Send notifications to users:

```bash
curl -X POST https://vault.example.com/api/v1/notifications \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "usr_abc",
    "title": "Payment Received",
    "body": "You received $50.00 from Jane Doe",
    "category": "transaction",
    "priority": "normal",
    "data": {
      "transaction_id": "txn_123",
      "amount": 50.00
    },
    "action_url": "myapp://transactions/txn_123"
  }'
```

---

## API Reference

### Endpoints Summary

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/v1/contracts/invite` | Generate invite |
| `GET` | `/api/v1/contracts` | List contracts |
| `GET` | `/api/v1/contracts/{id}` | Get contract |
| `POST` | `/api/v1/contracts/{id}/accept` | Accept contract |
| `DELETE` | `/api/v1/contracts/{id}` | Cancel contract |
| `POST` | `/api/v1/auth/request` | Request authentication |
| `GET` | `/api/v1/auth/request/{id}` | Get auth request status |
| `POST` | `/api/v1/authz/request` | Request authorization |
| `GET` | `/api/v1/authz/request/{id}` | Get authz request status |
| `POST` | `/api/v1/data/browse` | Browse available data |
| `POST` | `/api/v1/data/request` | Request data |
| `POST` | `/api/v1/notifications` | Send notification |

### Request/Response Types

See [OpenAPI Specification](api/openapi.yaml) for complete type definitions.

---

## Error Handling

### Error Response Format

```json
{
  "error": {
    "code": "contract_required",
    "message": "User does not have an active contract",
    "details": {
      "user_id": "usr_abc"
    }
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `invalid_request` | 400 | Malformed request |
| `unauthorized` | 401 | Invalid or missing API key |
| `forbidden` | 403 | Operation not allowed |
| `not_found` | 404 | Resource not found |
| `conflict` | 409 | Resource already exists |
| `timeout` | 408 | Request timed out |
| `user_offline` | 503 | User is offline |
| `contract_required` | 403 | No active contract with user |
| `capability_denied` | 403 | Contract doesn't grant this capability |
| `signature_invalid` | 400 | Invalid cryptographic signature |
| `internal_error` | 500 | Internal server error |

### Handling Offline Users

When a user is offline, requests may still succeed within the `offline_grace` period:

```json
{
  "request_id": "auth_789",
  "status": "offline_approved",
  "offline_approved_at": "2024-01-01T12:01:30Z"
}
```

The `offline_approved` status indicates the user approved before going offline.

---

## Best Practices

### 1. Use Webhooks

Prefer webhooks over polling for responses. Configure a webhook URL in your vault settings.

### 2. Verify Signatures

Always verify the cryptographic signatures on responses to ensure authenticity:

```javascript
import { verifySignature } from '@vettid/sdk';

const isValid = verifySignature(
  response.data,
  response.signature,
  knownPublicKey
);
```

### 3. Handle Timeouts Gracefully

Set appropriate timeouts based on the action:

| Action | Recommended Timeout |
|--------|---------------------|
| Login | 5 minutes |
| Payment authorization | 2 minutes |
| Data sharing | 1 hour |
| General notification | No timeout |

### 4. Provide Context

Always include meaningful context to help users make informed decisions:

```json
{
  "purpose": "Authorize transfer of $500 to John Doe",
  "context": {
    "amount": 500,
    "recipient": "John Doe",
    "account": "****1234",
    "timestamp": "2024-01-01T12:00:00Z"
  }
}
```

### 5. Implement Retry Logic

For webhook delivery failures, implement exponential backoff:

```javascript
const retryDelays = [5000, 30000, 120000, 600000, 3600000]; // 5s, 30s, 2m, 10m, 1h
```

### 6. Cache Contract Status

Cache the contract status to avoid unnecessary API calls:

```javascript
const contractCache = new Map();

async function hasActiveContract(userId) {
  if (contractCache.has(userId)) {
    const cached = contractCache.get(userId);
    if (cached.expiresAt > Date.now()) {
      return cached.active;
    }
  }

  const contract = await vault.getContractByUser(userId);
  contractCache.set(userId, {
    active: contract?.status === 'active',
    expiresAt: Date.now() + 60000 // Cache for 1 minute
  });

  return contract?.status === 'active';
}
```

---

## Examples

### Node.js/TypeScript

```typescript
import { ServiceVaultClient } from '@vettid/sdk';

const vault = new ServiceVaultClient({
  endpoint: process.env.VAULT_ENDPOINT,
  apiKey: process.env.VAULT_API_KEY
});

// Request authentication
async function loginWithVettID(userId: string) {
  const request = await vault.requestAuth({
    userId,
    purpose: 'Login to your account',
    timeoutSeconds: 300,
    callbackUrl: 'https://api.example.com/webhooks/vettid'
  });

  return request.requestId;
}

// Handle webhook
app.post('/webhooks/vettid', async (req, res) => {
  const { type, request_id, status, signature } = req.body;

  // Verify signature
  if (!vault.verifyWebhook(req.body, req.headers['x-vettid-signature'])) {
    return res.status(401).send('Invalid signature');
  }

  if (type === 'auth.response' && status === 'approved') {
    await completeLogin(request_id);
  }

  res.status(200).send('OK');
});
```

### Python

```python
from vettid import ServiceVaultClient

vault = ServiceVaultClient(
    endpoint=os.environ['VAULT_ENDPOINT'],
    api_key=os.environ['VAULT_API_KEY']
)

# Request authentication
def login_with_vettid(user_id: str) -> str:
    request = vault.request_auth(
        user_id=user_id,
        purpose='Login to your account',
        timeout_seconds=300,
        callback_url='https://api.example.com/webhooks/vettid'
    )
    return request.request_id

# Handle webhook
@app.post('/webhooks/vettid')
def handle_webhook(request: Request):
    body = request.json()
    signature = request.headers.get('X-VettID-Signature')

    if not vault.verify_webhook(body, signature):
        return Response(status_code=401)

    if body['type'] == 'auth.response' and body['status'] == 'approved':
        complete_login(body['request_id'])

    return Response(status_code=200)
```

### Go

```go
package main

import (
    "github.com/vettid/sdk-go"
)

func main() {
    vault := vettid.NewServiceVaultClient(
        os.Getenv("VAULT_ENDPOINT"),
        os.Getenv("VAULT_API_KEY"),
    )

    // Request authentication
    request, err := vault.RequestAuth(&vettid.AuthRequest{
        UserID:         userID,
        Purpose:        "Login to your account",
        TimeoutSeconds: 300,
        CallbackURL:    "https://api.example.com/webhooks/vettid",
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Auth request created: %s\n", request.RequestID)
}
```

---

## Webhook Security

### Verifying Webhook Signatures

All webhooks include an HMAC-SHA256 signature in the `X-VettID-Signature` header:

```
X-VettID-Signature: sha256=abc123...
```

Verify using your webhook secret:

```javascript
import crypto from 'crypto';

function verifyWebhookSignature(payload, signature, secret) {
  const expected = 'sha256=' + crypto
    .createHmac('sha256', secret)
    .update(JSON.stringify(payload))
    .digest('hex');

  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expected)
  );
}
```

### Webhook Headers

| Header | Description |
|--------|-------------|
| `X-VettID-Event-ID` | Unique event identifier |
| `X-VettID-Signature` | HMAC-SHA256 signature |
| `X-VettID-Timestamp` | Event timestamp (ISO 8601) |

---

## Migration Guide

### From v0.x to v1.x

1. Update API endpoint from `/v0/` to `/api/v1/`
2. Replace `auth_token` with `api_key` in configuration
3. Update webhook handlers to verify new signature format
4. Update contract status values: `approved` → `active`

---

## Support

- **Documentation**: [docs.vettid.com](https://docs.vettid.com)
- **Issues**: [GitHub Issues](https://github.com/vettid/vettid-service-vault/issues)
- **Email**: support@vettid.com
