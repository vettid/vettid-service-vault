# VettID Service Vault Architecture

## Executive Summary

The **Service Vault** enables third-party applications and services to connect with users. Service Vaults are **fully self-sovereign** - they operate independently with their own infrastructure, just as users control their own vaults.

**VettID's role is infrastructure only:**
- Hosts user vaults (OwnerVaults) - encrypted, user-controlled
- Operates MessageSpace (NATS) - event routing infrastructure
- Provides event handlers for external transactions (e.g., putting BTC on-chain)
- **Does NOT** approve, control, or intermediate user-service interactions

**Service Vault characteristics:**
- **Self-sovereign identity** - key-derived, no central assignment
- **Fully standalone** - operates own NATS cluster, no VettID dependency
- **Direct contracts** - user signs connection contract with their key
- **Scalable multi-user support** - one Service Vault handles many users
- **Event-driven communication** - NATS as secure message transport
- **Optional registry** - can register with VettID or other registries for discoverability

**Connection model:**
- Users and services connect directly via NATS events
- Connection contracts are cryptographically signed by user's key
- Services offer multiple options (tiers, pricing, data requirements)
- User selects and signs - no approval needed from anyone

---

## 1. Core Concepts

### 1.1 Terminology

| Term | Description |
|------|-------------|
| **Service Vault** | A vault deployed and controlled by a 3rd party organization to integrate their service with users |
| **Service Provider** | The organization operating the Service Vault |
| **Service Identity** | Key-derived cryptographic identity (`service_id = base58(sha256(pubkey))`) controlled by the service provider |
| **OwnerVault** | The user's personal vault (existing VettID architecture) |
| **Service Connection** | An authorized relationship between a user and a service |
| **Connection Contract** | The agreement defining what capabilities a user grants to a service |
| **Service Registry** | Any directory of services that issues attestations (VettID operates one, others may exist) |
| **Registry Attestation** | A signed statement from a registry verifying claims about a service |
| **Domain Validation** | Optional DNS-based proof linking a service identity to a domain |

### 1.2 Relationship Model

Communication between users and services flows through NATS using a split model:
- **Service → User**: Via VettID's MessageSpace (required)
- **User → Service**: Via Service's own NATS cluster (ServiceSpace)

```
┌───────────────────────────────────────┐     ┌───────────────────────────────────────┐
│         VETTID INFRASTRUCTURE         │     │      SERVICE PROVIDER INFRASTRUCTURE  │
│                                       │     │                                       │
│  ┌─────────────────────────────────┐  │     │  ┌─────────────────────────────────┐  │
│  │       User Vaults (hosted)      │  │     │  │         Service Vault           │  │
│  │  ┌───────────┐  ┌───────────┐   │  │     │  │    (self-hosted, autonomous)    │  │
│  │  │  User A   │  │  User N   │   │  │     │  │                                 │  │
│  │  │  Vault    │  │  Vault    │   │  │     │  │  ┌─────────┐ ┌─────────┐       │  │
│  │  │           │  │           │   │  │     │  │  │ Identity│ │ Handler │       │  │
│  │  │┌─────────┐│  │┌─────────┐│   │  │     │  │  │ (HSM)   │ │ Engine  │       │  │
│  │  ││ Data    ││  ││ Data    ││   │  │     │  │  └─────────┘ └─────────┘       │  │
│  │  ││ Secrets ││  ││ Secrets ││   │  │     │  │  ┌─────────┐ ┌─────────┐       │  │
│  │  │└─────────┘│  │└─────────┘│   │  │     │  │  │Contract │ │  Audit  │       │  │
│  │  └─────┬─────┘  └─────┬─────┘   │  │     │  │  │ Manager │ │  Log    │       │  │
│  │        │              │         │  │     │  │  └─────────┘ └─────────┘       │  │
│  └────────┼──────────────┼─────────┘  │     │  └───────────────┬─────────────────┘  │
│           │              │            │     │                  │                    │
│           ▼              ▼            │     │                  ▼                    │
│  ┌─────────────────────────────────┐  │     │  ┌─────────────────────────────────┐  │
│  │      VettID NATS Cluster        │  │     │  │     Service's NATS Cluster      │  │
│  │                                 │  │     │  │        (ServiceSpace)           │  │
│  │  ┌───────────┐ ┌─────────────┐  │  │     │  │                                 │  │
│  │  │OwnerSpace │ │MessageSpace │  │  │     │  │  ┌─────────────────────────┐    │  │
│  │  │(User↔App) │ │(Svc→User)   │  │◄─┼─────┼─►│  │ ServiceSpace.<svc_id>/  │    │  │
│  │  └───────────┘ └─────────────┘  │  │     │  │  │   fromUser.<user>.*     │    │  │
│  │                                 │  │     │  │  │   toUser.* (setup only) │    │  │
│  └─────────────────────────────────┘  │     │  │  └─────────────────────────┘    │  │
│                                       │     │  │                                 │  │
│                                       │     │  └─────────────────────────────────┘  │
│                                       │     │                                       │
│                                       │     │                                       │
└───────────────────────────────────────┘     └───────────────────────────────────────┘

Communication Flows:
━━━━━━━━━━━━━━━━━━━━

DURING CONNECTION SETUP (before contract exists):
┌──────────────────────────────────────────────────────────────────────────┐
│  User Vault ◄────► Service NATS (ServiceSpace) ◄────► Service Vault     │
│                                                                          │
│  Bidirectional on Service NATS allows:                                   │
│  - User to fetch contract offerings                                      │
│  - Contract negotiation and signing                                      │
│  - Key exchange                                                          │
└──────────────────────────────────────────────────────────────────────────┘

AFTER CONTRACT ESTABLISHED (normal operation):
┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│  User Vault ──────► ServiceSpace.<svc>.fromUser.<user> ──────► Service   │
│                          (User → Service)                                │
│                     via Service's NATS cluster                           │
│                                                                          │
│  User Vault ◄────── MessageSpace.<user>.fromService.<svc> ◄────── Service│
│                          (Service → User)                                │
│                     via VettID's NATS cluster                            │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

**Key Principles:**

1. **Service → User via MessageSpace**: All service-to-user communication flows through VettID's MessageSpace. This is required, not optional.

2. **User → Service via ServiceSpace**: All user-to-service communication flows through the service's own NATS cluster.

3. **Bidirectional ServiceSpace during setup only**: Before a contract exists, the service's NATS handles both directions to allow contract negotiation. Once connected, service→user switches to MessageSpace.

4. **User Vault connects to each service**: For each contract, user vault establishes a connection to that service's NATS cluster for sending messages.

5. **User Vault Stores Secrets**: Service-specific user secrets are stored in the user's vault, not the service vault. The service vault only holds connection keys for message encryption.

5. **User Vault Stores Secrets**: Service-specific user secrets are stored in the user's vault, not the service vault. The service vault only holds connection keys for message encryption.

---

## 2. Identity & Authentication

### 2.1 Service Identity

Each Service Vault has a **self-sovereign cryptographic identity** derived from its public key:

```
Service Identity Structure:
├── service_id: string (derived from public key - see below)
├── public_key: Ed25519 public key (provider-generated)
├── encryption_key: X25519 public key (provider-generated, for key exchange)
├── service_name: string (human-readable display name)
├── service_type: enum (AUTHENTICATOR | AUTHORIZER | DATA_PROVIDER | INTEGRATION | SUPPORT | PAYMENT)
├── nats_endpoint: string (service's NATS cluster endpoint)
├── handler_manifest: object (supported event types/capabilities)
│
├── [Optional - Domain Validation]
│   ├── domain: string (e.g., "acme.com")
│   ├── domain_verified: boolean
│   └── dns_proof: string (TXT record value proving ownership)
│
├── [Optional - Registry Attestations]
│   └── attestations: [
│         { registry: "vettid", attestation: object, verified_at: string },
│         { registry: "other-registry", attestation: object, verified_at: string },
│         ...
│       ]
```

**Identity Derivation (Required):**
```
service_id = base58(sha256(public_key)[0:20])
```
This guarantees global uniqueness - the identity IS the key.

**⚠️ Private Key Security:**

The service's Ed25519 signing key is the root of trust for the entire service identity. Compromise of this key means complete service impersonation. Service providers **MUST** secure private keys appropriately:

| Environment | Recommended Approach |
|-------------|---------------------|
| **Production** | Hardware Security Module (HSM) or cloud KMS (AWS KMS, GCP Cloud HSM, Azure Key Vault) |
| **High-value services** | Dedicated HSM with FIPS 140-2 Level 3+ certification |
| **Startup/MVP** | Cloud KMS at minimum; never store keys in code, config files, or environment variables |

Key management requirements:
- Keys should never exist in plaintext outside secure hardware
- Implement key rotation procedures (rotate encryption keys periodically, signing key rotation requires identity migration)
- Maintain secure backup/recovery procedures
- Audit all key usage

**Domain Validation (Optional):**

Services can associate a domain with their identity for human-readable verification:

```
1. Service claims domain "acme.com"

2. Service creates DNS TXT record:
   _vettid-service.acme.com TXT "service_id=<service_id>;key=<public_key_fingerprint>"

3. Anyone can verify:
   - Lookup DNS TXT record
   - Confirm service_id matches the service's actual key-derived ID
   - Domain owner has proven they control this service identity
```

This allows users to see "Acme Service (acme.com ✓)" rather than just a cryptographic ID.

### 2.2 Service Operation Model

Services operate **fully standalone** by default. Registry registration is optional and registry-agnostic:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         STANDALONE SERVICE OPERATION                         │
│                            (Default - No Registry)                           │
│                                                                              │
│  ┌─────────────────┐                              ┌─────────────────┐       │
│  │  Service Admin  │                              │  User           │       │
│  └────────┬────────┘                              └────────┬────────┘       │
│           │                                                │                │
│           │  1. Generate service identity                  │                │
│           │     - Ed25519 signing keypair                  │                │
│           │     - X25519 encryption keypair                │                │
│           │     - service_id = base58(sha256(pubkey))      │                │
│           │     - Set up own NATS cluster                  │                │
│           │                                                │                │
│           │  2. Deploy Service Vault                       │                │
│           │     (fully self-hosted)                        │                │
│           │                                                │                │
│           │  3. [Optional] Add domain validation           │                │
│           │     (DNS TXT record)                           │                │
│           │                                                │                │
│           │  4. Share connection info with users           │                │
│           │     (QR code, link, direct exchange)           │                │
│           │ ──────────────────────────────────────────────►│                │
│           │   - service_id                                 │                │
│           │   - public_key + encryption_key                │                │
│           │   - nats_endpoint                              │                │
│           │   - handler_manifest                           │                │
│           │   - domain (if validated)                      │                │
│           │                                                │                │
│           │  5. User connects directly                     │                │
│           │◄────────────────────────────────────────────── │                │
│           │     (peer-to-peer contract)                    │                │
│           │                                                │                │
│  SERVICE IS FULLY OPERATIONAL - NO REGISTRY NEEDED         │                │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Optional Registry Registration

Services may choose to register with one or more service registries for discoverability. VettID operates one such registry, but others may exist:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      OPTIONAL REGISTRY REGISTRATION                          │
│                                                                              │
│  ┌─────────────────┐         ┌─────────────────┐                            │
│  │     Service     │         │  Service        │    (Any registry that      │
│  │  (Standalone)   │         │  Registry       │     implements the         │
│  └────────┬────────┘         │  ┌───────────┐  │     registry protocol)     │
│           │                  │  │  VettID   │  │                            │
│           │                  │  │  Registry │  │                            │
│           │                  │  └───────────┘  │                            │
│           │                  │  ┌───────────┐  │                            │
│           │                  │  │  Other    │  │                            │
│           │                  │  │  Registry │  │                            │
│           │                  │  └───────────┘  │                            │
│           │                  └────────┬────────┘                            │
│           │                           │                                      │
│           │  1. Request Registration  │                                      │
│           │ ─────────────────────────►│                                      │
│           │   - service_id (key-derived)                                     │
│           │   - public_key                                                   │
│           │   - domain (if validated)                                        │
│           │   - handler_manifest                                             │
│           │   - organization details                                         │
│           │                           │                                      │
│           │                           │  2. Registry verification            │
│           │                           │     - Verify key ownership           │
│           │                           │     - Verify domain (if claimed)     │
│           │                           │     - Organization vetting           │
│           │                           │       (registry-specific)            │
│           │                           │                                      │
│           │  3. Attestation issued    │                                      │
│           │◄───────────────────────── │                                      │
│           │   - Signed attestation                                           │
│           │   - Directory listing                                            │
│           │   - Registry-specific benefits                                   │
│           │                                                                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Registry Attestation Structure:**
```typescript
interface RegistryAttestation {
  registry_id: string;              // e.g., "vettid", "industry-consortium"
  registry_public_key: string;      // Registry's signing key
  service_id: string;               // The service being attested
  attestation_type: string;         // "verified_org", "domain_validated", etc.
  claims: {
    organization_name?: string;
    domain?: string;
    verification_level?: string;    // "basic", "enhanced", "audited"
    [key: string]: any;
  };
  issued_at: string;
  expires_at: string;
  signature: string;                // Registry's signature over the attestation
}
```

**VettID Registry Benefits (when registered with VettID):**
- Listed in VettID Service Directory (user discovery via VettID app)
- VettID attestation badge shown to users

**Key Principles:**
- Registry registration is optional (for discoverability and trust signals)
- All services use MessageSpace for service→user communication (user vaults are on VettID infrastructure)
- Services can register with multiple registries simultaneously
- No registry controls service identity (identity is key-derived)
- Users can connect to unregistered services via direct key exchange

### 2.4 NATS Authentication

Services and users authenticate to two separate NATS environments:

#### Service's NATS Cluster (ServiceSpace)

Services operate their own NATS cluster for receiving messages from users:

```typescript
// User JWT for Service's NATS (issued by service to connected user)
{
  "aud": "NATS",
  "exp": <timestamp + contract_duration>,
  "iat": <timestamp>,
  "iss": <service_operator_public_key>,  // Service is the NATS operator
  "jti": <unique_id>,
  "name": "user:<user_guid>",
  "nats": {
    "pub": {
      "allow": [
        "ServiceSpace.<service_id>.fromUser.<user_guid>.>"  // User → Service
      ]
    },
    "sub": {
      "allow": [
        "ServiceSpace.<service_id>.toUser.<user_guid>.>"    // Service → User (setup only)
      ]
    },
    "subs": 20,
    "data": 10000000,   // 10 MB/sec per user
    "payload": 1048576  // 1 MB max message
  },
  "sub": <user_connection_public_key>  // User's key for this service
}
```

#### VettID MessageSpace (User-Controlled)

Services send messages to users via VettID's MessageSpace. **The user's vault controls access** - only services with an active contract can publish to a user's MessageSpace topic:

```typescript
// User's MessageSpace topic structure
MessageSpace.<user_guid>.fromService.<service_id>.>

// Access control (enforced by user's vault, not VettID):
// - User vault subscribes to MessageSpace.<user_guid>.>
// - User vault only processes messages from services with valid contracts
// - Invalid/unauthorized messages are discarded
```

**Key point:** VettID provides MessageSpace infrastructure, but the **user controls who can reach them**. When a user signs a contract with a service, their vault begins accepting messages from that service's topic. VettID has no role in approving or denying service access - this is entirely user-controlled.

### 2.5 Handler Manifest

Similar to how VettID's Service Directory lists event handlers, each Service Vault publishes a **Handler Manifest** describing its capabilities:

```typescript
interface HandlerManifest {
  service_id: string;
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

### 3.1 VettID Infrastructure Role

**VettID provides infrastructure, not control:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        VETTID INFRASTRUCTURE                                 │
│                                                                              │
│  What VettID provides:                                                       │
│  ├── User Vaults (OwnerVaults) - hosted, encrypted, user-controlled         │
│  ├── MessageSpace (NATS) - event routing infrastructure                     │
│  └── Event Handlers - process external transactions (e.g., BTC on-chain)    │
│                                                                              │
│  What VettID does NOT do:                                                    │
│  ├── Approve or reject services                                             │
│  ├── Approve or reject connections                                          │
│  ├── Intermediate or route contract negotiations                            │
│  ├── Control what users or services do                                      │
│  └── Have visibility into encrypted vault contents                          │
│                                                                              │
│  Vault Communication Model:                                                  │
│  ├── IN:  Events received via NATS subscriptions                            │
│  ├── OUT: Events published via NATS                                         │
│  └── OUT: External transactions via event handlers (BTC, ETH, etc.)         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Connection Contract Establishment

Connection contracts are **direct agreements between user and service** - no intermediary. The contract is signed by the user's cryptographic key.

```
┌────────────┐     ┌────────────┐                         ┌──────────────┐
│  User App  │     │ User Vault │         NATS            │ Service Vault│
└─────┬──────┘     └─────┬──────┘    (MessageSpace/       └──────┬───────┘
      │                  │            ServiceSpace)              │
      │                  │                 │                     │
      │ 1. User obtains  │                 │                     │
      │    service info  │                 │                     │
      │    (QR, link,    │                 │                     │
      │     directory)   │                 │                     │
      │                  │                 │                     │
      │ 2. Request       │                 │                     │
      │    contract offer│                 │                     │
      │ ─────────────────►                 │                     │
      │                  │                 │                     │
      │                  │ 3. Fetch service│                     │
      │                  │    contract     │                     │
      │                  │    offerings    │                     │
      │                  │ ────────────────┼────────────────────►│
      │                  │                 │                     │
      │                  │                 │  4. Return contract │
      │                  │                 │     offer (multiple │
      │                  │                 │     options/tiers)  │
      │                  │◄────────────────┼─────────────────────│
      │                  │                 │                     │
      │ 5. Display       │                 │                     │
      │    contract      │                 │                     │
      │    options       │                 │                     │
      │◄─────────────────┤                 │                     │
      │                  │                 │                     │
      │ 6. User selects  │                 │                     │
      │    option &      │                 │                     │
      │    signs with    │                 │                     │
      │    their key     │                 │                     │
      │ ─────────────────►                 │                     │
      │                  │                 │                     │
      │                  │ 7. Generate     │                     │
      │                  │    connection   │                     │
      │                  │    keys (X25519)│                     │
      │                  │                 │                     │
      │                  │ 8. Send signed  │                     │
      │                  │    contract +   │                     │
      │                  │    user pubkey  │                     │
      │                  │ ────────────────┼────────────────────►│
      │                  │                 │                     │
      │                  │                 │  9. Service accepts │
      │                  │                 │     + sends pubkey  │
      │                  │                 │     + NATS creds    │
      │                  │◄────────────────┼─────────────────────│
      │                  │                 │                     │
      │                  │ 10. Store       │                     │
      │                  │     contract    │                     │
      │                  │     locally     │                     │
      │                  │                 │                     │
      │ 11. Contract     │                 │                     │
      │     active       │                 │                     │
      │◄─────────────────┤                 │                     │
      │                  │                 │                     │
```

**Key points:**
- User vault and service vault communicate directly via NATS
- No VettID approval or routing - VettID just provides infrastructure
- Contract is signed by user's cryptographic key
- Service offers multiple options; user selects one

### 3.3 Service Contract Offer

Services publish a **Contract Offer** with multiple options for users to choose from:

```typescript
interface ServiceContractOffer {
  service_id: string;               // Key-derived service identity
  service_name: string;
  service_public_key: string;       // Ed25519 for verification
  service_encryption_key: string;   // X25519 for key exchange
  service_nats_endpoint: string;    // Service's NATS cluster

  // Domain validation (optional)
  domain?: string;
  domain_verified?: boolean;

  // Registry attestations (optional)
  attestations?: RegistryAttestation[];

  // Multiple contract options
  offerings: ContractOffering[];

  // Offer metadata
  offer_version: string;
  valid_until?: string;             // Optional expiration
}

interface ContractOffering {
  offering_id: string;
  name: string;                     // e.g., "Basic", "Premium", "Enterprise"
  description: string;

  // What capabilities this offering includes
  capabilities: CapabilityGrant[];

  // What data the service requires from user
  required_data: DataRequirement[];

  // Pricing (optional)
  pricing?: {
    type: 'free' | 'one_time' | 'subscription';
    amount?: number;
    currency?: string;
    billing_cycle?: 'monthly' | 'yearly';
    trial_days?: number;
  };

  // Terms
  terms_url?: string;
  terms_hash?: string;              // Hash of terms for immutability
}

interface CapabilityGrant {
  capability: string;
  description: string;              // Human-readable explanation
  scope?: string[];                 // Scope limitations
  requires_approval_each_use: boolean;
}

interface DataRequirement {
  data_type: string;                // e.g., "email", "profile", "credential:drivers_license"
  required: boolean;
  purpose: string;                  // Why the service needs this
}
```

### 3.4 Signed Connection Contract

The user signs their chosen offering to create a binding contract:

```typescript
interface SignedConnectionContract {
  // Contract identification
  contract_id: string;              // Hash of contract content

  // Parties (both key-derived identities)
  user_id: string;                  // User's key-derived identity
  service_id: string;               // Service's key-derived identity

  // Selected offering
  offering_id: string;              // Which offering user selected
  offering_snapshot: ContractOffering; // Exact terms at time of signing

  // Connection keys (generated per-contract for forward secrecy)
  user_connection_key: string;      // User's X25519 public key for this connection
  service_connection_key: string;   // Service's X25519 public key (filled on acceptance)

  // Service NATS credentials (issued by service after acceptance)
  service_nats_credentials?: {
    endpoint: string;
    account_jwt: string;
    user_jwt: string;
    user_seed: string;
  };

  // User's cryptographic signature
  user_signature: {
    signed_at: string;              // ISO8601
    signing_key: string;            // User's Ed25519 public key
    signature: string;              // Ed25519 signature over contract
  };

  // Service's acceptance signature
  service_signature?: {
    accepted_at: string;
    signing_key: string;
    signature: string;
  };

  // Lifecycle
  status: 'pending' | 'active' | 'cancelled' | 'expired';
  created_at: string;
  activated_at?: string;
  cancelled_at?: string;
  cancelled_by?: 'user' | 'service';
}
```

### 3.5 Contract Verification

Anyone can verify a contract's authenticity without any central authority:

```typescript
function verifyContract(contract: SignedConnectionContract): boolean {
  // 1. Verify user's identity matches their signing key
  const expectedUserId = base58(sha256(contract.user_signature.signing_key).slice(0, 20));
  if (contract.user_id !== expectedUserId) return false;

  // 2. Verify user signed the contract
  const signedData = canonicalize({
    contract_id: contract.contract_id,
    user_id: contract.user_id,
    service_id: contract.service_id,
    offering_id: contract.offering_id,
    offering_snapshot: contract.offering_snapshot,
    user_connection_key: contract.user_connection_key,
    signed_at: contract.user_signature.signed_at
  });

  return ed25519.verify(
    contract.user_signature.signature,
    signedData,
    contract.user_signature.signing_key
  );
}
```

**Security properties:**
- User's signature cryptographically proves consent to specific terms
- `offering_snapshot` captures exact terms at signing time
- Contract ID is content hash - tamper-evident
- Both parties sign, creating mutual cryptographic agreement
- No central authority needed to verify

### 3.6 Capability Types

Standard capabilities that services can request:

#### User Profile (Automatic - Shared First)

**User profile is shared at the start of the connection process** - before contract negotiation. When a user scans a QR code or clicks a link to connect, the service immediately receives the user's public profile. This allows the service to:
- See basic information about the user (e.g., location, display name)
- Tailor contract offerings based on user context
- Personalize the connection experience

```typescript
interface UserProfile {
  user_guid: string;       // User's unique identifier
  display_name: string;    // Display name (1-100 chars)
  avatar_url?: string;     // Avatar image URL
  bio?: string;            // User biography (up to 500 chars)
  location?: string;       // User's location (up to 100 chars)
  last_updated: string;    // ISO8601 timestamp
}
```

**Profile sharing timeline:**
1. **Connection initiation** (QR scan/link click): Service receives user's public profile
2. **Contract negotiation**: Service can use profile to customize offerings (e.g., regional pricing)
3. **After contract signed**: Service continues receiving profile updates via MessageSpace

No capability is required for profile access - it's automatic for any connection attempt.

#### Identity & Authentication
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `authenticate` | Verify user identity | "Confirm your identity to this service" |
| `authenticate.continuous` | Periodic re-authentication | "Allow ongoing identity verification" |
| `verify_presence` | Check if user is available | "See when you're available" |

#### Data Access

Beyond the automatic profile, services access user data through a metadata-first model. Users control what metadata is visible and must explicitly consent to share actual values.

| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `browse_metadata` | See metadata about data in user's vault (user controls visibility) | "See what data you have stored" |
| `request_data` | Request specific data values based on metadata | "Request access to your [data type]" |
| `write_data` | Store data in user's vault (service namespace) | "Store data in your vault" |
| `read_data` | Read service-stored data from user's vault | "Access stored service data" |

**Data Request Flow:**
```
1. Service has `browse_metadata` capability
2. Service sees: { type: "drivers_license", issuer: "CA DMV", expires: "2027-03-15" }
   (User controls what metadata fields are visible)
3. Service requests specific value via `request_data`
4. User sees: "Acme Service wants to see your Driver's License
              [Share Once] [Add to Contract] [Deny]"
5. User chooses:
   - Share Once: One-time release, no contract change
   - Add to Contract: Service prepares contract update for ongoing access
```

**Contract Updates:**

When a service needs to modify an existing contract (add/remove capabilities, data access, etc.):

```typescript
interface ContractUpdate {
  contract_id: string;              // Existing contract being updated
  prepared_by: 'service' | 'user';  // Who initiated the update

  changes: {
    added: {
      capabilities?: CapabilityGrant[];
      data_access?: DataRequirement[];
      pricing_changes?: PricingChange;
    };
    removed: {
      capabilities?: string[];      // Capability IDs being removed
      data_access?: string[];       // Data types being removed
    };
  };

  // Clear summary for user
  summary: string;                  // "Add: Driver's License access. Remove: none."

  // New contract snapshot if approved
  updated_offering: ContractOffering;

  // Service signature on proposed update
  service_signature: {
    signed_at: string;
    signature: string;
  };
}
```

**Contract Update Flow:**
```
1. Service prepares ContractUpdate showing exactly what changes
2. Update delivered to user via MessageSpace
3. User sees clear diff: "Acme Service wants to update your contract:
                         + ADD: Access to Driver's License
                         + ADD: Access to email address
                         - REMOVE: (nothing)
                         [Approve & Sign] [Deny]"
4. User signs update with their key
5. Updated contract becomes active
```

#### Service Secrets (Stored in User's Protean Credential)

Services can request that users store secrets in their protean credential. These secrets are **opaque to the user** - the user cannot view them, only release them back to the service.

**Note:** Minor secret metadata can be included in `browse_metadata` results if the user allows, enabling services to know what secrets exist without accessing values.

| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `secret.minor.write` | Store minor secrets in user's protean credential | "Store service data securely in your credential" |
| `secret.minor.read` | Retrieve minor secrets (no user interaction) | *(granted with write)* |
| `secret.critical.write` | Store critical secrets in user's protean credential | "Store critical keys in your credential (password required to access)" |
| `secret.critical.read` | Retrieve critical secrets (requires password + approval) | "Release critical key to [Service]?" |
| `secret.user_owned.issue` | Issue a secret that becomes user's own property | "Accept a key that you will fully own and control" |

**Three Tiers of Service Secrets:**

All service secrets are stored in the **user's protean credential** (not the vault). The protean credential is the secure, user-controlled container for sensitive cryptographic material.

| Tier | Examples | Storage Location | Retrieval | User Can View | Who Controls |
|------|----------|------------------|-----------|---------------|--------------|
| **Minor** | Encryption keys, session tokens, API keys | Protean credential | Service retrieves automatically | No | Service |
| **Critical** | Service's master keys, signing keys | Protean credential | Password + approval each time | No | Service |
| **User-Owned** | User's crypto wallet keys, personal signing keys | Protean credential | User has full control | Yes | User |

**Minor Secrets Flow:**
```
1. Service requests: secret.minor.write
2. User sees: "Acme Service wants to store encrypted data in your credential"
3. User approves once
4. Secret stored in user's protean credential
5. Service can store/retrieve minor secrets without further interaction
6. User cannot view the secret contents
```

**Critical Secrets Flow:**
```
1. Service requests: secret.critical.write
2. User sees: "Acme Service wants to store a critical key in your credential.
              You'll need to enter your password to release it."
3. User approves + enters password to confirm
4. Secret stored in user's protean credential (encrypted, protected)

Later, when service needs the secret:
1. Service requests: secret.critical.read
2. User sees: "Acme Service is requesting your critical key.
              Purpose: Sign transaction #12345
              [Enter Password] [Deny]"
3. User enters password + approves
4. Protean credential releases secret to service (user never sees the value)
```

**User-Owned Secrets Flow:**
```
1. Service requests: secret.user_owned.issue
2. User sees: "Acme Wallet wants to issue you a private key.
              This key will be YOURS - you can view it, export it,
              and use it independently of Acme Wallet.
              [Accept + Enter Password] [Decline]"
3. User enters password to accept
4. Secret stored in protean credential as USER'S OWN KEY

User has full control:
- View the key anytime (with password)
- Export/backup the key
- Use with any compatible service
- Service can request to USE the key (user approves each time)
- User can revoke service's access while keeping the key
```

**User-Owned vs Service-Controlled:**
| Aspect | Minor/Critical (Service) | User-Owned |
|--------|--------------------------|------------|
| Who created it | Service | Service (issued to user) |
| Who owns it | Service | User |
| User can view | No | Yes (with password) |
| User can export | No | Yes |
| User can use independently | No | Yes |
| On contract revocation | Return or delete | User keeps it |
| Service can request use | Automatic (minor) or approval (critical) | User approves each use |

**Security Properties:**
- Secrets are encrypted within the user's protean credential
- User cannot export, copy, or view secret contents
- Critical secrets require active user participation to release
- All access is logged in the user's audit trail
- If user revokes contract, secrets can be:
  - Returned to service (if service requests)
  - Permanently deleted (user's choice)

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

#### Recovery & Delegation (Critical)
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `recovery.designated_contact` | Be a designated recovery contact for user | "Become a recovery contact (help restore access if locked out)" |
| `recovery.request_assist` | Request recovery assistance from user's designated contacts | "Request help from your recovery contacts" |
| `emergency.access` | Emergency access to specified data/capabilities (e.g., medical emergency) | "Access emergency information if you're incapacitated" |
| `delegate.temporary` | Receive temporary delegated access from user | "Act on your behalf temporarily" |

**Recovery/Emergency properties:**
- Designated contacts are cryptographically bound to user
- Emergency access may require multiple contacts or time delay
- All emergency access is fully logged
- User defines what data/capabilities are accessible in emergencies

#### Verification & Compliance (Critical)
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `verify.age` | Verify user meets minimum age requirement | "Verify you are over [age]" |
| `verify.identity_level` | Verify identity verification level (KYC tier) | "Verify your identity verification level" |
| `verify.location` | Verify user is in allowed jurisdiction | "Verify your current location" |
| `compliance.audit_request` | Request compliance/audit data (with user approval) | "Provide audit data for [purpose]" |

**Verification properties:**
- Returns boolean/level only - not underlying documents
- Service doesn't see how verification was achieved
- User can use any qualifying credential

#### Multi-Party Authorization (Critical)
| Capability | Description | User Prompt |
|------------|-------------|-------------|
| `multiparty.require` | Require multiple users to approve an action | "Require approval from [N] people" |
| `multiparty.cosign` | Act as a co-signer for another user's action | "Co-sign [action] for [user]" |

**Multi-party properties:**
- Useful for business accounts, high-value transactions
- Each party signs independently with their own key
- Configurable threshold (e.g., 2-of-3, 3-of-5)

Services can define custom capabilities beyond these standard types. Custom capabilities are self-describing - no approval required.

---

## 4. NATS Topic Architecture

### 4.1 Dual-NATS Model

Communication between services and users spans **two separate NATS environments**:

| NATS Environment | Operator | Purpose | Direction |
|------------------|----------|---------|-----------|
| **VettID NATS** (MessageSpace) | VettID | Service sends TO users | Service → User |
| **Service NATS** (ServiceSpace) | Service Provider | Users send TO service | User → Service |

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         VettID NATS Cluster                                  │
│                                                                              │
│  NATS Topics (VettID operated):                                             │
│  │                                                                           │
│  ├── OwnerSpace.<user_guid>/              # Existing: User ↔ User App       │
│  │   ├── forVault.>                                                         │
│  │   ├── forApp.>                                                           │
│  │   └── ...                                                                │
│  │                                                                           │
│  ├── MessageSpace.<user_guid>/            # Service → User communication    │
│  │   └── fromService.<service_id>/      # Messages from specific service  │
│  │       ├── auth.request                 # Authentication requests         │
│  │       ├── authz.request                # Authorization requests          │
│  │       ├── data.request                 # Data requests                   │
│  │       ├── payment.request              # Payment requests                │
│  │       ├── call.initiate                # Initiate call                   │
│  │       ├── call.signal.<session_id>     # Call signaling (svc → user)    │
│  │       └── notify.>                     # Notifications                   │
│  │                                                                           │
│  ├── Control/                             # Existing control plane          │
│  │   ├── global.*                                                           │
│  │   ├── enclave.*                                                          │
│  │   └── user.*                                                             │
│  │                                                                           │
│  └── Directory/                           # VettID Service Directory        │
│      ├── services.list                    # List all approved services      │
│      ├── services.<service_id>.info     # Service public info             │
│      └── services.search                  # Search services                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                    Service Provider's NATS Cluster                           │
│                       (Operated by Service Provider)                         │
│                                                                              │
│  NATS Topics (Service operated):                                            │
│  │                                                                           │
│  ├── ServiceSpace.<service_id>/         # Service's namespace             │
│  │   │                                                                       │
│  │   ├── fromUser.<user_guid>/            # User Vault → Service            │
│  │   │   ├── auth.response.<event_id>     # Auth responses                  │
│  │   │   ├── authz.response.<event_id>    # Authorization responses         │
│  │   │   ├── data.response.<event_id>     # Data responses                  │
│  │   │   ├── payment.response.<event_id>  # Payment confirmations           │
│  │   │   ├── call.signal.<session_id>     # Call signaling (user → svc)    │
│  │   │   └── events.>                     # User-initiated events           │
│  │   │                                                                       │
│  │   ├── directory/                       # Service discovery               │
│  │   │   ├── manifest                     # Handler manifest                │
│  │   │   └── status                       # Service health/availability     │
│  │   │                                                                       │
│  │   └── internal/                        # Service-internal messaging      │
│  │       ├── control.>                                                      │
│  │       ├── health.>                                                       │
│  │       └── metrics.>                                                      │
│  │                                                                           │
│  └── (Service can add custom internal topics as needed)                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 User Vault NATS Connections

Each user vault maintains connections to:
1. **VettID NATS** - Always connected (for OwnerSpace and receiving from MessageSpace)
2. **Service NATS** - Connected per service contract (for sending to ServiceSpace)

```typescript
// User vault connection configuration per service contract
interface ServiceNATSConnection {
  service_id: string;
  nats_endpoint: string;           // Service's NATS cluster endpoint
  nats_credentials: {
    account_jwt: string;           // Issued by service for this user
    user_jwt: string;
    user_seed: string;             // User's nkey for this service
  };
  connection_key: string;          // X25519 key for message encryption
  topics: {
    publish_prefix: string;        // ServiceSpace.<svc>.fromUser.<user>
  };
}
```

### 4.3 Message Flow: Request with Timeout & Offline Support

Services can set request timeouts and support offline users. Note the different NATS clusters for each direction:

```
┌─────────────────┐                                        ┌─────────────────┐
│  Service Vault  │                                        │   User Vault    │
└────────┬────────┘                                        └────────┬────────┘
         │                                                          │
         │ 1. Publish request (via VettID MessageSpace)             │
         │ ────────────────────────────────────────────────────────►│
         │    NATS: VettID Cluster                                  │
         │    Topic: MessageSpace.<user>.fromService.<svc>.         │
         │           auth.request                                   │
         │    Payload: {                                            │
         │      event_id: "uuid",                                   │
         │      event_type: "auth.request",                         │
         │      timestamp: "ISO8601",                               │
         │      expires_at: "ISO8601",    // Service-defined        │
         │      offline_grace: 300,       // 5 min after online     │
         │      service_nats_endpoint: "nats://svc.example.com",    │
         │      encrypted_payload: {...}                            │
         │    }                                                     │
         │                                                          │
         │    (Message queued in VettID JetStream)                  │
         │                                                          │
         │                         ─────────────────────────────    │
         │                         User may be offline              │
         │                         Message waits in VettID queue    │
         │                         ─────────────────────────────    │
         │                                                          │
         │                         2. User comes online             │
         │                            Vault retrieves from VettID   │
         │                                                          │
         │                         3. Check expiry:                 │
         │                            - If past expires_at: discard │
         │                            - If within offline_grace:    │
         │                              process normally            │
         │                                                          │
         │                         4. Process & respond             │
         │◄──────────────────────────────────────────────────────── │
         │    NATS: Service's Cluster                               │
         │    Topic: ServiceSpace.<svc>.fromUser.<user>.            │
         │           auth.response.<event_id>                       │
         │                                                          │
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

Service Vaults are **entirely controlled by the service provider**, including their own NATS cluster for receiving messages from users. No Nitro Enclave is required since the service vault doesn't hold high-value secrets like cryptocurrency private keys. User-specific secrets are stored in user vaults, not service vaults.

```
Service Vault Deployment (Provider-Controlled):
┌──────────────────────────────────────────────────────────────────────────────┐
│                      Service Provider Infrastructure                          │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │              Service's NATS Cluster (ServiceSpace)                       │ │
│  │                                                                          │ │
│  │  ┌────────────────────────────────────────────────────────────────────┐ │ │
│  │  │  ServiceSpace.<service_id>/                                      │ │ │
│  │  │    fromUser.*.>     ← Receives messages from connected users       │ │ │
│  │  │    internal.>       ← Service internal messaging                   │ │ │
│  │  └────────────────────────────────────────────────────────────────────┘ │ │
│  │                                                                          │ │
│  │  (Provider operates: can be NATS cluster, Synadia Cloud, etc.)          │ │
│  └───────────────────────────────────┬─────────────────────────────────────┘ │
│                                      │                                        │
│  ┌───────────────────────────────────┼───────────────────────────────────┐   │
│  │                      SERVICE VAULT│                                    │   │
│  │                                   │                                    │   │
│  │  ┌──────────────┐  ┌──────────────┴──┐  ┌──────────────┐              │   │
│  │  │ ServiceSpace │  │ VettID NATS     │  │ Handler      │              │   │
│  │  │ Subscriber   │  │ Publisher       │  │ Engine       │              │   │
│  │  │ (own NATS)   │  │ (MessageSpace)  │  │              │              │   │
│  │  └──────────────┘  └─────────────────┘  └──────────────┘              │   │
│  │                                                                        │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │   │
│  │  │ Contract Mgr │  │ Service API  │  │ Audit Logger │                 │   │
│  │  │              │  │              │  │              │                 │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘                 │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                      │                                        │
│  ┌───────────────────────────────────┼───────────────────────────────────┐   │
│  │                    Storage Layer  │                                    │   │
│  │  ┌─────────────────┐  ┌──────────┴──┐  ┌─────────────────┐            │   │
│  │  │ Hardened DB     │  │ Key Store   │  │ Audit Log       │            │   │
│  │  │ (Contracts,     │  │ (HSM/KMS)   │  │ (Immutable)     │            │   │
│  │  │  User NATS creds│  │             │  │                 │            │   │
│  │  │  state)         │  │             │  │                 │            │   │
│  │  └─────────────────┘  └─────────────┘  └─────────────────┘            │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
│                                                                               │
│  Security Layer:                                                              │
│  - Service's NATS with TLS + authentication                                  │
│  - Service key in HSM/KMS                                                    │
│  - Encrypted database for contracts and user credentials                     │
│  - Audit logging for all operations                                          │
│                                                                               │
└───────────────────────────────────────┬───────────────────────────────────────┘
                                        │
                                   TLS/mTLS
                            (Outbound to VettID)
                                        │
                                        ▼
                             ┌──────────────────┐
                             │  VettID NATS     │
                             │  (MessageSpace)  │
                             │                  │
                             │ Service publishes│
                             │ TO users here    │
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

// Service Secrets (stored in user's protean credential)

// Store a minor secret (user approves once, then automatic access)
POST /api/v1/secrets/minor
{
  user_id: string,
  secret_id: string,              // Service-defined identifier
  secret_data: string,            // Base64-encoded secret (encrypted in transit)
  metadata: {
    description: string,          // Shown to user: "Encryption key for your files"
    created_at: string,
    expires_at?: string           // Optional expiration
  }
}
Response: {
  request_id: string,
  status: "pending" | "stored" | "denied",
  secret_ref: string              // Reference for retrieval
}

// Retrieve a minor secret (no user interaction required)
GET /api/v1/secrets/minor/{secret_id}?user_id={user_id}
Response: {
  secret_id: string,
  secret_data: string,            // Base64-encoded secret
  metadata: object
}

// Store a critical secret (user approves + authenticates)
POST /api/v1/secrets/critical
{
  user_id: string,
  secret_id: string,
  secret_data: string,            // Base64-encoded secret
  metadata: {
    description: string,          // "Master signing key for your wallet"
    purpose: string,              // Why this needs critical protection
    created_at: string
  }
}
Response: {
  request_id: string,
  status: "pending" | "stored" | "denied",
  secret_ref: string
}

// Retrieve a critical secret (requires user password + approval)
POST /api/v1/secrets/critical/request
{
  user_id: string,
  secret_id: string,
  purpose: string,                // "Sign transaction #12345" - shown to user
  context: object,                // Additional context for user
  expires_in_seconds?: number,
  callback_url?: string
}
Response: {
  request_id: string,
  status: "pending" | "released" | "denied" | "expired",
  secret_data?: string            // Only if released
}

// Issue a user-owned secret (user gains full ownership)
POST /api/v1/secrets/user-owned/issue
{
  user_id: string,
  secret_id: string,
  secret_data: string,            // Base64-encoded secret
  secret_type: string,            // "crypto_key", "signing_key", "recovery_phrase", etc.
  metadata: {
    name: string,                 // User-visible name: "My Acme Wallet Key"
    description: string,          // "Your personal signing key for Acme Wallet"
    key_type?: string,            // "ed25519", "secp256k1", etc.
    created_at: string,
    issued_by: string             // Service name (for provenance)
  }
}
Response: {
  request_id: string,
  status: "pending" | "accepted" | "declined",
  user_secret_id?: string         // User's reference to their own key
}

// Request to USE a user-owned secret (user must approve)
POST /api/v1/secrets/user-owned/use
{
  user_id: string,
  user_secret_id: string,         // Reference to user's key
  purpose: string,                // "Sign transaction to 0x1234..."
  operation: string,              // "sign", "decrypt", "derive", etc.
  payload?: string,               // Data to operate on (if applicable)
  expires_in_seconds?: number,
  callback_url?: string
}
Response: {
  request_id: string,
  status: "pending" | "completed" | "denied" | "expired",
  result?: string                 // Operation result (e.g., signature)
}

// Delete a service-controlled secret (service-initiated)
DELETE /api/v1/secrets/{type}/{secret_id}?user_id={user_id}
Response: {
  status: "deleted" | "not_found"
}
// Note: User-owned secrets cannot be deleted by service

// List secrets for a user (metadata only, not values)
GET /api/v1/secrets?user_id={user_id}
Response: {
  minor: [{ secret_id, description, created_at, expires_at }],
  critical: [{ secret_id, description, purpose, created_at }],
  user_owned: [{ user_secret_id, name, secret_type, issued_by, created_at }]
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
     target_service_id: "<service_b_guid>",
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
    service_id: string;
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
    service_id: string;
    operation: 'read' | 'write' | 'delete';
    field: string;
    summary: string;              // Human-readable description
  }[];
}
```

---

## 7. Payments & Subscriptions

### 7.1 Payment Model

VettID does not directly process payments or take fees. Payment processing depends on the payment method:

- **Traditional payments (credit/debit/ACH)**: User shares payment details with service; service processes via their own payment processor
- **Cryptocurrency**: Service provides a transaction; user approves; user's vault puts transaction on-chain via event handlers

```
Payment Negotiation Flow:
┌────────────────┐          ┌────────────────┐          ┌────────────────┐
│  Service Vault │          │  User's Vault  │          │  User App      │
└───────┬────────┘          └───────┬────────┘          └───────┬────────┘
        │                           │                           │
        │ 1. Request payment options│                           │
        │ ─────────────────────────►│                           │
        │   "What payment methods   │                           │
        │    does user have?"       │                           │
        │                           │                           │
        │ 2. User's payment options │                           │
        │◄───────────────────────── │                           │
        │   {credit_card: true,     │                           │
        │    debit_card: true,      │                           │
        │    ach: false,            │                           │
        │    btc: true,             │                           │
        │    eth: false}            │                           │
        │                           │                           │
        │ 3. Present supported      │                           │
        │    options + contract     │                           │
        │ ─────────────────────────►│──────────────────────────►│
        │   "Service accepts:       │                           │
        │    credit/debit, BTC.     │  4. User selects         │
        │    Select payment method" │     payment method        │
        │                           │◄──────────────────────────│
        │                           │                           │
```

#### Traditional Payment Flow (Credit/Debit/ACH)

For traditional payment methods, actual payment details are sent to the service for processing:

```
┌────────────────┐          ┌────────────────┐          ┌────────────────┐
│  Service Vault │          │  User's Vault  │          │Service's Payment│
│                │          │                │          │   Processor     │
└───────┬────────┘          └───────┬────────┘          └───────┬────────┘
        │                           │                           │
        │ 1. User selected          │                           │
        │    credit card            │                           │
        │                           │                           │
        │ 2. Request card details   │                           │
        │ ─────────────────────────►│                           │
        │   {amount: 29.99,         │                           │
        │    currency: "USD",       │                           │
        │    description: "..."}    │                           │
        │                           │                           │
        │ 3. User approves in app   │                           │
        │    (confirms amount)      │                           │
        │                           │                           │
        │ 4. Card details sent      │                           │
        │◄───────────────────────── │                           │
        │   {card_number: "...",    │                           │
        │    exp: "...", cvv: "..."} │                          │
        │                           │                           │
        │ 5. Service processes      │                           │
        │    payment                │                           │
        │ ──────────────────────────┼──────────────────────────►│
        │                           │                           │
        │ 6. Payment result         │                           │
        │◄──────────────────────────┼───────────────────────────│
        │                           │                           │
        │ 7. Confirm to user        │                           │
        │ ─────────────────────────►│                           │
        │                           │                           │
```

**Note:** Traditional payment processing is entirely handled by the service and their payment processor. VettID facilitates the secure transmission of payment details but does not process or store them.

#### Cryptocurrency Payment Flow (VettID-Supported Chains)

For cryptocurrency payments, the service cannot access user's private keys. Instead:

```
┌────────────────┐          ┌────────────────┐          ┌────────────────┐
│  Service Vault │          │  User's Vault  │          │   Blockchain   │
│                │          │  (has crypto   │          │                │
│                │          │   private key) │          │                │
└───────┬────────┘          └───────┬────────┘          └───────┬────────┘
        │                           │                           │
        │ 1. User selected BTC      │                           │
        │                           │                           │
        │ 2. Service creates        │                           │
        │    unsigned transaction   │                           │
        │ ─────────────────────────►│                           │
        │   {to: <service_btc_addr>,│                           │
        │    amount: 0.001,         │                           │
        │    chain: "bitcoin"}      │                           │
        │                           │                           │
        │ 3. User sees transaction  │                           │
        │    details in app:        │                           │
        │    "Send 0.001 BTC to     │                           │
        │     Acme Service?"        │                           │
        │    [Approve] [Reject]     │                           │
        │                           │                           │
        │    (User CANNOT modify    │                           │
        │     amount or recipient)  │                           │
        │                           │                           │
        │ 4. User approves          │                           │
        │                           │                           │
        │ 5. Vault signs & submits  │                           │
        │    transaction            │                           │
        │                           │ ─────────────────────────►│
        │                           │   (via event handler)     │
        │                           │                           │
        │                           │ 6. Transaction confirmed  │
        │                           │◄───────────────────────── │
        │                           │                           │
        │ 7. Confirmation to service│                           │
        │◄───────────────────────── │                           │
        │   {tx_hash: "...",        │                           │
        │    status: "confirmed"}   │                           │
        │                           │                           │
```

**Crypto payment properties:**
- User's private keys never leave their vault
- User can only approve or reject - cannot modify transaction
- Vault's event handler puts signed transaction on-chain
- Service receives confirmation with transaction hash
- This is the only way VettID indirectly facilitates payment processing

**Supported cryptocurrencies:** Determined by vault event handlers (e.g., Bitcoin, Ethereum, etc.)

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
  service_id: string;
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
  service_id: string;
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

### 9.1 Decentralized Service Identity

**Decision**: Service identity is key-derived and self-sovereign. Registry registration is optional and registry-agnostic.

**Rationale**:
- Services should have the same self-sovereign principles as users
- Identity must be cryptographically verifiable without any central authority
- No registry should gate service operation
- Multiple registries can coexist (VettID is one option among many)
- Enables fully peer-to-peer connections

**Implementation**:
- Services generate their own cryptographic identity (Ed25519 + X25519)
- `service_id = base58(sha256(public_key)[0:20])` - identity IS the key
- Optional domain validation via DNS TXT records (human-readable association)
- Services operate fully standalone on their own NATS infrastructure
- Optional registration with any service registry (VettID, industry consortiums, etc.)
- Registries provide attestations but don't control identity
- Users can connect to unregistered services via direct key exchange

### 9.2 Security Model

**Decision**: Service Vaults do not require Nitro Enclaves.

**Rationale**:
- Service vaults hold connection keys, not high-value secrets like cryptocurrency keys
- User-specific secrets are stored in the user's vault, not the service vault
- Standard security practices (HSM for service key, encrypted DB, TLS) are sufficient
- Service providers have flexibility in their deployment infrastructure

**Recommendation**: Use HSM or cloud KMS for service signing key; encrypted database for connection keys.

### 9.3 Connection Persistence

**Decision**: Connections persist until one party cancels.

**Rationale**:
- NATS acts as a secure drop-box; no persistent network connections needed
- Users and services may go offline for extended periods
- Inactive connections don't consume resources (messages queue in NATS)
- Explicit cancellation gives both parties clear control

### 9.4 Offline Support

**Decision**: Full offline support with service-defined expiration.

**Implementation**:
- Services set `expires_at` on each request (1 minute to 30 days)
- Services set `offline_grace_seconds` for users coming back online
- Expired requests are discarded (or notification sent)
- Supports both immediate interactions and long-lived offers

### 9.5 Multi-Vault Services

**Decision**: Not required for MVP; scale horizontally within single logical service.

**Rationale**:
- Service vault doesn't store PII, just coordinates with user vaults
- No data residency requirements for coordination layer
- Horizontal scaling (multiple instances behind load balancer) handles capacity
- May revisit for geographic latency optimization in future

### 9.6 Service Registries

**Decision**: Registry-agnostic architecture. VettID operates one registry; others may exist.

**Implementation**:
- Services operate standalone by default (no registry required)
- Services may register with any registry (VettID, industry consortiums, etc.)
- Registries issue attestations that services can present to users
- VettID's registry provides: directory listing in VettID app, verified badge, optional MessageSpace access
- Users can discover services via registries OR direct key exchange
- Multiple attestations from different registries can coexist

### 9.7 Billing Model

**Decision**: VettID does not charge for service vaults or connections.

**Implementation**:
- Service operators offer free or paid services
- Payments flow directly between user and service (via user's payment methods)
- VettID has no visibility into payment transactions
- Supports one-time payments, subscriptions, and auto-renewal

### 9.8 Cross-Service Communication

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
| **Traditional Payments** | Shares card details with service on approval | Processes via own payment processor |
| **Crypto Payments** | Signs transactions, puts on-chain via event handlers | Creates unsigned transactions for user approval |

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
