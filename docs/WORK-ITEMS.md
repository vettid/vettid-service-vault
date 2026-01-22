# Service Vault Implementation Work Items

This document outlines the work items required to implement the Service Vault architecture across all VettID repositories.

---

## Overview

The Service Vault enables third-party services to connect with VettID users. Implementation spans four repositories:

| Repository | Role |
|------------|------|
| **vettid-service-vault** | New service vault codebase (reference implementation, SDK, CDK) |
| **vettid-dev** | Backend infrastructure changes (NATS topics, registry, MessageSpace) |
| **vettid-ios** | iOS app changes (service connections, contracts, UI) |
| **vettid-android** | Android app changes (service connections, contracts, UI) |

---

## vettid-service-vault

### Epic 1: Project Foundation

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-001 | Task | Initialize Go project structure | Create `vault/` directory with Go modules, cmd/, internal/, pkg/ structure per architecture doc | P0 |
| SV-002 | Task | Set up CDK infrastructure project | Initialize AWS CDK TypeScript project in `cdk/` for VettID-side infrastructure | P0 |
| SV-003 | Task | Create SDK project scaffolding | Set up `sdk/` directory with Node.js, Python, and Go SDK placeholders | P1 |
| SV-004 | Task | Set up CI/CD pipeline | GitHub Actions for linting, testing, and building the vault binary and CDK | P1 |

### Epic 2: Service Identity & Cryptography

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-010 | Feature | Implement service identity generation | Ed25519 signing keypair + X25519 encryption keypair generation with `service_id = base58(sha256(pubkey)[0:20])` | P0 |
| SV-011 | Feature | Implement key storage interface | Abstract key storage with HSM/KMS and local file implementations | P0 |
| SV-012 | Feature | Implement message encryption | X25519 + XChaCha20-Poly1305 encryption for service messages with ephemeral keys | P0 |
| SV-013 | Feature | Implement signature verification | Ed25519 signature creation and verification for contracts and messages | P0 |
| SV-014 | Feature | Implement DNS domain validation | TXT record verification for `_vettid-service.<domain>` | P1 |

### Epic 3: NATS Integration

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-020 | Feature | Implement ServiceSpace NATS client | Connect to service's own NATS cluster, handle reconnection, subscription management | P0 |
| SV-021 | Feature | Implement MessageSpace NATS client | Connect to VettID's MessageSpace for service→user communication | P0 |
| SV-022 | Feature | Implement topic routing | Route messages to correct handlers based on topic structure (`ServiceSpace.<svc>.fromUser.<user>.*`) | P0 |
| SV-023 | Feature | Implement message queuing | Handle offline users with JetStream persistence and expiration | P1 |
| SV-024 | Feature | Implement per-user NATS credential issuance | Issue NATS JWTs for connected users to publish to ServiceSpace | P0 |

### Epic 4: Contract Management

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-030 | Feature | Define contract offering schema | Implement `ServiceContractOffer`, `ContractOffering`, `CapabilityGrant`, `DataRequirement` types | P0 |
| SV-031 | Feature | Implement contract state storage | DynamoDB or PostgreSQL storage for active contracts with encryption | P0 |
| SV-032 | Feature | Implement contract negotiation flow | Handle contract offer requests, user selection, signing, and acceptance | P0 |
| SV-033 | Feature | Implement contract verification | Verify user signatures on contracts using Ed25519 | P0 |
| SV-034 | Feature | Implement contract update flow | Support adding/removing capabilities with user approval | P1 |
| SV-035 | Feature | Implement contract cancellation | Handle user or service-initiated contract termination | P1 |

### Epic 5: Handler Engine

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-040 | Feature | Implement authentication handler | Process `auth.request`/`auth.response` messages | P0 |
| SV-041 | Feature | Implement authorization handler | Process `authz.request`/`authz.response` messages | P0 |
| SV-042 | Feature | Implement data request handler | Process `data.request`/`data.response` for browse_metadata and request_data | P1 |
| SV-043 | Feature | Implement notification handler | Send notifications via MessageSpace | P1 |
| SV-044 | Feature | Implement timeout and retry logic | Handle request expiration, offline grace periods, and retries | P1 |

### Epic 6: Service API

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-050 | Feature | Implement REST API server | HTTP server with routes for `/api/v1/*` endpoints | P0 |
| SV-051 | Feature | Implement auth request endpoint | `POST /api/v1/auth/request` for authentication requests | P0 |
| SV-052 | Feature | Implement authz request endpoint | `POST /api/v1/authz/request` for authorization requests | P0 |
| SV-053 | Feature | Implement contract management endpoints | `GET/POST/DELETE /api/v1/contracts/*` | P0 |
| SV-054 | Feature | Implement webhook delivery | Deliver event callbacks to service backend | P1 |
| SV-055 | Feature | Implement call initiation endpoint | `POST /api/v1/call/initiate` for voice/video calls | P2 |
| SV-056 | Feature | Implement payment request endpoints | `POST /api/v1/payment/request` | P2 |
| SV-057 | Feature | Implement secrets API | `POST/GET/DELETE /api/v1/secrets/*` for minor/critical/user-owned secrets | P2 |

### Epic 7: AWS CDK Infrastructure (VettID Side)

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-060 | Feature | Create ServiceRegistryStack | DynamoDB table for registered services, Lambda handlers for registration | P0 |
| SV-061 | Feature | Create service registration Lambda | Handle service registration requests, issue attestations | P0 |
| SV-062 | Feature | Create service directory Lambda | List/search services in directory | P0 |
| SV-063 | Feature | Update MessageSpace permissions | Allow registered services to publish to `MessageSpace.<user>.fromService.<svc>.*` | P0 |
| SV-064 | Feature | Implement multi-tenant pool infrastructure | Shared ECS/Fargate, DynamoDB with partition isolation, shared NATS accounts per AWS-ARCHITECTURE.md | P1 |
| SV-065 | Feature | Implement silo tier infrastructure | Dedicated resources for large services | P2 |

### Epic 8: SDKs

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-070 | Feature | Node.js SDK | TypeScript SDK for integrating services with Service Vault | P1 |
| SV-071 | Feature | Python SDK | Python SDK for integrating services | P2 |
| SV-072 | Feature | Go SDK | Go SDK for integrating services | P2 |

### Epic 9: Documentation & Examples

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| SV-080 | Docs | API Reference documentation | OpenAPI spec for Service Vault REST API | P0 |
| SV-081 | Docs | Security Model documentation | Document threat model, security boundaries, audit requirements | P0 |
| SV-082 | Docs | Deployment Guide | Self-hosted and VettID-managed deployment instructions | P1 |
| SV-083 | Docs | SDK Integration Guide | How to integrate using SDKs | P1 |
| SV-084 | Example | Auth service example | Simple authentication integration example | P1 |
| SV-085 | Example | Support center example | Voice/video call example | P2 |

---

## vettid-dev

### Epic 1: NATS Infrastructure Updates

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| DEV-001 | Feature | Add MessageSpace service topics | Add `MessageSpace.<user>.fromService.<service_id>.*` topic structure to NATS config | P0 |
| DEV-002 | Feature | Implement service NATS authentication | Issue NATS credentials for registered services to publish to MessageSpace | P0 |
| DEV-003 | Feature | Update vault NATS subscriptions | Subscribe vaults to `MessageSpace.<user>.fromService.>` | P0 |
| DEV-004 | Feature | Add Directory namespace | Add `Directory/services.*` topics for service discovery | P1 |

### Epic 2: Service Registry Infrastructure

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| DEV-010 | Feature | Create ServiceRegistry DynamoDB table | Table for registered services with attestations, domain validation status | P0 |
| DEV-011 | Feature | Create service registration API | `POST /admin/services/register` for service registration | P0 |
| DEV-012 | Feature | Create service attestation Lambda | Issue signed attestations for registered services | P0 |
| DEV-013 | Feature | Create service directory API | `GET /public/services` and `GET /public/services/{id}` | P0 |
| DEV-014 | Feature | Implement domain validation | Verify DNS TXT records for service domain claims | P1 |

### Epic 3: User Vault Updates

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| DEV-020 | Feature | Add service connection storage | Store active service contracts in vault's database | P0 |
| DEV-021 | Feature | Implement contract signing | Sign contracts with user's Ed25519 key | P0 |
| DEV-022 | Feature | Implement service message routing | Route incoming service messages to appropriate handlers | P0 |
| DEV-023 | Feature | Implement capability enforcement | Enforce granted capabilities per contract | P0 |
| DEV-024 | Feature | Implement connection key management | Generate and store X25519 connection keys per service | P0 |
| DEV-025 | Feature | Add audit logging for service interactions | Log all service requests/responses to user's audit trail | P1 |
| DEV-026 | Feature | Implement profile auto-sharing | Share user profile on connection initiation | P1 |

### Epic 4: Service Message Handlers (Enclave)

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| DEV-030 | Feature | Implement auth request handler | Process `auth.request` from services, prompt user, send response | P0 |
| DEV-031 | Feature | Implement authz request handler | Process `authz.request` from services | P0 |
| DEV-032 | Feature | Implement data request handler | Process `data.request`, enforce capabilities, return data | P1 |
| DEV-033 | Feature | Implement notification handler | Receive and queue notifications from services | P1 |
| DEV-034 | Feature | Implement call signaling handler | WebRTC signaling for service calls via MessageSpace | P2 |
| DEV-035 | Feature | Implement payment request handler | Process payment requests, prompt user, execute payment | P2 |

### Epic 5: Secrets Storage (Protean Credential)

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| DEV-040 | Feature | Implement minor secret storage | Store service secrets in protean credential (automatic access after approval) | P2 |
| DEV-041 | Feature | Implement critical secret storage | Store critical secrets requiring password + approval each access | P2 |
| DEV-042 | Feature | Implement user-owned secret issuance | Accept keys that become user's property | P2 |

### Epic 6: Combined Datastores

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| DEV-050 | Feature | Implement combined datastore creation | Create shared data spaces between services with user approval | P3 |
| DEV-051 | Feature | Implement combined datastore access control | Enforce read/write permissions per service | P3 |
| DEV-052 | Feature | Implement combined datastore audit log | Track all cross-service data access | P3 |

### Epic 7: Documentation Updates

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| DEV-060 | Docs | Update NATS-MESSAGING-ARCHITECTURE.md | Add MessageSpace service topics, service authentication | P0 |
| DEV-061 | Docs | Create SERVICE-CONNECTIONS-SPEC.md | Document service connection flow from vault perspective | P0 |
| DEV-062 | Docs | Update specs/nats-api.md | Add service-related NATS topics and message formats | P1 |

---

## vettid-ios

### Epic 1: Service Connection UI

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| IOS-001 | Feature | Service connection initiation screen | UI for scanning QR code or opening deep link to connect to service | P0 |
| IOS-002 | Feature | Contract review screen | Display service offerings, capabilities, data requirements for user review | P0 |
| IOS-003 | Feature | Contract signing flow | Biometric/PIN confirmation to sign contract with user's key | P0 |
| IOS-004 | Feature | Connected services list | View all active service connections with details | P0 |
| IOS-005 | Feature | Service connection details screen | View contract details, granted capabilities, disconnect option | P0 |

### Epic 2: Service Directory

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| IOS-010 | Feature | Service directory browser | Browse/search VettID service directory | P1 |
| IOS-011 | Feature | Service detail view | View service info, attestations, domain verification, connect button | P1 |

### Epic 3: Service Interactions

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| IOS-020 | Feature | Authentication request prompt | Display auth request from service, approve/deny | P0 |
| IOS-021 | Feature | Authorization request prompt | Display authz request with action details, approve/deny | P0 |
| IOS-022 | Feature | Data request prompt | Display data request, show what's being shared, approve/deny | P1 |
| IOS-023 | Feature | Notification display | Show notifications from connected services | P1 |
| IOS-024 | Feature | Payment request prompt | Display payment details, select payment method, approve | P2 |
| IOS-025 | Feature | Call UI (voice/video) | Answer/initiate calls with verified services | P2 |

### Epic 4: Contract Management

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| IOS-030 | Feature | Contract update review | Display proposed contract changes, approve/deny | P1 |
| IOS-031 | Feature | Contract cancellation | Disconnect from service, confirm action | P1 |
| IOS-032 | Feature | Capability management | View/revoke specific capabilities | P2 |

### Epic 5: Core Infrastructure

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| IOS-040 | Feature | Service NATS connection manager | Connect to multiple service NATS clusters per active contracts | P0 |
| IOS-041 | Feature | Contract storage | Secure storage of signed contracts and connection keys | P0 |
| IOS-042 | Feature | Service message encryption | Encrypt/decrypt messages using connection keys | P0 |
| IOS-043 | Feature | Contract signing crypto | Sign contracts with Ed25519 key | P0 |

### Epic 6: Audit & History

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| IOS-050 | Feature | Service activity history | View all interactions with a service | P1 |
| IOS-051 | Feature | Audit log viewer | Full audit trail of service data access | P2 |

---

## vettid-android

### Epic 1: Service Connection UI

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| AND-001 | Feature | Service connection initiation screen | Compose UI for QR scan/deep link to connect to service | P0 |
| AND-002 | Feature | Contract review screen | Display service offerings with Compose UI | P0 |
| AND-003 | Feature | Contract signing flow | Biometric confirmation to sign contract | P0 |
| AND-004 | Feature | Connected services list | View active connections in main navigation | P0 |
| AND-005 | Feature | Service connection details screen | Contract details and disconnect option | P0 |

### Epic 2: Service Directory

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| AND-010 | Feature | Service directory browser | Browse/search service directory | P1 |
| AND-011 | Feature | Service detail view | Service info and connect flow | P1 |

### Epic 3: Service Interactions

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| AND-020 | Feature | Authentication request prompt | Bottom sheet for auth requests | P0 |
| AND-021 | Feature | Authorization request prompt | Authz request with action details | P0 |
| AND-022 | Feature | Data request prompt | Data sharing approval UI | P1 |
| AND-023 | Feature | Notification display | Service notification handling | P1 |
| AND-024 | Feature | Payment request prompt | Payment approval with method selection | P2 |
| AND-025 | Feature | Call UI (voice/video) | WebRTC call UI with verified badge | P2 |

### Epic 4: Contract Management

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| AND-030 | Feature | Contract update review | Review and approve contract changes | P1 |
| AND-031 | Feature | Contract cancellation | Disconnect confirmation flow | P1 |
| AND-032 | Feature | Capability management | Granular capability control | P2 |

### Epic 5: Core Infrastructure

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| AND-040 | Feature | Service NATS connection manager | Multi-cluster NATS connections with coroutines | P0 |
| AND-041 | Feature | Contract storage | EncryptedSharedPreferences for contracts | P0 |
| AND-042 | Feature | Service message encryption | XChaCha20-Poly1305 via Tink | P0 |
| AND-043 | Feature | Contract signing crypto | Ed25519 signing with Android Keystore | P0 |

### Epic 6: Audit & History

| ID | Type | Title | Description | Priority |
|----|------|-------|-------------|----------|
| AND-050 | Feature | Service activity history | Service interaction timeline | P1 |
| AND-051 | Feature | Audit log viewer | Complete audit trail UI | P2 |

---

## Implementation Phases

### Phase 1: Foundation (MVP)

**Goal**: Enable basic service connections and authentication

| Repository | Items |
|------------|-------|
| vettid-service-vault | SV-001, SV-002, SV-010-013, SV-020-024, SV-030-033, SV-040-041, SV-050-053, SV-060-063, SV-080-081 |
| vettid-dev | DEV-001-003, DEV-010-013, DEV-020-024, DEV-030-031, DEV-060-061 |
| vettid-ios | IOS-001-005, IOS-020-021, IOS-040-043 |
| vettid-android | AND-001-005, AND-020-021, AND-040-043 |

### Phase 2: Core Features

**Goal**: Authorization, data access, notifications, offline support

| Repository | Items |
|------------|-------|
| vettid-service-vault | SV-014, SV-023, SV-034-035, SV-042-044, SV-054, SV-082-084 |
| vettid-dev | DEV-014, DEV-025-026, DEV-032-033, DEV-062 |
| vettid-ios | IOS-010-011, IOS-022-023, IOS-030-032, IOS-050 |
| vettid-android | AND-010-011, AND-022-023, AND-030-032, AND-050 |

### Phase 3: Communication & Payments

**Goal**: Calls, payments, subscriptions

| Repository | Items |
|------------|-------|
| vettid-service-vault | SV-055-057, SV-085 |
| vettid-dev | DEV-034-035, DEV-040-042 |
| vettid-ios | IOS-024-025, IOS-051 |
| vettid-android | AND-024-025, AND-051 |

### Phase 4: Advanced Features

**Goal**: Cross-service collaboration, SDKs, scale

| Repository | Items |
|------------|-------|
| vettid-service-vault | SV-003, SV-064-065, SV-070-072 |
| vettid-dev | DEV-050-052 |

---

## Dependencies

```
Phase 1 Dependencies:
SV-010 → SV-012, SV-013 (crypto foundation)
DEV-001 → DEV-002 → DEV-003 (NATS setup)
SV-020, SV-021 → SV-030-033 (NATS before contracts)
DEV-020-024 → IOS-040-043, AND-040-043 (vault before mobile)
SV-060-063 → DEV-010-013 (registry infrastructure)
```

---

## Estimated Effort (T-shirt sizing)

| Size | Meaning | Examples |
|------|---------|----------|
| XS | < 1 day | SV-001, SV-003, documentation tasks |
| S | 1-2 days | SV-014, DEV-060, IOS-050 |
| M | 3-5 days | SV-020-024, DEV-020-024, IOS-001-005 |
| L | 1-2 weeks | SV-040-044, DEV-030-035, complete mobile epic |
| XL | 2+ weeks | SV-064-065, SV-070-072 |

---

*Generated: 2026-01-22*
*Based on: SERVICE-VAULT-ARCHITECTURE.md, AWS-ARCHITECTURE.md*
