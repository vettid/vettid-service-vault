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
Each Service Vault has a unique cryptographic identity (Ed25519 signing key, X25519 encryption key) registered with VettID.

### User Connections
Users must explicitly authorize services via their VettID app. Each connection has:
- Specific capability grants (what the service can access)
- Per-connection encryption keys
- User-controlled revocation

### Communication Model
```
Service Backend → Service API → Service Vault → NATS → User Vault
                                                     ↓
                                                User App (approval if needed)
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

## Contributing

This project follows the same development practices as the main VettID repository.

## License

Proprietary - VettID
