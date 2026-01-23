"""
VettID Service Vault SDK for Python

A Python SDK for integrating with VettID Service Vault, enabling services
to authenticate and authorize users, handle payments, manage secrets, and more.

Example:
    >>> from vettid_vault import ServiceVaultClient
    >>>
    >>> client = ServiceVaultClient(
    ...     base_url="http://localhost:8080",
    ...     api_key="your-api-key"
    ... )
    >>>
    >>> # Request authentication
    >>> request_id = await client.request_auth(
    ...     user_id="user123",
    ...     purpose="Login to dashboard"
    ... )
"""

from .client import ServiceVaultClient, VaultError
from .types import (
    ServiceType,
    ServiceIdentity,
    ContractStatus,
    CapabilityType,
    CapabilityGrant,
    ConnectionContract,
    RequestStatus,
    AuthResponse,
    AuthzResponse,
    CallType,
    CallStatus,
    CallResult,
    PaymentStatus,
    PaymentResult,
    SecretType,
    SecretResult,
    Money,
)
from .crypto import (
    generate_service_identity,
    derive_service_id,
    sign,
    verify,
    encrypt,
    decrypt,
)
from .webhook import WebhookRouter

__version__ = "0.1.0"

__all__ = [
    # Client
    "ServiceVaultClient",
    "VaultError",
    # Types
    "ServiceType",
    "ServiceIdentity",
    "ContractStatus",
    "CapabilityType",
    "CapabilityGrant",
    "ConnectionContract",
    "RequestStatus",
    "AuthResponse",
    "AuthzResponse",
    "CallType",
    "CallStatus",
    "CallResult",
    "PaymentStatus",
    "PaymentResult",
    "SecretType",
    "SecretResult",
    "Money",
    # Crypto
    "generate_service_identity",
    "derive_service_id",
    "sign",
    "verify",
    "encrypt",
    "decrypt",
    # Webhooks
    "WebhookRouter",
]
