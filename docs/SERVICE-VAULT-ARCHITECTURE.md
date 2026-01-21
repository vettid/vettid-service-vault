# VettID Service Vault Architecture

## Executive Summary

The **Service Vault** enables third-party applications and services to securely integrate with VettID users. While a user's personal vault (OwnerVault) is designed for direct user interaction via mobile apps, the Service Vault provides:

- **Secure API integration** for organizational services
- **Scalable multi-user support** (one Service Vault handles many users)
- **Event-driven communication** using NATS as a secure message drop-box
- **Zero-knowledge authentication** - services can verify user identity without accessing user credentials
- **User-controlled authorization** - users explicitly grant services specific capabilities via Connection Contracts
- **User-centric data model** - services store user-specific secrets in the user's vault, not in the service vault

---

## 1. Core Concepts

### 1.1 Terminology

| Term | Description |
|------|-------------|
| **Service Vault** | A vault deployed and controlled by a 3rd party organization to integrate their service with VettID |
| **Service Provider** | The organization operating the Service Vault |
| **OwnerVault** | The user's personal vault (existing VettID architecture) |
| **Service Connection** | An authorized relationship between a user and a service |
| **Connection Contract** | The agreement defining what capabilities a user grants to a service |
| **Service Registry** | VettID's directory of approved services (requires VettID approval) |
| **Service Directory** | User-facing catalog for discovering and connecting to services |

### 1.2 Relationship Model

The Service Vault operates its own ServiceSpace on the VettID NATS cluster. User vaults communicate with services through this ServiceSpace, enabling asynchronous message-based communication without requiring persistent network connections.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           VettID Infrastructure                              │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         NATS Cluster                                 │    │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐  │    │
│  │  │   OwnerSpace    │  │  ServiceSpace   │  │   Control Plane     │  │    │
│  │  │  (User ↔ App)   │  │ (User ↔ Service)│  │   (Lambda/API)      │  │    │
│  │  └────────┬────────┘  └────────┬────────┘  └─────────────────────┘  │    │
│  │           │                    │                                     │    │
│  └───────────┼────────────────────┼─────────────────────────────────────┘    │
│              │                    │                                          │
│              │    ┌───────────────┴───────────────┐                          │
│              │    │                               │                          │
│              ▼    ▼                               ▼                          │
│  ┌──────────────────────┐             ┌──────────────────────┐              │
│  │  User A OwnerVault   │             │  User N OwnerVault   │              │
│  │  ┌────────────────┐  │             │  ┌────────────────┐  │              │
│  │  │ User Data      │  │             │  │ User Data      │  │              │
│  │  │ Service Secrets│◄─┼─────────────┼──│ Service Secrets│  │              │
│  │  │ (per-service)  │  │             │  │ (per-service)  │  │              │
│  │  └────────────────┘  │             │  └────────────────┘  │              │
│  └──────────────────────┘             └──────────────────────┘              │
│              │                               │                               │
│              │      NATS: Secure Drop-Box    │                               │
│              │      (No persistent connections)                              │
│              │                               │                               │
└──────────────┼───────────────────────────────┼───────────────────────────────┘
               │                               │
               │    ServiceSpace.<svc_guid>    │
               │    (Messages queued until     │
               │     recipient picks up)       │
               │                               │
               ▼                               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Service Provider Infrastructure                         │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐     │
│  │                        SERVICE VAULT                                │     │
│  │  (Controlled entirely by Service Provider)                          │     │
│  │                                                                      │     │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐     │     │
│  │  │ Service Identity │  │ Contract Mgr    │  │  Event Router   │     │     │
│  │  │ (Provider's keys)│  │ (User Contracts)│  │                 │     │     │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘     │     │
│  │                                                                      │     │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐     │     │
│  │  │  API Gateway    │  │ Handler Engine  │  │  Audit Logger   │     │     │
│  │  │  (Service API)  │  │ (Business Logic)│  │                 │     │     │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘     │     │
│  └────────────────────────────────────────────────────────────────────┘     │
│                                                                              │
│  ┌───────────────────────┐    ┌───────────────────────┐                     │
│  │   Service Backend     │    │   Service Frontend    │                     │
│  │   (3rd Party App)     │◄──►│   (Web/Mobile/API)    │                     │
│  └───────────────────────┘    └───────────────────────┘                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Key Principles:**
1. **NATS as Drop-Box**: Messages are queued in NATS until the recipient picks them up. No persistent connections required between user vaults and service vaults.
2. **User Vault Stores Secrets**: Service-specific user secrets are stored in the user's vault, not the service vault. The service vault only holds connection keys for message encryption.
3. **Service Controls Everything**: The service provider generates and manages all their own keys, infrastructure, and deployment.

---

## 2. Identity & Authentication

### 2.1 Service Identity

Each Service Vault has a unique cryptographic identity **generated and controlled by the service provider**:

```
Service Identity Structure:
├── service_guid: UUID (globally unique, assigned by VettID registry)
├── organization_id: string (registered organization)
├── service_name: string (human-readable)
├── service_type: enum (AUTHENTICATOR | AUTHORIZER | DATA_PROVIDER | INTEGRATION | SUPPORT | PAYMENT)
├── public_key: Ed25519 public key (provider-generated, registered with VettID)
├── encryption_key: X25519 public key (provider-generated, for key exchange)
├── nats_account_id: string (NATS account for service)
├── service_directory_entry: object (public listing information)
└── handler_manifest: object (supported event types/capabilities)
```

### 2.2 Service Registration Flow

VettID provides a **Service Registry** where services register their details. VettID may require approval before a service can connect to users. **The service provider controls all their own keys and infrastructure.**

```
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│  Service Admin  │         │  VettID Portal  │         │  Control Plane  │
│  (3rd Party)    │         │  (Registry)     │         │                 │
└────────┬────────┘         └────────┬────────┘         └────────┬────────┘
         │                           │                           │
         │  1. Generate service keys │                           │
         │     locally (Ed25519,     │                           │
         │     X25519 keypairs)      │                           │
         │                           │                           │
         │  2. Register Service      │                           │
         │ ─────────────────────────►│                           │
         │   - Organization details  │                           │
         │   - Service metadata      │                           │
         │   - PUBLIC keys only      │                           │
         │   - Handler manifest      │                           │
         │   - Directory listing     │                           │
         │                           │                           │
         │                           │  3. Review & Approve      │
         │                           │     (VettID vetting)      │
         │                           │ ─────────────────────────►│
         │                           │                           │
         │                           │  4. Create NATS account   │
         │                           │     (permissions based    │
         │                           │      on service type)     │
         │                           │◄───────────────────────── │
         │                           │                           │
         │  5. Return Registration   │                           │
         │◄───────────────────────── │                           │
         │   - service_guid          │                           │
         │   - NATS account details  │                           │
         │   - Endpoint configuration│                           │
         │                           │                           │
         │  6. Deploy Service Vault  │                           │
         │     with own keys         │                           │
         │     (provider controls)   │                           │
         │                           │                           │
```

**Important**: VettID never possesses the service's private keys. The service provider:
- Generates their own Ed25519 signing keypair
- Generates their own X25519 encryption keypair
- Registers only public keys with VettID
- Maintains complete control over their private keys and infrastructure

### 2.3 Service Authentication to NATS

Services authenticate to NATS using the nkeys/JWT pattern. The service signs its connection request with its private key, proving ownership of the registered public key.

```typescript
// Service NATS JWT Structure (issued by VettID after registration approval)
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
        "ServiceSpace.<service_guid>.>",              // Own namespace
        "ServiceSpace.<service_guid>.forUser.*.>"    // Messages to users
      ]
    },
    "sub": {
      "allow": [
        "ServiceSpace.<service_guid>.>",
        "ServiceSpace.<service_guid>.fromUser.*.>",  // Messages from users
        "Control.service.<service_guid>.>"           // Control commands
      ]
    },
    "subs": 10000,      // Higher limit for multi-user
    "data": 50000000,   // 50 MB/sec for scale
    "payload": 1048576  // 1 MB max message
  },
  "sub": <service_account_public_key>
}
```

### 2.4 Handler Manifest (Service Directory Integration)

Similar to how VettID's Service Directory lists event handlers, each Service Vault publishes a **Handler Manifest** describing its capabilities:

```typescript
interface HandlerManifest {
  service_guid: string;
  version: string;

  // Capabilities this service offers
  capabilities: {
    capability_id: string;
    name: string;
    description: string;
    required_user_grants: string[];  // What user must approve
    request_schema: object;          // JSON schema for requests
    response_schema: object;         // JSON schema for responses
  }[];

  // Event types this service can handle
  event_handlers: {
    event_type: string;
    description: string;
    timeout_default: number;         // Default timeout in seconds
    supports_offline: boolean;       // Can queue for offline users
  }[];

  // Subscription/payment options (if applicable)
  subscription_plans?: SubscriptionPlan[];
}
```

---

## 3. User-Service Connections

### 3.1 Connection Contract Establishment

Users must explicitly authorize services via a **Connection Contract**. The contract defines what the service can do and persists until either party cancels it.

```
┌────────────┐     ┌────────────┐     ┌─────────────┐     ┌──────────────┐
│  User App  │     │ User Vault │     │ VettID API  │     │ Service Vault│
└─────┬──────┘     └─────┬──────┘     └──────┬──────┘     └──────┬───────┘
      │                  │                   │                   │
      │ 1. User browses  │                   │                   │
      │    Service       │                   │                   │
      │    Directory     │                   │                   │
      │ ─────────────────┼──────────────────►│                   │
      │                  │                   │                   │
      │ 2. Select service│                   │                   │
      │    to connect    │                   │                   │
      │ ─────────────────►                   │                   │
      │                  │                   │                   │
      │                  │ 3. Fetch service  │                   │
      │                  │    contract terms │                   │
      │                  │ ─────────────────►│                   │
      │                  │                   │                   │
      │ 4. Display       │                   │                   │
      │    contract      │                   │                   │
      │    (capabilities,│                   │                   │
      │     terms, costs)│                   │                   │
      │◄─────────────────┤                   │                   │
      │                  │                   │                   │
      │ 5. User reviews  │                   │                   │
      │    & signs       │                   │                   │
      │    contract      │                   │                   │
      │ ─────────────────►                   │                   │
      │                  │                   │                   │
      │                  │ 6. Generate       │                   │
      │                  │    connection keys│                   │
      │                  │    (X25519 pair)  │                   │
      │                  │                   │                   │
      │                  │ 7. Store contract │                   │
      │                  │    + notify       │                   │
      │                  │ ─────────────────►│                   │
      │                  │                   │                   │
      │                  │                   │ 8. Route to       │
      │                  │                   │    service        │
      │                  │                   │ ─────────────────►│
      │                  │                   │                   │
      │                  │                   │  9. Service acks  │
      │                  │                   │     + sends pubkey│
      │                  │                   │◄───────────────── │
      │                  │                   │                   │
      │                  │ 10. Complete key  │                   │
      │                  │     exchange      │                   │
      │                  │◄──────────────────┼───────────────────│
      │                  │                   │                   │
      │ 11. Contract     │                   │                   │
      │     active       │                   │                   │
      │◄─────────────────┤                   │                   │
      │                  │                   │                   │
```

### 3.2 Connection Contract Structure

```typescript
interface ConnectionContract {
  contract_id: string;            // Unique contract identifier
  user_guid: string;              // User's VettID GUID
  service_guid: string;           // Service's identifier

  // Contract terms
  capabilities: CapabilityContract[];

  // Connection security
  user_connection_key: string;    // User's X25519 public key for this connection
  service_connection_key: string; // Service's X25519 public key for this connection

  // Lifecycle - persists until cancelled
  created_at: string;             // ISO8601
  last_activity_at: string;       // Updated on each interaction
  cancelled_at: string | null;
  cancelled_by: 'user' | 'service' | null;
  cancellation_reason: string | null;

  // Payment terms (if applicable)
  subscription: SubscriptionContract | null;

  // Consent record
  consent: ConsentRecord;

  // Service-specific storage in user vault
  vault_storage_allocation: {
    private_namespace: string;    // Only this service can access
    shared_namespaces: string[];  // Shared with approved services
  };
}

interface CapabilityContract {
  capability: string;
  scope: string[];
  constraints: object;
  granted_at: string;

  // Request behavior
  request_timeout: number;        // Service-defined timeout (1 min to 30 days)
  offline_grace_period: number;   // Time after user comes online before expiry
  requires_user_approval: boolean;// Whether to prompt user for each use
}

interface ConsentRecord {
  contract_version: string;       // Service's terms version
  consent_timestamp: string;
  user_signature: string;         // User signed the contract
  presented_terms: object;        // Exact terms user agreed to
}

interface SubscriptionContract {
  plan_id: string;
  plan_name: string;
  billing_cycle: 'monthly' | 'yearly' | 'one_time';
  amount: number;
  currency: string;
  payment_method_ref: string;     // Reference to user's payment method in vault
  auto_renew: boolean;
  next_billing_date: string | null;
  started_at: string;
  expires_at: string | null;
}
```

### 3.3 Capability Types

Standard capabilities that services can request:

#### Identity & Authentication
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `authenticate` | Verify user identity | "Confirm your identity to this service" |
| `authenticate.continuous` | Periodic re-authentication | "Allow ongoing identity verification" |
| `verify_presence` | Check if user is available | "See when you're available" |

#### Data Access
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `read_profile` | Read basic profile info | "Share your name and contact info" |
| `read_credentials` | Read specific credential types | "Share your [credential type]" |
| `write_data` | Store data in user's vault (service namespace) | "Store data in your vault" |
| `read_data` | Read service-stored data from user's vault | "Access stored service data" |

#### Authorization & Signing
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `authorization_request` | Request authorization decisions | "Ask for your approval on actions" |
| `sign_document` | Request digital signatures | "Sign documents on your behalf" |
| `sign_transaction` | Request transaction signatures | "Approve transactions" |

#### Communication
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `send_notifications` | Send notifications to user | "Send you notifications" |
| `text_chat` | Initiate text chat sessions | "Start text conversations with you" |
| `voice_call` | Initiate voice calls | "Make voice calls to you" |
| `video_call` | Initiate video calls | "Make video calls to you" |

**Support Call Security**: The `voice_call` and `video_call` capabilities enable verified support channels. When a service calls a user, the user's app shows the verified service identity, eliminating support scam calls. Users can trust that "Acme Bank Support" is actually Acme Bank.

#### Payments & Subscriptions
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `payment.one_time` | Request one-time payments | "Request payments from you" |
| `payment.subscription` | Manage recurring payments | "Set up recurring payments" |
| `payment.auto_renew` | Auto-renew subscriptions | "Automatically renew your subscription" |

**Payment Flow**: VettID does not process payments. The user's vault stores payment method references (credit card tokens, BTC wallet refs). When a service requests payment, the user approves in their app, and the vault initiates payment through the user's configured payment provider.

#### Service Discovery
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `list_connections` | See what other services user is connected to | "See your other service connections" |
| `request_collaboration` | Request shared data access with another service | "Share data between services you approve" |

Custom capabilities can be defined by services and must be approved during registration.

---

## 4. NATS Topic Architecture

### 4.1 ServiceSpace Namespace

All user-service communication flows through the service's **ServiceSpace**. NATS acts as a secure drop-box - messages are queued until the recipient retrieves them.

```
NATS Topic Hierarchy:
│
├── OwnerSpace.<user_guid>/              # Existing: User ↔ User App
│   ├── forVault.>
│   ├── forApp.>
│   └── ...
│
├── ServiceSpace.<service_guid>/          # Service's namespace
│   │
│   ├── fromUser.<user_guid>/             # User Vault → Service
│   │   ├── auth.response.<event_id>      # Auth responses
│   │   ├── authz.response.<event_id>     # Authorization responses
│   │   ├── data.response.<event_id>      # Data responses
│   │   ├── payment.response.<event_id>   # Payment confirmations
│   │   ├── call.signal.<session_id>      # Call signaling
│   │   └── events.>                      # User-initiated events
│   │
│   ├── forUser.<user_guid>/              # Service → User Vault
│   │   ├── auth.request                  # Authentication requests
│   │   ├── authz.request                 # Authorization requests
│   │   ├── data.request                  # Data requests
│   │   ├── payment.request               # Payment requests
│   │   ├── call.initiate                 # Initiate call
│   │   ├── call.signal.<session_id>      # Call signaling
│   │   └── notify.>                      # Notifications
│   │
│   ├── directory/                        # Service discovery
│   │   ├── manifest                      # Handler manifest
│   │   └── status                        # Service health/availability
│   │
│   └── internal/                         # Service-internal (not for users)
│       ├── control.>
│       ├── health.>
│       └── metrics.>
│
├── Control/                              # Existing + Extended
│   ├── global.*
│   ├── enclave.*
│   ├── user.*
│   └── service.<service_guid>.*          # Service control
│
└── Directory/                            # VettID Service Directory
    ├── services.list                     # List all approved services
    ├── services.<service_guid>.info      # Service public info
    └── services.search                   # Search services
```

### 4.2 Message Flow: Request with Timeout & Offline Support

Services can set request timeouts and support offline users:

```
┌─────────────────┐                                   ┌─────────────────┐
│  Service Vault  │                                   │   User Vault    │
└────────┬────────┘                                   └────────┬────────┘
         │                                                     │
         │ 1. Publish request with timeout                     │
         │ ───────────────────────────────────────────────────►│
         │    Topic: ServiceSpace.<svc>.forUser.<user>.        │
         │           auth.request                              │
         │    Payload: {                                       │
         │      event_id: "uuid",                              │
         │      event_type: "auth.request",                    │
         │      timestamp: "ISO8601",                          │
         │      expires_at: "ISO8601",    // Service-defined   │
         │      offline_grace: 300,       // 5 min after online│
         │      encrypted_payload: {...}                       │
         │    }                                                │
         │                                                     │
         │    (Message queued in NATS JetStream)               │
         │                                                     │
         │                    ─────────────────────────────    │
         │                    User may be offline              │
         │                    Message waits in queue           │
         │                    ─────────────────────────────    │
         │                                                     │
         │                    2. User comes online             │
         │                       Vault retrieves queued msgs   │
         │                                                     │
         │                    3. Check expiry:                 │
         │                       - If past expires_at:         │
         │                         discard (or notify expired) │
         │                       - If within offline_grace     │
         │                         after coming online:        │
         │                         process normally            │
         │                                                     │
         │                    4. Process & respond             │
         │◄─────────────────────────────────────────────────── │
         │    Topic: ServiceSpace.<svc>.fromUser.<user>.       │
         │           auth.response.<event_id>                  │
         │                                                     │
```

### 4.3 Message Encryption

All ServiceSpace messages use X25519 + XChaCha20-Poly1305:

```typescript
interface EncryptedMessage {
  event_id: string;
  event_type: string;
  timestamp: string;

  // Timeout handling
  expires_at: string;             // When request expires
  offline_grace_seconds: number;  // Grace period after user comes online

  // Ephemeral key for this message (perfect forward secrecy)
  ephemeral_public_key: string;   // X25519

  // Encrypted with: ECDH(ephemeral_private, recipient_connection_key)
  ciphertext: string;             // XChaCha20-Poly1305
  nonce: string;                  // 24-byte nonce

  // Signature for authenticity
  signature: string;              // Ed25519 signature over (event_id || ciphertext)
}
```

---

## 5. Service Vault Architecture

### 5.1 Deployment Model

Service Vaults are **entirely controlled by the service provider**. No Nitro Enclave is required since the service vault doesn't hold high-value secrets like cryptocurrency private keys. User-specific secrets are stored in user vaults, not service vaults.

```
Service Vault Deployment (Provider-Controlled):
┌──────────────────────────────────────────────────────────────┐
│                 Service Provider Infrastructure               │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  Service Vault (Container/VM/K8s)                       │ │
│  │                                                          │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │ │
│  │  │ NATS Client  │  │ Handler Eng. │  │ Service API  │   │ │
│  │  │              │  │              │  │              │   │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘   │ │
│  │                                                          │ │
│  └────────────────────────────┬─────────────────────────────┘ │
│                               │                               │
│  ┌────────────────────────────┼────────────────────────────┐ │
│  │                            │                             │ │
│  │  ┌─────────────────┐  ┌────┴────┐  ┌─────────────────┐  │ │
│  │  │ Hardened DB     │  │ Key     │  │ Audit Log       │  │ │
│  │  │ (Connection     │  │ Store   │  │ (Immutable)     │  │ │
│  │  │  contracts,     │  │ (HSM or │  │                 │  │ │
│  │  │  state)         │  │  KMS)   │  │                 │  │ │
│  │  └─────────────────┘  └─────────┘  └─────────────────┘  │ │
│  │                                                          │ │
│  │  Security Layer:                                         │ │
│  │  - Encrypted at rest (provider's choice)                 │ │
│  │  - TLS for all connections                               │ │
│  │  - Service key in HSM/KMS (recommended)                  │ │
│  │  - Connection keys in hardened DB                        │ │
│  │                                                          │ │
│  └──────────────────────────────────────────────────────────┘ │
│                               │                               │
│                          TLS/mTLS                             │
│                      (Outbound only)                          │
└───────────────────────────────┼───────────────────────────────┘
                                │
                                ▼
                     ┌──────────────────┐
                     │  VettID NATS     │
                     │  Cluster         │
                     └──────────────────┘
```

**Security Recommendations for Service Vaults:**
1. Store service signing key in HSM or cloud KMS
2. Use encrypted database for connection contracts and keys
3. Implement audit logging for all operations
4. Use TLS 1.3 for all network connections
5. Apply standard hardening (no root, minimal permissions, etc.)

**What Service Vaults Store:**
- Service identity keys (signing + encryption)
- Connection contracts metadata
- Per-user connection keys (for message encryption)
- Operational state

**What Service Vaults Do NOT Store:**
- User credentials or PII
- User payment details
- User-specific secrets (these go in user's vault)

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
│  │  │ - Service key │  │ - Signature   │  │ - All operations  │    │    │
│  │  │   (in HSM)    │  │   verify      │  │ - Tamper-evident  │    │    │
│  │  │ - Connection  │  │ - Contract    │  │                   │    │    │
│  │  │   keys (DB)   │  │   validation  │  │                   │    │    │
│  │  └───────────────┘  └───────────────┘  └───────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                      CONNECTION LAYER                            │    │
│  │  ┌───────────────────────────┐  ┌───────────────────────────┐   │    │
│  │  │    NATS Client Manager    │  │   Contract State Store    │   │    │
│  │  │ - Reconnection handling   │  │ - Active contracts        │   │    │
│  │  │ - Subscription management │  │ - Key rotation tracking   │   │    │
│  │  │ - Message routing         │  │ - Request state           │   │    │
│  │  └───────────────────────────┘  └───────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                       BUSINESS LAYER                             │    │
│  │  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐    │    │
│  │  │ Handler Engine│  │ Request Queue │  │ Rate Limiter      │    │    │
│  │  │ - Auth        │  │ - Timeout mgmt│  │ - Per-user limits │    │    │
│  │  │ - Data        │  │ - Offline     │  │ - Global limits   │    │    │
│  │  │ - Payment     │  │   support     │  │                   │    │    │
│  │  │ - Calls       │  │ - Retry logic │  │                   │    │    │
│  │  └───────────────┘  └───────────────┘  └───────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                        API LAYER                                 │    │
│  │  ┌───────────────────────────┐  ┌───────────────────────────┐   │    │
│  │  │    Service API Gateway    │  │   Webhook Dispatcher      │   │    │
│  │  │ - REST/gRPC endpoints     │  │ - Event notifications     │   │    │
│  │  │ - SDK integration points  │  │ - Delivery guarantees     │   │    │
│  │  └───────────────────────────┘  └───────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.3 Service API for Third Parties

The Service Vault exposes an API for the service provider's backend:

```typescript
// Service Vault API (for service provider's backend)

// Authentication
POST /api/v1/auth/request
{
  user_id: string,              // User's contract ID or VettID
  purpose: string,              // Why authentication is needed
  context: object,              // Additional context
  expires_in_seconds?: number,  // Request timeout (default: 60, max: 30 days)
  offline_grace_seconds?: number,// Grace period after online (default: 300)
  callback_url?: string         // Webhook for async response
}
Response: {
  request_id: string,
  status: "pending" | "completed" | "expired" | "denied",
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
  action: string,
  resource: string,
  context: object,
  expires_in_seconds?: number,
  offline_grace_seconds?: number,
  callback_url?: string
}
Response: {
  request_id: string,
  status: "pending" | "approved" | "denied" | "expired",
  decision?: {
    allowed: boolean,
    constraints?: object
  }
}

// Initiate Call (text/voice/video)
POST /api/v1/call/initiate
{
  user_id: string,
  call_type: "text" | "voice" | "video",
  purpose: string,              // e.g., "Support call regarding order #123"
  agent_info: {
    name: string,
    role: string,
    avatar_url?: string
  },
  expires_in_seconds?: number   // How long to wait for user to answer
}
Response: {
  session_id: string,
  status: "ringing" | "connected" | "declined" | "expired",
  signaling_topic: string       // NATS topic for WebRTC signaling
}

// Payment Request
POST /api/v1/payment/request
{
  user_id: string,
  amount: number,
  currency: string,
  description: string,
  payment_type: "one_time" | "subscription",
  subscription_details?: {
    plan_id: string,
    billing_cycle: string,
    auto_renew: boolean
  },
  expires_in_seconds?: number
}
Response: {
  request_id: string,
  status: "pending" | "completed" | "declined" | "expired",
  transaction?: {
    transaction_id: string,
    amount: number,
    currency: string,
    timestamp: string
  }
}

// Connection/Contract Management
GET  /api/v1/contracts
GET  /api/v1/contracts/{contract_id}
POST /api/v1/contracts/invite           // Generate connection invite
DELETE /api/v1/contracts/{contract_id}  // Service cancels contract

// Webhooks (from Service Vault to service backend)
POST {callback_url}
{
  event_type: string,           // "auth.completed", "payment.completed", etc.
  event_id: string,
  timestamp: string,
  contract_id: string,
  data: object
}
```

---

## 6. Cross-Service Data Sharing

### 6.1 User-Controlled Collaboration

VettID maintains that **the user is in charge**. Services cannot communicate directly with each other. Instead, when services want to share data, they must request user approval for a **Combined Datastore**.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         User's Vault Storage                             │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    Service Namespaces                            │    │
│  │                                                                   │    │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐   │    │
│  │  │ Service A       │  │ Service B       │  │ Service C       │   │    │
│  │  │ Private Space   │  │ Private Space   │  │ Private Space   │   │    │
│  │  │                 │  │                 │  │                 │   │    │
│  │  │ (Only A can     │  │ (Only B can     │  │ (Only C can     │   │    │
│  │  │  read/write)    │  │  read/write)    │  │  read/write)    │   │    │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘   │    │
│  │                                                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                 Combined Datastores (User Approved)              │    │
│  │                                                                   │    │
│  │  ┌─────────────────────────────────────────────────────────┐     │    │
│  │  │  Combined: "Health & Fitness"                           │     │    │
│  │  │  Participants: [Service A, Service B]                   │     │    │
│  │  │  Approved by user: 2024-01-15                           │     │    │
│  │  │                                                          │     │    │
│  │  │  ┌──────────────────┐  ┌──────────────────┐             │     │    │
│  │  │  │ Shared Data      │  │ Audit Log        │             │     │    │
│  │  │  │ - User profile   │  │ - A read profile │             │     │    │
│  │  │  │ - Health metrics │  │ - B wrote metrics│             │     │    │
│  │  │  │ - Preferences    │  │ - A read metrics │             │     │    │
│  │  │  └──────────────────┘  └──────────────────┘             │     │    │
│  │  └─────────────────────────────────────────────────────────┘     │    │
│  │                                                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Collaboration Request Flow

```
Service A wants to collaborate with Service B (both connected to user):

1. Service A requests user's connected services
   → User must have granted 'list_connections' capability
   → Returns: [Service B, Service C, Service D]

2. Service A requests collaboration with Service B
   POST /api/v1/collaboration/request
   {
     target_service_guid: "<service_b_guid>",
     purpose: "Share health data for integrated fitness tracking",
     shared_data_schema: {
       fields: ["health_metrics", "activity_log"],
       access: {
         service_a: ["read", "write"],
         service_b: ["read", "write"]
       }
     }
   }

3. User receives approval request in app:
   "Service A (Fitness App) wants to share data with Service B (Health Tracker):
    - Health metrics (read/write for both)
    - Activity log (read/write for both)

    This creates a shared space both services can access.
    All access will be logged and visible to you.

    [Approve] [Deny]"

4. User approves → Combined datastore created in user's vault

5. Both services can now:
   - Read/write to shared namespace
   - All operations logged with full audit trail
   - User can view all activity
   - User can revoke at any time
```

### 6.3 Combined Datastore Structure

```typescript
interface CombinedDatastore {
  datastore_id: string;
  name: string;                   // User-visible name
  purpose: string;

  participants: {
    service_guid: string;
    service_name: string;
    permissions: ('read' | 'write')[];
    joined_at: string;
  }[];

  schema: {
    fields: DataField[];
  };

  created_at: string;
  created_by_service: string;     // Which service initiated
  approved_by_user_at: string;

  // Full audit trail
  audit_log: {
    timestamp: string;
    service_guid: string;
    operation: 'read' | 'write' | 'delete';
    field: string;
    summary: string;              // Human-readable description
  }[];
}
```

---

## 7. Payments & Subscriptions

### 7.1 Payment Model

VettID does not process payments or take fees. Payments are between the user and service:

```
┌────────────────┐          ┌────────────────┐          ┌────────────────┐
│  Service Vault │          │  User's Vault  │          │ Payment Provider│
│                │          │                │          │ (User's choice) │
└───────┬────────┘          └───────┬────────┘          └───────┬────────┘
        │                           │                           │
        │ 1. Payment request        │                           │
        │ ─────────────────────────►│                           │
        │   {amount, currency,      │                           │
        │    description}           │                           │
        │                           │                           │
        │           2. User approves│in app                     │
        │                           │                           │
        │                           │ 3. Initiate payment       │
        │                           │ ─────────────────────────►│
        │                           │   (Using stored payment   │
        │                           │    method reference)      │
        │                           │                           │
        │                           │ 4. Payment confirmation   │
        │                           │◄───────────────────────── │
        │                           │                           │
        │ 5. Payment response       │                           │
        │◄───────────────────────── │                           │
        │   {transaction_id,        │                           │
        │    status: "completed"}   │                           │
        │                           │                           │
```

### 7.2 Subscription Management

```typescript
interface SubscriptionPlan {
  plan_id: string;
  name: string;
  description: string;

  pricing: {
    amount: number;
    currency: string;
    billing_cycle: 'monthly' | 'yearly' | 'one_time';
  };

  features: string[];

  // Auto-renewal settings
  auto_renewal: {
    supported: boolean;
    default: boolean;
    reminder_days_before: number;  // Notify user X days before renewal
  };
}

// User's subscription in their vault
interface UserSubscription {
  subscription_id: string;
  service_guid: string;
  plan: SubscriptionPlan;

  status: 'active' | 'cancelled' | 'expired' | 'payment_failed';

  started_at: string;
  current_period_end: string;

  auto_renew: boolean;
  payment_method_ref: string;     // Reference to user's payment method

  history: {
    timestamp: string;
    event: 'created' | 'renewed' | 'cancelled' | 'payment_failed';
    amount?: number;
    transaction_id?: string;
  }[];
}
```

---

## 8. Security Considerations

### 8.1 Threat Model

| Threat | Mitigation |
|--------|------------|
| **Compromised Service Vault** | Connection keys are per-user; service vault doesn't hold user secrets (stored in user vault) |
| **Service Impersonation** | Ed25519 signatures on all messages; NATS JWT authentication; VettID registry approval |
| **Replay Attacks** | Event IDs, timestamps, request expiry, idempotency tracking |
| **Support Scam Calls** | Verified service identity displayed to user; only contracted services can call |
| **Unauthorized Data Access** | Capability-based access; user approval required; full audit trail |
| **Payment Fraud** | User approves each payment; no stored payment details in service vault |
| **Cross-Service Data Leak** | Services have isolated namespaces; combined datastores require explicit user approval |
| **Key Compromise** | Automatic key rotation; perfect forward secrecy with ephemeral keys |

### 8.2 Security Boundaries

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        TRUST BOUNDARY: VettID                            │
│                                                                          │
│  ┌────────────────┐        ┌────────────────┐        ┌───────────────┐  │
│  │  User Vault    │◄──────►│  NATS Cluster  │◄──────►│ Control Plane │  │
│  │  (Full Trust)  │        │  (Transport)   │        │ (Admin Trust) │  │
│  │                │        │                │        │               │  │
│  │ Stores:        │        │ Stores:        │        │ Stores:       │  │
│  │ - User secrets │        │ - Messages     │        │ - Registry    │  │
│  │ - Service data │        │   (encrypted)  │        │ - Approvals   │  │
│  │ - Payment refs │        │                │        │               │  │
│  │ - Audit logs   │        │                │        │               │  │
│  └────────────────┘        └───────┬────────┘        └───────────────┘  │
│                                    │                                     │
└────────────────────────────────────┼─────────────────────────────────────┘
                                     │
                    Message-level encryption
                    + Contract enforcement
                                     │
┌────────────────────────────────────┼─────────────────────────────────────┐
│                        TRUST BOUNDARY: Service Provider                  │
│                                     │                                    │
│  ┌────────────────┐                │                                    │
│  │ Service Vault  │◄───────────────┘                                    │
│  │ (Limited Trust)│                                                     │
│  │                │                                                     │
│  │ Stores:        │    Can only:                                        │
│  │ - Service keys │    - Access data user granted                       │
│  │ - Connection   │    - Send allowed message types                     │
│  │   keys         │    - Request payments (user approves)               │
│  │ - Contract     │    - Call users (with capability)                   │
│  │   metadata     │                                                     │
│  │                │    Cannot:                                          │
│  │ Does NOT store:│    - Access user secrets                            │
│  │ - User secrets │    - See other services' data                       │
│  │ - User PII     │    - Bypass user approval                           │
│  │ - Payment info │    - Impersonate other services                     │
│  └────────────────┘                                                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 8.3 Audit Requirements

All operations logged with full user visibility:

```typescript
interface AuditEvent {
  event_id: string;
  timestamp: string;

  // Parties
  service_guid: string;
  user_guid: string;
  contract_id: string;

  // Operation
  operation: string;
  capability_used: string;

  // Outcome
  status: "success" | "denied" | "error";

  // Context (sanitized)
  request_summary: string;        // Human-readable
  response_summary: string;

  // Integrity
  previous_hash: string;
  event_hash: string;
  signature: string;
}
```

Users can view their complete audit log in the VettID app, seeing exactly what each service has accessed.

---

## 9. Design Decisions

Based on review feedback, the following design decisions have been made:

### 9.1 Security Model

**Decision**: Service Vaults do not require Nitro Enclaves.

**Rationale**:
- Service vaults hold connection keys, not high-value secrets like cryptocurrency keys
- User-specific secrets are stored in the user's vault, not the service vault
- Standard security practices (HSM for service key, encrypted DB, TLS) are sufficient
- Service providers have flexibility in their deployment infrastructure

**Recommendation**: Use HSM or cloud KMS for service signing key; encrypted database for connection keys.

### 9.2 Connection Persistence

**Decision**: Connections persist until one party cancels.

**Rationale**:
- NATS acts as a secure drop-box; no persistent network connections needed
- Users and services may go offline for extended periods
- Inactive connections don't consume resources (messages queue in NATS)
- Explicit cancellation gives both parties clear control

### 9.3 Offline Support

**Decision**: Full offline support with service-defined expiration.

**Implementation**:
- Services set `expires_at` on each request (1 minute to 30 days)
- Services set `offline_grace_seconds` for users coming back online
- Expired requests are discarded (or notification sent)
- Supports both immediate interactions and long-lived offers

### 9.4 Multi-Vault Services

**Decision**: Not required for MVP; scale horizontally within single logical service.

**Rationale**:
- Service vault doesn't store PII, just coordinates with user vaults
- No data residency requirements for coordination layer
- Horizontal scaling (multiple instances behind load balancer) handles capacity
- May revisit for geographic latency optimization in future

### 9.5 Service Directory

**Decision**: VettID provides the Service Directory; services publish Handler Manifests.

**Implementation**:
- VettID maintains the user-facing Service Directory
- Services publish capability/handler manifests to their ServiceSpace
- Users browse Directory in VettID app to discover and connect to services
- Similar pattern to existing VettID supported services (renamed to Service Directory)

### 9.6 Billing Model

**Decision**: VettID does not charge for service vaults or connections.

**Implementation**:
- Service operators offer free or paid services
- Payments flow directly between user and service (via user's payment methods)
- VettID has no visibility into payment transactions
- Supports one-time payments, subscriptions, and auto-renewal

### 9.7 Cross-Service Communication

**Decision**: No direct service-to-service communication. User-controlled combined datastores.

**Rationale**:
- User is in charge; their vault is the center of their world
- Services can request `list_connections` capability to see other services
- Compatible services request combined datastore through user approval
- Creates full audit trail of all cross-service data sharing
- User maintains complete visibility and control

---

## 10. Implementation Phases

### Phase 1: Foundation (MVP)
- [ ] Service Registry (VettID side)
- [ ] Service Vault core (NATS connection, key management)
- [ ] Connection Contract establishment
- [ ] Basic authentication flow
- [ ] Service Directory integration
- [ ] Simple REST API for service providers

### Phase 2: Core Features
- [ ] Authorization request/response flow
- [ ] User data access (read/write to user vault)
- [ ] Notification delivery
- [ ] Request timeout and offline support
- [ ] Webhook delivery system
- [ ] Audit logging

### Phase 3: Communication & Payments
- [ ] Text chat capability
- [ ] Voice call capability (WebRTC signaling)
- [ ] Video call capability
- [ ] One-time payment requests
- [ ] Subscription management
- [ ] Auto-renewal support

### Phase 4: Collaboration & Scale
- [ ] Cross-service data sharing (combined datastores)
- [ ] Horizontal scaling patterns
- [ ] SDK for common platforms (Node.js, Python, Go)
- [ ] Analytics dashboard for service providers

---

## 11. File Structure (Proposed)

```
vettid-service-vault/
├── README.md
├── docs/
│   ├── SERVICE-VAULT-ARCHITECTURE.md  (this document)
│   ├── API-REFERENCE.md
│   ├── SECURITY-MODEL.md
│   ├── DEPLOYMENT-GUIDE.md
│   └── SDK-INTEGRATION.md
├── cdk/                               # AWS CDK infrastructure (VettID side)
│   ├── lib/
│   │   ├── service-registry-stack.ts
│   │   └── service-nats-stack.ts
│   └── lambda/
│       ├── handlers/
│       │   ├── service-registration.ts
│       │   └── service-directory.ts
│       └── common/
│           ├── service-jwt.ts
│           └── service-crypto.ts
├── vault/                             # Service Vault reference implementation
│   ├── cmd/
│   │   └── service-vault/
│   │       └── main.go
│   ├── internal/
│   │   ├── nats/                     # NATS connection management
│   │   ├── crypto/                   # Key management, encryption
│   │   ├── handlers/                 # Message handlers
│   │   ├── api/                      # REST/gRPC API
│   │   ├── store/                    # Contract state storage
│   │   └── audit/                    # Audit logging
│   └── pkg/
│       └── sdk/                      # Embeddable SDK components
├── sdk/                              # Client SDKs for service providers
│   ├── node/
│   ├── python/
│   └── go/
└── examples/
    ├── auth-service/                 # Simple auth integration
    ├── support-center/               # Voice/video support example
    └── subscription-service/         # Payment/subscription example
```

---

## Appendix A: Comparison with User Vault

| Aspect | User Vault (OwnerVault) | Service Vault |
|--------|------------------------|---------------|
| **Operator** | VettID (managed) | Service Provider (self-managed) |
| **Users** | 1 user per vault | Many users per vault |
| **Primary Interface** | Mobile App | REST/gRPC API |
| **Runs In** | Nitro Enclave (required) | Container/VM (provider's choice) |
| **Holds** | User credentials, secrets, service data | Service identity + connection keys |
| **NATS Namespace** | OwnerSpace | ServiceSpace |
| **Trust Level** | Full (user's own vault) | Limited (only contracted capabilities) |
| **Key Generation** | VettID manages | Provider generates & manages |
| **PII Storage** | Yes (encrypted) | No |
| **Payment Processing** | Yes (user's methods) | No (requests only) |

---

## Appendix B: Message Sequence Examples

### B.1 Verified Support Call Flow

```
User has issue → Calls "Acme Bank" support through VettID app
(Eliminates scam calls - user initiates from verified service listing)

┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ User App │     │User Vault│     │  NATS    │     │ Service  │
│          │     │          │     │          │     │ Vault    │
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │
     │ 1. User taps   │                │                │
     │ "Call Support" │                │                │
     │ on Acme Bank   │                │                │
     │ ──────────────►│                │                │
     │                │                │                │
     │                │ 2. Initiate    │                │
     │                │ call request   │                │
     │                │ ──────────────►│                │
     │                │                │ ──────────────►│
     │                │                │                │
     │                │                │ 3. Service     │
     │                │                │ assigns agent  │
     │                │                │◄────────────── │
     │                │                │                │
     │                │ 4. Call setup  │                │
     │                │◄──────────────►│◄──────────────►│
     │                │  (WebRTC       │                │
     │                │   signaling)   │                │
     │                │                │                │
     │ 5. Connected   │                │                │
     │◄──────────────►│                │                │
     │                │                │                │
     │ Display:       │                │                │
     │ ┌────────────┐ │                │                │
     │ │ ✓ Verified │ │                │                │
     │ │ Acme Bank  │ │                │                │
     │ │ Support    │ │                │                │
     │ │            │ │                │                │
     │ │ Agent: Sam │ │                │                │
     │ └────────────┘ │                │                │
     │                │                │                │
```

### B.2 Subscription Payment Flow

```
Monthly subscription renewal with auto-pay:

┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Service Vault│     │  User Vault  │     │Payment Provdr│
└──────┬───────┘     └──────┬───────┘     └──────┬───────┘
       │                    │                    │
       │ 1. Renewal due     │                    │
       │    (auto-renew ON) │                    │
       │ ──────────────────►│                    │
       │   {plan_id,        │                    │
       │    amount: $9.99}  │                    │
       │                    │                    │
       │                    │ 2. Check user      │
       │                    │    settings:       │
       │                    │    auto_renew=true │
       │                    │                    │
       │                    │ 3. Process payment │
       │                    │ ──────────────────►│
       │                    │   (using stored    │
       │                    │    payment method) │
       │                    │                    │
       │                    │ 4. Payment success │
       │                    │◄────────────────── │
       │                    │                    │
       │ 5. Renewal         │                    │
       │    confirmed       │                    │
       │◄────────────────── │                    │
       │                    │                    │
       │                    │ 6. Notify user     │
       │                    │    (receipt in app)│
       │                    │                    │
```

### B.3 Cross-Service Data Sharing

```
Fitness App wants to share data with Health Tracker:

┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ Fitness  │     │User Vault│     │ User App │     │ Health   │
│ App (A)  │     │          │     │          │     │Tracker(B)│
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │
     │ 1. Request     │                │                │
     │ list_connections                │                │
     │ ──────────────►│                │                │
     │                │                │                │
     │ 2. Return      │                │                │
     │ [Health Tracker│                │                │
     │  Service B]    │                │                │
     │◄────────────── │                │                │
     │                │                │                │
     │ 3. Request     │                │                │
     │ collaboration  │                │                │
     │ with B         │                │                │
     │ ──────────────►│                │                │
     │                │                │                │
     │                │ 4. Prompt user │                │
     │                │ ──────────────►│                │
     │                │  "Fitness App  │                │
     │                │   wants to     │                │
     │                │   share data   │                │
     │                │   with Health  │                │
     │                │   Tracker"     │                │
     │                │                │                │
     │                │ 5. User approves                │
     │                │◄────────────── │                │
     │                │                │                │
     │                │ 6. Create combined datastore    │
     │                │    + notify both services       │
     │                │                │                │
     │ 7. Access      │                │                │
     │ granted        │                │                │
     │◄──────────────►│◄──────────────────────────────►│
     │                │                │                │
     │  Both services can now read/write shared data   │
     │  All access logged in user's audit trail        │
     │                │                │                │
```
