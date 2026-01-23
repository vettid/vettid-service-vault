"""
Webhook handling for VettID Service Vault.
"""

import base64
import hashlib
import hmac
import json
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from datetime import datetime
from typing import Any

from .types import (
    RequestStatus,
    CallStatus,
    PaymentStatus,
)


@dataclass
class AuthEvent:
    """Authentication webhook event."""
    request_id: str
    user_id: str
    status: RequestStatus
    timestamp: datetime
    session_key: str | None = None


@dataclass
class AuthzEvent:
    """Authorization webhook event."""
    request_id: str
    user_id: str
    action: str
    resource: str
    status: RequestStatus
    timestamp: datetime
    expires_at: datetime | None = None


@dataclass
class ContractEvent:
    """Contract webhook event."""
    contract_id: str
    user_id: str
    service_id: str
    offering_id: str
    event_type: str  # "accepted", "cancelled", "expired"
    timestamp: datetime


@dataclass
class CallEvent:
    """Call webhook event."""
    call_id: str
    request_id: str
    user_id: str
    status: CallStatus
    timestamp: datetime
    answer: dict[str, Any] | None = None
    duration: int | None = None


@dataclass
class PaymentEvent:
    """Payment webhook event."""
    request_id: str
    user_id: str
    status: PaymentStatus
    timestamp: datetime
    payment_id: str | None = None
    transaction_ref: str | None = None
    failure_reason: str | None = None


@dataclass
class SecretEvent:
    """Secret operation webhook event."""
    request_id: str
    user_id: str
    secret_id: str | None
    operation: str  # "stored", "retrieved", "deleted"
    status: RequestStatus
    timestamp: datetime
    data: bytes | None = None


# Type aliases for handlers
AuthHandler = Callable[[AuthEvent], Awaitable[None]]
AuthzHandler = Callable[[AuthzEvent], Awaitable[None]]
ContractHandler = Callable[[ContractEvent], Awaitable[None]]
CallHandler = Callable[[CallEvent], Awaitable[None]]
PaymentHandler = Callable[[PaymentEvent], Awaitable[None]]
SecretHandler = Callable[[SecretEvent], Awaitable[None]]


class WebhookRouter:
    """
    Router for handling Service Vault webhook callbacks.

    Example:
        >>> router = WebhookRouter(signing_secret="your-webhook-secret")
        >>>
        >>> @router.on_auth
        ... async def handle_auth(event: AuthEvent):
        ...     if event.status == RequestStatus.APPROVED:
        ...         print(f"User {event.user_id} authenticated!")
        >>>
        >>> # In your web framework
        >>> async def webhook_endpoint(request):
        ...     signature = request.headers.get("X-Vault-Signature")
        ...     await router.handle(request.body, signature)
    """

    def __init__(self, signing_secret: str | None = None):
        """
        Initialize webhook router.

        Args:
            signing_secret: Secret for verifying webhook signatures (optional but recommended)
        """
        self._signing_secret = signing_secret
        self._auth_handlers: list[AuthHandler] = []
        self._authz_handlers: list[AuthzHandler] = []
        self._contract_handlers: list[ContractHandler] = []
        self._call_handlers: list[CallHandler] = []
        self._payment_handlers: list[PaymentHandler] = []
        self._secret_handlers: list[SecretHandler] = []

    def on_auth(self, handler: AuthHandler) -> AuthHandler:
        """Register an authentication event handler."""
        self._auth_handlers.append(handler)
        return handler

    def on_authz(self, handler: AuthzHandler) -> AuthzHandler:
        """Register an authorization event handler."""
        self._authz_handlers.append(handler)
        return handler

    def on_contract(self, handler: ContractHandler) -> ContractHandler:
        """Register a contract event handler."""
        self._contract_handlers.append(handler)
        return handler

    def on_call(self, handler: CallHandler) -> CallHandler:
        """Register a call event handler."""
        self._call_handlers.append(handler)
        return handler

    def on_payment(self, handler: PaymentHandler) -> PaymentHandler:
        """Register a payment event handler."""
        self._payment_handlers.append(handler)
        return handler

    def on_secret(self, handler: SecretHandler) -> SecretHandler:
        """Register a secret operation event handler."""
        self._secret_handlers.append(handler)
        return handler

    def verify_signature(self, payload: bytes, signature: str) -> bool:
        """
        Verify webhook signature.

        Args:
            payload: Raw request body
            signature: Signature from X-Vault-Signature header

        Returns:
            True if signature is valid
        """
        if not self._signing_secret:
            return True  # No verification if secret not configured

        expected = hmac.new(
            self._signing_secret.encode(),
            payload,
            hashlib.sha256,
        ).hexdigest()

        return hmac.compare_digest(f"sha256={expected}", signature)

    async def handle(
        self,
        payload: bytes | str,
        signature: str | None = None,
    ) -> None:
        """
        Handle an incoming webhook.

        Args:
            payload: Request body (bytes or string)
            signature: Signature from X-Vault-Signature header (optional)

        Raises:
            ValueError: If signature verification fails or payload is invalid
        """
        if isinstance(payload, str):
            payload_bytes = payload.encode()
        else:
            payload_bytes = payload

        # Verify signature if configured
        if self._signing_secret and signature:
            if not self.verify_signature(payload_bytes, signature):
                raise ValueError("Invalid webhook signature")

        # Parse payload
        try:
            data = json.loads(payload_bytes)
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid JSON payload: {e}") from e

        event_type = data.get("event_type")
        if not event_type:
            raise ValueError("Missing event_type in payload")

        # Route to appropriate handlers
        if event_type == "auth":
            await self._handle_auth(data)
        elif event_type == "authz":
            await self._handle_authz(data)
        elif event_type == "contract":
            await self._handle_contract(data)
        elif event_type == "call":
            await self._handle_call(data)
        elif event_type == "payment":
            await self._handle_payment(data)
        elif event_type == "secret":
            await self._handle_secret(data)
        else:
            raise ValueError(f"Unknown event type: {event_type}")

    async def _handle_auth(self, data: dict[str, Any]) -> None:
        event = AuthEvent(
            request_id=data["request_id"],
            user_id=data["user_id"],
            status=RequestStatus(data["status"]),
            timestamp=datetime.fromisoformat(data["timestamp"]),
            session_key=data.get("session_key"),
        )
        for handler in self._auth_handlers:
            await handler(event)

    async def _handle_authz(self, data: dict[str, Any]) -> None:
        event = AuthzEvent(
            request_id=data["request_id"],
            user_id=data["user_id"],
            action=data["action"],
            resource=data["resource"],
            status=RequestStatus(data["status"]),
            timestamp=datetime.fromisoformat(data["timestamp"]),
            expires_at=(
                datetime.fromisoformat(data["expires_at"])
                if data.get("expires_at")
                else None
            ),
        )
        for handler in self._authz_handlers:
            await handler(event)

    async def _handle_contract(self, data: dict[str, Any]) -> None:
        event = ContractEvent(
            contract_id=data["contract_id"],
            user_id=data["user_id"],
            service_id=data["service_id"],
            offering_id=data["offering_id"],
            event_type=data["contract_event"],
            timestamp=datetime.fromisoformat(data["timestamp"]),
        )
        for handler in self._contract_handlers:
            await handler(event)

    async def _handle_call(self, data: dict[str, Any]) -> None:
        event = CallEvent(
            call_id=data["call_id"],
            request_id=data.get("request_id", ""),
            user_id=data["user_id"],
            status=CallStatus(data["status"]),
            timestamp=datetime.fromisoformat(data["timestamp"]),
            answer=data.get("answer"),
            duration=data.get("duration_seconds"),
        )
        for handler in self._call_handlers:
            await handler(event)

    async def _handle_payment(self, data: dict[str, Any]) -> None:
        event = PaymentEvent(
            request_id=data["request_id"],
            user_id=data["user_id"],
            status=PaymentStatus(data["status"]),
            timestamp=datetime.fromisoformat(data["timestamp"]),
            payment_id=data.get("payment_id"),
            transaction_ref=data.get("transaction_id"),
            failure_reason=data.get("failure_reason"),
        )
        for handler in self._payment_handlers:
            await handler(event)

    async def _handle_secret(self, data: dict[str, Any]) -> None:
        secret_data = None
        if data.get("data"):
            secret_data = base64.b64decode(data["data"])

        event = SecretEvent(
            request_id=data["request_id"],
            user_id=data["user_id"],
            secret_id=data.get("secret_id"),
            operation=data["operation"],
            status=RequestStatus(data["status"]),
            timestamp=datetime.fromisoformat(data["timestamp"]),
            data=secret_data,
        )
        for handler in self._secret_handlers:
            await handler(event)
