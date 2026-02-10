# VettID Service Vault

Secure integration layer enabling third-party services to connect with VettID users.

## Overview

The Service Vault allows organizations to integrate their applications and services with VettID's privacy-first identity platform. Unlike user vaults (which serve individual users via mobile apps), Service Vaults:

- Handle **many users** per vault instance
- Provide a **secure API** for backend integration
- Use **event-driven communication** via NATS
- Enforce **user-controlled authorization** for all data access

## Documentation

- [Architecture Design](docs/SERVICE-VAULT-ARCHITECTURE.md) - Comprehensive system design
- API Reference (coming soon)
- Deployment Guide (coming soon)
- SDK Integration Guide (coming soon)

## Project Status

**Phase: Design Review**

This project is currently in the design phase. The architecture document is ready for review.

## Key Concepts

### Service Identity
Each Service Vault has a **self-sovereign cryptographic identity**:
- Key-derived ID: `service_id = base58(sha256(public_key))`
- Ed25519 signing key + X25519 encryption key (provider-generated and controlled)
- Optional domain validation via DNS
- Optional registry registration (VettID or others) for discoverability

### User Connections
Users authorize services by **signing connection contracts** with their cryptographic key:
- Direct user-service agreement (no VettID approval needed)
- Services offer multiple contract options (tiers, pricing, data requirements)
- User profile automatically shared with all connections
- Per-connection encryption keys for forward secrecy
- User-controlled revocation

### Communication Model
```
Service → User: Via VettID MessageSpace (user's vault controls access)
User → Service: Via Service's NATS cluster (ServiceSpace)

┌─────────────┐                              ┌─────────────┐
│ User Vault  │                              │Service Vault│
└──────┬──────┘                              └──────┬──────┘
       │                                            │
       ▼                                            ▼
┌─────────────┐         NATS Layer          ┌─────────────┐
│ VettID NATS │◄───────────────────────────►│Service NATS │
│(MessageSpace)│                            │(ServiceSpace)│
└─────────────┘                              └─────────────┘
```

## Proposed Directory Structure

```
vettid-service-vault/
├── README.md
├── docs/                          # Documentation
│   └── SERVICE-VAULT-ARCHITECTURE.md
├── cdk/                           # AWS CDK infrastructure (future)
│   ├── lib/
│   └── lambda/
├── vault/                         # Service Vault implementation (future)
│   ├── cmd/
│   └── internal/
├── sdk/                           # Client SDKs (future)
│   ├── node/
│   ├── python/
│   └── go/
└── examples/                      # Integration examples (future)
```

## Security Principles

1. **Zero Knowledge**: Service Vaults never access user credentials directly
2. **User Consent**: All data access requires explicit user authorization
3. **Capability-Based**: Services can only perform actions users have granted
4. **Message-Level Encryption**: All communication uses X25519 + XChaCha20-Poly1305
5. **Audit Trail**: All operations are logged with tamper-evident signatures

## Open Questions

See the architecture document for open questions requiring team input:

1. Should self-hosted Service Vaults require Nitro Enclaves?
2. How long should inactive connections persist?
3. Should services cache authorization for offline scenarios?
4. Multi-vault deployment strategy for regional services?
5. Service directory for user discovery?

## Related Repositories

- [vettid-dev](https://github.com/vettid/vettid-dev) - Backend infrastructure
- [vettid-android](https://github.com/vettid/vettid-android) - Android app
- [vettid-ios](https://github.com/vettid/vettid-ios) - iOS app
- [vettid-desktop](https://github.com/vettid/vettid-desktop) - Desktop app (Tauri/Rust/Svelte)
- [vettid-agent](https://github.com/vettid/vettid-agent) - Agent connector (Go sidecar)

## Contributing

This project follows the same development practices as the main VettID repository.

## License

This project is licensed under the GNU Affero General Public License v3.0 - see the [LICENSE](LICENSE) file for details.
