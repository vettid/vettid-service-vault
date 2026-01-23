# VettID Service Vault Security Model

This document describes the security model, threat analysis, and security requirements for the VettID Service Vault.

---

## Table of Contents

1. [Security Overview](#security-overview)
2. [Trust Model](#trust-model)
3. [Threat Model](#threat-model)
4. [Security Boundaries](#security-boundaries)
5. [Cryptographic Design](#cryptographic-design)
6. [Key Management](#key-management)
7. [Authentication & Authorization](#authentication--authorization)
8. [Data Protection](#data-protection)
9. [Audit Logging](#audit-logging)
10. [Operational Security](#operational-security)

---

## Security Overview

The VettID Service Vault enables third-party services to interact with VettID users through a secure, user-controlled authorization model. Security is achieved through:

- **Self-sovereign identity**: Services control their own cryptographic identity
- **User consent**: All interactions require explicit user approval
- **End-to-end encryption**: Messages are encrypted between service and user
- **Capability-based access**: Services can only perform user-granted actions
- **Cryptographic verification**: All contracts and messages are signed

### Security Principles

1. **Zero Trust**: Every request is authenticated and authorized independently
2. **Least Privilege**: Services only receive capabilities explicitly granted
3. **Defense in Depth**: Multiple layers of security controls
4. **User Control**: Users can revoke access at any time
5. **Transparency**: All actions are auditable

---

## Trust Model

### Trusted Entities

| Entity | Trust Level | Responsibilities |
|--------|-------------|------------------|
| VettID Infrastructure | High | MessageSpace routing, service registry, attestations |
| User's Device/Vault | High | Key custody, contract signing, approval decisions |
| Service Vault | Medium | Request handling, contract management, message encryption |
| Service Backend | Low | Business logic, API consumption |

### Trust Relationships

```
┌─────────────────────────────────────────────────────────────────┐
│                        VettID Infrastructure                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │  Registry   │  │ MessageSpace│  │     Attestation CA      │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
         │                  │                      │
         │ Attestation      │ Encrypted Messages   │ Signed Certs
         ▼                  ▼                      ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Service Vault  │◄───│   User Vault    │───►│  User Device    │
│  (self-hosted)  │    │   (VettID)      │    │  (iOS/Android)  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                                            │
         │ API Calls                                  │ UI Approval
         ▼                                            ▼
┌─────────────────┐                          ┌─────────────────┐
│ Service Backend │                          │      User       │
└─────────────────┘                          └─────────────────┘
```

### Trust Assumptions

1. **User device is secure**: Private keys are protected by device security (Secure Enclave, TEE)
2. **VettID infrastructure is honest**: Routes messages correctly, doesn't forge attestations
3. **Cryptographic primitives are secure**: Ed25519, X25519, XChaCha20-Poly1305
4. **TLS is properly implemented**: Network transport is encrypted

---

## Threat Model

### Assets to Protect

| Asset | Sensitivity | Protection Goal |
|-------|-------------|-----------------|
| Service private keys | Critical | Confidentiality, integrity |
| User connection keys | Critical | Confidentiality, integrity |
| Contract data | High | Integrity, authenticity |
| User identifiers | Medium | Privacy, unlinkability |
| Request/response content | High | Confidentiality, integrity |
| Audit logs | Medium | Integrity, availability |

### Threat Actors

#### 1. External Attackers
- **Capability**: Network access, public information
- **Goals**: Steal credentials, impersonate services/users, intercept data
- **Mitigations**: TLS, message encryption, signature verification

#### 2. Malicious Services
- **Capability**: Valid service identity, API access
- **Goals**: Access unauthorized user data, exceed granted capabilities
- **Mitigations**: Capability enforcement, user approval, contract limits

#### 3. Compromised Service Vault
- **Capability**: Access to service private keys, contract data
- **Goals**: Impersonate service, access all user data
- **Mitigations**: HSM key storage, per-user connection keys, audit logging

#### 4. Malicious Insiders (Service Operator)
- **Capability**: Administrative access to service infrastructure
- **Goals**: Access user data without authorization
- **Mitigations**: Key separation, audit logs, user notifications

#### 5. Network Attackers (MITM)
- **Capability**: Intercept/modify network traffic
- **Goals**: Eavesdrop, inject messages
- **Mitigations**: TLS, end-to-end encryption, message signing

### Threat Analysis (STRIDE)

| Threat | Category | Risk | Mitigation |
|--------|----------|------|------------|
| Forge service identity | Spoofing | High | Ed25519 signatures, registry attestations |
| Modify contract terms | Tampering | High | Dual signatures (user + service) |
| Deny sending request | Repudiation | Medium | Signed requests, audit logs |
| Read user messages | Info Disclosure | High | X25519 encryption, per-connection keys |
| Flood with requests | DoS | Medium | Rate limiting, request timeouts |
| Exceed capabilities | Elevation | High | Capability verification per request |

### Attack Scenarios

#### Scenario 1: Service Impersonation
- **Attack**: Attacker creates fake service with similar name
- **Detection**: Domain verification, registry attestations
- **Prevention**: Users verify domain, check attestations before connecting

#### Scenario 2: Replay Attack
- **Attack**: Attacker replays old auth request
- **Detection**: Request ID uniqueness, timestamp validation
- **Prevention**: Request expiration, nonce in request ID

#### Scenario 3: Connection Key Theft
- **Attack**: Attacker steals service's connection key for a user
- **Impact**: Can decrypt messages for that user only
- **Prevention**: HSM storage, key rotation, per-user keys limit blast radius

#### Scenario 4: Contract Modification
- **Attack**: Service attempts to modify contract after signing
- **Detection**: User has signed copy, signature verification fails
- **Prevention**: Dual signatures, immutable contract storage

---

## Security Boundaries

### Boundary 1: Service Vault ↔ Service Backend

```
┌─────────────────────────────────────────────────────────┐
│                    Service Vault                         │
│  ┌─────────────────────────────────────────────────┐    │
│  │              Security Boundary                   │    │
│  │  • API key authentication                       │    │
│  │  • Request validation                           │    │
│  │  • Rate limiting                                │    │
│  │  • Audit logging                                │    │
│  └─────────────────────────────────────────────────┘    │
│                         │                                │
│                    REST API                              │
│                         │                                │
└─────────────────────────┼───────────────────────────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │ Service Backend │
                 └─────────────────┘
```

**Controls:**
- Bearer token authentication (API keys)
- Request body validation
- Rate limiting per API key
- TLS required (non-development)

### Boundary 2: Service Vault ↔ User Vault

```
┌─────────────────┐         NATS (TLS)         ┌─────────────────┐
│  Service Vault  │◄─────────────────────────►│   User Vault    │
│                 │                            │                 │
│  ┌───────────┐  │  End-to-End Encryption     │  ┌───────────┐  │
│  │ Service   │  │  ═══════════════════════   │  │   User    │  │
│  │ Private   │  │  X25519 + XChaCha20        │  │  Private  │  │
│  │ Keys      │  │                            │  │   Keys    │  │
│  └───────────┘  │                            │  └───────────┘  │
└─────────────────┘                            └─────────────────┘
```

**Controls:**
- NATS authentication (JWT credentials)
- End-to-end encryption (X25519 key exchange)
- Message signing (Ed25519)
- Topic-level authorization

### Boundary 3: Service Vault ↔ Key Storage

```
┌─────────────────────────────────────────────────────────┐
│                    Service Vault                         │
│                         │                                │
│                    KeyStore API                          │
│                         │                                │
│  ┌──────────────────────┼──────────────────────────┐    │
│  │           Security Boundary                      │    │
│  │  • Key never leaves boundary in plaintext       │    │
│  │  • All operations via KeyStore interface        │    │
│  │  • Audit logging of key usage                   │    │
│  └──────────────────────┼──────────────────────────┘    │
│                         │                                │
└─────────────────────────┼───────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
    ┌──────────┐   ┌──────────┐   ┌──────────┐
    │  Memory  │   │   File   │   │   KMS    │
    │  (dev)   │   │  (dev)   │   │  (prod)  │
    └──────────┘   └──────────┘   └──────────┘
```

**Controls:**
- Abstract KeyStore interface
- Keys encrypted at rest (KMS)
- Sign operations performed in HSM (KMS)
- No plaintext key export in production

### Boundary 4: Multi-Tenant Isolation (Pool Tier)

```
┌─────────────────────────────────────────────────────────┐
│                   Shared Infrastructure                  │
│  ┌─────────────────────────────────────────────────┐    │
│  │              Tenant Isolation                    │    │
│  │                                                  │    │
│  │  DynamoDB: service_id in partition key          │    │
│  │  NATS: Account-based isolation                  │    │
│  │  API: service_id from auth context              │    │
│  │                                                  │    │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐         │    │
│  │  │Service A│  │Service B│  │Service C│         │    │
│  │  └─────────┘  └─────────┘  └─────────┘         │    │
│  └─────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

**Controls:**
- Partition key includes `service_id` for all DynamoDB queries
- NATS accounts provide topic isolation
- API authentication binds requests to service identity
- No cross-tenant data access possible at application layer

---

## Cryptographic Design

### Algorithms

| Purpose | Algorithm | Key Size | Notes |
|---------|-----------|----------|-------|
| Identity signing | Ed25519 | 256-bit | Fast, secure, deterministic |
| Key exchange | X25519 | 256-bit | ECDH for session keys |
| Symmetric encryption | XChaCha20-Poly1305 | 256-bit | AEAD, extended nonce |
| Hashing | SHA-256 | 256-bit | For service_id derivation |
| Encoding | Base58 | N/A | For service_id display |

### Service Identity Derivation

```
service_id = base58(sha256(ed25519_public_key)[0:20])
```

- Truncated to 20 bytes (160 bits) for shorter IDs
- Base58 encoding for URL-safe, human-readable format
- Deterministic: same key always produces same ID

### Message Encryption Flow

```
1. Sender generates ephemeral X25519 keypair (e_priv, e_pub)
2. Sender computes shared_secret = X25519(e_priv, recipient_pub)
3. Sender derives key = HKDF-SHA256(shared_secret, "vettid-message-v1")
4. Sender generates random 24-byte nonce
5. Sender encrypts: ciphertext = XChaCha20-Poly1305(key, nonce, plaintext)
6. Message = (e_pub, nonce, ciphertext)
```

### Contract Signing

```
1. Contract data canonicalized to deterministic JSON
2. User signs: user_signature = Ed25519-Sign(user_priv, canonical_json)
3. Service counter-signs: service_signature = Ed25519-Sign(service_priv, canonical_json)
4. Both signatures stored with contract
```

---

## Key Management

### Key Types

| Key Type | Algorithm | Storage | Rotation | Backup |
|----------|-----------|---------|----------|--------|
| Service signing key | Ed25519 | HSM/KMS | Rare (identity change) | Secure offline |
| Service encryption key | X25519 | HSM/KMS | Annual | Secure offline |
| Connection keys | X25519 | KMS-encrypted | Per connection | Not backed up |
| API keys | Random | Hashed in DB | On compromise | Not backed up |

### Key Lifecycle

#### Service Keys
1. **Generation**: Created during service initialization
2. **Storage**: Encrypted by KMS CMK, stored in vault
3. **Usage**: Sign contracts, decrypt messages
4. **Rotation**: Requires new service identity (rare)
5. **Destruction**: Secure deletion on decommission

#### Connection Keys
1. **Generation**: Created during contract negotiation
2. **Storage**: Encrypted per contract, stored with contract
3. **Usage**: Encrypt/decrypt messages with specific user
4. **Rotation**: New key on contract renewal
5. **Destruction**: Deleted on contract cancellation

### Production Key Storage Requirements

1. **HSM/KMS Required**: Service signing key must be stored in HSM or cloud KMS
2. **No Plaintext Export**: Private keys never leave secure boundary
3. **Access Logging**: All key operations logged
4. **Separation of Duties**: Key administrators ≠ application operators

### Key Compromise Response

| Key Type | Impact | Response |
|----------|--------|----------|
| Service signing key | Critical - full impersonation | Revoke attestations, notify users, generate new identity |
| Service encryption key | High - decrypt past messages | Rotate key, re-encrypt active data |
| Single connection key | Medium - one user affected | Revoke contract, notify user, re-establish |
| API key | Medium - unauthorized API access | Revoke key, audit recent activity |

---

## Authentication & Authorization

### API Authentication

```
Authorization: Bearer <api-key>
```

- API keys are random 256-bit tokens
- Keys are hashed (SHA-256) before storage
- Keys are scoped to specific service identity
- Rate limited per key

### NATS Authentication

- Services authenticate to NATS using JWT credentials
- JWTs issued by VettID for MessageSpace access
- JWTs issued by service for user ServiceSpace access
- Permissions encoded in JWT claims

### Capability Model

```go
type Capability string

const (
    CapabilityAuthenticate  Capability = "authenticate"
    CapabilityAuthorize     Capability = "authorize"
    CapabilityBrowseData    Capability = "browse_data"
    CapabilityRequestData   Capability = "request_data"
    CapabilityNotify        Capability = "notify"
    CapabilityCall          Capability = "call"
    CapabilityRequestPayment Capability = "request_payment"
)
```

### Capability Enforcement

```
For each request:
1. Load user's contract
2. Verify contract is active
3. Check required capability is granted
4. Verify capability constraints (e.g., data scope)
5. Execute if all checks pass
6. Log the access
```

---

## Data Protection

### Data Classification

| Classification | Examples | Protection |
|----------------|----------|------------|
| Critical | Private keys, API keys | HSM/KMS, never logged |
| Sensitive | User IDs, contract data | Encrypted at rest, access logged |
| Internal | Request metadata, timestamps | Encrypted at rest |
| Public | Service name, capabilities | Integrity protected |

### Encryption at Rest

- DynamoDB: AWS-managed encryption (SSE)
- Contract data: Application-level encryption optional
- Keys: KMS-encrypted before storage
- Logs: CloudWatch encryption

### Encryption in Transit

- All API calls: TLS 1.3 required (production)
- NATS connections: TLS required
- User messages: End-to-end encrypted (X25519 + XChaCha20)

### Data Retention

| Data Type | Retention | Deletion |
|-----------|-----------|----------|
| Active contracts | Until cancelled | 30 days after cancellation |
| Cancelled contracts | 90 days | Automatic purge |
| Request/response | 24 hours (pending) | Immediate on completion |
| Audit logs | 1 year | Automatic purge |
| Connection keys | With contract | With contract |

---

## Audit Logging

### Required Audit Events

| Event | Data Logged | Retention |
|-------|-------------|-----------|
| Contract created | contract_id, user_id, offering_id, timestamp | 1 year |
| Contract activated | contract_id, timestamp | 1 year |
| Contract cancelled | contract_id, reason, cancelled_by, timestamp | 1 year |
| Auth request created | request_id, user_id, purpose, timestamp | 90 days |
| Auth request responded | request_id, status, timestamp | 90 days |
| Authz request created | request_id, user_id, action, resource, timestamp | 90 days |
| Authz request responded | request_id, status, timestamp | 90 days |
| API key created | key_id (not key), timestamp | 1 year |
| API key revoked | key_id, reason, timestamp | 1 year |

### Audit Log Format

```json
{
  "timestamp": "2026-01-22T19:00:00Z",
  "event_type": "auth.request.created",
  "service_id": "3Kj9mNxPqRsT5vWy",
  "user_id": "7XhK3mNpQr9sT5vW",
  "request_id": "abc123xyz789",
  "metadata": {
    "purpose": "Login to dashboard",
    "ip_address": "192.168.1.1"
  }
}
```

### Audit Log Protection

1. **Integrity**: Logs are append-only, checksummed
2. **Confidentiality**: Encrypted at rest
3. **Availability**: Replicated storage
4. **Access Control**: Separate from application access

### Alerting

| Condition | Alert Level | Response |
|-----------|-------------|----------|
| Multiple failed auth attempts | Warning | Review, possible block |
| Unusual request volume | Warning | Review for abuse |
| Contract cancelled by user | Info | Review service behavior |
| Key rotation required | Info | Schedule rotation |
| Capability denied | Info | Review contract configuration |

---

## Operational Security

### Deployment Requirements

| Requirement | Development | Production |
|-------------|-------------|------------|
| TLS | Optional | Required |
| Key storage | Memory/File | KMS/HSM |
| API authentication | Optional | Required |
| Audit logging | Console | Persistent storage |
| Rate limiting | Disabled | Enabled |

### Security Checklist

#### Pre-Deployment
- [ ] Service signing key stored in HSM/KMS
- [ ] API keys generated with sufficient entropy
- [ ] TLS certificates configured
- [ ] Rate limiting configured
- [ ] Audit logging enabled
- [ ] Backup procedures tested

#### Ongoing
- [ ] Monitor audit logs for anomalies
- [ ] Review API key usage monthly
- [ ] Rotate API keys annually
- [ ] Test backup restoration quarterly
- [ ] Security patches applied promptly

### Incident Response

1. **Detection**: Monitor alerts, user reports, anomaly detection
2. **Containment**: Revoke compromised credentials, isolate affected systems
3. **Analysis**: Review audit logs, determine scope
4. **Remediation**: Patch vulnerabilities, rotate keys
5. **Recovery**: Restore service, notify affected users
6. **Lessons Learned**: Document incident, update procedures

---

## Compliance Considerations

### Data Protection (GDPR, CCPA)

- User data access requires explicit consent (contract)
- Users can revoke consent at any time (cancel contract)
- Audit logs provide data access history
- Data deletion procedures defined

### Security Standards

- Cryptographic algorithms follow NIST recommendations
- Key management follows industry best practices
- Audit logging supports SOC 2 requirements

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-22 | Initial security model |

---

*This document should be reviewed and updated whenever significant security-relevant changes are made to the Service Vault architecture.*
