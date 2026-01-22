# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VettID Service Vault is a secure integration layer enabling third-party services to connect with VettID users. This repository is currently in the **design phase** - containing architecture documentation but no implementation code yet.

Unlike user vaults (which serve individual users via mobile apps), Service Vaults:
- Handle **many users** per vault instance
- Provide a **secure API** for backend integration
- Use **event-driven communication** via NATS
- Enforce **user-controlled authorization** for all data access

## Architecture Summary

### Core Components (Planned)

```
vettid-service-vault/
├── docs/                 # Architecture documentation (current)
├── cdk/                  # AWS CDK infrastructure (future)
├── vault/                # Go service vault implementation (future)
│   ├── cmd/              # Main entrypoints
│   └── internal/         # Core logic
└── sdk/                  # Client SDKs: Node.js, Python, Go (future)
```

### Communication Model

**Split NATS topology:**
- **Service → User**: Via VettID's MessageSpace (VettID-hosted NATS)
- **User → Service**: Via Service's own NATS cluster (ServiceSpace)

Services are **self-sovereign** - they generate their own identity (`service_id = base58(sha256(public_key)[0:20])`) and operate independently with optional registry registration.

### Key Security Concepts

1. **Self-sovereign service identity**: Ed25519 signing + X25519 encryption keys (provider-controlled)
2. **Direct user-service contracts**: User signs with their cryptographic key, no VettID approval needed
3. **Message-level encryption**: X25519 + XChaCha20-Poly1305
4. **Capability-based authorization**: Services only perform user-granted actions

### AWS Deployment Tiers

| Tier | Users | Infrastructure | Cost |
|------|-------|----------------|------|
| Pool | 10-10K | Shared (tenant-isolated) | $20-50/mo |
| Pro | 10K-100K | Dedicated tables, shared compute | $150-250/mo |
| Silo | 100K+ | Fully dedicated | $600-1400/mo |

## Key Documentation

- `docs/SERVICE-VAULT-ARCHITECTURE.md` - Comprehensive system design (identity, contracts, message flows)
- `docs/AWS-ARCHITECTURE.md` - AWS deployment architecture (CDK, multi-tenancy, scaling)
- `docs/WORK-ITEMS.md` - Implementation work items across all VettID repositories

## Implementation Context

When implementation begins, key patterns to follow:

### Cryptography
- Service ID: `base58(sha256(ed25519_public_key)[0:20])`
- Message encryption: X25519 key exchange + XChaCha20-Poly1305
- Signatures: Ed25519 for contracts and messages
- Private keys: HSM/KMS required in production (never plaintext)

### Multi-Tenant Isolation (Pool Tier)
- DynamoDB: Partition key must include `service_id` for all queries
- NATS: Account-based isolation per service with explicit permissions
- Application-level enforcement is primary (IAM is defense-in-depth only)

### NATS Topics
```
ServiceSpace.<service_id>.fromUser.<user_id>.>   # User → Service
ServiceSpace.<service_id>.toUser.<user_id>.>     # Service → User (setup only)
MessageSpace.<user_id>.fromService.<service_id>.> # Service → User (post-contract)
```

## Related Repositories

| Repository | Role |
|------------|------|
| vettid-dev | Backend infrastructure (NATS, registry, MessageSpace) |
| vettid-ios | iOS app (service connections, contract UI) |
| vettid-android | Android app (service connections, contract UI) |
