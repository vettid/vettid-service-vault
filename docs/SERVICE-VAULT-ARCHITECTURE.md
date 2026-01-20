# VettID Service Vault Architecture

## Executive Summary

The **Service Vault** enables third-party applications and services to securely integrate with VettID users. While a user's personal vault (OwnerVault) is designed for direct user interaction via mobile apps, the Service Vault provides:

- **Secure API integration** for organizational services
- **Scalable multi-user support** (one Service Vault handles many users)
- **Event-driven communication** using the established NATS infrastructure
- **Zero-knowledge authentication** - services can verify user identity without accessing user credentials
- **Fine-grained authorization** - users explicitly grant services specific capabilities

---

## 1. Core Concepts

### 1.1 Terminology

| Term | Description |
|------|-------------|
| **Service Vault** | A vault deployed by a 3rd party organization to integrate their service with VettID |
| **Service Provider** | The organization operating the Service Vault |
| **OwnerVault** | The user's personal vault (existing VettID architecture) |
| **Service Connection** | An authorized relationship between a user and a service |
| **Capability Grant** | A specific permission granted by a user to a service |
| **Service Token** | A cryptographic token proving a service's identity |

### 1.2 Relationship Model

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           VettID Infrastructure                          │
│  ┌───────────────────┐                      ┌───────────────────┐       │
│  │    NATS Cluster   │◄────────────────────►│   Control Plane   │       │
│  │   (OwnerSpace)    │                      │    (Lambda/API)   │       │
│  │  (ServiceSpace)   │                      │                   │       │
│  └─────────┬─────────┘                      └───────────────────┘       │
│            │                                                             │
│     ┌──────┴──────┬───────────────────────────────────┐                 │
│     │             │                                   │                 │
│     ▼             ▼                                   ▼                 │
│  ┌──────┐     ┌──────┐                          ┌──────────┐           │
│  │User A│     │User B│         ...              │  User N  │           │
│  │Vault │     │Vault │                          │  Vault   │           │
│  └──┬───┘     └──┬───┘                          └────┬─────┘           │
│     │            │                                   │                 │
└─────┼────────────┼───────────────────────────────────┼─────────────────┘
      │            │                                   │
      │    Service Connections (Authorized)            │
      │            │                                   │
      ▼            ▼                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      Service Provider Infrastructure                     │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                        SERVICE VAULT                              │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐   │   │
│  │  │ Service Identity │  │ Connection Mgr  │  │  Event Router   │   │   │
│  │  │   & Auth Keys   │  │  (User Grants)  │  │                 │   │   │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘   │   │
│  │                                                                   │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐   │   │
│  │  │  API Gateway    │  │ Handler Engine  │  │  Audit Logger   │   │   │
│  │  │  (Service API)  │  │ (Business Logic)│  │                 │   │   │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌───────────────────────┐    ┌───────────────────────┐                 │
│  │   Service Backend     │    │   Service Frontend    │                 │
│  │   (3rd Party App)     │◄──►│   (Web/Mobile/API)    │                 │
│  └───────────────────────┘    └───────────────────────┘                 │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Identity & Authentication

### 2.1 Service Identity

Each Service Vault has a unique cryptographic identity, distinct from user identities:

```
Service Identity Structure:
├── service_guid: UUID (globally unique service identifier)
├── organization_id: string (registered organization)
├── service_name: string (human-readable)
├── service_type: enum (AUTHENTICATOR | AUTHORIZER | DATA_PROVIDER | INTEGRATION)
├── public_key: Ed25519 public key (for signature verification)
├── encryption_key: X25519 public key (for key exchange)
├── nats_account_id: string (NATS account for service)
└── registration_attestation: object (proof of legitimate registration)
```

### 2.2 Service Registration Flow

Services must be registered with VettID before deployment:

```
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│  Service Admin  │         │  VettID Portal  │         │  Control Plane  │
└────────┬────────┘         └────────┬────────┘         └────────┬────────┘
         │                           │                           │
         │  1. Register Service      │                           │
         │ ─────────────────────────►│                           │
         │   (org credentials,       │                           │
         │    service metadata,      │                           │
         │    callback URLs)         │                           │
         │                           │                           │
         │                           │  2. Create Service Record │
         │                           │ ─────────────────────────►│
         │                           │                           │
         │                           │  3. Generate Service Keys │
         │                           │◄───────────────────────── │
         │                           │   (signing key pair,      │
         │                           │    encryption key pair,   │
         │                           │    NATS account)          │
         │                           │                           │
         │  4. Return Service Bundle │                           │
         │◄───────────────────────── │                           │
         │   (service_guid,          │                           │
         │    private keys (secure), │                           │
         │    NATS credentials,      │                           │
         │    deployment config)     │                           │
         │                           │                           │
```

### 2.3 Service Authentication to NATS

Services authenticate to NATS using the same nkeys/JWT pattern as user vaults:

```typescript
// Service NATS JWT Structure
{
  "aud": "NATS",
  "exp": <timestamp + 30 days>,
  "iat": <timestamp>,
  "iss": <vettid_operator_public_key>,
  "jti": <unique_id>,
  "name": "service:<service_guid>",
  "nats": {
    "pub": {
      "allow": [
        "ServiceSpace.<service_guid>.>",           // Own namespace
        "ConnectionSpace.*.forService.<service_guid>.>" // User connections
      ]
    },
    "sub": {
      "allow": [
        "ServiceSpace.<service_guid>.>",
        "ConnectionSpace.*.forOwner.<service_guid>.>",  // From users
        "Control.service.<service_guid>.>"              // Control commands
      ]
    },
    "subs": 10000,      // Higher limit for multi-user
    "data": 50000000,   // 50 MB/sec for scale
    "payload": 1048576  // 1 MB max message
  },
  "sub": <service_account_public_key>
}
```

### 2.4 Service-to-VettID Mutual Authentication

```
┌─────────────────┐                              ┌─────────────────┐
│  Service Vault  │                              │  NATS + Control │
└────────┬────────┘                              └────────┬────────┘
         │                                                │
         │  1. Connect with Service JWT                   │
         │ ──────────────────────────────────────────────►│
         │                                                │
         │  2. TLS handshake (server cert validation)     │
         │◄──────────────────────────────────────────────►│
         │                                                │
         │  3. JWT signature validation (Ed25519)         │
         │                                                │ ✓
         │                                                │
         │  4. Connection established                     │
         │◄────────────────────────────────────────────── │
         │                                                │
         │  5. Subscribe to service topics                │
         │ ──────────────────────────────────────────────►│
         │     ServiceSpace.<service_guid>.>              │
         │     ConnectionSpace.*.forOwner.<service_guid>.>│
         │                                                │
```

---

## 3. User-Service Connections

### 3.1 Connection Authorization Flow

Users must explicitly authorize services to interact with their vault:

```
┌────────────┐     ┌────────────┐     ┌─────────────┐     ┌──────────────┐
│  User App  │     │ User Vault │     │ Control API │     │ Service Vault│
└─────┬──────┘     └─────┬──────┘     └──────┬──────┘     └──────┬───────┘
      │                  │                   │                   │
      │ 1. User initiates│connection        │                   │
      │   (scan QR/link) │                   │                   │
      │ ─────────────────►                   │                   │
      │                  │                   │                   │
      │ 2. Fetch service │info              │                   │
      │ ─────────────────┼──────────────────►│                   │
      │                  │                   │                   │
      │ 3. Return service│details           │                   │
      │◄─────────────────┼───────────────── │                   │
      │   (name, org,    │                   │                   │
      │    capabilities  │                   │                   │
      │    requested)    │                   │                   │
      │                  │                   │                   │
      │ 4. User reviews &│approves          │                   │
      │   grants         │                   │                   │
      │ ─────────────────►                   │                   │
      │                  │                   │                   │
      │                  │ 5. Create connection                 │
      │                  │    grant record   │                   │
      │                  │ ─────────────────►│                   │
      │                  │                   │                   │
      │                  │ 6. Notify service │of new connection │
      │                  │                   │ ─────────────────►│
      │                  │                   │                   │
      │                  │                   │  7. Service acks  │
      │                  │                   │◄───────────────── │
      │                  │                   │                   │
      │                  │ 8. Exchange initial keys             │
      │                  │◄─────────────────────────────────────►│
      │                  │   (X25519 key agreement)             │
      │                  │                   │                   │
      │ 9. Connection    │established       │                   │
      │◄─────────────────┼──────────────────┼───────────────────►
      │                  │                   │                   │
```

### 3.2 Connection Grant Structure

```typescript
interface ConnectionGrant {
  connection_id: string;          // Unique connection identifier
  user_guid: string;              // User's VettID GUID
  service_guid: string;           // Service's identifier

  // What the service can do
  capabilities: CapabilityGrant[];

  // Connection security
  user_connection_key: string;    // User's X25519 public key for this connection
  service_connection_key: string; // Service's X25519 public key for this connection
  key_rotation_schedule: string;  // e.g., "7d" for weekly rotation

  // Lifecycle
  created_at: string;             // ISO8601
  expires_at: string | null;      // null = no expiration
  last_used_at: string;
  revoked_at: string | null;
  revoked_by: 'user' | 'service' | 'admin' | null;

  // Audit
  consent_record: ConsentRecord;  // What user agreed to
}

interface CapabilityGrant {
  capability: string;             // e.g., "authenticate", "read_profile", "sign_document"
  scope: string[];                // Specific resources/contexts
  constraints: object;            // Additional limitations
  granted_at: string;
  expires_at: string | null;
}

interface ConsentRecord {
  consent_version: string;        // Service's ToS version
  consent_timestamp: string;
  user_signature: string;         // User signed the consent
  presented_capabilities: string[];
}
```

### 3.3 Capability Types

Standard capabilities that services can request:

| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `authenticate` | Verify user identity | "Confirm your identity to this service" |
| `read_profile` | Read basic profile info | "Share your name and contact info" |
| `read_credentials` | Read specific credential types | "Share your [credential type]" |
| `sign_request` | Request user signature | "Sign documents on your behalf" |
| `receive_notifications` | Send notifications to user | "Send you notifications" |
| `verify_presence` | Check if user is online | "See when you're available" |
| `authorization_request` | Request authorization decisions | "Ask for your approval on actions" |

Custom capabilities can be defined by services and must be approved during registration.

---

## 4. NATS Topic Architecture

### 4.1 New Namespaces for Services

```
NATS Topic Hierarchy:
│
├── OwnerSpace.<user_guid>/           # Existing: User ↔ User Vault
│   ├── forVault.>
│   ├── forApp.>
│   └── ...
│
├── ServiceSpace.<service_guid>/       # NEW: Service internal operations
│   ├── control.>                      # Service control commands
│   ├── health.>                       # Health/status
│   ├── metrics.>                      # Operational metrics
│   └── internal.>                     # Service-internal messaging
│
├── ConnectionSpace.<user_guid>/       # NEW: User ↔ Service communication
│   ├── forOwner.<service_guid>.>      # Service → User Vault
│   │   ├── auth.request               # Authentication requests
│   │   ├── authz.request              # Authorization requests
│   │   ├── data.request               # Data requests
│   │   └── notify.>                   # Notifications
│   │
│   └── forService.<service_guid>.>    # User Vault → Service
│       ├── auth.response.<event_id>
│       ├── authz.response.<event_id>
│       ├── data.response.<event_id>
│       └── events.>                   # User-initiated events
│
├── Control/                           # Existing + Extended
│   ├── global.*
│   ├── enclave.*
│   ├── user.*
│   └── service.<service_guid>.*       # NEW: Service control
│
└── Directory/                         # NEW: Service discovery
    ├── services.list
    ├── services.<service_guid>.info
    └── services.<service_guid>.status
```

### 4.2 Message Flow: Authentication Request

```
┌─────────────────┐                                   ┌─────────────────┐
│  Service Vault  │                                   │   User Vault    │
└────────┬────────┘                                   └────────┬────────┘
         │                                                     │
         │ 1. Publish auth request                             │
         │ ───────────────────────────────────────────────────►│
         │    Topic: ConnectionSpace.<user_guid>.              │
         │           forOwner.<service_guid>.auth.request      │
         │    Payload: {                                       │
         │      event_id: "uuid",                              │
         │      event_type: "auth.request",                    │
         │      timestamp: "ISO8601",                          │
         │      encrypted_payload: {                           │
         │        challenge: "random_bytes",                   │
         │        purpose: "login",                            │
         │        context: {...}                               │
         │      }                                              │
         │    }                                                │
         │                                                     │
         │                    2. Vault processes request       │
         │                       - Decrypts with connection key│
         │                       - Validates service identity  │
         │                       - Checks capability grants    │
         │                       - May prompt user via app     │
         │                                                     │
         │                    3. Publish response              │
         │◄─────────────────────────────────────────────────── │
         │    Topic: ConnectionSpace.<user_guid>.              │
         │           forService.<service_guid>.auth.response.  │
         │           <event_id>                                │
         │    Payload: {                                       │
         │      response_id: "uuid",                           │
         │      event_id: "original_event_id",                 │
         │      status: "success|denied|error",                │
         │      encrypted_payload: {                           │
         │        signed_challenge: "...",                     │
         │        user_attestation: {...},                     │
         │        new_connection_key: "..."  // Key rotation   │
         │      }                                              │
         │    }                                                │
         │                                                     │
```

### 4.3 Message Encryption

All ConnectionSpace messages use X25519 + XChaCha20-Poly1305, matching the existing pattern:

```typescript
interface EncryptedMessage {
  event_id: string;
  event_type: string;
  timestamp: string;

  // Ephemeral key for this message (perfect forward secrecy)
  ephemeral_public_key: string;  // X25519

  // Encrypted with: ECDH(ephemeral_private, recipient_connection_key)
  ciphertext: string;            // XChaCha20-Poly1305
  nonce: string;                 // 24-byte nonce

  // Signature for authenticity
  signature: string;             // Ed25519 signature over (event_id || ciphertext)
}
```

---

## 5. Service Vault Architecture

### 5.1 Deployment Model

Unlike user vaults (which run in Nitro Enclaves for hardware security), Service Vaults are deployed by the service provider:

```
Option A: VettID-Hosted Service Vault (Recommended for SMBs)
┌──────────────────────────────────────────────────────────────┐
│                    VettID Cloud                              │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Nitro Enclave Cluster                                 │  │
│  │  ┌─────────────────┐  ┌─────────────────┐              │  │
│  │  │ Service Vault A │  │ Service Vault B │  ...         │  │
│  │  │ (Tenant: Acme)  │  │ (Tenant: Corp)  │              │  │
│  │  └─────────────────┘  └─────────────────┘              │  │
│  └────────────────────────────────────────────────────────┘  │
│                              │                               │
│                         NATS Cluster                         │
└──────────────────────────────────────────────────────────────┘
         │                     │
         ▼                     ▼
   Acme Backend           Corp Backend
   (Customer App)         (Customer App)


Option B: Self-Hosted Service Vault (Enterprise)
┌──────────────────────────────────────────────────────────────┐
│                 Enterprise Infrastructure                     │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Service Vault Container/VM                            │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │  │
│  │  │ NATS Client  │  │ Handler Eng. │  │ Service API  │  │  │
│  │  └──────────────┘  └──────────────┘  └──────────────┘  │  │
│  └───────────────────────────┬────────────────────────────┘  │
│                              │ Outbound only                 │
│                         TLS/mTLS                             │
└──────────────────────────────┼───────────────────────────────┘
                               │
                               ▼
                    ┌──────────────────┐
                    │  VettID NATS     │
                    │  (Public Edge)   │
                    └──────────────────┘
```

### 5.2 Internal Components

```
Service Vault Process Architecture:
┌─────────────────────────────────────────────────────────────────────────┐
│                           SERVICE VAULT                                  │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                        SECURITY LAYER                            │    │
│  │  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐    │    │
│  │  │ Key Manager   │  │ Auth Verifier │  │ Audit Logger      │    │    │
│  │  │ - Service keys│  │ - JWT valid.  │  │ - All operations  │    │    │
│  │  │ - Connection  │  │ - Signature   │  │ - Tamper-proof    │    │    │
│  │  │   key store   │  │   verification│  │                   │    │    │
│  │  └───────────────┘  └───────────────┘  └───────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                      CONNECTION LAYER                            │    │
│  │  ┌───────────────────────────┐  ┌───────────────────────────┐   │    │
│  │  │    NATS Client Manager    │  │   Connection State Store  │   │    │
│  │  │ - Reconnection handling   │  │ - Active connections      │   │    │
│  │  │ - Subscription management │  │ - Key rotation tracking   │   │    │
│  │  │ - Message routing         │  │ - Session state           │   │    │
│  │  └───────────────────────────┘  └───────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                       BUSINESS LAYER                             │    │
│  │  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐    │    │
│  │  │ Handler Engine│  │ Request Queue │  │ Rate Limiter      │    │    │
│  │  │ - Auth handler│  │ - Priority    │  │ - Per-user limits │    │    │
│  │  │ - Data handler│  │ - Timeout     │  │ - Global limits   │    │    │
│  │  │ - Custom      │  │ - Retry logic │  │                   │    │    │
│  │  └───────────────┘  └───────────────┘  └───────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                        API LAYER                                 │    │
│  │  ┌───────────────────────────┐  ┌───────────────────────────┐   │    │
│  │  │    Service API Gateway    │  │   Webhook Dispatcher      │   │    │
│  │  │ - REST/gRPC endpoints     │  │ - Event notifications     │   │    │
│  │  │ - SDK integration points  │  │ - Delivery guarantees     │   │    │
│  │  │ - mTLS authentication     │  │ - Retry with backoff      │   │    │
│  │  └───────────────────────────┘  └───────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.3 Service API for Third Parties

The Service Vault exposes an API for the 3rd party's backend:

```typescript
// Service Vault API (for 3rd party backend integration)

// Authentication
POST /api/v1/auth/request
{
  user_id: string,           // VettID user identifier (from connection)
  purpose: string,           // Why authentication is needed
  context: object,           // Additional context
  callback_url?: string,     // Webhook for async response
  timeout_seconds?: number   // Max wait time (default: 30)
}
Response: {
  request_id: string,
  status: "pending" | "completed" | "timeout" | "denied",
  result?: {
    authenticated: boolean,
    user_attestation: object,
    timestamp: string
  }
}

// Authorization
POST /api/v1/authz/request
{
  user_id: string,
  action: string,            // What action needs authorization
  resource: string,          // What resource
  context: object,
  callback_url?: string
}
Response: {
  request_id: string,
  status: "pending" | "approved" | "denied" | "timeout",
  decision?: {
    allowed: boolean,
    reason?: string,
    constraints?: object,
    expires_at?: string
  }
}

// User Data (with explicit consent)
GET /api/v1/users/{user_id}/profile
Response: {
  user_id: string,
  display_name: string,      // Only if granted
  // ... other consented fields
}

// Connection Management
GET /api/v1/connections
POST /api/v1/connections/invite    // Generate connection invite
DELETE /api/v1/connections/{id}    // Revoke connection

// Webhooks (from Service Vault to 3rd party)
POST {callback_url}
{
  event_type: "auth.completed" | "authz.decision" | "connection.established" | "connection.revoked",
  event_id: string,
  timestamp: string,
  data: object
}
```

---

## 6. Scalability Design

### 6.1 Challenges

Unlike user vaults (1 vault = 1 user), Service Vaults face:

1. **Many-to-One**: One service vault handles thousands/millions of users
2. **Burst Traffic**: Login spikes, batch operations
3. **State Management**: Tracking many active connections
4. **Key Management**: Per-user connection keys at scale

### 6.2 Scaling Architecture

```
                                    Load Balancer (NLB)
                                           │
                    ┌──────────────────────┼──────────────────────┐
                    │                      │                      │
                    ▼                      ▼                      ▼
            ┌──────────────┐       ┌──────────────┐       ┌──────────────┐
            │ SV Instance 1│       │ SV Instance 2│       │ SV Instance N│
            │              │       │              │       │              │
            │ ┌──────────┐ │       │ ┌──────────┐ │       │ ┌──────────┐ │
            │ │NATS Conn │ │       │ │NATS Conn │ │       │ │NATS Conn │ │
            │ │Pool      │ │       │ │Pool      │ │       │ │Pool      │ │
            │ └──────────┘ │       │ └──────────┘ │       │ └──────────┘ │
            └──────┬───────┘       └──────┬───────┘       └──────┬───────┘
                   │                      │                      │
                   └──────────────────────┼──────────────────────┘
                                          │
                                          ▼
                              ┌───────────────────────┐
                              │   Shared State Store  │
                              │   (Redis Cluster)     │
                              │                       │
                              │ - Connection state    │
                              │ - Key cache           │
                              │ - Rate limit counters │
                              │ - Request dedup       │
                              └───────────────────────┘
                                          │
                   ┌──────────────────────┼──────────────────────┐
                   │                      │                      │
                   ▼                      ▼                      ▼
            ┌──────────────┐       ┌──────────────┐       ┌──────────────┐
            │Persistent    │       │Audit Log     │       │Key Vault     │
            │Store (DB)    │       │(Immutable)   │       │(HSM/KMS)     │
            │              │       │              │       │              │
            │- Connections │       │- All events  │       │- Master keys │
            │- Grants      │       │- Signatures  │       │- Key derivation
            │- Metadata    │       │              │       │              │
            └──────────────┘       └──────────────┘       └──────────────┘
```

### 6.3 Connection Sharding

For services with millions of users, shard connections:

```
User Connection Routing:
┌─────────────────────────────────────────────────────────────────┐
│                         NATS Cluster                             │
│                                                                  │
│  ConnectionSpace.<user_guid>.forOwner.<service_guid>.*          │
│                     │                                            │
│                     │ Consistent hash routing                    │
│                     ▼                                            │
│  ┌─────────────┬─────────────┬─────────────┬─────────────┐      │
│  │  Shard 0    │  Shard 1    │  Shard 2    │  Shard N    │      │
│  │ Users A-F   │ Users G-M   │ Users N-S   │ Users T-Z   │      │
│  │             │             │             │             │      │
│  │ SV Pod 0-2  │ SV Pod 3-5  │ SV Pod 6-8  │ SV Pod 9-11 │      │
│  └─────────────┴─────────────┴─────────────┴─────────────┘      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 6.4 Request Processing Pipeline

```
Incoming Request → Request Queue → Worker Pool → Response
                        │
                        ▼
                 ┌──────────────────┐
                 │ Priority Levels  │
                 │                  │
                 │ P0: Auth (real-  │
                 │     time login)  │
                 │                  │
                 │ P1: Authz        │
                 │     (decisions)  │
                 │                  │
                 │ P2: Data         │
                 │     (queries)    │
                 │                  │
                 │ P3: Batch        │
                 │     (bulk ops)   │
                 └──────────────────┘
```

---

## 7. Security Considerations

### 7.1 Threat Model

| Threat | Mitigation |
|--------|------------|
| **Compromised Service Vault** | Connection keys are per-user; compromise affects only that service's access, not user's vault |
| **Service Impersonation** | Ed25519 signatures on all messages; NATS JWT authentication |
| **Replay Attacks** | Event IDs, timestamps (5-min window), idempotency tracking |
| **Key Compromise** | Automatic key rotation; perfect forward secrecy with ephemeral keys |
| **Denial of Service** | Per-user and global rate limits; priority queuing |
| **Data Exfiltration** | Users control what data is shared; audit logging |
| **MITM on NATS** | TLS 1.3; message-level encryption; PCR attestation binding |
| **Malicious Service** | Registration vetting; capability approval; user consent required |

### 7.2 Security Boundaries

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        TRUST BOUNDARY: VettID                            │
│                                                                          │
│  ┌────────────────┐        ┌────────────────┐        ┌───────────────┐  │
│  │  User Vault    │◄──────►│  NATS Cluster  │◄──────►│ Control Plane │  │
│  │  (Full Trust)  │        │  (Transport)   │        │ (Admin Trust) │  │
│  └────────────────┘        └───────┬────────┘        └───────────────┘  │
│                                    │                                     │
└────────────────────────────────────┼─────────────────────────────────────┘
                                     │
                    Message-level encryption
                    + Capability enforcement
                                     │
┌────────────────────────────────────┼─────────────────────────────────────┐
│                        TRUST BOUNDARY: Service Provider                  │
│                                     │                                    │
│  ┌────────────────┐                │                                    │
│  │ Service Vault  │◄───────────────┘                                    │
│  │ (Limited Trust)│                                                     │
│  │                │                                                     │
│  │ Can only:      │                                                     │
│  │ - Access data  │                                                     │
│  │   user granted │                                                     │
│  │ - Send allowed │                                                     │
│  │   message types│                                                     │
│  │ - Within rate  │                                                     │
│  │   limits       │                                                     │
│  └────────────────┘                                                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 7.3 Audit Requirements

All Service Vault operations must be logged:

```typescript
interface AuditEvent {
  event_id: string;
  timestamp: string;

  // Actor
  service_guid: string;
  service_instance_id: string;

  // Target
  user_guid: string;
  connection_id: string;

  // Operation
  operation: string;          // e.g., "auth.request", "data.read"
  capability_used: string;

  // Outcome
  status: "success" | "denied" | "error";
  error_code?: string;

  // Context
  request_context: object;    // Sanitized (no PII)
  response_summary: object;   // What was returned (summary only)

  // Integrity
  previous_event_hash: string;
  event_hash: string;         // SHA-256(event || previous_hash)
  signature: string;          // Service's Ed25519 signature
}
```

### 7.4 Key Rotation

```
Connection Key Rotation Schedule:
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│  Initial Connection:                                           │
│  User generates: X25519 keypair (user_connection_key)          │
│  Service generates: X25519 keypair (service_connection_key)    │
│  Exchange public keys during connection establishment          │
│                                                                │
│  Ongoing Rotation (every message OR time-based):               │
│                                                                │
│  Message N:                                                    │
│  - Encrypt with current shared secret                          │
│  - Include new_ephemeral_public_key in response                │
│  - Derive new shared secret for Message N+1                    │
│                                                                │
│  Time-based Rotation (configurable, default 7 days):           │
│  - Service initiates: "key_rotation" message                   │
│  - User vault responds with new public key                     │
│  - Both parties update connection keys                         │
│  - Old keys retained for 1 hour (in-flight messages)           │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

---

## 8. Implementation Phases

### Phase 1: Foundation (MVP)
- [ ] Service registration system
- [ ] Service Vault core (NATS connection, key management)
- [ ] Basic authentication flow (service → user → response)
- [ ] Connection establishment flow
- [ ] Simple REST API for 3rd party integration

### Phase 2: Core Features
- [ ] Authorization request/response flow
- [ ] Capability grant management
- [ ] User data access (profile, credentials)
- [ ] Webhook delivery system
- [ ] Audit logging

### Phase 3: Scale & Security
- [ ] Horizontal scaling (sharding, load balancing)
- [ ] Rate limiting and priority queuing
- [ ] Key rotation automation
- [ ] VettID-hosted deployment option

### Phase 4: Advanced
- [ ] SDK for common platforms (Node.js, Python, Go)
- [ ] Custom handler support (service-defined capabilities)
- [ ] Batch operations
- [ ] Analytics dashboard

---

## 9. Open Questions for Review

1. **Enclave Requirement**: Should self-hosted Service Vaults require Nitro Enclaves, or is TLS + message encryption sufficient?

2. **Connection Persistence**: How long should inactive connections persist before auto-revocation?

3. **Offline Support**: Should services be able to cache user authorization for offline scenarios?

4. **Multi-Vault Services**: Can a service have multiple vaults (e.g., regional deployment)?

5. **Service Directory**: Should there be a public directory of registered services for users to browse?

6. **Billing Model**: Per-connection? Per-request? Tiered by capability?

7. **Cross-Service Communication**: Should services be able to communicate with each other via VettID?

---

## 10. File Structure (Proposed)

```
vettid-service-vault/
├── README.md
├── docs/
│   ├── SERVICE-VAULT-ARCHITECTURE.md  (this document)
│   ├── API-REFERENCE.md
│   ├── SECURITY-MODEL.md
│   ├── DEPLOYMENT-GUIDE.md
│   └── SDK-INTEGRATION.md
├── cdk/                               # AWS CDK infrastructure
│   ├── lib/
│   │   ├── service-vault-stack.ts
│   │   ├── service-registration-stack.ts
│   │   └── service-nats-stack.ts
│   └── lambda/
│       ├── handlers/
│       │   ├── service-registration.ts
│       │   ├── connection-management.ts
│       │   └── service-control.ts
│       └── common/
│           ├── service-jwt.ts
│           └── service-crypto.ts
├── vault/                             # Service Vault implementation
│   ├── cmd/
│   │   └── service-vault/
│   │       └── main.go
│   ├── internal/
│   │   ├── nats/                     # NATS connection management
│   │   ├── crypto/                   # Key management, encryption
│   │   ├── handlers/                 # Message handlers
│   │   ├── api/                      # REST/gRPC API
│   │   ├── store/                    # Connection state storage
│   │   └── audit/                    # Audit logging
│   └── pkg/
│       └── sdk/                      # Embeddable SDK components
├── sdk/                              # Client SDKs for 3rd parties
│   ├── node/
│   ├── python/
│   └── go/
└── examples/
    ├── basic-auth-service/
    └── enterprise-integration/
```

---

## Appendix A: Comparison with User Vault

| Aspect | User Vault (OwnerVault) | Service Vault |
|--------|------------------------|---------------|
| **Operator** | VettID (managed) | Service Provider (self or VettID-hosted) |
| **Users** | 1 user per vault | Many users per vault |
| **Primary Interface** | Mobile App | REST/gRPC API |
| **Runs In** | Nitro Enclave (required) | Container/VM (enclave optional) |
| **Holds** | User's encrypted credentials | Service identity + connection keys |
| **NATS Namespace** | OwnerSpace | ServiceSpace + ConnectionSpace |
| **Trust Level** | Full (user's own vault) | Limited (only granted capabilities) |
| **Key Management** | User holds master key | Service holds service keys; user holds connection keys |

---

## Appendix B: Message Sequence Examples

### B.1 SSO Login Flow

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│   User   │     │  3P App  │     │  Service │     │   NATS   │     │   User   │
│ Browser  │     │  Backend │     │  Vault   │     │          │     │  Vault   │
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │                │
     │ 1. Click       │                │                │                │
     │ "Login with    │                │                │                │
     │  VettID"       │                │                │                │
     │ ──────────────►│                │                │                │
     │                │                │                │                │
     │                │ 2. POST        │                │                │
     │                │ /auth/request  │                │                │
     │                │ ──────────────►│                │                │
     │                │                │                │                │
     │                │                │ 3. Publish     │                │
     │                │                │ auth.request   │                │
     │                │                │ ──────────────►│                │
     │                │                │                │                │
     │                │                │                │ 4. Route to    │
     │                │                │                │ user vault     │
     │                │                │                │ ──────────────►│
     │                │                │                │                │
     │                │                │                │                │ 5. Process
     │                │                │                │                │ (may prompt
     │                │                │                │                │  via app)
     │                │                │                │                │
     │                │                │                │ 6. Publish     │
     │                │                │                │ auth.response  │
     │                │                │                │◄────────────── │
     │                │                │                │                │
     │                │                │ 7. Receive     │                │
     │                │                │ response       │                │
     │                │                │◄────────────── │                │
     │                │                │                │                │
     │                │ 8. Return      │                │                │
     │                │ auth result    │                │                │
     │                │◄────────────── │                │                │
     │                │                │                │                │
     │ 9. Redirect    │                │                │                │
     │ logged in      │                │                │                │
     │◄────────────── │                │                │                │
     │                │                │                │                │
```

### B.2 Authorization Decision Flow

```
User Action: "Transfer $500 to external account"
    │
    ▼
┌────────────┐
│ 3P Backend │ ─── POST /authz/request ───►┌──────────────┐
│            │     {                        │ Service Vault│
│            │       user_id: "...",        │              │
│            │       action: "transfer",    │              │
│            │       resource: "account",   │              │
│            │       context: {             │              │
│            │         amount: 500,         │              │
│            │         destination: "ext"   │              │
│            │       }                      │              │
│            │     }                        │              │
└────────────┘                              └──────┬───────┘
                                                   │
                                                   │ NATS: authz.request
                                                   ▼
                                            ┌──────────────┐
                                            │  User Vault  │
                                            │              │
                                            │ Policy eval: │
                                            │ - Amount>100 │
                                            │   requires   │
                                            │   user OK    │
                                            └──────┬───────┘
                                                   │
                                                   │ Push notification
                                                   ▼
                                            ┌──────────────┐
                                            │  User App    │
                                            │              │
                                            │ "Acme Bank   │
                                            │  wants to    │
                                            │  transfer    │
                                            │  $500"       │
                                            │              │
                                            │ [Approve]    │
                                            │ [Deny]       │
                                            └──────┬───────┘
                                                   │
                                                   │ User taps Approve
                                                   ▼
                                            ┌──────────────┐
                                            │  User Vault  │
                                            │              │
                                            │ authz.response
                                            │ {allowed:true}
                                            └──────┬───────┘
                                                   │
                                                   │ NATS: authz.response
                                                   ▼
                                            ┌──────────────┐
                                            │ Service Vault│
                                            └──────┬───────┘
                                                   │
                                                   │ Webhook/Response
                                                   ▼
                                            ┌──────────────┐
                                            │ 3P Backend   │
                                            │              │
                                            │ Proceed with │
                                            │ transfer     │
                                            └──────────────┘
```
