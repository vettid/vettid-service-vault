"""
Type definitions for VettID Service Vault SDK.
"""

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any, Literal


class ServiceType(str, Enum):
    """Service type categories."""
    GENERIC = "generic"
    PAYMENT = "payment"
    IDENTITY = "identity"
    COMMERCE = "commerce"
    HEALTHCARE = "healthcare"
    FINANCE = "finance"


class ContractStatus(str, Enum):
    """Contract status."""
    PENDING = "pending"
    ACTIVE = "active"
    PAUSED = "paused"
    CANCELLED = "cancelled"
    EXPIRED = "expired"


class RequestStatus(str, Enum):
    """Request status."""
    PENDING = "pending"
    APPROVED = "approved"
    DENIED = "denied"
    EXPIRED = "expired"
    OFFLINE_APPROVED = "offline_approved"


class CapabilityType(str, Enum):
    """Capability types that can be granted."""
    AUTHENTICATE = "authenticate"
    AUTHORIZE = "authorize"
    READ_DATA = "read_data"
    WRITE_DATA = "write_data"
    SIGN = "sign"
    BROWSE_DATA = "browse_data"
    REQUEST_DATA = "request_data"
    NOTIFY = "notify"
    CALL = "call"
    PAYMENT = "payment"
    SECRETS = "secrets"


class CallType(str, Enum):
    """Call type."""
    VOICE = "voice"
    VIDEO = "video"


class CallStatus(str, Enum):
    """Call status."""
    INITIATING = "initiating"
    RINGING = "ringing"
    CONNECTING = "connecting"
    CONNECTED = "connected"
    ENDED = "ended"
    FAILED = "failed"
    REJECTED = "rejected"
    MISSED = "missed"
    BUSY = "busy"


class PaymentStatus(str, Enum):
    """Payment status."""
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"
    REFUNDED = "refunded"
    PARTIAL_REFUND = "partial_refund"


class SecretType(str, Enum):
    """Secret type/security level."""
    MINOR = "minor"
    CRITICAL = "critical"
    USER_OWNED = "user_owned"


@dataclass
class ServiceIdentity:
    """Service identity with cryptographic keys."""
    service_id: str
    service_name: str
    service_type: ServiceType
    signing_public_key: str  # Base64 Ed25519 public key
    encryption_public_key: str  # Base64 X25519 public key
    domain: str | None = None
    domain_verified: bool = False
    nats_endpoint: str | None = None


@dataclass
class ServiceKeyPair:
    """Service cryptographic keypair."""
    signing_private_key: bytes  # Ed25519 private key
    signing_public_key: bytes  # Ed25519 public key
    encryption_private_key: bytes  # X25519 private key
    encryption_public_key: bytes  # X25519 public key


@dataclass
class CapabilityGrant:
    """A capability granted to a service."""
    capability: CapabilityType
    scope: str | None = None
    constraints: dict[str, Any] | None = None
    expires_at: datetime | None = None


@dataclass
class Money:
    """Monetary amount."""
    amount: int  # In smallest currency unit (cents)
    currency: str  # ISO 4217


@dataclass
class ConnectionContract:
    """A signed connection contract."""
    contract_id: str
    user_id: str
    service_id: str
    offering_id: str
    status: ContractStatus
    created_at: datetime
    activated_at: datetime | None = None
    cancelled_at: datetime | None = None


@dataclass
class AuthResponse:
    """Authentication response."""
    request_id: str
    status: RequestStatus
    user_id: str
    timestamp: datetime
    session_key: str | None = None


@dataclass
class AuthzResponse:
    """Authorization response."""
    request_id: str
    status: RequestStatus
    user_id: str
    action: str
    resource: str
    timestamp: datetime
    expires_at: datetime | None = None


@dataclass
class CallResult:
    """Call result."""
    call_id: str
    request_id: str
    status: CallStatus
    answer: dict[str, Any] | None = None
    started_at: datetime | None = None
    connected_at: datetime | None = None
    ended_at: datetime | None = None
    duration: int | None = None  # seconds


@dataclass
class PaymentResult:
    """Payment result."""
    request_id: str
    status: PaymentStatus
    payment_id: str | None = None
    transaction_ref: str | None = None
    receipt_url: str | None = None
    completed_at: datetime | None = None
    failed_at: datetime | None = None
    failure_reason: str | None = None


@dataclass
class SecretResult:
    """Secret operation result."""
    request_id: str
    status: RequestStatus
    secret_id: str | None = None
    data: bytes | None = None


@dataclass
class VaultConfig:
    """SDK configuration."""
    base_url: str
    api_key: str
    timeout: float = 30.0
    retries: int = 3
    headers: dict[str, str] = field(default_factory=dict)


@dataclass
class APIError:
    """API error."""
    code: str
    message: str
    details: Any = None
